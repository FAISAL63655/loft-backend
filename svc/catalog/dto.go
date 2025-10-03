package catalog

import (
	"strings"
	"time"

	"encore.app/pkg/errs"
)

// ProductsListRequest represents the request to list products with filters
type ProductsListRequest struct {
	TypeStr   string  `query:"type"`      // Filter by product type (pigeon/supply)
	StatusStr string  `query:"status"`    // Filter by product status
	Q         string  `query:"q"`         // Search query (title, description)
	PriceMin  float64 `query:"price_min"` // Minimum price filter (0 for no filter)
	PriceMax  float64 `query:"price_max"` // Maximum price filter (0 for no filter)
	Page      int     `query:"page"`      // Page number (default: 1)
	Limit     int     `query:"limit"`     // Items per page (default: 20, max: 100)
	SortStr   string  `query:"sort"`      // Sort order: "newest", "oldest", "price_asc", "price_desc"
}

// Validate validates the products list request and converts string parameters
func (req *ProductsListRequest) Validate() error {
	// Set default values
	if req.Page <= 0 {
		req.Page = 1
	}

	if req.Limit <= 0 {
		req.Limit = 20
	} else if req.Limit > 100 {
		req.Limit = 100
	}

	// Validate type filter
	if req.TypeStr != "" && req.TypeStr != "pigeon" && req.TypeStr != "supply" {
		return errs.New(errs.InvalidArgument, "type يجب أن يكون 'pigeon' أو 'supply'")
	}

	// Validate status filter
	validStatuses := []string{"available", "reserved", "payment_in_progress", "in_auction",
		"auction_hold", "sold", "out_of_stock", "archived"}
	if req.StatusStr != "" {
		valid := false
		for _, status := range validStatuses {
			if req.StatusStr == status {
				valid = true
				break
			}
		}
		if !valid {
			return errs.New(errs.InvalidArgument, "قيمة الحالة غير صالحة")
		}
	}

	// Validate price range
	if req.PriceMin < 0 {
		return errs.New(errs.InvalidArgument, "price_min يجب ألا يكون سالبًا")
	}

	if req.PriceMax < 0 {
		return errs.New(errs.InvalidArgument, "price_max يجب ألا يكون سالبًا")
	}

	if req.PriceMin > 0 && req.PriceMax > 0 && req.PriceMin > req.PriceMax {
		return errs.New(errs.InvalidArgument, "price_min لا يمكن أن يكون أكبر من price_max")
	}

	// Validate sort parameter
	if req.SortStr != "" {
		validSorts := []string{"newest", "oldest", "price_asc", "price_desc"}
		valid := false
		for _, sort := range validSorts {
			if req.SortStr == sort {
				valid = true
				break
			}
		}
		if !valid {
			return errs.New(errs.InvalidArgument, "sort يجب أن يكون: newest, oldest, price_asc, price_desc")
		}
	}

	return nil
}

// GetType returns the ProductType from string, or nil if empty
func (req *ProductsListRequest) GetType() *ProductType {
	if req.TypeStr == "" {
		return nil
	}
	productType := ProductType(req.TypeStr)
	return &productType
}

// GetStatus returns the ProductStatus from string, or nil if empty
func (req *ProductsListRequest) GetStatus() *ProductStatus {
	if req.StatusStr == "" {
		return nil
	}
	productStatus := ProductStatus(req.StatusStr)
	return &productStatus
}

// GetPriceMin returns pointer to PriceMin, or nil if zero
func (req *ProductsListRequest) GetPriceMin() *float64 {
	if req.PriceMin <= 0 {
		return nil
	}
	return &req.PriceMin
}

// GetPriceMax returns pointer to PriceMax, or nil if zero
func (req *ProductsListRequest) GetPriceMax() *float64 {
	if req.PriceMax <= 0 {
		return nil
	}
	return &req.PriceMax
}

// GetSort returns pointer to Sort, or nil if empty
func (req *ProductsListRequest) GetSort() *string {
	if req.SortStr == "" {
		return nil
	}
	return &req.SortStr
}

// GetQ returns pointer to Q, or nil if empty
func (req *ProductsListRequest) GetQ() *string {
	if req.Q == "" {
		return nil
	}
	return &req.Q
}

// ProductsListResponse represents the response for listing products
type ProductsListResponse struct {
	Products   []ProductSummary `json:"products"`
	Pagination PaginationMeta   `json:"pagination"`
}

// ProductSummary represents a product summary for listings
type ProductSummary struct {
	ID          int64         `json:"id"`
	Type        ProductType   `json:"type"`
	Title       string        `json:"title"`
	Slug        string        `json:"slug"`
	Description *string       `json:"description"`
	PriceNet    float64       `json:"price_net"`
	PriceGross  float64       `json:"price_gross"` // Calculated with current VAT
	Status      ProductStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`

	// Type-specific summary info
	RingNumber *string    `json:"ring_number,omitempty"` // For pigeons
	Sex        *PigeonSex `json:"sex,omitempty"`         // For pigeons
	BirthDate  *time.Time `json:"birth_date,omitempty"`  // For pigeons
	Lineage    *string    `json:"lineage,omitempty"`     // For pigeons
	StockQty   *int       `json:"stock_qty,omitempty"`   // For supplies

	// Media info
	ThumbnailURL *string `json:"thumbnail_url,omitempty"`
	MediaCount   int     `json:"media_count"`
}

// PaginationMeta represents pagination metadata
type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
}

// ProductDetailResponse represents the response for getting product details
type ProductDetailResponse struct {
	Product ProductWithDetails `json:"product"`
}

// CreateProductRequest represents a request to create a new product
type CreateProductRequest struct {
	Type        ProductType `json:"type"`
	Title       string      `json:"title"`
	Description *string     `json:"description"`
	PriceNet    float64     `json:"price_net"`

	// Pigeon-specific fields
	RingNumber         *string    `json:"ring_number,omitempty"`
	Sex                *PigeonSex `json:"sex,omitempty"`
	BirthDate          *time.Time `json:"birth_date,omitempty"`
	Lineage            *string    `json:"lineage,omitempty"`
	OriginProofURL     *string    `json:"origin_proof_url,omitempty"`
	OriginProofFileRef *string    `json:"origin_proof_file_ref,omitempty"`

	// Supply-specific fields
	SKU               *string `json:"sku,omitempty"`
	StockQty          *int    `json:"stock_qty,omitempty"`
	LowStockThreshold *int    `json:"low_stock_threshold,omitempty"`
}

// Validate validates the create product request
func (req *CreateProductRequest) Validate() error {
	if req.Title == "" {
		return errs.New(errs.InvalidArgument, "title مطلوب")
	}

	if req.PriceNet < 0 {
		return errs.New(errs.InvalidArgument, "price_net يجب ألا يكون سالبًا")
	}

	switch req.Type {
	case ProductTypePigeon:
		if req.RingNumber == nil || *req.RingNumber == "" {
			return errs.New(errs.InvalidArgument, "ring_number مطلوب لمنتجات الحمام")
		}

		// Supply fields should not be provided for pigeons
		if req.SKU != nil || req.StockQty != nil || req.LowStockThreshold != nil {
			return errs.New(errs.InvalidArgument, "حقول المستلزمات غير مسموحة لمنتجات الحمام")
		}

	case ProductTypeSupply:
		if req.StockQty == nil {
			return errs.New(errs.InvalidArgument, "stock_qty مطلوب لمنتجات المستلزمات")
		}

		if *req.StockQty < 0 {
			return errs.New(errs.InvalidArgument, "stock_qty يجب ألا يكون سالبًا")
		}

		if req.LowStockThreshold != nil && *req.LowStockThreshold <= 0 {
			return errs.New(errs.InvalidArgument, "low_stock_threshold يجب أن يكون موجبًا")
		}

		// Pigeon fields should not be provided for supplies
		if req.RingNumber != nil || req.Sex != nil || req.BirthDate != nil ||
			req.Lineage != nil || req.OriginProofURL != nil || req.OriginProofFileRef != nil {
			return errs.New(errs.InvalidArgument, "حقول الحمام غير مسموحة لمنتجات المستلزمات")
		}

	default:
		return errs.New(errs.InvalidArgument, "type يجب أن يكون 'pigeon' أو 'supply'")
	}

	return nil
}

// CreateProductResponse represents the response after creating a product
type CreateProductResponse struct {
	Product ProductWithDetails `json:"product"`
}

// UpdateProductRequest represents a request to update an existing product
type UpdateProductRequest struct {
	Title       *string     `json:"title,omitempty"`
	Description **string    `json:"description,omitempty"` // Double pointer to allow setting to null
	PriceNet    *float64    `json:"price_net,omitempty"`
	Status      *ProductStatus `json:"status,omitempty"`

	// Pigeon-specific fields
	RingNumber         *string    `json:"ring_number,omitempty"`
	Sex                *PigeonSex `json:"sex,omitempty"`
	BirthDate          *time.Time `json:"birth_date,omitempty"`
	Lineage            *string    `json:"lineage,omitempty"`
	OriginProofURL     *string    `json:"origin_proof_url,omitempty"`
	OriginProofFileRef *string    `json:"origin_proof_file_ref,omitempty"`

	// Supply-specific fields
	SKU               *string `json:"sku,omitempty"`
	StockQty          *int    `json:"stock_qty,omitempty"`
	LowStockThreshold *int    `json:"low_stock_threshold,omitempty"`
}

// Validate validates the update product request
func (req *UpdateProductRequest) Validate() error {
	if req.PriceNet != nil && *req.PriceNet < 0 {
		return errs.New(errs.InvalidArgument, "price_net يجب ألا يكون سالبًا")
	}

	if req.StockQty != nil && *req.StockQty < 0 {
		return errs.New(errs.InvalidArgument, "stock_qty يجب ألا يكون سالبًا")
	}

	if req.LowStockThreshold != nil && *req.LowStockThreshold <= 0 {
		return errs.New(errs.InvalidArgument, "low_stock_threshold يجب أن يكون موجبًا")
	}

	return nil
}

// UpdateProductResponse represents the response after updating a product
type UpdateProductResponse struct {
	Product ProductWithDetails `json:"product"`
}

// DeleteProductResponse represents the response after deleting a product
type DeleteProductResponse struct {
	Message string `json:"message"`
	ID      string `json:"id"`
}

// UploadMediaRequest represents a request to upload media for a product
type UploadMediaRequest struct {
	ProductID int64 `json:"id"`
	// File upload is handled separately by the endpoint
}

// UploadMediaResponse represents the response after uploading media
type UploadMediaResponse struct {
	Media Media `json:"media"`
}

// UpdateMediaRequest represents a request to update media
type UpdateMediaRequest struct {
	ProductID int64 `json:"product_id"`
	MediaID   int64 `json:"media_id"`

	ArchivedAt *time.Time `json:"archived_at"` // Set to archive/unarchive
}

// UpdateMediaResponse represents the response after updating media
type UpdateMediaResponse struct {
	Media Media `json:"media"`
}

// MediaUploadConfig represents configuration for media upload
type MediaUploadConfig struct {
	MaxFileSizeImage int64 // 10MB for images
	MaxFileSizeVideo int64 // 100MB for videos
	MaxFileSizeDoc   int64 // 10MB for documents
	AllowedImageExt  []string
	AllowedVideoExt  []string
	AllowedDocExt    []string
}

// DefaultMediaUploadConfig returns the default upload configuration per PRD
func DefaultMediaUploadConfig() MediaUploadConfig {
	return MediaUploadConfig{
		MaxFileSizeImage: 10 * 1024 * 1024,  // 10MB
		MaxFileSizeVideo: 100 * 1024 * 1024, // 100MB
		MaxFileSizeDoc:   10 * 1024 * 1024,  // 10MB
		AllowedImageExt:  []string{".jpg", ".jpeg", ".png", ".webp"},
		AllowedVideoExt:  []string{".mp4"},
		// Allow common document types: PDF and spreadsheets
		AllowedDocExt:    []string{".pdf", ".xlsx", ".xls", ".csv"},
	}
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Code          string                 `json:"code"`
	Message       string                 `json:"message"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Details       map[string]interface{} `json:"details,omitempty"`
}

// HealthCheckResponse represents a health check response
type HealthCheckResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// ========================= Q&A DTOs =========================

// CreateQuestionRequest represents a request to create a new question
type CreateQuestionRequest struct {
	Question string `json:"question"`
}

// Validate validates CreateQuestionRequest
func (r *CreateQuestionRequest) Validate() error {
	if len(strings.TrimSpace(r.Question)) < 3 {
		return errs.New(errs.InvalidArgument, "السؤال قصير جدًا (٣ أحرف على الأقل)")
	}
	if len(r.Question) > 2000 {
		return errs.New(errs.InvalidArgument, "السؤال طويل جدًا")
	}
	return nil
}

// AnswerQuestionRequest represents an admin answer to a question
type AnswerQuestionRequest struct {
	Answer string `json:"answer"`
}

func (r *AnswerQuestionRequest) Validate() error {
	if len(strings.TrimSpace(r.Answer)) < 2 {
		return errs.New(errs.InvalidArgument, "الإجابة قصيرة جدًا")
	}
	return nil
}

// SetQuestionStatusRequest represents a moderation status update
type SetQuestionStatusRequest struct {
	Status string `json:"status"`
}

// ListQuestionsAdminRequest represents filters for admin listing
type ListQuestionsAdminRequest struct {
	ProductID int64  `query:"product_id"`
	AuctionID int64  `query:"auction_id"`
	StatusStr string `query:"status"` // pending/approved/rejected
}

// ProductQuestionsResponse wraps product questions
type ProductQuestionsResponse struct {
	Items []ProductQuestion `json:"items"`
}

// AuctionQuestionsResponse wraps auction questions
type AuctionQuestionsResponse struct {
	Items []AuctionQuestion `json:"items"`
}

// MessageResponse is a simple message wrapper
type MessageResponse struct {
	Message string `json:"message"`
}
