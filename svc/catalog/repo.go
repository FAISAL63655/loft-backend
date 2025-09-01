package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"encore.dev/storage/sqldb"
)

// Repository handles database operations for catalog
type Repository struct {
	db *sqldb.Database
}

// NewRepository creates a new catalog repository
func NewRepository(db *sqldb.Database) *Repository {
	return &Repository{db: db}
}

// ProductsFilter represents filters for product queries
type ProductsFilter struct {
	Type     *ProductType
	Status   *ProductStatus
	Search   *string
	PriceMin *float64
	PriceMax *float64
}

// ProductsSort represents sorting options for products
type ProductsSort struct {
	Field     string // "created_at", "price_net", "title"
	Direction string // "ASC", "DESC"
}

// GetProducts retrieves products with optional filters, sorting, and pagination
func (r *Repository) GetProducts(ctx context.Context, filter ProductsFilter, sort ProductsSort, offset, limit int) ([]Product, int64, error) {
	// Build WHERE clause
	whereClauses := []string{}
	args := []interface{}{}
	argCount := 0

	if filter.Type != nil {
		argCount++
		whereClauses = append(whereClauses, fmt.Sprintf("p.type = $%d", argCount))
		args = append(args, *filter.Type)
	}

	if filter.Status != nil {
		argCount++
		whereClauses = append(whereClauses, fmt.Sprintf("p.status = $%d", argCount))
		args = append(args, *filter.Status)
	}

	if filter.Search != nil && *filter.Search != "" {
		argCount++
		whereClauses = append(whereClauses, fmt.Sprintf("(p.title ILIKE $%d OR p.description ILIKE $%d)", argCount, argCount))
		args = append(args, "%"+*filter.Search+"%")
	}

	if filter.PriceMin != nil {
		argCount++
		whereClauses = append(whereClauses, fmt.Sprintf("p.price_net >= $%d", argCount))
		args = append(args, *filter.PriceMin)
	}

	if filter.PriceMax != nil {
		argCount++
		whereClauses = append(whereClauses, fmt.Sprintf("p.price_net <= $%d", argCount))
		args = append(args, *filter.PriceMax)
	}

	// Build WHERE clause string
	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Build ORDER BY clause
	orderBy := fmt.Sprintf("ORDER BY p.%s %s", sort.Field, sort.Direction)

	// Count total items
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM products p
		%s
	`, whereClause)

	var totalCount int64
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count products: %w", err)
	}

	// Get products with pagination
	argCount++
	args = append(args, limit)
	argCount++
	args = append(args, offset)

	query := fmt.Sprintf(`
		SELECT 
			p.id, p.type, p.title, p.slug, p.description, 
			p.price_net, p.status, p.created_at, p.updated_at
		FROM products p
		%s
		%s
		LIMIT $%d OFFSET $%d
	`, whereClause, orderBy, argCount-1, argCount)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query products: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		err := rows.Scan(
			&p.ID, &p.Type, &p.Title, &p.Slug, &p.Description,
			&p.PriceNet, &p.Status, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan product: %w", err)
		}
		products = append(products, p)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating products: %w", err)
	}

	return products, totalCount, nil
}

// GetProductByID retrieves a single product by ID
func (r *Repository) GetProductByID(ctx context.Context, id int64) (*Product, error) {
	query := `
		SELECT 
			id, type, title, slug, description, 
			price_net, status, created_at, updated_at
		FROM products
		WHERE id = $1
	`

	var p Product
	err := r.db.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.Type, &p.Title, &p.Slug, &p.Description,
		&p.PriceNet, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Product not found
		}
		return nil, fmt.Errorf("failed to get product: %w", err)
	}

	return &p, nil
}

// GetProductBySlug retrieves a single product by slug
func (r *Repository) GetProductBySlug(ctx context.Context, slug string) (*Product, error) {
	query := `
		SELECT 
			id, type, title, slug, description, 
			price_net, status, created_at, updated_at
		FROM products
		WHERE slug = $1
	`

	var p Product
	err := r.db.QueryRow(ctx, query, slug).Scan(
		&p.ID, &p.Type, &p.Title, &p.Slug, &p.Description,
		&p.PriceNet, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Product not found
		}
		return nil, fmt.Errorf("failed to get product by slug: %w", err)
	}

	return &p, nil
}

// CreateProduct creates a new product
func (r *Repository) CreateProduct(ctx context.Context, product *Product) error {
	query := `
		INSERT INTO products (type, title, slug, description, price_net, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		product.Type, product.Title, product.Slug, product.Description,
		product.PriceNet, product.Status,
	).Scan(&product.ID, &product.CreatedAt, &product.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create product: %w", err)
	}

	return nil
}

// UpdateProduct updates an existing product
func (r *Repository) UpdateProduct(ctx context.Context, product *Product) error {
	query := `
		UPDATE products 
		SET title = $2, description = $3, price_net = $4, status = $5, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		WHERE id = $1
		RETURNING updated_at
	`

	err := r.db.QueryRow(ctx, query,
		product.ID, product.Title, product.Description,
		product.PriceNet, product.Status,
	).Scan(&product.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update product: %w", err)
	}

	return nil
}

// GetPigeonByProductID retrieves pigeon details for a product
func (r *Repository) GetPigeonByProductID(ctx context.Context, productID int64) (*Pigeon, error) {
	query := `
		SELECT 
			product_id, ring_number, sex, birth_date, lineage,
			origin_proof_url, origin_proof_file_ref, created_at, updated_at
		FROM pigeons
		WHERE product_id = $1
	`

	var p Pigeon
	err := r.db.QueryRow(ctx, query, productID).Scan(
		&p.ProductID, &p.RingNumber, &p.Sex, &p.BirthDate, &p.Lineage,
		&p.OriginProofURL, &p.OriginProofFileRef, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Pigeon not found
		}
		return nil, fmt.Errorf("failed to get pigeon: %w", err)
	}

	return &p, nil
}

// CreatePigeon creates pigeon details for a product
func (r *Repository) CreatePigeon(ctx context.Context, pigeon *Pigeon) error {
	query := `
		INSERT INTO pigeons (
			product_id, ring_number, sex, birth_date, lineage,
			origin_proof_url, origin_proof_file_ref
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		pigeon.ProductID, pigeon.RingNumber, pigeon.Sex, pigeon.BirthDate,
		pigeon.Lineage, pigeon.OriginProofURL, pigeon.OriginProofFileRef,
	).Scan(&pigeon.CreatedAt, &pigeon.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create pigeon: %w", err)
	}

	return nil
}

// GetSupplyByProductID retrieves supply details for a product
func (r *Repository) GetSupplyByProductID(ctx context.Context, productID int64) (*Supply, error) {
	query := `
		SELECT 
			product_id, sku, stock_qty, low_stock_threshold, created_at, updated_at
		FROM supplies
		WHERE product_id = $1
	`

	var s Supply
	err := r.db.QueryRow(ctx, query, productID).Scan(
		&s.ProductID, &s.SKU, &s.StockQty, &s.LowStockThreshold,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Supply not found
		}
		return nil, fmt.Errorf("failed to get supply: %w", err)
	}

	return &s, nil
}

// CreateSupply creates supply details for a product
func (r *Repository) CreateSupply(ctx context.Context, supply *Supply) error {
	query := `
		INSERT INTO supplies (product_id, sku, stock_qty, low_stock_threshold)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		supply.ProductID, supply.SKU, supply.StockQty, supply.LowStockThreshold,
	).Scan(&supply.CreatedAt, &supply.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create supply: %w", err)
	}

	return nil
}

// UpdateSupplyStock updates the stock quantity for a supply
func (r *Repository) UpdateSupplyStock(ctx context.Context, productID int64, newStockQty int) error {
	query := `
		UPDATE supplies 
		SET stock_qty = $2, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		WHERE product_id = $1
	`

	result, err := r.db.Exec(ctx, query, productID, newStockQty)
	if err != nil {
		return fmt.Errorf("failed to update supply stock: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("supply not found for product ID %d", productID)
	}

	return nil
}

// GetMediaByProductID retrieves all media for a product
func (r *Repository) GetMediaByProductID(ctx context.Context, productID int64, includeArchived bool) ([]Media, error) {
	query := `
		SELECT 
			id, product_id, kind, gcs_path, thumb_path, watermark_applied,
			file_size, mime_type, original_filename, archived_at, created_at, updated_at
		FROM media
		WHERE product_id = $1
	`

	args := []interface{}{productID}

	if !includeArchived {
		query += " AND archived_at IS NULL"
	}

	query += " ORDER BY created_at ASC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query media: %w", err)
	}
	defer rows.Close()

	var mediaList []Media
	for rows.Next() {
		var m Media
		err := rows.Scan(
			&m.ID, &m.ProductID, &m.Kind, &m.GCSPath, &m.ThumbPath, &m.WatermarkApplied,
			&m.FileSize, &m.MimeType, &m.OriginalFilename, &m.ArchivedAt,
			&m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan media: %w", err)
		}
		mediaList = append(mediaList, m)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating media: %w", err)
	}

	return mediaList, nil
}

// CreateMedia creates a new media entry
func (r *Repository) CreateMedia(ctx context.Context, media *Media) error {
	query := `
		INSERT INTO media (
			product_id, kind, gcs_path, thumb_path, watermark_applied,
			file_size, mime_type, original_filename
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		media.ProductID, media.Kind, media.GCSPath, media.ThumbPath, media.WatermarkApplied,
		media.FileSize, media.MimeType, media.OriginalFilename,
	).Scan(&media.ID, &media.CreatedAt, &media.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create media: %w", err)
	}

	return nil
}

// UpdateMedia updates media properties
func (r *Repository) UpdateMedia(ctx context.Context, media *Media) error {
	query := `
		UPDATE media 
		SET archived_at = $2, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		WHERE id = $1
		RETURNING updated_at
	`

	err := r.db.QueryRow(ctx, query, media.ID, media.ArchivedAt).Scan(&media.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to update media: %w", err)
	}

	return nil
}

// GetMediaByID retrieves a media entry by ID
func (r *Repository) GetMediaByID(ctx context.Context, mediaID int64) (*Media, error) {
	query := `
		SELECT 
			id, product_id, kind, gcs_path, thumb_path, watermark_applied,
			file_size, mime_type, original_filename, archived_at, created_at, updated_at
		FROM media
		WHERE id = $1
	`

	var m Media
	err := r.db.QueryRow(ctx, query, mediaID).Scan(
		&m.ID, &m.ProductID, &m.Kind, &m.GCSPath, &m.ThumbPath, &m.WatermarkApplied,
		&m.FileSize, &m.MimeType, &m.OriginalFilename, &m.ArchivedAt,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Media not found
		}
		return nil, fmt.Errorf("failed to get media: %w", err)
	}

	return &m, nil
}

// CheckSlugExists checks if a slug already exists for products
func (r *Repository) CheckSlugExists(ctx context.Context, slug string) (bool, error) {
	query := `SELECT COUNT(*) FROM products WHERE slug = $1`

	var count int
	err := r.db.QueryRow(ctx, query, slug).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check slug existence: %w", err)
	}

	return count > 0, nil
}

// CheckRingNumberExists checks if a ring number already exists for pigeons
func (r *Repository) CheckRingNumberExists(ctx context.Context, ringNumber string) (bool, error) {
	query := `SELECT COUNT(*) FROM pigeons WHERE ring_number = $1`

	var count int
	err := r.db.QueryRow(ctx, query, ringNumber).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check ring number existence: %w", err)
	}

	return count > 0, nil
}
