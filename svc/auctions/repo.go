package auctions

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"encore.dev/storage/sqldb"
)

// Repository handles auction data access
type Repository struct {
	db *sqldb.Database
}

// NewRepository creates a new auction repository
func NewRepository(db *sqldb.Database) *Repository {
    return &Repository{db: db}
}

// HasActiveAuctionForProduct checks if there's an active (scheduled/live) auction for a product
func (r *Repository) HasActiveAuctionForProduct(ctx context.Context, productID int64) (bool, error) {
    var exists bool
    err := r.db.QueryRow(ctx, `
        SELECT EXISTS(
            SELECT 1 FROM auctions
            WHERE product_id = $1 AND status IN ('scheduled','live')
        )
    `, productID).Scan(&exists)
    return exists, err
}

// CreateAuction creates a new auction in the database
func (r *Repository) CreateAuction(ctx context.Context, auction *Auction) (*Auction, error) {
	query := `
		INSERT INTO auctions (
			product_id, start_price, bid_step, reserve_price, 
			start_at, end_at, anti_sniping_minutes, status, 
			extensions_count, max_extensions_override
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		) RETURNING id, created_at, updated_at`

	err := r.db.QueryRow(ctx, query,
		auction.ProductID,
		auction.StartPrice,
		auction.BidStep,
		auction.ReservePrice,
		auction.StartAt,
		auction.EndAt,
		auction.AntiSnipingMinutes,
		auction.Status,
		auction.ExtensionsCount,
		auction.MaxExtensionsOverride,
	).Scan(&auction.ID, &auction.CreatedAt, &auction.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create auction: %w", err)
	}

	return auction, nil
}

// GetAuction retrieves an auction by ID
func (r *Repository) GetAuction(ctx context.Context, auctionID int64) (*Auction, error) {
	query := `
		SELECT id, product_id, start_price, bid_step, reserve_price,
			   start_at, end_at, anti_sniping_minutes, status,
			   extensions_count, max_extensions_override, created_at, updated_at
		FROM auctions 
		WHERE id = $1`

	auction := &Auction{}
	err := r.db.QueryRow(ctx, query, auctionID).Scan(
		&auction.ID,
		&auction.ProductID,
		&auction.StartPrice,
		&auction.BidStep,
		&auction.ReservePrice,
		&auction.StartAt,
		&auction.EndAt,
		&auction.AntiSnipingMinutes,
		&auction.Status,
		&auction.ExtensionsCount,
		&auction.MaxExtensionsOverride,
		&auction.CreatedAt,
		&auction.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return auction, nil
}

// GetAuctionWithDetails retrieves an auction with additional details
func (r *Repository) GetAuctionWithDetails(ctx context.Context, auctionID int64) (*AuctionWithDetails, error) {
	query := `
		SELECT 
			a.id, a.product_id, a.start_price, a.bid_step, a.reserve_price,
			a.start_at, a.end_at, a.anti_sniping_minutes, a.status,
			a.extensions_count, a.max_extensions_override, a.created_at, a.updated_at,
			p.title as product_title, p.slug as product_slug,
			COALESCE(MAX(b.amount), a.start_price) as current_price,
			COUNT(b.id) as bids_count,
			(SELECT u.name FROM bids b2 
			 JOIN users u ON u.id = b2.user_id 
			 WHERE b2.auction_id = a.id 
			 ORDER BY b2.amount DESC, b2.created_at DESC 
			 LIMIT 1) as highest_bidder,
			(SELECT c.name_ar FROM bids b3
			 JOIN users u2 ON u2.id = b3.user_id
			 LEFT JOIN cities c ON c.id = u2.city_id
			 WHERE b3.auction_id = a.id
			 ORDER BY b3.amount DESC, b3.created_at DESC
			 LIMIT 1) as highest_bidder_city
		FROM auctions a
		JOIN products p ON p.id = a.product_id
		LEFT JOIN bids b ON b.auction_id = a.id
		WHERE a.id = $1
		GROUP BY a.id, p.title, p.slug`

	auction := &AuctionWithDetails{}
	var highestBidder, highestBidderCity sql.NullString

	err := r.db.QueryRow(ctx, query, auctionID).Scan(
		&auction.ID,
		&auction.ProductID,
		&auction.StartPrice,
		&auction.BidStep,
		&auction.ReservePrice,
		&auction.StartAt,
		&auction.EndAt,
		&auction.AntiSnipingMinutes,
		&auction.Status,
		&auction.ExtensionsCount,
		&auction.MaxExtensionsOverride,
		&auction.CreatedAt,
		&auction.UpdatedAt,
		&auction.ProductTitle,
		&auction.ProductSlug,
		&auction.CurrentPrice,
		&auction.BidsCount,
		&highestBidder,
		&highestBidderCity,
	)

	if err != nil {
		return nil, err
	}

	if highestBidder.Valid {
		auction.HighestBidder = &highestBidder.String
	}
	if highestBidderCity.Valid {
		auction.HighestBidderCity = &highestBidderCity.String
	}

	return auction, nil
}

// ListAuctionsWithDetails retrieves auctions with filters and details
func (r *Repository) ListAuctionsWithDetails(ctx context.Context, filters *AuctionFilters) ([]*AuctionWithDetails, int, error) {
	// Build WHERE clause
	var conditions []string
	var args []interface{}
	argIndex := 1

	if filters.Status != nil {
		conditions = append(conditions, fmt.Sprintf("a.status = $%d", argIndex))
		args = append(args, *filters.Status)
		argIndex++
	}

	if filters.EndingSoon {
		conditions = append(conditions, fmt.Sprintf("a.status = 'live' AND a.end_at <= NOW() + INTERVAL '1 hour'"))
	}

	if filters.Query != "" {
		conditions = append(conditions, fmt.Sprintf("(p.title ILIKE $%d OR p.slug ILIKE $%d)", argIndex, argIndex))
		args = append(args, "%"+filters.Query+"%")
		argIndex++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Build ORDER BY clause
	orderBy := "ORDER BY a.created_at DESC"
	switch filters.Sort {
	case "ending_soon":
		orderBy = "ORDER BY CASE WHEN a.status = 'live' THEN a.end_at END ASC NULLS LAST, a.created_at DESC"
	case "newest":
		orderBy = "ORDER BY a.created_at DESC"
	case "oldest":
		orderBy = "ORDER BY a.created_at ASC"
	case "price_high":
		orderBy = "ORDER BY COALESCE(MAX(b.amount), a.start_price) DESC"
	case "price_low":
		orderBy = "ORDER BY COALESCE(MAX(b.amount), a.start_price) ASC"
	}

	// Count total
	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT a.id)
		FROM auctions a
		JOIN products p ON p.id = a.product_id
		LEFT JOIN bids b ON b.auction_id = a.id
		%s`, whereClause)

	var total int
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count auctions: %w", err)
	}

	// Get auctions
	offset := (filters.Page - 1) * filters.Limit
	query := fmt.Sprintf(`
		SELECT 
			a.id, a.product_id, a.start_price, a.bid_step, a.reserve_price,
			a.start_at, a.end_at, a.anti_sniping_minutes, a.status,
			a.extensions_count, a.max_extensions_override, a.created_at, a.updated_at,
			p.title as product_title, p.slug as product_slug,
			COALESCE(MAX(b.amount), a.start_price) as current_price,
			COUNT(b.id) as bids_count,
			(SELECT u.name FROM bids b2 
			 JOIN users u ON u.id = b2.user_id 
			 WHERE b2.auction_id = a.id 
			 ORDER BY b2.amount DESC, b2.created_at DESC 
			 LIMIT 1) as highest_bidder,
			(SELECT c.name_ar FROM bids b3
			 JOIN users u2 ON u2.id = b3.user_id
			 LEFT JOIN cities c ON c.id = u2.city_id
			 WHERE b3.auction_id = a.id
			 ORDER BY b3.amount DESC, b3.created_at DESC
			 LIMIT 1) as highest_bidder_city
		FROM auctions a
		JOIN products p ON p.id = a.product_id
		LEFT JOIN bids b ON b.auction_id = a.id
		%s
		GROUP BY a.id, p.title, p.slug
		%s
		LIMIT $%d OFFSET $%d`, whereClause, orderBy, argIndex, argIndex+1)

	args = append(args, filters.Limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query auctions: %w", err)
	}
	defer rows.Close()

	var auctions []*AuctionWithDetails
	for rows.Next() {
		auction := &AuctionWithDetails{}
		var highestBidder, highestBidderCity sql.NullString

		err := rows.Scan(
			&auction.ID,
			&auction.ProductID,
			&auction.StartPrice,
			&auction.BidStep,
			&auction.ReservePrice,
			&auction.StartAt,
			&auction.EndAt,
			&auction.AntiSnipingMinutes,
			&auction.Status,
			&auction.ExtensionsCount,
			&auction.MaxExtensionsOverride,
			&auction.CreatedAt,
			&auction.UpdatedAt,
			&auction.ProductTitle,
			&auction.ProductSlug,
			&auction.CurrentPrice,
			&auction.BidsCount,
			&highestBidder,
			&highestBidderCity,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan auction: %w", err)
		}

		if highestBidder.Valid {
			auction.HighestBidder = &highestBidder.String
		}
		if highestBidderCity.Valid {
			auction.HighestBidderCity = &highestBidderCity.String
		}

		auctions = append(auctions, auction)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate auctions: %w", err)
	}

	return auctions, total, nil
}

// UpdateAuctionStatus updates the status of an auction
func (r *Repository) UpdateAuctionStatus(ctx context.Context, tx *sqldb.Tx, auctionID int64, status AuctionStatus) error {
	query := `UPDATE auctions SET status = $1, updated_at = NOW() WHERE id = $2`

	if tx != nil {
		_, err := tx.Exec(ctx, query, status, auctionID)
		return err
	}

	_, err := r.db.Exec(ctx, query, status, auctionID)
	return err
}

// UpdateAuctionEndTime updates the end time of an auction (for anti-sniping)
func (r *Repository) UpdateAuctionEndTime(ctx context.Context, auctionID int64, oldEndAt, newEndAt time.Time) error {
	// Use optimistic locking to prevent race conditions
	query := `
		UPDATE auctions 
		SET end_at = $1, extensions_count = extensions_count + 1, updated_at = NOW() 
		WHERE id = $2 AND end_at = $3`

	result, err := r.db.Exec(ctx, query, newEndAt, auctionID, oldEndAt)
	if err != nil {
		return fmt.Errorf("failed to update auction end time: %w", err)
	}

	rowsAffected := result.RowsAffected()

	if rowsAffected == 0 {
		return fmt.Errorf("auction end time was already updated by another process")
	}

	return nil
}

// CreateAuctionExtension creates a record of auction extension
func (r *Repository) CreateAuctionExtension(ctx context.Context, extension *AuctionExtension) error {
	query := `
		INSERT INTO auction_extensions (auction_id, extended_by_bid_id, old_end_at, new_end_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	err := r.db.QueryRow(ctx, query,
		extension.AuctionID,
		extension.ExtendedByBidID,
		extension.OldEndAt,
		extension.NewEndAt,
	).Scan(&extension.ID, &extension.CreatedAt)

	return err
}

// GetAuctionExtensions retrieves extensions for an auction
func (r *Repository) GetAuctionExtensions(ctx context.Context, auctionID int64) ([]*AuctionExtension, error) {
	query := `
		SELECT id, auction_id, extended_by_bid_id, old_end_at, new_end_at, created_at
		FROM auction_extensions
		WHERE auction_id = $1
		ORDER BY created_at ASC`

	rows, err := r.db.Query(ctx, query, auctionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query auction extensions: %w", err)
	}
	defer rows.Close()

	var extensions []*AuctionExtension
	for rows.Next() {
		ext := &AuctionExtension{}
		err := rows.Scan(
			&ext.ID,
			&ext.AuctionID,
			&ext.ExtendedByBidID,
			&ext.OldEndAt,
			&ext.NewEndAt,
			&ext.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan auction extension: %w", err)
		}
		extensions = append(extensions, ext)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate auction extensions: %w", err)
	}

	return extensions, nil
}

// GetAuctionsToClose retrieves auctions that should be closed
func (r *Repository) GetAuctionsToClose(ctx context.Context) ([]*Auction, error) {
	query := `
		SELECT id, product_id, start_price, bid_step, reserve_price,
		       start_at, end_at, anti_sniping_minutes, status,
		       extensions_count, max_extensions_override, created_at, updated_at
		FROM auctions 
		WHERE status = 'live' AND end_at <= NOW()
		ORDER BY end_at ASC`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query auctions to close: %w", err)
	}
	defer rows.Close()

	var auctions []*Auction
	for rows.Next() {
		auction := &Auction{}
		err := rows.Scan(
			&auction.ID,
			&auction.ProductID,
			&auction.StartPrice,
			&auction.BidStep,
			&auction.ReservePrice,
			&auction.StartAt,
			&auction.EndAt,
			&auction.AntiSnipingMinutes,
			&auction.Status,
			&auction.ExtensionsCount,
			&auction.MaxExtensionsOverride,
			&auction.CreatedAt,
			&auction.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan auction: %w", err)
		}
		auctions = append(auctions, auction)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate auctions: %w", err)
	}

	return auctions, nil
}

// ListAuctions lists auctions with filtering and pagination
func (r *Repository) ListAuctions(ctx context.Context, filters *AuctionFilters) ([]*Auction, int, error) {
	// Build WHERE clause
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if filters.Status != nil {
		whereClause += fmt.Sprintf(" AND a.status = $%d", argIndex)
		args = append(args, *filters.Status)
		argIndex++
	}

	if filters.EndingSoon {
		whereClause += fmt.Sprintf(" AND a.end_at BETWEEN NOW() AND NOW() + INTERVAL '1 hour' AND a.status = $%d", argIndex)
		args = append(args, AuctionStatusLive)
		argIndex++
	}

	if filters.Query != "" {
		whereClause += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM products p WHERE p.id = a.product_id AND p.title ILIKE $%d)", argIndex)
		args = append(args, "%"+filters.Query+"%")
		argIndex++
	}

	// Count total
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM auctions a %s`, whereClause)
	var total int
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count auctions: %w", err)
	}

	// Build ORDER BY clause
	orderBy := "ORDER BY created_at DESC"
	if filters.Sort == "ending_soon" {
		orderBy = "ORDER BY end_at ASC"
	}

	// Main query with pagination including current price and bids count
	query := fmt.Sprintf(`
		SELECT a.id, a.product_id, a.start_price, a.bid_step, a.reserve_price,
			   a.start_at, a.end_at, a.anti_sniping_minutes, a.status,
			   a.extensions_count, a.max_extensions_override, a.created_at, a.updated_at,
			   COALESCE(MAX(b.amount), a.start_price) as current_price,
			   COUNT(b.id) as bids_count
		FROM auctions a
		LEFT JOIN bids b ON a.id = b.auction_id
		%s
		GROUP BY a.id, a.product_id, a.start_price, a.bid_step, a.reserve_price,
				 a.start_at, a.end_at, a.anti_sniping_minutes, a.status,
				 a.extensions_count, a.max_extensions_override, a.created_at, a.updated_at
		%s LIMIT $%d OFFSET $%d`,
		whereClause, orderBy, argIndex, argIndex+1)

	args = append(args, filters.Limit, (filters.Page-1)*filters.Limit)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query auctions: %w", err)
	}
	defer rows.Close()

	var auctions []*Auction
	for rows.Next() {
		auction := &Auction{}
		err := rows.Scan(
			&auction.ID, &auction.ProductID, &auction.StartPrice, &auction.BidStep,
			&auction.ReservePrice, &auction.StartAt, &auction.EndAt, &auction.AntiSnipingMinutes,
			&auction.Status, &auction.ExtensionsCount, &auction.MaxExtensionsOverride,
			&auction.CreatedAt, &auction.UpdatedAt, &auction.CurrentPrice, &auction.BidsCount,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan auction: %w", err)
		}
		auctions = append(auctions, auction)
	}

	return auctions, total, nil
}

// GetAuctionsToStart retrieves auctions that should be started
func (r *Repository) GetAuctionsToStart(ctx context.Context) ([]*Auction, error) {
	query := `
		SELECT id, product_id, start_price, bid_step, reserve_price,
			   start_at, end_at, anti_sniping_minutes, status,
			   extensions_count, max_extensions_override, created_at, updated_at
		FROM auctions 
		WHERE status = 'scheduled' AND start_at <= NOW()
		ORDER BY start_at ASC`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query auctions to start: %w", err)
	}
	defer rows.Close()

	var auctions []*Auction
	for rows.Next() {
		auction := &Auction{}
		err := rows.Scan(
			&auction.ID,
			&auction.ProductID,
			&auction.StartPrice,
			&auction.BidStep,
			&auction.ReservePrice,
			&auction.StartAt,
			&auction.EndAt,
			&auction.AntiSnipingMinutes,
			&auction.Status,
			&auction.ExtensionsCount,
			&auction.MaxExtensionsOverride,
			&auction.CreatedAt,
			&auction.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan auction: %w", err)
		}
		auctions = append(auctions, auction)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate auctions: %w", err)
	}

	return auctions, nil
}
