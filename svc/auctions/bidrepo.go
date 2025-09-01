package auctions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"encore.dev/storage/sqldb"
)

// BidRepository handles bid data access
type BidRepository struct {
	db *sqldb.Database
}

// NewBidRepository creates a new bid repository
func NewBidRepository(db *sqldb.Database) *BidRepository {
	return &BidRepository{db: db}
}

// CreateBid creates a new bid in the database
func (r *BidRepository) CreateBid(ctx context.Context, tx *sqldb.Tx, bid *Bid) (*Bid, error) {
	query := `
		INSERT INTO bids (auction_id, user_id, amount, bidder_name_snapshot, bidder_city_id_snapshot)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`

	err := tx.QueryRow(ctx, query,
		bid.AuctionID,
		bid.UserID,
		bid.Amount,
		bid.BidderNameSnapshot,
		bid.BidderCityIDSnapshot,
	).Scan(&bid.ID, &bid.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create bid: %w", err)
	}

	return bid, nil
}

// GetBid retrieves a bid by ID
func (r *BidRepository) GetBid(ctx context.Context, bidID int64) (*Bid, error) {
	query := `
		SELECT id, auction_id, user_id, amount, bidder_name_snapshot, 
			   bidder_city_id_snapshot, created_at
		FROM bids 
		WHERE id = $1`

	bid := &Bid{}
	err := r.db.QueryRow(ctx, query, bidID).Scan(
		&bid.ID,
		&bid.AuctionID,
		&bid.UserID,
		&bid.Amount,
		&bid.BidderNameSnapshot,
		&bid.BidderCityIDSnapshot,
		&bid.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return bid, nil
}

// GetAuctionBidsWithDetails retrieves bids for an auction with details
func (r *BidRepository) GetAuctionBidsWithDetails(ctx context.Context, auctionID int64, filters *BidFilters) ([]*BidWithDetails, int, error) {
	// Count total bids
	countQuery := `SELECT COUNT(*) FROM bids WHERE auction_id = $1`
	var total int
	err := r.db.QueryRow(ctx, countQuery, auctionID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count bids: %w", err)
	}

	// Get bids with details
	offset := (filters.Page - 1) * filters.Limit
	query := `
		SELECT 
			b.id, b.auction_id, b.user_id, b.amount, 
			b.bidder_name_snapshot, b.bidder_city_id_snapshot, b.created_at,
			c.name_ar as bidder_city_name
		FROM bids b
		LEFT JOIN cities c ON c.id = b.bidder_city_id_snapshot
		WHERE b.auction_id = $1
		ORDER BY b.amount DESC, b.created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, auctionID, filters.Limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query bids: %w", err)
	}
	defer rows.Close()

	var bids []*BidWithDetails
	for rows.Next() {
		bid := &BidWithDetails{}
		var bidderCityName sql.NullString

		err := rows.Scan(
			&bid.ID,
			&bid.AuctionID,
			&bid.UserID,
			&bid.Amount,
			&bid.BidderNameSnapshot,
			&bid.BidderCityIDSnapshot,
			&bid.CreatedAt,
			&bidderCityName,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan bid: %w", err)
		}

		if bidderCityName.Valid {
			bid.BidderCityName = &bidderCityName.String
		}

		bids = append(bids, bid)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate bids: %w", err)
	}

	return bids, total, nil
}

// GetHighestBid retrieves the highest bid for an auction
func (r *BidRepository) GetHighestBid(ctx context.Context, auctionID int64) (*Bid, error) {
	query := `
		SELECT id, auction_id, user_id, amount, bidder_name_snapshot, 
			   bidder_city_id_snapshot, created_at
		FROM bids 
		WHERE auction_id = $1
		ORDER BY amount DESC, created_at DESC
		LIMIT 1`

	bid := &Bid{}
	err := r.db.QueryRow(ctx, query, auctionID).Scan(
		&bid.ID,
		&bid.AuctionID,
		&bid.UserID,
		&bid.Amount,
		&bid.BidderNameSnapshot,
		&bid.BidderCityIDSnapshot,
		&bid.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No bids found
		}
		return nil, err
	}

	return bid, nil
}

// GetUserBids retrieves bids placed by a specific user
func (r *BidRepository) GetUserBids(ctx context.Context, userID int64, filters *BidFilters) ([]*BidWithDetails, int, error) {
	// Count total bids
	countQuery := `SELECT COUNT(*) FROM bids WHERE user_id = $1`
	var total int
	err := r.db.QueryRow(ctx, countQuery, userID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count user bids: %w", err)
	}

	// Get bids with details
	offset := (filters.Page - 1) * filters.Limit
	query := `
		SELECT 
			b.id, b.auction_id, b.user_id, b.amount, 
			b.bidder_name_snapshot, b.bidder_city_id_snapshot, b.created_at,
			c.name_ar as bidder_city_name
		FROM bids b
		LEFT JOIN cities c ON c.id = b.bidder_city_id_snapshot
		WHERE b.user_id = $1
		ORDER BY b.created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, userID, filters.Limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query user bids: %w", err)
	}
	defer rows.Close()

	var bids []*BidWithDetails
	for rows.Next() {
		bid := &BidWithDetails{}
		var bidderCityName sql.NullString

		err := rows.Scan(
			&bid.ID,
			&bid.AuctionID,
			&bid.UserID,
			&bid.Amount,
			&bid.BidderNameSnapshot,
			&bid.BidderCityIDSnapshot,
			&bid.CreatedAt,
			&bidderCityName,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan user bid: %w", err)
		}

		if bidderCityName.Valid {
			bid.BidderCityName = &bidderCityName.String
		}

		bids = append(bids, bid)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate user bids: %w", err)
	}

	return bids, total, nil
}

// RemoveBid removes a bid from the database
func (r *BidRepository) RemoveBid(ctx context.Context, tx *sqldb.Tx, bidID int64) error {
	query := `DELETE FROM bids WHERE id = $1`
	_, err := tx.Exec(ctx, query, bidID)
	return err
}

// GetBidsAfterAmount retrieves bids with amount greater than specified amount
func (r *BidRepository) GetBidsAfterAmount(ctx context.Context, auctionID int64, amount float64) ([]*Bid, error) {
	query := `
		SELECT id, auction_id, user_id, amount, bidder_name_snapshot, 
			   bidder_city_id_snapshot, created_at
		FROM bids 
		WHERE auction_id = $1 AND amount > $2
		ORDER BY amount ASC, created_at ASC`

	rows, err := r.db.Query(ctx, query, auctionID, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to query bids after amount: %w", err)
	}
	defer rows.Close()

	var bids []*Bid
	for rows.Next() {
		bid := &Bid{}
		err := rows.Scan(
			&bid.ID,
			&bid.AuctionID,
			&bid.UserID,
			&bid.Amount,
			&bid.BidderNameSnapshot,
			&bid.BidderCityIDSnapshot,
			&bid.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan bid: %w", err)
		}
		bids = append(bids, bid)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate bids: %w", err)
	}

	return bids, nil
}

// GetBidCount retrieves the total number of bids for an auction
func (r *BidRepository) GetBidCount(ctx context.Context, auctionID int64) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM bids WHERE auction_id = $1`
	err := r.db.QueryRow(ctx, query, auctionID).Scan(&count)
	return count, err
}

// GetCurrentPrice retrieves the current highest bid amount for an auction
func (r *BidRepository) GetCurrentPrice(ctx context.Context, auctionID int64, startPrice float64) (float64, error) {
	var currentPrice sql.NullFloat64
	query := `SELECT MAX(amount) FROM bids WHERE auction_id = $1`
	err := r.db.QueryRow(ctx, query, auctionID).Scan(&currentPrice)
	if err != nil {
		return 0, fmt.Errorf("failed to get current price: %w", err)
	}

	if currentPrice.Valid {
		return currentPrice.Float64, nil
	}

	return startPrice, nil
}

// GetOutbidUsers returns user IDs who should receive outbid notifications (excluding current highest bidder)
func (r *BidRepository) GetOutbidUsers(ctx context.Context, auctionID int64, newAmount float64, excludeUserID int64) ([]int64, error) {
	query := `
		SELECT DISTINCT user_id
		FROM bids
		WHERE auction_id = $1
		  AND amount < $2
		  AND user_id <> $3
	`

	rows, err := r.db.Query(ctx, query, auctionID, newAmount, excludeUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to query outbid users: %w", err)
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("failed to scan user id: %w", err)
		}
		userIDs = append(userIDs, uid)
	}
	return userIDs, nil
}

// GetRecentBidsByUser retrieves recent bids by a user for rate limiting
func (r *BidRepository) GetRecentBidsByUser(ctx context.Context, userID int64, minutes int) (int, error) {
	var count int
	query := `
		SELECT COUNT(*) 
		FROM bids 
		WHERE user_id = $1 AND created_at > NOW() - INTERVAL '%d minutes'`

	formattedQuery := fmt.Sprintf(query, minutes)
	err := r.db.QueryRow(ctx, formattedQuery, userID).Scan(&count)
	return count, err
}

// GetWinningBid retrieves the winning bid for an ended auction
func (r *BidRepository) GetWinningBid(ctx context.Context, auctionID int64) (*BidWithDetails, error) {
	query := `
		SELECT 
			b.id, b.auction_id, b.user_id, b.amount, 
			b.bidder_name_snapshot, b.bidder_city_id_snapshot, b.created_at,
			c.name_ar as bidder_city_name
		FROM bids b
		LEFT JOIN cities c ON c.id = b.bidder_city_id_snapshot
		WHERE b.auction_id = $1
		ORDER BY b.amount DESC, b.created_at DESC
		LIMIT 1`

	bid := &BidWithDetails{}
	var bidderCityName sql.NullString

	err := r.db.QueryRow(ctx, query, auctionID).Scan(
		&bid.ID,
		&bid.AuctionID,
		&bid.UserID,
		&bid.Amount,
		&bid.BidderNameSnapshot,
		&bid.BidderCityIDSnapshot,
		&bid.CreatedAt,
		&bidderCityName,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No winning bid
		}
		return nil, err
	}

	if bidderCityName.Valid {
		bid.BidderCityName = &bidderCityName.String
	}

	return bid, nil
}
