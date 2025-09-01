package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"encore.dev/beta/auth"
	"encore.dev/storage/sqldb"

	"encore.app/pkg/config"
	"encore.app/pkg/errs"
	"encore.app/pkg/slugify"
	"encore.app/pkg/storagegcs"
)

// Database connection
var db = sqldb.Named("coredb")

// Service instance
var catalogService *Service

// Encore secrets for GCS configuration
var secrets struct {
	GCSProjectID       string //encore:secret
	GCSBucketName      string //encore:secret
	GCSCredentialsJSON string //encore:secret
}

// checkAdminPermission validates that the current user has admin role
func checkAdminPermission(ctx context.Context) error {
	userIDStr, ok := auth.UserID()
	if !ok {
		return errs.E(ctx, "USR_UNAUTHENTICATED", "مطلوب تسجيل الدخول")
	}

	// Convert auth.UserID (string) to int64 for database consistency
	userID, err := strconv.ParseInt(string(userIDStr), 10, 64)
	if err != nil {
		return errs.E(ctx, "USR_AUTH_ID_INVALID", "معرّف المستخدم غير صالح")
	}

	// Proper role checking implementation using database lookup
	// This integrates with the existing user management and auth system

	var role string
	err = db.QueryRow(ctx, `
		SELECT role FROM users WHERE id = $1 AND state = 'active'
	`, userID).Scan(&role)

	if err != nil {
		return errs.E(ctx, "USR_PERM_CHECK_FAILED", "فشل التحقق من صلاحيات المستخدم")
	}

	if role != "admin" {
		return errs.E(ctx, "USR_FORBIDDEN_ADMIN", "يتطلب صلاحيات مدير")
	}

	return nil
}

// InitService initializes the catalog service
func InitService() error {
	// Initialize slugifier
	slugifier := slugify.NewSlugifier(db.Stdlib())

	// Initialize storage client using Encore secrets
	// GCS secrets configured via Encore secrets management:
	// Production: encore secret set --prod GCS_PROJECT_ID <project-id>
	// Production: encore secret set --prod GCS_BUCKET_NAME <bucket-name>  
	// Production: encore secret set --prod GCS_CREDENTIALS_JSON <base64-credentials>
	// Development: encore secret set --dev GCS_PROJECT_ID <dev-project-id>
	// Development: encore secret set --dev GCS_BUCKET_NAME <dev-bucket-name>
	// Development: encore secret set --dev GCS_CREDENTIALS_JSON <dev-base64-credentials>
	storageConfig := storagegcs.Config{
		ProjectID:      secrets.GCSProjectID,       // From Encore secrets
		BucketName:     secrets.GCSBucketName,      // From Encore secrets
		CredentialsKey: secrets.GCSCredentialsJSON, // From Encore secrets
		IsPublic:       false,                      // Private bucket by default
	}

	storageClient, err := storagegcs.NewClient(context.Background(), storageConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize storage client: %w", err)
	}

	// Initialize config manager (global, hot-reload every 5 minutes)
	configMgr := config.Initialize(db, 5*time.Minute)

	// Initialize service
	catalogService = NewService(db, slugifier, storageClient, configMgr)

	return nil
}

// GetProducts retrieves products with optional filtering and pagination
//
//encore:api public method=GET path=/products
func GetProducts(ctx context.Context, req *ProductsListRequest) (*ProductsListResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}

	// Set default values if request is nil
	if req == nil {
		req = &ProductsListRequest{
			Page:  1,
			Limit: 20,
		}
	}

	return catalogService.GetProducts(ctx, *req)
}

// GetProduct retrieves a single product by ID
//
//encore:api public method=GET path=/products/:id
func GetProduct(ctx context.Context, id string) (*ProductDetailResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}

	// Parse product ID
	productID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errs.E(ctx, "CAT_INVALID_PRODUCT_ID", "معرّف المنتج غير صالح")
	}

	return catalogService.GetProductByID(ctx, productID)
}

// CreateProduct creates a new product (Admin only)
//
//encore:api auth method=POST path=/products
func CreateProduct(ctx context.Context, req *CreateProductRequest) (*CreateProductResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}

	// Check admin permissions
	if err := checkAdminPermission(ctx); err != nil {
		return nil, err
	}

	return catalogService.CreateProduct(ctx, *req)
}

// UploadProductMedia uploads media files for a product using raw endpoint for multipart support
//
//encore:api auth raw method=POST path=/products/:id/media
func UploadProductMedia(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// Initialize service if needed
	if catalogService == nil {
		if err := InitService(); err != nil {
			writeErrorResponse(w, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج"))
			return
		}
	}

	// Check admin permissions
	if err := checkAdminPermission(ctx); err != nil {
		writeErrorResponse(w, err)
		return
	}

	// Extract product ID from URL path
	// Parse ID from URL path manually since we're using raw endpoint
	path := req.URL.Path
	pathParts := strings.Split(path, "/")

	var productIDStr string
	for i, part := range pathParts {
		if part == "products" && i+1 < len(pathParts) {
			productIDStr = pathParts[i+1]
			break
		}
	}

	if productIDStr == "" {
		writeErrorResponse(w, errs.E(ctx, "CAT_PRODUCT_ID_REQUIRED", "مطلوب معرّف المنتج"))
		return
	}

	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil {
		writeErrorResponse(w, errs.E(ctx, "CAT_INVALID_PRODUCT_ID", "معرّف المنتج غير صالح"))
		return
	}

	// Parse multipart form
	err = req.ParseMultipartForm(100 << 20) // 100MB limit
	if err != nil {
		writeErrorResponse(w, errs.E(ctx, "CAT_PARSE_MULTIPART_FAILED", "تعذر قراءة الطلب متعدد الأجزاء"))
		return
	}

	// Get file from form
	file, header, err := req.FormFile("file")
	if err != nil {
		writeErrorResponse(w, errs.E(ctx, "CAT_FILE_REQUIRED", "ملف الرفع مطلوب"))
		return
	}
	defer file.Close()

	// Call service method
	response, err := catalogService.UploadMedia(ctx, productID, file, header)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

	// Write successful response
	writeJSONResponse(w, http.StatusCreated, response)
}

// UpdateProductMedia updates media properties (mainly for archiving)
//
//encore:api auth method=PATCH path=/products/:productId/media/:mediaId
func UpdateProductMedia(ctx context.Context, productId, mediaId string, req *UpdateMediaRequest) (*UpdateMediaResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}

	// Parse IDs
	productID, err := strconv.ParseInt(productId, 10, 64)
	if err != nil {
		return nil, errs.E(ctx, "CAT_INVALID_PRODUCT_ID", "معرّف المنتج غير صالح")
	}

	mediaID, err := strconv.ParseInt(mediaId, 10, 64)
	if err != nil {
		return nil, errs.E(ctx, "CAT_INVALID_MEDIA_ID", "معرّف الوسائط غير صالح")
	}

	// Check admin permissions
	if err := checkAdminPermission(ctx); err != nil {
		return nil, err
	}

	// Set archived_at to now if archiving
	var archivedAt *time.Time
	if req != nil && req.ArchivedAt != nil {
		archivedAt = req.ArchivedAt
	} else {
		// Default to archiving (set to current time)
		now := time.Now()
		archivedAt = &now
	}

	return catalogService.UpdateMedia(ctx, productID, mediaID, archivedAt)
}

// GetProductMediaList retrieves all media for a product (helper endpoint)
//
//encore:api public method=GET path=/products/:id/media
func GetProductMediaList(ctx context.Context, id string) (*ProductMediaListResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}

	// Parse product ID
	productID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errs.E(ctx, "CAT_INVALID_PRODUCT_ID", "معرّف المنتج غير صالح")
	}

	// Check if product exists
	product, err := catalogService.repo.GetProductByID(ctx, productID)
	if err != nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_READ_FAILED", "فشل جلب المنتج")
	}

	if product == nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_NOT_FOUND", "المنتج غير موجود")
	}

	// Get media list (only active media for public endpoint)
	mediaList, err := catalogService.repo.GetMediaByProductID(ctx, productID, false)
	if err != nil {
		return nil, errs.E(ctx, "CAT_MEDIA_READ_FAILED", "فشل جلب الوسائط")
	}

	return &ProductMediaListResponse{
		ProductID: productID,
		Media:     mediaList,
	}, nil
}

// HealthCheck endpoint for catalog service
//
//encore:api public method=GET path=/catalog/health
func HealthCheck(ctx context.Context) (*HealthCheckResponse, error) {
	return &HealthCheckResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
	}, nil
}

// ProductMediaListResponse represents the response for getting product media list
type ProductMediaListResponse struct {
	ProductID int64   `json:"product_id"`
	Media     []Media `json:"media"`
}

// Helper functions for HTTP response handling

// writeErrorResponse writes an error response in JSON format
func writeErrorResponse(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")

	// Convert to pkg errs.Error if needed
	var e *errs.Error
	if pe, ok := err.(*errs.Error); ok {
		e = pe
	} else {
		e = errs.New(errs.Internal, err.Error())
	}

	statusCode := e.HTTPStatus()
	w.WriteHeader(statusCode)

	response := ErrorResponse{
		Code:          e.Code,
		Message:       e.Message,
		CorrelationID: e.CorrelationID,
	}

	_ = json.NewEncoder(w).Encode(response)
}

// writeJSONResponse writes a successful JSON response
func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}
