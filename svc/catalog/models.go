package catalog

import (
	"time"
	"database/sql/driver"
	"fmt"
)

// ProductType represents the type of product
type ProductType string

const (
	ProductTypePigeon ProductType = "pigeon"
	ProductTypeSupply ProductType = "supply"
)

// Value implements driver.Valuer interface for database storage
func (pt ProductType) Value() (driver.Value, error) {
	return string(pt), nil
}

// Scan implements sql.Scanner interface for database retrieval
func (pt *ProductType) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	if str, ok := value.(string); ok {
		*pt = ProductType(str)
		return nil
	}
	return fmt.Errorf("cannot scan %T into ProductType", value)
}

// ProductStatus represents the status of a product
type ProductStatus string

const (
	// Common statuses
	ProductStatusAvailable ProductStatus = "available"
	ProductStatusArchived  ProductStatus = "archived"

	// Pigeon-only statuses
	ProductStatusReserved            ProductStatus = "reserved"
	ProductStatusPaymentInProgress   ProductStatus = "payment_in_progress"
	ProductStatusInAuction          ProductStatus = "in_auction"
	ProductStatusAuctionHold        ProductStatus = "auction_hold"
	ProductStatusSold               ProductStatus = "sold"

	// Supply-only statuses
	ProductStatusOutOfStock ProductStatus = "out_of_stock"
)

// Value implements driver.Valuer interface for database storage
func (ps ProductStatus) Value() (driver.Value, error) {
	return string(ps), nil
}

// Scan implements sql.Scanner interface for database retrieval
func (ps *ProductStatus) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	if str, ok := value.(string); ok {
		*ps = ProductStatus(str)
		return nil
	}
	return fmt.Errorf("cannot scan %T into ProductStatus", value)
}

// IsValidForPigeon checks if status is valid for pigeon products
func (ps ProductStatus) IsValidForPigeon() bool {
	validStatuses := []ProductStatus{
		ProductStatusAvailable,
		ProductStatusReserved,
		ProductStatusPaymentInProgress,
		ProductStatusInAuction,
		ProductStatusAuctionHold,
		ProductStatusSold,
		ProductStatusArchived,
	}
	
	for _, status := range validStatuses {
		if ps == status {
			return true
		}
	}
	return false
}

// IsValidForSupply checks if status is valid for supply products
func (ps ProductStatus) IsValidForSupply() bool {
	validStatuses := []ProductStatus{
		ProductStatusAvailable,
		ProductStatusOutOfStock,
		ProductStatusArchived,
	}
	
	for _, status := range validStatuses {
		if ps == status {
			return true
		}
	}
	return false
}

// PigeonSex represents the sex of a pigeon
type PigeonSex string

const (
	PigeonSexMale    PigeonSex = "male"
	PigeonSexFemale  PigeonSex = "female"
	PigeonSexUnknown PigeonSex = "unknown"
)

// Value implements driver.Valuer interface for database storage
func (ps PigeonSex) Value() (driver.Value, error) {
	return string(ps), nil
}

// Scan implements sql.Scanner interface for database retrieval
func (ps *PigeonSex) Scan(value interface{}) error {
	if value == nil {
		*ps = PigeonSexUnknown
		return nil
	}
	if str, ok := value.(string); ok {
		*ps = PigeonSex(str)
		return nil
	}
	return fmt.Errorf("cannot scan %T into PigeonSex", value)
}

// MediaKind represents the type of media
type MediaKind string

const (
	MediaKindImage MediaKind = "image"
	MediaKindVideo MediaKind = "video"
	MediaKindFile  MediaKind = "file"
)

// Value implements driver.Valuer interface for database storage
func (mk MediaKind) Value() (driver.Value, error) {
	return string(mk), nil
}

// Scan implements sql.Scanner interface for database retrieval
func (mk *MediaKind) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	if str, ok := value.(string); ok {
		*mk = MediaKind(str)
		return nil
	}
	return fmt.Errorf("cannot scan %T into MediaKind", value)
}

// Product represents a product in the catalog
type Product struct {
	ID          int64         `json:"id" db:"id"`
	Type        ProductType   `json:"type" db:"type"`
	Title       string        `json:"title" db:"title"`
	Slug        string        `json:"slug" db:"slug"`
	Description *string       `json:"description" db:"description"`
	PriceNet    float64       `json:"price_net" db:"price_net"`
	Status      ProductStatus `json:"status" db:"status"`
	CreatedAt   time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at" db:"updated_at"`
	
	// Related entities (loaded separately)
	Pigeon *Pigeon `json:"pigeon,omitempty"`
	Supply *Supply `json:"supply,omitempty"`
	Media  []Media `json:"media,omitempty"`
}

// Pigeon represents pigeon-specific details
type Pigeon struct {
	ProductID           int64      `json:"product_id" db:"product_id"`
	RingNumber          string     `json:"ring_number" db:"ring_number"`
	Sex                 PigeonSex  `json:"sex" db:"sex"`
	BirthDate           *time.Time `json:"birth_date" db:"birth_date"`
	Lineage             *string    `json:"lineage" db:"lineage"`
	OriginProofURL      *string    `json:"origin_proof_url" db:"origin_proof_url"`
	OriginProofFileRef  *string    `json:"origin_proof_file_ref" db:"origin_proof_file_ref"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}

// Supply represents supply-specific details
type Supply struct {
	ProductID          int64     `json:"product_id" db:"product_id"`
	SKU                *string   `json:"sku" db:"sku"`
	StockQty           int       `json:"stock_qty" db:"stock_qty"`
	LowStockThreshold  int       `json:"low_stock_threshold" db:"low_stock_threshold"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

// IsLowStock returns true if the current stock is at or below the threshold
func (s Supply) IsLowStock() bool {
	return s.StockQty <= s.LowStockThreshold
}

// IsOutOfStock returns true if the stock quantity is zero
func (s Supply) IsOutOfStock() bool {
	return s.StockQty == 0
}

// Media represents media files associated with products
type Media struct {
	ID                  int64      `json:"id" db:"id"`
	ProductID           int64      `json:"product_id" db:"product_id"`
	Kind                MediaKind  `json:"kind" db:"kind"`
	GCSPath             string     `json:"gcs_path" db:"gcs_path"`
	ThumbPath           *string    `json:"thumb_path" db:"thumb_path"`
	WatermarkApplied    bool       `json:"watermark_applied" db:"watermark_applied"`
	FileSize            *int64     `json:"file_size" db:"file_size"`
	MimeType            *string    `json:"mime_type" db:"mime_type"`
	OriginalFilename    *string    `json:"original_filename" db:"original_filename"`
	ArchivedAt          *time.Time `json:"archived_at" db:"archived_at"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}

// IsArchived returns true if the media is archived
func (m Media) IsArchived() bool {
	return m.ArchivedAt != nil
}

// ProductWithDetails represents a complete product with all its details
type ProductWithDetails struct {
	Product
	Media []Media `json:"media"`
}

// GetPriceGross calculates the gross price including VAT
func (p Product) GetPriceGross(vatRate float64) float64 {
	return p.PriceNet * (1 + vatRate)
}

// IsAvailableForPurchase returns true if the product can be purchased
func (p Product) IsAvailableForPurchase() bool {
	switch p.Type {
	case ProductTypePigeon:
		return p.Status == ProductStatusAvailable
	case ProductTypeSupply:
		return p.Status == ProductStatusAvailable
	default:
		return false
	}
}

// IsAvailableForAuction returns true if the product can be put in auction
func (p Product) IsAvailableForAuction() bool {
	return p.Type == ProductTypePigeon && p.Status == ProductStatusAvailable
}
