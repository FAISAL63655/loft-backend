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

// UploadMediaDraft allows uploading draft media before a product exists.
// It stores files in GCS and returns their paths without creating DB rows.
//
//encore:api auth raw method=POST path=/media-drafts/:session_id
func UploadMediaDraft(w http.ResponseWriter, req *http.Request) {
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

	// Extract session_id from URL path
	path := req.URL.Path
	parts := strings.Split(path, "/")
	var sessionID string
	for i, part := range parts {
		if part == "media-drafts" && i+1 < len(parts) {
			sessionID = parts[i+1]
			break
		}
	}
	if strings.TrimSpace(sessionID) == "" {
		writeErrorResponse(w, errs.E(ctx, "CAT_SESSION_ID_REQUIRED", "مطلوب session_id"))
		return
	}

	// Parse multipart form
	if err := req.ParseMultipartForm(100 << 20); err != nil { // 100MB
		writeErrorResponse(w, errs.E(ctx, "CAT_PARSE_MULTIPART_FAILED", "تعذر قراءة الطلب متعدد الأجزاء"))
		return
	}

	// Prepare upload config from settings
	mediaSettings, _ := catalogService.getMediaSettings(ctx)
	// Map service media settings to storagegcs settings type
	sgcsSettings := storagegcs.MediaSettings{
		WatermarkEnabled:  mediaSettings.WatermarkEnabled,
		WatermarkPosition: mediaSettings.WatermarkPosition,
		WatermarkOpacity:  mediaSettings.WatermarkOpacity,
		ThumbnailsEnabled: mediaSettings.ThumbnailsEnabled,
		ThumbnailSizes:    mediaSettings.ThumbnailSizes,
		MaxFileSize:       mediaSettings.MaxFileSize,
		AllowedTypes:      mediaSettings.AllowedTypes,
	}
	uploadCfg := storagegcs.CreateUploadConfigFromSettings(sgcsSettings)

	type DraftFile struct {
		Kind    string `json:"kind"`
		GCSPath string `json:"gcs_path"`
		Thumb   string `json:"thumb_path,omitempty"`
		Size    int64  `json:"size,omitempty"`
		Name    string `json:"name,omitempty"`
	}
	type DraftLink struct {
		Kind string `json:"kind"`
		URL  string `json:"url"`
	}
	var files []DraftFile
	var links []DraftLink

	// Handle links first (zero or many link_url values)
	if form := req.MultipartForm; form != nil {
		kind := strings.TrimSpace(req.FormValue("kind"))
		for _, url := range form.Value["link_url"] {
			u := strings.TrimSpace(url)
			if u != "" {
				links = append(links, DraftLink{Kind: kind, URL: u})
			}
		}
	}

	// Handle files (one or many)
	if form := req.MultipartForm; form != nil {
		filesHeaders := form.File["file"]
		for _, fh := range filesHeaders {
			f, err := fh.Open()
			if err != nil {
				writeErrorResponse(w, errs.E(ctx, "CAT_MEDIA_OPEN_FAILED", "تعذر فتح الملف"))
				return
			}
			defer f.Close()

			// Upload via storage client (no DB record)
			res, err := catalogService.storage.Upload(ctx, f, fh.Filename, uploadCfg)
			if err != nil {
				writeErrorResponse(w, errs.E(ctx, "CAT_MEDIA_UPLOAD_FAILED", "فشل رفع الملف إلى التخزين"))
				return
			}

			kind := string(res.Kind)
			df := DraftFile{
				Kind:    kind,
				GCSPath: res.GCSPath,
				Thumb:   res.ThumbPath,
				Size:    res.Size,
				Name:    fh.Filename,
			}
			files = append(files, df)
		}
	}

	// Respond with draft results (session-scoped client-side)
	writeJSONResponse(w, http.StatusCreated, map[string]interface{}{
		"session_id": sessionID,
		"files":      files,
		"links":      links,
	})
}

// FinalizeMediaRequest represents draft items to attach to a product
type FinalizeMediaRequest struct {
	SessionID string `json:"session_id"`
	Files     []struct {
		Kind     string `json:"kind"`
		GCSPath  string `json:"gcs_path"`
		Thumb    string `json:"thumb_path,omitempty"`
		Size     int64  `json:"size,omitempty"`
		Name     string `json:"name,omitempty"`
		MimeType string `json:"mime_type,omitempty"`
	} `json:"files"`
	Links []struct {
		Kind string `json:"kind"`
		URL  string `json:"url"`
	} `json:"links"`
}

type FinalizeMediaResponse struct {
	ProductID int64 `json:"product_id"`
	Created   int   `json:"created"`
}

// FinalizeDraftMedia attaches previously uploaded draft items to a product by creating DB rows.
//
//encore:api auth method=POST path=/products/:id/media/finalize
func FinalizeDraftMedia(ctx context.Context, id string, req *FinalizeMediaRequest) (*FinalizeMediaResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}

	// Check admin permissions
	if err := checkAdminPermission(ctx); err != nil {
		return nil, err
	}

	productID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errs.E(ctx, "CAT_INVALID_PRODUCT_ID", "معرّف المنتج غير صالح")
	}

	// Ensure the product exists
	p, err := catalogService.repo.GetProductByID(ctx, productID)
	if err != nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_READ_FAILED", "فشل جلب المنتج")
	}
	if p == nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_NOT_FOUND", "المنتج غير موجود")
	}

	created := 0
	// Create media rows for files
	for _, f := range req.Files {
		mk := MediaKindFile
		switch strings.ToLower(f.Kind) {
		case "image":
			mk = MediaKindImage
		case "video":
			mk = MediaKindVideo
		}

		var thumbPtr *string
		if strings.TrimSpace(f.Thumb) != "" {
			t := f.Thumb
			thumbPtr = &t
		}
		var sizePtr *int64
		if f.Size > 0 {
			s := f.Size
			sizePtr = &s
		}
		var mimePtr *string
		if strings.TrimSpace(f.MimeType) != "" {
			m := f.MimeType
			mimePtr = &m
		}
		var namePtr *string
		if strings.TrimSpace(f.Name) != "" {
			n := f.Name
			namePtr = &n
		}

		m := &Media{
			ProductID:        productID,
			Kind:             mk,
			GCSPath:          f.GCSPath,
			ThumbPath:        thumbPtr,
			WatermarkApplied: false,
			FileSize:         sizePtr,
			MimeType:         mimePtr,
			OriginalFilename: namePtr,
		}
		if err := catalogService.repo.CreateMedia(ctx, m); err != nil {
			return nil, errs.E(ctx, "CAT_MEDIA_SAVE_FAILED", "فشل حفظ سجل الوسائط")
		}
		created++
	}

	// Create media rows for links
	for _, l := range req.Links {
		_, err := catalogService.CreateExternalMedia(ctx, productID, l.URL, l.Kind, nil, nil, nil, nil)
		if err != nil {
			return nil, err
		}
		created++
	}

	return &FinalizeMediaResponse{ProductID: productID, Created: created}, nil
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
		// In local/dev environments it's common to not have GCS credentials configured.
		// Do not fail service initialization – continue without storage so read-only endpoints work.
		// Media uploads will be disabled until storage is configured.
		fmt.Printf("[catalog] storage init failed, continuing without GCS (thumbnails disabled): %v\n", err)
		storageClient = nil
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

// GetProduct retrieves a single product by ID or slug
//
//encore:api public method=GET path=/products/:id
func GetProduct(ctx context.Context, id string) (*ProductDetailResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}

	// Try to parse as ID first
	productID, err := strconv.ParseInt(id, 10, 64)
	if err == nil {
		// It's a valid ID, use it
		return catalogService.GetProductByID(ctx, productID)
	}

	// Not a valid ID, treat as slug
	return catalogService.GetProductBySlug(ctx, id)
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

// UpdateProduct updates an existing product (Admin only)
//
//encore:api auth method=PUT path=/products/:id
func UpdateProduct(ctx context.Context, id string, req *UpdateProductRequest) (*UpdateProductResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}

	// Check admin permissions
	if err := checkAdminPermission(ctx); err != nil {
		return nil, err
	}

	return catalogService.UpdateProduct(ctx, id, *req)
}

// DeleteProduct soft deletes a product (Admin only)
//
//encore:api auth method=DELETE path=/products/:id
func DeleteProduct(ctx context.Context, id string) (*DeleteProductResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}

	// Check admin permissions
	if err := checkAdminPermission(ctx); err != nil {
		return nil, err
	}

	return catalogService.DeleteProduct(ctx, id)
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

	// First, check if this is an external link upload (no file)
	linkURL := strings.TrimSpace(req.FormValue("link_url"))
	if linkURL != "" {
		kind := strings.TrimSpace(req.FormValue("kind"))
		response, err := catalogService.CreateExternalMedia(ctx, productID, linkURL, kind, nil, nil, nil, nil)
		if err != nil {
			writeErrorResponse(w, err)
			return
		}
		writeJSONResponse(w, http.StatusCreated, response)
		return
	}

	// Otherwise, expect a file upload
	file, header, err := req.FormFile("file")
	if err != nil {
		writeErrorResponse(w, errs.E(ctx, "CAT_FILE_REQUIRED", "ملف الرفع مطلوب"))
		return
	}
	defer file.Close()

	// Get description from form data (optional)
	var description *string
	descValue := strings.TrimSpace(req.FormValue("description"))
	if descValue != "" {
		description = &descValue
	}

	// Call service method
	response, err := catalogService.UploadMedia(ctx, productID, file, header, description)
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

	// Extract description and archived_at from request
	var description *string
	var archivedAt *time.Time

	if req != nil {
		description = req.Description
		archivedAt = req.ArchivedAt
	}

	return catalogService.UpdateMedia(ctx, productID, mediaID, description, archivedAt)
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

	// Generate signed URLs for media (valid for 1 hour)
	mediaWithURLs := make([]MediaWithURL, len(mediaList))
	for i, media := range mediaList {
		mediaWithURLs[i] = MediaWithURL{
			Media: media,
		}

		// Generate signed URL for main image
		if catalogService.storage != nil {
			signedURL, err := catalogService.storage.GetSecureURL(ctx, media.GCSPath, 1*time.Hour)
			if err == nil {
				mediaWithURLs[i].SignedURL = &signedURL
			}

			// Generate signed URL for thumbnail if exists
			if media.ThumbPath != nil && *media.ThumbPath != "" {
				thumbURL, err := catalogService.storage.GetSecureURL(ctx, *media.ThumbPath, 1*time.Hour)
				if err == nil {
					mediaWithURLs[i].ThumbSignedURL = &thumbURL
				}
			}
		}
	}

	return &ProductMediaListResponse{
		ProductID: productID,
		Media:     mediaWithURLs,
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

// ========================= Q&A: Products =========================

// GetProductQuestions lists approved questions for a product (public)
//
//encore:api public method=GET path=/catalog/products/:id/questions
func GetProductQuestions(ctx context.Context, id string) (*ProductQuestionsResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}

	productID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errs.E(ctx, "CAT_INVALID_PRODUCT_ID", "معرّف المنتج غير صالح")
	}
	// Optional: verify product exists (soft-fail if missing)
	if p, _ := catalogService.repo.GetProductByID(ctx, productID); p == nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_NOT_FOUND", "المنتج غير موجود")
	}

	items, err := catalogService.repo.ListProductQuestionsPublic(ctx, productID)
	if err != nil {
		return nil, errs.E(ctx, "CAT_Q_LIST_FAILED", "فشل جلب الأسئلة")
	}
	return &ProductQuestionsResponse{Items: items}, nil
}

// CreateProductQuestion creates a new question for a product (public)
//
//encore:api public method=POST path=/catalog/products/:id/questions
func CreateProductQuestion(ctx context.Context, id string, req *CreateQuestionRequest) (*ProductQuestion, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}
	if req == nil {
		return nil, errs.New(errs.InvalidArgument, "الطلب فارغ")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	productID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errs.E(ctx, "CAT_INVALID_PRODUCT_ID", "معرّف المنتج غير صالح")
	}
	if p, _ := catalogService.repo.GetProductByID(ctx, productID); p == nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_NOT_FOUND", "المنتج غير موجود")
	}
	var userIDPtr *int64
	if uidStr, ok := auth.UserID(); ok {
		if uid, e := strconv.ParseInt(string(uidStr), 10, 64); e == nil {
			userIDPtr = &uid
		}
	}
	q, err := catalogService.repo.CreateProductQuestion(ctx, productID, userIDPtr, req.Question)
	if err != nil {
		return nil, errs.E(ctx, "CAT_Q_CREATE_FAILED", "فشل إنشاء السؤال")
	}
	return q, nil
}

// Admin: list product questions with filters
//
//encore:api auth method=GET path=/catalog/admin/questions/products
func AdminListProductQuestions(ctx context.Context, req *ListQuestionsAdminRequest) (*ProductQuestionsResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}
	if err := checkAdminPermission(ctx); err != nil {
		return nil, err
	}
	var pidPtr *int64
	if req != nil && req.ProductID > 0 {
		pid := req.ProductID
		pidPtr = &pid
	}
	var statusPtr *QuestionStatus
	if req != nil && strings.TrimSpace(req.StatusStr) != "" {
		s := QuestionStatus(strings.ToLower(strings.TrimSpace(req.StatusStr)))
		switch s {
		case QuestionStatusPending, QuestionStatusApproved, QuestionStatusRejected:
			statusPtr = &s
		}
	}
	items, err := catalogService.repo.ListProductQuestionsAdmin(ctx, pidPtr, statusPtr)
	if err != nil {
		return nil, errs.E(ctx, "CAT_Q_LIST_FAILED", "فشل جلب الأسئلة")
	}
	return &ProductQuestionsResponse{Items: items}, nil
}

// Admin: answer a product question and approve it
//
//encore:api auth method=POST path=/catalog/admin/questions/products/:qid/answer
func AdminAnswerProductQuestion(ctx context.Context, qid string, req *AnswerQuestionRequest) (*ProductQuestion, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}
	if err := checkAdminPermission(ctx); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errs.New(errs.InvalidArgument, "الطلب فارغ")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	qidInt, err := strconv.ParseInt(qid, 10, 64)
	if err != nil {
		return nil, errs.New(errs.InvalidArgument, "معرّف السؤال غير صالح")
	}
	uidStr, _ := auth.UserID()
	answeredBy, _ := strconv.ParseInt(string(uidStr), 10, 64)
	q, err := catalogService.repo.AnswerProductQuestion(ctx, qidInt, req.Answer, answeredBy)
	if err != nil {
		return nil, errs.E(ctx, "CAT_Q_ANSWER_FAILED", "فشل حفظ الإجابة")
	}
	return q, nil
}

// Admin: set product question status
//
//encore:api auth method=PATCH path=/catalog/admin/questions/products/:qid/status
func AdminSetProductQuestionStatus(ctx context.Context, qid string, req *SetQuestionStatusRequest) (*MessageResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}
	if err := checkAdminPermission(ctx); err != nil {
		return nil, err
	}
	if req == nil || strings.TrimSpace(req.Status) == "" {
		return nil, errs.New(errs.InvalidArgument, "الحالة مطلوبة")
	}
	s := QuestionStatus(strings.ToLower(strings.TrimSpace(req.Status)))
	if s != QuestionStatusPending && s != QuestionStatusApproved && s != QuestionStatusRejected {
		return nil, errs.New(errs.InvalidArgument, "قيمة الحالة غير صالحة")
	}
	qidInt, err := strconv.ParseInt(qid, 10, 64)
	if err != nil {
		return nil, errs.New(errs.InvalidArgument, "معرّف السؤال غير صالح")
	}
	if err := catalogService.repo.SetProductQuestionStatus(ctx, qidInt, s); err != nil {
		return nil, errs.E(ctx, "CAT_Q_SET_STATUS_FAILED", "فشل تحديث الحالة")
	}
	return &MessageResponse{Message: "تم التحديث"}, nil
}

// Admin: list auction questions with filters
//
//encore:api auth method=GET path=/catalog/admin/questions/auctions
func AdminListAuctionQuestions(ctx context.Context, req *ListQuestionsAdminRequest) (*AuctionQuestionsResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}
	if err := checkAdminPermission(ctx); err != nil {
		return nil, err
	}
	var aidPtr *int64
	if req != nil && req.AuctionID > 0 {
		aid := req.AuctionID
		aidPtr = &aid
	}
	var statusPtr *QuestionStatus
	if req != nil && strings.TrimSpace(req.StatusStr) != "" {
		s := QuestionStatus(strings.ToLower(strings.TrimSpace(req.StatusStr)))
		switch s {
		case QuestionStatusPending, QuestionStatusApproved, QuestionStatusRejected:
			statusPtr = &s
		}
	}
	items, err := catalogService.repo.ListAuctionQuestionsAdmin(ctx, aidPtr, statusPtr)
	if err != nil {
		return nil, errs.E(ctx, "AUC_Q_LIST_FAILED", "فشل جلب الأسئلة")
	}
	return &AuctionQuestionsResponse{Items: items}, nil
}

// Admin: answer an auction question and approve it
//
//encore:api auth method=POST path=/catalog/admin/questions/auctions/:qid/answer
func AdminAnswerAuctionQuestion(ctx context.Context, qid string, req *AnswerQuestionRequest) (*AuctionQuestion, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}
	if err := checkAdminPermission(ctx); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errs.New(errs.InvalidArgument, "الطلب فارغ")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	qidInt, err := strconv.ParseInt(qid, 10, 64)
	if err != nil {
		return nil, errs.New(errs.InvalidArgument, "معرّف السؤال غير صالح")
	}
	uidStr, _ := auth.UserID()
	answeredBy, _ := strconv.ParseInt(string(uidStr), 10, 64)
	q, err := catalogService.repo.AnswerAuctionQuestion(ctx, qidInt, req.Answer, answeredBy)
	if err != nil {
		return nil, errs.E(ctx, "AUC_Q_ANSWER_FAILED", "فشل حفظ الإجابة")
	}
	return q, nil
}

// Admin: set auction question status
//
//encore:api auth method=PATCH path=/catalog/admin/questions/auctions/:qid/status
func AdminSetAuctionQuestionStatus(ctx context.Context, qid string, req *SetQuestionStatusRequest) (*MessageResponse, error) {
	if catalogService == nil {
		if err := InitService(); err != nil {
			return nil, errs.E(ctx, "CAT_INIT_FAILED", "فشل تهيئة خدمة الكتالوج")
		}
	}
	if err := checkAdminPermission(ctx); err != nil {
		return nil, err
	}
	if req == nil || strings.TrimSpace(req.Status) == "" {
		return nil, errs.New(errs.InvalidArgument, "الحالة مطلوبة")
	}
	s := QuestionStatus(strings.ToLower(strings.TrimSpace(req.Status)))
	if s != QuestionStatusPending && s != QuestionStatusApproved && s != QuestionStatusRejected {
		return nil, errs.New(errs.InvalidArgument, "قيمة الحالة غير صالحة")
	}
	qidInt, err := strconv.ParseInt(qid, 10, 64)
	if err != nil {
		return nil, errs.New(errs.InvalidArgument, "معرّف السؤال غير صالح")
	}
	if err := catalogService.repo.SetAuctionQuestionStatus(ctx, qidInt, s); err != nil {
		return nil, errs.E(ctx, "AUC_Q_SET_STATUS_FAILED", "فشل تحديث الحالة")
	}
	return &MessageResponse{Message: "تم التحديث"}, nil
}

// MediaWithURL extends Media with signed URLs
type MediaWithURL struct {
	Media          Media   `json:"media"`
	SignedURL      *string `json:"signed_url,omitempty"`       // Signed URL for main image
	ThumbSignedURL *string `json:"thumb_signed_url,omitempty"` // Signed URL for thumbnail
}

// ProductMediaListResponse represents the response for getting product media list
type ProductMediaListResponse struct {
	ProductID int64          `json:"product_id"`
	Media     []MediaWithURL `json:"media"`
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
