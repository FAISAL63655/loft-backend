package catalog

import (
	"context"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strconv"
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

// parseProductID parses a product ID string to int64
func parseProductID(id string) (int64, error) {
	return strconv.ParseInt(id, 10, 64)
}

// CreateExternalMedia creates a media entry for an external link (e.g., YouTube or general link)
func (s *Service) CreateExternalMedia(
    ctx context.Context,
    productID int64,
    linkURL string,
    kind string,
    thumbPath *string,
    fileSize *int64,
    mimeType *string,
    originalFilename *string,
) (*UploadMediaResponse, error) {
    // Basic validation
    if !strings.HasPrefix(linkURL, "http://") && !strings.HasPrefix(linkURL, "https://") {
        return nil, errs.E(ctx, "CAT_MEDIA_LINK_INVALID", "رابط غير صالح")
    }

    // Ensure product exists
    product, err := s.repo.GetProductByID(ctx, productID)
    if err != nil {
        return nil, errs.E(ctx, "CAT_PRODUCT_READ_FAILED", "فشل التحقق من وجود المنتج")
    }
    if product == nil {
        return nil, errs.E(ctx, "CAT_PRODUCT_NOT_FOUND", "المنتج غير موجود")
    }

    // Map provided kind to media kind
    mk := MediaKindFile
    switch strings.ToLower(kind) {
    case "youtube", "video":
        mk = MediaKindVideo
    case "image":
        mk = MediaKindImage
    default:
        mk = MediaKindFile
    }

    // Create media record pointing to the external URL directly in GCSPath field
    media := &Media{
        ProductID:        productID,
        Kind:             mk,
        GCSPath:          linkURL,
        ThumbPath:        nil,
        WatermarkApplied: false,
        FileSize:         nil,
        MimeType:         nil,
        OriginalFilename: nil,
    }

    if thumbPath != nil && *thumbPath != "" {
        media.ThumbPath = thumbPath
    }
    if fileSize != nil && *fileSize > 0 {
        media.FileSize = fileSize
    }
    if mimeType != nil && *mimeType != "" {
        media.MimeType = mimeType
    }
    if originalFilename != nil && *originalFilename != "" {
        media.OriginalFilename = originalFilename
    }

    if err := s.repo.CreateMedia(ctx, media); err != nil {
        return nil, errs.E(ctx, "CAT_MEDIA_SAVE_FAILED", "فشل حفظ سجل الوسائط")
    }

    return &UploadMediaResponse{Media: *media}, nil
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

// UpdateProduct updates an existing product with provided details
func (s *Service) UpdateProduct(ctx context.Context, id string, req UpdateProductRequest) (*UpdateProductResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Parse product ID
	productID, err := parseProductID(id)
	if err != nil {
		return nil, errs.E(ctx, "CAT_INVALID_PRODUCT_ID", "معرّف المنتج غير صالح")
	}

	// Get existing product
	existingProduct, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_READ_FAILED", "فشل قراءة المنتج")
	}
	if existingProduct == nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_NOT_FOUND", "المنتج غير موجود")
	}

	// Check for pigeon ring number uniqueness if updating ring number
	if existingProduct.Type == ProductTypePigeon && req.RingNumber != nil {
		// Check if another pigeon has this ring number (excluding current product)
		exists, err := s.repo.CheckRingNumberExistsExcluding(ctx, *req.RingNumber, productID)
		if err != nil {
			return nil, errs.E(ctx, "CAT_RING_CHECK_FAILED", "فشل التحقق من رقم الحلقة")
		}
		if exists {
			return nil, errs.E(ctx, "CAT_RING_ALREADY_EXISTS", "رقم الحلقة موجود مسبقًا")
		}
	}

	// Use wrapper function for atomic product update
	var updatedProduct *Product
	var updateErr error
	
	// Wrapper function to execute product update atomically
	executeProductUpdate := func(ctx context.Context) error {
		// Update product fields
		updates := make(map[string]interface{})
		
		if req.Title != nil {
			updates["title"] = *req.Title
			// Generate new slug if title changed
			if *req.Title != existingProduct.Title {
				newSlug, err := s.slugifier.GenerateUnique(ctx, *req.Title, "products", "slug")
				if err != nil {
					return errs.E(ctx, "CAT_SLUG_GENERATE_FAILED", "فشل إنشاء معرّف slug للمنتج")
				}
				updates["slug"] = newSlug
			}
		}
		
		if req.Description != nil {
			updates["description"] = *req.Description
		}
		
		if req.PriceNet != nil {
			updates["price_net"] = *req.PriceNet
		}
		
		if req.Status != nil {
			updates["status"] = *req.Status
		}

		// Update product table
		if len(updates) > 0 {
			if err := s.repo.UpdateProductPartial(ctx, productID, updates); err != nil {
				return err
			}
		}

		// Update type-specific details
		switch existingProduct.Type {
		case ProductTypePigeon:
			pigeonUpdates := make(map[string]interface{})
			
			if req.RingNumber != nil {
				pigeonUpdates["ring_number"] = *req.RingNumber
			}
			if req.Sex != nil {
				pigeonUpdates["sex"] = *req.Sex
			}
			if req.BirthDate != nil {
				pigeonUpdates["birth_date"] = *req.BirthDate
			}
			if req.Lineage != nil {
				pigeonUpdates["lineage"] = *req.Lineage
			}
			if req.OriginProofURL != nil {
				pigeonUpdates["origin_proof_url"] = *req.OriginProofURL
			}
			if req.OriginProofFileRef != nil {
				pigeonUpdates["origin_proof_file_ref"] = *req.OriginProofFileRef
			}

			if len(pigeonUpdates) > 0 {
				if err := s.repo.UpdatePigeon(ctx, productID, pigeonUpdates); err != nil {
					return err
				}
			}

		case ProductTypeSupply:
			supplyUpdates := make(map[string]interface{})
			
			if req.SKU != nil {
				supplyUpdates["sku"] = *req.SKU
			}
			if req.StockQty != nil {
				supplyUpdates["stock_qty"] = *req.StockQty
			}
			if req.LowStockThreshold != nil {
				supplyUpdates["low_stock_threshold"] = *req.LowStockThreshold
			}

			if len(supplyUpdates) > 0 {
				if err := s.repo.UpdateSupply(ctx, productID, supplyUpdates); err != nil {
					return err
				}
			}
		}
		return nil
	}
	
	// Execute within transaction using wrapper
	updateErr = s.executeWithTransaction(ctx, executeProductUpdate)
	if updateErr != nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_UPDATE_FAILED", "فشل تحديث المنتج: " + updateErr.Error())
	}

	// Get updated product with details (outside transaction)
	updatedProduct, err = s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_READ_FAILED", "فشل قراءة المنتج المحدث")
	}

	productWithDetails, err := s.getProductWithDetails(ctx, updatedProduct)
	if err != nil {
		return nil, err
	}

	return &UpdateProductResponse{
		Product: *productWithDetails,
	}, nil
}

// DeleteProduct soft deletes a product by setting its status to archived
func (s *Service) DeleteProduct(ctx context.Context, id string) (*DeleteProductResponse, error) {
	// Parse product ID
	productID, err := parseProductID(id)
	if err != nil {
		return nil, errs.E(ctx, "CAT_INVALID_PRODUCT_ID", "معرّف المنتج غير صالح")
	}

	// Get existing product
	existingProduct, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_READ_FAILED", "فشل قراءة المنتج")
	}
	if existingProduct == nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_NOT_FOUND", "المنتج غير موجود")
	}

	// Check if product is already archived
	if existingProduct.Status == ProductStatusArchived {
		return nil, errs.E(ctx, "CAT_PRODUCT_ALREADY_ARCHIVED", "المنتج محذوف بالفعل")
	}

	// Soft delete by setting status to archived
	updates := map[string]interface{}{
		"status": ProductStatusArchived,
	}

	if err := s.repo.UpdateProductPartial(ctx, productID, updates); err != nil {
		return nil, errs.E(ctx, "CAT_PRODUCT_DELETE_FAILED", "فشل حذف المنتج: " + err.Error())
	}

	return &DeleteProductResponse{
		Message: "تم حذف المنتج بنجاح",
		ID:      id,
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
			summary.Sex = &pigeon.Sex
			summary.BirthDate = pigeon.BirthDate
			summary.Lineage = pigeon.Lineage
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
				// If storage is not configured (e.g., local without GCS), skip generating public URL
				if s.storage != nil {
					publicURL := s.storage.GetPublicURL(*media.ThumbPath)
					summary.ThumbnailURL = &publicURL
				}
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
