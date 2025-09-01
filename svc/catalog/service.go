package catalog

import (
	"context"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"encore.dev/storage/sqldb"

	"encore.app/pkg/config"
	"encore.app/pkg/errs"
	"encore.app/pkg/slugify"
	"encore.app/pkg/storagegcs"
)

// Service handles catalog business logic
type Service struct {
	repo      *Repository
	slugifier *slugify.Slugifier
	storage   *storagegcs.Client
	config    *config.ConfigManager
}

// NewService creates a new catalog service
func NewService(
	db *sqldb.Database,
	slugifier *slugify.Slugifier,
	storage *storagegcs.Client,
	configMgr *config.ConfigManager,
) *Service {
	return &Service{
		repo:      NewRepository(db),
		slugifier: slugifier,
		storage:   storage,
		config:    configMgr,
	}
}

// executeWithTransaction executes a function within a database transaction
func (s *Service) executeWithTransaction(ctx context.Context, fn func(context.Context) error) error {
	tx, err := s.repo.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("فشل بدء المعاملة: %w", err)
	}
	defer tx.Rollback()
	
	// Execute the function
	if err := fn(ctx); err != nil {
		return err
	}
	
	// Commit if successful
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("فشل تأكيد المعاملة: %w", err)
	}
	
	return nil
}

// GetProducts retrieves products with filtering, sorting, and pagination
func (s *Service) GetProducts(ctx context.Context, req ProductsListRequest) (*ProductsListResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Convert request to filter using helper methods
	filter := ProductsFilter{
		Type:     req.GetType(),
		Status:   req.GetStatus(),
		Search:   req.GetQ(),
		PriceMin: req.GetPriceMin(),
		PriceMax: req.GetPriceMax(),
	}

	// Convert sort parameter
	sort := ProductsSort{
		Field:     "created_at",
		Direction: "DESC",
	}

	if sortPtr := req.GetSort(); sortPtr != nil {
		switch *sortPtr {
		case "newest":
			sort.Field = "created_at"
			sort.Direction = "DESC"
		case "oldest":
			sort.Field = "created_at"
			sort.Direction = "ASC"
		case "price_asc":
			sort.Field = "price_net"
			sort.Direction = "ASC"
		case "price_desc":
			sort.Field = "price_net"
			sort.Direction = "DESC"
		}
	}

	// Calculate offset
	offset := (req.Page - 1) * req.Limit

	// Get products from repository
	products, totalCount, err := s.repo.GetProducts(ctx, filter, sort, offset, req.Limit)
	if err != nil {
		return nil, errs.E(ctx, "CAT_PRODUCTS_READ_FAILED", "فشل جلب قائمة المنتجات")
	}

	// Get current VAT settings from config manager
	var vatSettings *VATSettings
	if s.config != nil {
		st := s.config.GetSettings()
		vatSettings = &VATSettings{Enabled: st.VATEnabled, Rate: st.VATRate}
	} else {
		vatSettings = &VATSettings{Enabled: true, Rate: 0.15}
	}

	// Convert to summary format
	summaries := make([]ProductSummary, len(products))
	for i, product := range products {
		summary, err := s.convertToSummary(ctx, product, vatSettings)
		if err != nil {
			return nil, errs.E(ctx, "CAT_PRODUCT_SUMMARY_FAILED", "فشل تحويل بيانات المنتج")
		}
		summaries[i] = *summary
	}

	// Calculate pagination
	totalPages := int((totalCount + int64(req.Limit) - 1) / int64(req.Limit))
	pagination := PaginationMeta{
		Page:       req.Page,
		Limit:      req.Limit,
		TotalItems: totalCount,
		TotalPages: totalPages,
		HasNext:    req.Page < totalPages,
		HasPrev:    req.Page > 1,
	}

	return &ProductsListResponse{
		Products:   summaries,
		Pagination: pagination,
	}, nil
}

// GetProductByID retrieves a single product by ID with full details
func (s *Service) GetProductByID(ctx context.Context, id int64) (*ProductDetailResponse, error) {
	// Get product
	product, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_READ_FAILED", "فشل جلب المنتج")
	}

	if product == nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_NOT_FOUND", "المنتج غير موجود")
	}

	// Get full details
	productWithDetails, err := s.getProductWithDetails(ctx, product)
	if err != nil {
		return nil, err
	}

	return &ProductDetailResponse{
		Product: *productWithDetails,
	}, nil
}

// CreateProduct creates a new product with all details using database transaction
func (s *Service) CreateProduct(ctx context.Context, req CreateProductRequest) (*CreateProductResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Generate slug
	slug, err := s.slugifier.GenerateUnique(ctx, req.Title, "products", "slug")
	if err != nil {
		return nil, errs.E(ctx, "CAT_SLUG_GENERATE_FAILED", "فشل إنشاء معرّف slug للمنتج")
	}

	// Check for pigeon ring number uniqueness
	if req.Type == ProductTypePigeon && req.RingNumber != nil {
		exists, err := s.repo.CheckRingNumberExists(ctx, *req.RingNumber)
		if err != nil {
			return nil, errs.E(ctx, "CAT_RING_CHECK_FAILED", "فشل التحقق من رقم الحلقة")
		}
		if exists {
			return nil, errs.E(ctx, "CAT_RING_ALREADY_EXISTS", "رقم الحلقة موجود مسبقًا")
		}
	}

	// Use wrapper function for atomic product creation
	var product *Product
	var createErr error
	
	// Wrapper function to execute product creation atomically
	executeProductCreation := func(ctx context.Context) error {
		// Create product
		product = &Product{
			Type:        req.Type,
			Title:       req.Title,
			Slug:        slug,
			Description: req.Description,
			PriceNet:    req.PriceNet,
			Status:      ProductStatusAvailable,
		}

		if err := s.repo.CreateProduct(ctx, product); err != nil {
			return err
		}

		// Create type-specific details
		switch req.Type {
		case ProductTypePigeon:
			pigeon := &Pigeon{
				ProductID:          product.ID,
				RingNumber:         *req.RingNumber,
				Sex:                PigeonSexUnknown,
				BirthDate:          req.BirthDate,
				Lineage:            req.Lineage,
				OriginProofURL:     req.OriginProofURL,
				OriginProofFileRef: req.OriginProofFileRef,
			}

			if req.Sex != nil {
				pigeon.Sex = *req.Sex
			}

			if err := s.repo.CreatePigeon(ctx, pigeon); err != nil {
				return err
			}

		case ProductTypeSupply:
			threshold := 5 // default
			if req.LowStockThreshold != nil {
				threshold = *req.LowStockThreshold
			}

			supply := &Supply{
				ProductID:         product.ID,
				SKU:               req.SKU,
				StockQty:          *req.StockQty,
				LowStockThreshold: threshold,
			}

			if err := s.repo.CreateSupply(ctx, supply); err != nil {
				return err
			}
		}
		return nil
	}
	
	// Execute within transaction using wrapper
	createErr = s.executeWithTransaction(ctx, executeProductCreation)
	if createErr != nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_CREATE_FAILED", "فشل إنشاء المنتج: " + createErr.Error())
	}

	// Get full product with details (outside transaction)
	productWithDetails, err := s.getProductWithDetails(ctx, product)
	if err != nil {
		return nil, err
	}

	return &CreateProductResponse{
		Product: *productWithDetails,
	}, nil
}

// UploadMedia uploads media file for a product
func (s *Service) UploadMedia(ctx context.Context, productID int64, file multipart.File, header *multipart.FileHeader) (*UploadMediaResponse, error) {
	// Check if product exists
	product, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_READ_FAILED", "فشل التحقق من وجود المنتج")
	}

	if product == nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_NOT_FOUND", "المنتج غير موجود")
	}

	// Validate file
	config := DefaultMediaUploadConfig()
	if err := s.validateMediaFile(header, config); err != nil {
		return nil, err
	}

	// Placeholder media settings
	mediaSettings := struct {
		ThumbnailsEnabled bool
		WatermarkEnabled  bool
		WatermarkOpacity  int
		WatermarkPosition string
	}{
		ThumbnailsEnabled: true,
		WatermarkEnabled:  true,
		WatermarkOpacity:  30,
		WatermarkPosition: "center",
	}

	// Prepare upload configuration
	uploadConfig := storagegcs.UploadConfig{
		GenerateThumbnails: mediaSettings.ThumbnailsEnabled,
		ApplyWatermark:     mediaSettings.WatermarkEnabled,
		WatermarkOpacity:   mediaSettings.WatermarkOpacity,
		WatermarkPosition:  mediaSettings.WatermarkPosition,
		ThumbnailSizes:     []int{200, 400}, // Standard thumbnail sizes
	}

	// Upload to GCS
	uploadResult, err := s.storage.Upload(ctx, file, header.Filename, uploadConfig)
	if err != nil {
		return nil, errs.E(ctx, "CAT_MEDIA_UPLOAD_FAILED", "فشل رفع الملف إلى التخزين")
	}

	// Create media record
	media := &Media{
		ProductID:        productID,
		Kind:             MediaKind(uploadResult.Kind),
		GCSPath:          uploadResult.GCSPath,
		ThumbPath:        &uploadResult.ThumbPath,
		WatermarkApplied: uploadResult.WatermarkApplied,
		FileSize:         &uploadResult.Size,
		MimeType:         func() *string { ct := header.Header.Get("Content-Type"); return &ct }(),
		OriginalFilename: &header.Filename,
	}

	if err := s.repo.CreateMedia(ctx, media); err != nil {
		return nil, errs.E(ctx, "CAT_MEDIA_SAVE_FAILED", "فشل حفظ سجل الوسائط")
	}

	return &UploadMediaResponse{
		Media: *media,
	}, nil
}

// UpdateMedia updates media properties (mainly for archiving/unarchiving)
func (s *Service) UpdateMedia(ctx context.Context, productID, mediaID int64, archivedAt *time.Time) (*UpdateMediaResponse, error) {
	// Get media
	media, err := s.repo.GetMediaByID(ctx, mediaID)
	if err != nil {
		return nil, errs.E(ctx, "CAT_MEDIA_READ_FAILED", "فشل جلب الوسائط")
	}

	if media == nil {
		return nil, errs.E(ctx, "CAT_MEDIA_NOT_FOUND", "الوسائط غير موجودة")
	}

	// Check if media belongs to the product
	if media.ProductID != productID {
		return nil, errs.E(ctx, "CAT_MEDIA_NOT_FOR_PRODUCT", "الوسائط غير تابعة لهذا المنتج")
	}

	// Update media
	media.ArchivedAt = archivedAt
	if err := s.repo.UpdateMedia(ctx, media); err != nil {
		return nil, errs.E(ctx, "CAT_MEDIA_UPDATE_FAILED", "فشل تحديث الوسائط")
	}

	return &UpdateMediaResponse{
		Media: *media,
	}, nil
}

// VATSettings represents VAT configuration
type VATSettings struct {
	Enabled bool
	Rate    float64
}

// getVATSettings retrieves VAT settings from system_settings table
func (s *Service) getVATSettings(ctx context.Context) (*VATSettings, error) {
	if s.config != nil {
		st := s.config.GetSettings()
		return &VATSettings{Enabled: st.VATEnabled, Rate: st.VATRate}, nil
	}
	return &VATSettings{Enabled: true, Rate: 0.15}, nil
}

// getMediaSettings retrieves media processing settings from system_settings table
func (s *Service) getMediaSettings(ctx context.Context) (*MediaSettings, error) {
	// Defaults
	settings := &MediaSettings{
		WatermarkEnabled:  true,
		WatermarkPosition: "bottom-right",
		WatermarkOpacity:  0.7,
		ThumbnailsEnabled: true,
		ThumbnailSizes:    []int{200, 400},
		MaxFileSize:       10485760, // 10MB
		AllowedTypes:      []string{"image/jpeg", "image/png", "image/webp", "video/mp4"},
	}
	if s.config == nil {
		return settings, nil
	}
	st := s.config.GetSettings()
	settings.WatermarkEnabled = st.MediaWatermarkEnabled
	if st.MediaWatermarkPosition != "" {
		settings.WatermarkPosition = st.MediaWatermarkPosition
	}
	if st.MediaWatermarkOpacity > 0 {
		settings.WatermarkOpacity = st.MediaWatermarkOpacity
	}
	if st.MediaMaxFileSize > 0 {
		settings.MaxFileSize = st.MediaMaxFileSize
	}
	if len(st.MediaAllowedTypes) > 0 {
		settings.AllowedTypes = st.MediaAllowedTypes
	}
	return settings, nil
}

// MediaSettings represents media processing configuration
type MediaSettings struct {
	WatermarkEnabled  bool
	WatermarkPosition string
	WatermarkOpacity  float64
	ThumbnailsEnabled bool
	ThumbnailSizes    []int
	MaxFileSize       int64
	AllowedTypes      []string
}

// convertToSummary converts a Product to ProductSummary with additional info
func (s *Service) convertToSummary(ctx context.Context, product Product, vatSettings *VATSettings) (*ProductSummary, error) {
	summary := ProductSummary{
		ID:          product.ID,
		Type:        product.Type,
		Title:       product.Title,
		Slug:        product.Slug,
		Description: product.Description,
		PriceNet:    product.PriceNet,
		PriceGross:  product.GetPriceGross(vatSettings.Rate),
		Status:      product.Status,
		CreatedAt:   product.CreatedAt,
	}

	// Add type-specific information
	switch product.Type {
	case ProductTypePigeon:
		pigeon, err := s.repo.GetPigeonByProductID(ctx, product.ID)
		if err == nil && pigeon != nil {
			summary.RingNumber = &pigeon.RingNumber
		}

	case ProductTypeSupply:
		supply, err := s.repo.GetSupplyByProductID(ctx, product.ID)
		if err == nil && supply != nil {
			summary.StockQty = &supply.StockQty
		}
	}

	// Add media information
	mediaList, err := s.repo.GetMediaByProductID(ctx, product.ID, false)
	if err == nil {
		summary.MediaCount = len(mediaList)

		// Find first image for thumbnail
		for _, media := range mediaList {
			if media.Kind == MediaKindImage && media.ThumbPath != nil {
				publicURL := s.storage.GetPublicURL(*media.ThumbPath)
				summary.ThumbnailURL = &publicURL
				break
			}
		}
	}

	return &summary, nil
}

// getProductWithDetails retrieves a product with all its details
func (s *Service) getProductWithDetails(ctx context.Context, product *Product) (*ProductWithDetails, error) {
	details := ProductWithDetails{
		Product: *product,
	}

	// Get type-specific details
	switch product.Type {
	case ProductTypePigeon:
		pigeon, err := s.repo.GetPigeonByProductID(ctx, product.ID)
		if err != nil {
			return nil, errs.E(ctx, "CAT_PIGEON_READ_FAILED", "فشل جلب تفاصيل الحمامة")
		}
		details.Pigeon = pigeon

	case ProductTypeSupply:
		supply, err := s.repo.GetSupplyByProductID(ctx, product.ID)
		if err != nil {
			return nil, errs.E(ctx, "CAT_SUPPLY_READ_FAILED", "فشل جلب تفاصيل المستلزم")
		}
		details.Supply = supply
	}

	// Get media
	media, err := s.repo.GetMediaByProductID(ctx, product.ID, false)
	if err != nil {
		return nil, errs.E(ctx, "CAT_MEDIA_READ_FAILED", "فشل جلب الوسائط")
	}
	details.Media = media

	return &details, nil
}

// validateMediaFile validates uploaded media file with comprehensive checks
func (s *Service) validateMediaFile(header *multipart.FileHeader, config MediaUploadConfig) error {
	filename := header.Filename
	fileSize := header.Size
	ext := strings.ToLower(filepath.Ext(filename))

	// Basic validation
	if filename == "" {
		return errs.New(errs.InvalidArgument, "اسم الملف مطلوب")
	}

	if fileSize <= 0 {
		return errs.New(errs.InvalidArgument, "الملف فارغ")
	}

	// Check for dangerous file extensions
	dangerousExt := []string{".exe", ".bat", ".cmd", ".com", ".scr", ".vbs", ".js", ".jar"}
	for _, dangerous := range dangerousExt {
		if ext == dangerous {
			return errs.New(errs.InvalidArgument, "نوع الملف غير مسموح لأسباب أمنية")
		}
	}

	// Check file extension and size based on type
	switch {
	case contains(config.AllowedImageExt, ext):
		if fileSize > config.MaxFileSizeImage {
			return errs.New(errs.InvalidArgument, fmt.Sprintf("صورة كبيرة جدًا. الحد الأقصى: %d م.ب", config.MaxFileSizeImage/(1024*1024)))
		}

	case contains(config.AllowedVideoExt, ext):
		if fileSize > config.MaxFileSizeVideo {
			return errs.New(errs.InvalidArgument, fmt.Sprintf("فيديو كبير جدًا. الحد الأقصى: %d م.ب", config.MaxFileSizeVideo/(1024*1024)))
		}

	case contains(config.AllowedDocExt, ext):
		if fileSize > config.MaxFileSizeDoc {
			return errs.New(errs.InvalidArgument, fmt.Sprintf("مستند كبير جدًا. الحد الأقصى: %d م.ب", config.MaxFileSizeDoc/(1024*1024)))
		}

	default:
		return errs.New(errs.InvalidArgument, fmt.Sprintf("النوع %s غير مدعوم. الأنواع المسموحة: %v, %v, %v",
			ext, config.AllowedImageExt, config.AllowedVideoExt, config.AllowedDocExt))
	}

	return nil
}

// Helper function to check if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
