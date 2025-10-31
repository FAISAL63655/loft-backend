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

// ========================= Q&A Repository =========================

// CreateProductQuestion inserts a new question for a product
func (r *Repository) CreateProductQuestion(ctx context.Context, productID int64, userID *int64, question string) (*ProductQuestion, error) {
	query := `
        INSERT INTO product_questions (product_id, user_id, question, status)
        VALUES ($1, $2, $3, 'pending')
        RETURNING id, product_id, user_id, question, answer, answered_by, status, created_at, answered_at, updated_at
    `
	var q ProductQuestion
	err := r.db.QueryRow(ctx, query, productID, userID, question).Scan(
		&q.ID, &q.ProductID, &q.UserID, &q.Question, &q.Answer, &q.AnsweredBy, &q.Status, &q.CreatedAt, &q.AnsweredAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create product question: %w", err)
	}
	return &q, nil
}

// ListProductQuestionsPublic lists approved questions for a product
func (r *Repository) ListProductQuestionsPublic(ctx context.Context, productID int64) ([]ProductQuestion, error) {
	rows, err := r.db.Query(ctx, `
        SELECT id, product_id, user_id, question, answer, answered_by, status, created_at, answered_at, updated_at
        FROM product_questions
        WHERE product_id = $1 AND status = 'approved'
        ORDER BY created_at ASC
    `, productID)
	if err != nil {
		return nil, fmt.Errorf("failed to list product questions: %w", err)
	}
	defer rows.Close()
	var out []ProductQuestion
	for rows.Next() {
		var q ProductQuestion
		if err := rows.Scan(&q.ID, &q.ProductID, &q.UserID, &q.Question, &q.Answer, &q.AnsweredBy, &q.Status, &q.CreatedAt, &q.AnsweredAt, &q.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan product question: %w", err)
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter product questions: %w", err)
	}
	return out, nil
}

// ListProductQuestionsAdmin lists questions for a product with optional status filter
func (r *Repository) ListProductQuestionsAdmin(ctx context.Context, productID *int64, status *QuestionStatus) ([]ProductQuestion, error) {
	sb := strings.Builder{}
	sb.WriteString(`SELECT id, product_id, user_id, question, answer, answered_by, status, created_at, answered_at, updated_at FROM product_questions`)
	args := []interface{}{}
	where := []string{}
	if productID != nil {
		where = append(where, fmt.Sprintf("product_id = $%d", len(args)+1))
		args = append(args, *productID)
	}
	if status != nil {
		where = append(where, fmt.Sprintf("status = $%d", len(args)+1))
		args = append(args, *status)
	}
	if len(where) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(where, " AND "))
	}
	sb.WriteString(" ORDER BY created_at DESC")
	rows, err := r.db.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list product questions (admin): %w", err)
	}
	defer rows.Close()
	var out []ProductQuestion
	for rows.Next() {
		var q ProductQuestion
		if err := rows.Scan(&q.ID, &q.ProductID, &q.UserID, &q.Question, &q.Answer, &q.AnsweredBy, &q.Status, &q.CreatedAt, &q.AnsweredAt, &q.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan product question: %w", err)
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter product questions: %w", err)
	}
	return out, nil
}

// AnswerProductQuestion sets the answer and approves the question
func (r *Repository) AnswerProductQuestion(ctx context.Context, qid int64, answer string, answeredBy int64) (*ProductQuestion, error) {
	query := `
        UPDATE product_questions
        SET answer = $2, answered_by = $3, answered_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), status = 'approved', updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
        WHERE id = $1
        RETURNING id, product_id, user_id, question, answer, answered_by, status, created_at, answered_at, updated_at
    `
	var q ProductQuestion
	err := r.db.QueryRow(ctx, query, qid, answer, answeredBy).Scan(&q.ID, &q.ProductID, &q.UserID, &q.Question, &q.Answer, &q.AnsweredBy, &q.Status, &q.CreatedAt, &q.AnsweredAt, &q.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to answer product question: %w", err)
	}
	return &q, nil
}

// SetProductQuestionStatus updates the status (pending/approved/rejected)
func (r *Repository) SetProductQuestionStatus(ctx context.Context, qid int64, status QuestionStatus) error {
	_, err := r.db.Exec(ctx, `
        UPDATE product_questions SET status = $2, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id = $1
    `, qid, status)
	if err != nil {
		return fmt.Errorf("failed to set product question status: %w", err)
	}
	return nil
}

// CreateAuctionQuestion inserts a new question for an auction
func (r *Repository) CreateAuctionQuestion(ctx context.Context, auctionID int64, userID *int64, question string) (*AuctionQuestion, error) {
	query := `
        INSERT INTO auction_questions (auction_id, user_id, question, status)
        VALUES ($1, $2, $3, 'pending')
        RETURNING id, auction_id, user_id, question, answer, answered_by, status, created_at, answered_at, updated_at
    `
	var q AuctionQuestion
	err := r.db.QueryRow(ctx, query, auctionID, userID, question).Scan(
		&q.ID, &q.AuctionID, &q.UserID, &q.Question, &q.Answer, &q.AnsweredBy, &q.Status, &q.CreatedAt, &q.AnsweredAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create auction question: %w", err)
	}
	return &q, nil
}

// ListAuctionQuestionsPublic lists approved questions for an auction
func (r *Repository) ListAuctionQuestionsPublic(ctx context.Context, auctionID int64) ([]AuctionQuestion, error) {
	rows, err := r.db.Query(ctx, `
        SELECT id, auction_id, user_id, question, answer, answered_by, status, created_at, answered_at, updated_at
        FROM auction_questions
        WHERE auction_id = $1 AND status = 'approved'
        ORDER BY created_at ASC
    `, auctionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list auction questions: %w", err)
	}
	defer rows.Close()
	var out []AuctionQuestion
	for rows.Next() {
		var q AuctionQuestion
		if err := rows.Scan(&q.ID, &q.AuctionID, &q.UserID, &q.Question, &q.Answer, &q.AnsweredBy, &q.Status, &q.CreatedAt, &q.AnsweredAt, &q.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan auction question: %w", err)
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter auction questions: %w", err)
	}
	return out, nil
}

// ListAuctionQuestionsAdmin lists questions with optional status filter
func (r *Repository) ListAuctionQuestionsAdmin(ctx context.Context, auctionID *int64, status *QuestionStatus) ([]AuctionQuestion, error) {
	sb := strings.Builder{}
	sb.WriteString(`SELECT id, auction_id, user_id, question, answer, answered_by, status, created_at, answered_at, updated_at FROM auction_questions`)
	args := []interface{}{}
	where := []string{}
	if auctionID != nil {
		where = append(where, fmt.Sprintf("auction_id = $%d", len(args)+1))
		args = append(args, *auctionID)
	}
	if status != nil {
		where = append(where, fmt.Sprintf("status = $%d", len(args)+1))
		args = append(args, *status)
	}
	if len(where) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(where, " AND "))
	}
	sb.WriteString(" ORDER BY created_at DESC")
	rows, err := r.db.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list auction questions (admin): %w", err)
	}
	defer rows.Close()
	var out []AuctionQuestion
	for rows.Next() {
		var q AuctionQuestion
		if err := rows.Scan(&q.ID, &q.AuctionID, &q.UserID, &q.Question, &q.Answer, &q.AnsweredBy, &q.Status, &q.CreatedAt, &q.AnsweredAt, &q.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan auction question: %w", err)
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter auction questions: %w", err)
	}
	return out, nil
}

// AnswerAuctionQuestion sets the answer and approves the question
func (r *Repository) AnswerAuctionQuestion(ctx context.Context, qid int64, answer string, answeredBy int64) (*AuctionQuestion, error) {
	query := `
        UPDATE auction_questions
        SET answer = $2, answered_by = $3, answered_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), status = 'approved', updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
        WHERE id = $1
        RETURNING id, auction_id, user_id, question, answer, answered_by, status, created_at, answered_at, updated_at
    `
	var q AuctionQuestion
	err := r.db.QueryRow(ctx, query, qid, answer, answeredBy).Scan(&q.ID, &q.AuctionID, &q.UserID, &q.Question, &q.Answer, &q.AnsweredBy, &q.Status, &q.CreatedAt, &q.AnsweredAt, &q.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to answer auction question: %w", err)
	}
	return &q, nil
}

// SetAuctionQuestionStatus updates the status (pending/approved/rejected)
func (r *Repository) SetAuctionQuestionStatus(ctx context.Context, qid int64, status QuestionStatus) error {
	_, err := r.db.Exec(ctx, `
        UPDATE auction_questions SET status = $2, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id = $1
    `, qid, status)
	if err != nil {
		return fmt.Errorf("failed to set auction question status: %w", err)
	}
	return nil
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
			p.price_net, p.status, p.created_at, p.updated_at,
			(SELECT m.gcs_path
			 FROM media m
			 WHERE m.product_id = p.id
			   AND m.kind = 'image'
			   AND m.archived_at IS NULL
			 ORDER BY m.created_at ASC
			 LIMIT 1) as thumbnail_url
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
			&p.ThumbnailURL,
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
	`

	_, err := r.db.Exec(ctx, query,
		product.ID,
		product.Title,
		product.Description,
		product.PriceNet,
		product.Status,
	)

	return err
}

// UpdateProductPartial updates specific fields of a product using a map
func (r *Repository) UpdateProductPartial(ctx context.Context, productID int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	setParts := []string{}
	args := []interface{}{productID}
	argIndex := 2

	for field, value := range updates {
		setParts = append(setParts, fmt.Sprintf("%s = $%d", field, argIndex))
		args = append(args, value)
		argIndex++
	}

	query := fmt.Sprintf(`
		UPDATE products 
		SET %s, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		WHERE id = $1
	`, strings.Join(setParts, ", "))

	_, err := r.db.Exec(ctx, query, args...)
	return err
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
			file_size, mime_type, original_filename, description, archived_at, created_at, updated_at
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
			&m.FileSize, &m.MimeType, &m.OriginalFilename, &m.Description, &m.ArchivedAt,
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
			file_size, mime_type, original_filename, description
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		media.ProductID, media.Kind, media.GCSPath, media.ThumbPath, media.WatermarkApplied,
		media.FileSize, media.MimeType, media.OriginalFilename, media.Description,
	).Scan(&media.ID, &media.CreatedAt, &media.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create media: %w", err)
	}

	return nil
}

// ArchiveAllProductMedia archives all media for a product
func (r *Repository) ArchiveAllProductMedia(ctx context.Context, productID int64) error {
	query := `
		UPDATE media
		SET archived_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),
		    updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		WHERE product_id = $1 AND archived_at IS NULL
	`
	_, err := r.db.Exec(ctx, query, productID)
	return err
}

// UpdateMedia updates media properties
func (r *Repository) UpdateMedia(ctx context.Context, media *Media) error {
	query := `
		UPDATE media
		SET description = $2, archived_at = $3, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		WHERE id = $1
		RETURNING updated_at
	`

	err := r.db.QueryRow(ctx, query, media.ID, media.Description, media.ArchivedAt).Scan(&media.UpdatedAt)
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
			file_size, mime_type, original_filename, description, archived_at, created_at, updated_at
		FROM media
		WHERE id = $1
	`

	var m Media
	err := r.db.QueryRow(ctx, query, mediaID).Scan(
		&m.ID, &m.ProductID, &m.Kind, &m.GCSPath, &m.ThumbPath, &m.WatermarkApplied,
		&m.FileSize, &m.MimeType, &m.OriginalFilename, &m.Description, &m.ArchivedAt,
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

// CheckRingNumberExistsExcluding checks if a ring number exists excluding a specific product
func (r *Repository) CheckRingNumberExistsExcluding(ctx context.Context, ringNumber string, excludeProductID int64) (bool, error) {
	query := `SELECT COUNT(*) FROM pigeons WHERE ring_number = $1 AND product_id != $2`

	var count int
	err := r.db.QueryRow(ctx, query, ringNumber, excludeProductID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check ring number existence: %w", err)
	}

	return count > 0, nil
}

// UpdatePigeon updates pigeon details using a map of fields
func (r *Repository) UpdatePigeon(ctx context.Context, productID int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	setParts := []string{}
	args := []interface{}{productID}
	argIndex := 2

	for field, value := range updates {
		setParts = append(setParts, fmt.Sprintf("%s = $%d", field, argIndex))
		args = append(args, value)
		argIndex++
	}

	query := fmt.Sprintf(`
		UPDATE pigeons 
		SET %s, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		WHERE product_id = $1
	`, strings.Join(setParts, ", "))

	_, err := r.db.Exec(ctx, query, args...)
	return err
}

// UpdateSupply updates supply details using a map of fields
func (r *Repository) UpdateSupply(ctx context.Context, productID int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	setParts := []string{}
	args := []interface{}{productID}
	argIndex := 2

	for field, value := range updates {
		setParts = append(setParts, fmt.Sprintf("%s = $%d", field, argIndex))
		args = append(args, value)
		argIndex++
	}

	query := fmt.Sprintf(`
		UPDATE supplies 
		SET %s, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		WHERE product_id = $1
	`, strings.Join(setParts, ", "))

	_, err := r.db.Exec(ctx, query, args...)
	return err
}
