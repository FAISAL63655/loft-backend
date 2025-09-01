package auctions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"encore.app/pkg/audit"
	"encore.app/pkg/errs"
	"encore.app/svc/notifications"
	"encore.dev/storage/sqldb"
)

// BidService handles bidding business logic
type BidService struct {
	db   *sqldb.Database
	repo *BidRepository
}

// NewBidService creates a new bid service
func NewBidService(db *sqldb.Database) *BidService {
	return &BidService{
		db:   db,
		repo: NewBidRepository(db),
	}
}

// PlaceBid places a bid on an auction (Verified users only)
func (s *BidService) PlaceBid(ctx context.Context, auctionID int64, userID int64, amount float64) (*Bid, error) {
	// Validate user permissions (must be verified and email verified)
	if err := s.validateBidderPermissions(ctx, userID); err != nil {
		return nil, err
	}

    // Check rate limiting via RateLimitService (early reject before locking)
    rl := NewRateLimitService(s.db)
    if err := rl.CheckBidRateLimit(ctx, userID); err != nil {
        return nil, err
    }

    // Start transaction for bid placement and lock auction row to prevent races
    tx, err := s.db.Begin(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to start transaction: %w", err)
    }
    defer tx.Rollback()

    // Get auction details with row lock inside the transaction
    auction, err := s.getAuctionForBiddingTx(ctx, tx, auctionID)
    if err != nil {
        return nil, err
    }

    // Validate auction state
    if err := s.validateAuctionForBidding(auction); err != nil {
        return nil, err
    }

    // Get current highest bid inside the same transaction
    currentPrice, err := s.getCurrentPriceTx(ctx, tx, auctionID, auction.StartPrice)
    if err != nil {
        return nil, err
    }

    // Validate bid amount
    if err := s.validateBidAmount(amount, currentPrice, auction.BidStep); err != nil {
        return nil, err
    }

	// Get user snapshot data
	bidderName, bidderCityID, err := s.getUserSnapshotData(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user data: %w", err)
	}

	// Create bid
	bid := &Bid{
		AuctionID:            auctionID,
		UserID:               userID,
		Amount:               amount,
		BidderNameSnapshot:   bidderName,
		BidderCityIDSnapshot: bidderCityID,
	}

	// Insert bid
	createdBid, err := s.repo.CreateBid(ctx, tx, bid)
	if err != nil {
		return nil, fmt.Errorf("failed to create bid: %w", err)
	}

	// Handle anti-sniping if needed
	extended, err := s.handleAntiSniping(ctx, tx, auction, createdBid)
	if err != nil {
		return nil, fmt.Errorf("failed to handle anti-sniping: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Broadcast bid_placed event
	realtimeService := GetRealtimeService()
	if realtimeService != nil {
		currentPrice := createdBid.Amount
		// Convert Bid to BidWithDetails for broadcasting
		bidWithDetails := &BidWithDetails{
			Bid: *createdBid,
		}

		if err := realtimeService.BroadcastBidPlaced(ctx, auctionID, bidWithDetails, currentPrice); err != nil {
			// Log error but don't fail the bid
			fmt.Printf("Failed to broadcast bid_placed event: %v\n", err)
		}

		// Send outbid notifications to previous bidders
		outbidUsers, err := s.repo.GetOutbidUsers(ctx, auctionID, createdBid.Amount, userID)
		if err == nil && len(outbidUsers) > 0 {
			if err := realtimeService.BroadcastOutbid(ctx, auctionID, outbidUsers, currentPrice); err != nil {
				fmt.Printf("Failed to broadcast outbid event: %v\n", err)
			}
		}

		// If extended, broadcast extended event
		if extended {
			// Create a temporary repository to get auction details
			auctionRepo := NewRepository(s.db)
			updatedAuction, err := auctionRepo.GetAuction(ctx, auctionID)
			if err == nil {
				if err := realtimeService.BroadcastExtended(ctx, auctionID, auction.EndAt, updatedAuction.EndAt, updatedAuction.ExtensionsCount); err != nil {
					fmt.Printf("Failed to broadcast extended event: %v\n", err)
				}
			}
		}
	}

	// Send audit notification for bid placement
	fmt.Printf("[AUDIT] BID.PLACED - Auction %d: Bid %d by User %d for %.2f\n",
		auctionID, createdBid.ID, userID, amount)

	return createdBid, nil
}

// GetAuctionBids retrieves bids for an auction with pagination
func (s *BidService) GetAuctionBids(ctx context.Context, auctionID int64, filters *BidFilters) ([]*BidWithDetails, int, error) {
	// Verify auction exists
	if err := s.verifyAuctionExists(ctx, auctionID); err != nil {
		return nil, 0, err
	}

	// Set defaults (can be overridden by caller)
	if filters.Page <= 0 {
		filters.Page = 1
	}
	if filters.Limit <= 0 || filters.Limit > 1000 {
		filters.Limit = 50
	}

	bids, total, err := s.repo.GetAuctionBidsWithDetails(ctx, auctionID, filters)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get auction bids: %w", err)
	}

	return bids, total, nil
}

// RemoveBid removes a bid (Admin only)
func (s *BidService) RemoveBid(ctx context.Context, bidID int64, reason string, removedBy int64) error {
	// Get bid details
	bid, err := s.repo.GetBid(ctx, bidID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &errs.Error{
				Code:    errs.NotFound,
				Message: "المزايدة غير موجودة",
			}
		}
		return fmt.Errorf("failed to get bid: %w", err)
	}

	// Get auction to check if it's still live
	auction, err := s.getAuctionForBidding(ctx, bid.AuctionID)
	if err != nil {
		return err
	}

	// Start transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Remove the bid
	if err := s.repo.RemoveBid(ctx, tx, bidID); err != nil {
		return fmt.Errorf("failed to remove bid: %w", err)
	}

	// Remove any extensions caused by this bid
	if err := s.removeExtensionsByBid(ctx, tx, bidID); err != nil {
		return fmt.Errorf("failed to remove extensions: %w", err)
	}

	// Recalculate auction end time if extensions were removed
	if err := s.recalculateAuctionEndTime(ctx, tx, auction.ID); err != nil {
		return fmt.Errorf("failed to recalculate end time: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Broadcast bid_removed event
	realtimeService := GetRealtimeService()
	if realtimeService != nil {
		if err := realtimeService.BroadcastBidRemoved(ctx, auction.ID, bidID, reason, "admin"); err != nil {
			fmt.Printf("Failed to broadcast bid_removed event: %v\n", err)
		}

		// Get fresh auction state and new current price after removal
		auctionRepo := NewRepository(s.db)
		updatedAuction, err := auctionRepo.GetAuction(ctx, auction.ID)
		if err == nil {
			newCurrentPrice, err := s.getCurrentPrice(ctx, auction.ID, updatedAuction.StartPrice)
			if err == nil {
				if err := realtimeService.BroadcastPriceRecomputed(ctx, auction.ID, newCurrentPrice, updatedAuction.ExtensionsCount, "bid_removed"); err != nil {
					fmt.Printf("Failed to broadcast price_recomputed event: %v\n", err)
				}
			}
		}
	}

	// Log audit entry for bid removal
	auditEntry := audit.Entry{
		EntityType: "bid",
		EntityID:   fmt.Sprintf("%d", bidID),
		Action:     "removed",
		Reason:     &reason,
		Meta: map[string]interface{}{
			"auction_id":   auction.ID,
			"bid_amount":   bid.Amount,
			"bidder_id":    bid.UserID,
			"removed_by":   removedBy,
		},
	}
	
	if _, err := audit.Log(ctx, s.db, auditEntry); err != nil {
		fmt.Printf("Failed to log audit entry for bid removal: %v\n", err)
	}

	// Send notification to bidder about removal
	go s.sendBidRemovedNotification(ctx, bid, auction, reason, removedBy)

	return nil
}

// sendBidRemovedNotification sends notifications when a bid is removed
func (s *BidService) sendBidRemovedNotification(ctx context.Context, bid *Bid, auction *Auction, reason string, removedBy int64) {
	// Get user info for notification
	var userEmail, userName string
	if err := s.db.QueryRow(ctx, "SELECT email, name FROM users WHERE id = $1", bid.UserID).Scan(&userEmail, &userName); err != nil {
		fmt.Printf("Failed to get user info for notification: %v\n", err)
		return
	}

	// Get product title for context
	var productTitle string
	if err := s.db.QueryRow(ctx, "SELECT title FROM products WHERE id = $1", auction.ProductID).Scan(&productTitle); err != nil {
		productTitle = fmt.Sprintf("المزاد #%d", auction.ID)
	}

	payload := map[string]interface{}{
		"bidder_name":   bid.BidderNameSnapshot,
		"auction_id":    auction.ID,
		"product_title": productTitle,
		"bid_amount":    fmt.Sprintf("%.2f", bid.Amount),
		"reason":        reason,
		"removed_by":    removedBy,
		"email":         userEmail,
		"name":          userName,
		"language":      "ar",
	}

	// Send internal notification
	if _, err := notifications.EnqueueInternal(ctx, bid.UserID, "bid_removed", payload); err != nil {
		fmt.Printf("Failed to send internal notification: %v\n", err)
	}

	// Send email notification
	if _, err := notifications.EnqueueEmail(ctx, bid.UserID, "bid_removed", payload); err != nil {
		fmt.Printf("Failed to send email notification: %v\n", err)
	}
}

// validateBidderPermissions validates that user can place bids
func (s *BidService) validateBidderPermissions(ctx context.Context, userID int64) error {
	var role, state string
	var emailVerifiedAt sql.NullTime

	query := `SELECT role, state, email_verified_at FROM users WHERE id = $1`
	err := s.db.QueryRow(ctx, query, userID).Scan(&role, &state, &emailVerifiedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &errs.Error{
				Code:    errs.NotFound,
				Message: "المستخدم غير موجود",
			}
		}
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Check if user is active
	if state != "active" {
		return &errs.Error{
			Code:    errs.Forbidden,
			Message: "حسابك غير نشط",
		}
	}

	// Check if email is verified
	if !emailVerifiedAt.Valid {
		return &errs.Error{
			Code:    errs.Forbidden,
			Message: "يجب تفعيل بريدك الإلكتروني للمزايدة",
		}
	}

	// Check if user is verified or admin (required for bidding)
	if role != "verified" && role != "admin" {
		return &errs.Error{
			Code:    errs.Forbidden,
			Message: "المزايدة متاحة للمستخدمين الموثقين فقط",
		}
	}

	return nil
}

// getAuctionForBidding gets auction with row lock
func (s *BidService) getAuctionForBidding(ctx context.Context, auctionID int64) (*Auction, error) {
	query := `
		SELECT id, product_id, start_price, bid_step, reserve_price,
			   start_at, end_at, anti_sniping_minutes, status,
			   extensions_count, max_extensions_override, created_at, updated_at
		FROM auctions 
		WHERE id = $1
		FOR UPDATE`

	auction := &Auction{}
	err := s.db.QueryRow(ctx, query, auctionID).Scan(
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &errs.Error{
				Code:    errs.NotFound,
				Message: "المزاد غير موجود",
			}
		}
		return nil, fmt.Errorf("failed to get auction: %w", err)
	}

	return auction, nil
}

// getAuctionForBiddingTx gets auction with row lock using a transaction
func (s *BidService) getAuctionForBiddingTx(ctx context.Context, tx *sqldb.Tx, auctionID int64) (*Auction, error) {
    query := `
        SELECT id, product_id, start_price, bid_step, reserve_price,
               start_at, end_at, anti_sniping_minutes, status,
               extensions_count, max_extensions_override, created_at, updated_at
        FROM auctions 
        WHERE id = $1
        FOR UPDATE`

    auction := &Auction{}
    err := tx.QueryRow(ctx, query, auctionID).Scan(
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
        if errors.Is(err, sql.ErrNoRows) {
            return nil, &errs.Error{
                Code:    errs.NotFound,
                Message: "المزاد غير موجود",
            }
        }
        return nil, fmt.Errorf("failed to get auction: %w", err)
    }

    return auction, nil
}

// validateAuctionForBidding validates auction state for bidding
func (s *BidService) validateAuctionForBidding(auction *Auction) error {
	// Check if auction is live
	if auction.Status != AuctionStatusLive {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "لا يمكن المزايدة على مزاد غير نشط",
		}
	}

	// Check if auction has ended
	if auction.EndAt.Before(time.Now().UTC()) {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "انتهى وقت المزاد",
		}
	}

	return nil
}

// getCurrentPrice gets the current highest bid amount
func (s *BidService) getCurrentPrice(ctx context.Context, auctionID int64, startPrice float64) (float64, error) {
	var currentPrice sql.NullFloat64
	query := `SELECT MAX(amount) FROM bids WHERE auction_id = $1`
	err := s.db.QueryRow(ctx, query, auctionID).Scan(&currentPrice)
	if err != nil {
		return 0, fmt.Errorf("failed to get current price: %w", err)
	}

	if currentPrice.Valid {
		return currentPrice.Float64, nil
	}

	return startPrice, nil
}

// getCurrentPriceTx gets the current highest bid amount using the provided transaction
func (s *BidService) getCurrentPriceTx(ctx context.Context, tx *sqldb.Tx, auctionID int64, startPrice float64) (float64, error) {
    var currentPrice sql.NullFloat64
    query := `SELECT MAX(amount) FROM bids WHERE auction_id = $1`
    err := tx.QueryRow(ctx, query, auctionID).Scan(&currentPrice)
    if err != nil {
        return 0, fmt.Errorf("failed to get current price: %w", err)
    }

    if currentPrice.Valid {
        return currentPrice.Float64, nil
    }

    return startPrice, nil
}

// validateBidAmount validates that bid amount follows the step rules
func (s *BidService) validateBidAmount(amount, currentPrice float64, bidStep int) error {
	// Calculate required minimum
	requiredMin := currentPrice + float64(bidStep)

	// Check if amount meets minimum
	if amount < requiredMin {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: fmt.Sprintf("المبلغ يجب أن يكون على الأقل %.2f ر.س", requiredMin),
		}
	}

	// Check if amount is a valid multiple of bid step
	diff := amount - currentPrice
	remainder := math.Mod(diff, float64(bidStep))

	// Allow small floating point precision errors (less than 0.01)
	if remainder > 0.01 && remainder < float64(bidStep)-0.01 {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: fmt.Sprintf("المبلغ يجب أن يكون مضاعفاً لخطوة الزيادة (%d ر.س)", bidStep),
		}
	}

	return nil
}

// getUserSnapshotData gets user data for bid snapshot
func (s *BidService) getUserSnapshotData(ctx context.Context, userID int64) (string, *int64, error) {
	var name string
	var cityID sql.NullInt64

	query := `SELECT name, city_id FROM users WHERE id = $1`
	err := s.db.QueryRow(ctx, query, userID).Scan(&name, &cityID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get user data: %w", err)
	}

	var cityIDPtr *int64
	if cityID.Valid {
		cityIDPtr = &cityID.Int64
	}

	return name, cityIDPtr, nil
}

// handleAntiSniping handles anti-sniping logic
func (s *BidService) handleAntiSniping(ctx context.Context, tx *sqldb.Tx, auction *Auction, bid *Bid) (bool, error) {
	// Calculate time remaining
	timeRemaining := auction.EndAt.Sub(time.Now().UTC())
	antiSnipingDuration := time.Duration(auction.AntiSnipingMinutes) * time.Minute

	// Check if bid is within anti-sniping window
	if timeRemaining <= antiSnipingDuration {
		// Get max extensions setting
		maxExtensions, err := s.getMaxExtensions(ctx, auction.MaxExtensionsOverride)
		if err != nil {
			return false, err
		}

		// Check if we can still extend (0 = unlimited)
		if maxExtensions == 0 || auction.ExtensionsCount < maxExtensions {
			// Calculate new end time
			newEndAt := auction.EndAt.Add(antiSnipingDuration)

			// Update auction with optimistic locking
			updateQuery := `
				UPDATE auctions 
				SET end_at = $1, extensions_count = extensions_count + 1, updated_at = NOW()
				WHERE id = $2 AND end_at = $3`

			result, err := tx.Exec(ctx, updateQuery, newEndAt, auction.ID, auction.EndAt)
			if err != nil {
				return false, fmt.Errorf("failed to extend auction: %w", err)
			}

			rowsAffected := result.RowsAffected()
			if rowsAffected > 0 {
				// Record the extension
				extensionQuery := `
					INSERT INTO auction_extensions (auction_id, extended_by_bid_id, old_end_at, new_end_at)
					VALUES ($1, $2, $3, $4)`
				_, err = tx.Exec(ctx, extensionQuery, auction.ID, bid.ID, auction.EndAt, newEndAt)
				if err != nil {
					return false, fmt.Errorf("failed to record extension: %w", err)
				}

				return true, nil
			}
		}
	}

	return false, nil
}

// getMaxExtensions gets max extensions setting
func (s *BidService) getMaxExtensions(ctx context.Context, override *int) (int, error) {
	if override != nil {
		return *override, nil
	}

	var maxExtensionsStr string
	query := `SELECT value FROM system_settings WHERE key = 'auctions.max_extensions'`
	err := s.db.QueryRow(ctx, query).Scan(&maxExtensionsStr)
	if err != nil {
		return 0, nil // default to unlimited
	}

	var maxExtensions int
	if _, err := fmt.Sscanf(maxExtensionsStr, "%d", &maxExtensions); err != nil {
		return 0, nil // default to unlimited
	}

	return maxExtensions, nil
}

// verifyAuctionExists verifies that an auction exists
func (s *BidService) verifyAuctionExists(ctx context.Context, auctionID int64) error {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM auctions WHERE id = $1)`
	err := s.db.QueryRow(ctx, query, auctionID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check auction existence: %w", err)
	}

	if !exists {
		return &errs.Error{
			Code:    errs.NotFound,
			Message: "المزاد غير موجود",
		}
	}

	return nil
}

// removeExtensionsByBid removes extensions caused by a specific bid
func (s *BidService) removeExtensionsByBid(ctx context.Context, tx *sqldb.Tx, bidID int64) error {
	query := `DELETE FROM auction_extensions WHERE extended_by_bid_id = $1`
	_, err := tx.Exec(ctx, query, bidID)
	return err
}

// recalculateAuctionEndTime recalculates auction end time after bid removal
func (s *BidService) recalculateAuctionEndTime(ctx context.Context, tx *sqldb.Tx, auctionID int64) error {
    // Fetch current auction state (end_at, extensions_count, anti_sniping_minutes)
    var endAt time.Time
    var extCount, antiSnipingMinutes int
    err := tx.QueryRow(ctx, `SELECT end_at, extensions_count, anti_sniping_minutes FROM auctions WHERE id = $1`, auctionID).
        Scan(&endAt, &extCount, &antiSnipingMinutes)
    if err != nil {
        return fmt.Errorf("failed to get auction state: %w", err)
    }

    // Count remaining extensions after deletions
    var remaining int
    if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM auction_extensions WHERE auction_id = $1`, auctionID).Scan(&remaining); err != nil {
        return fmt.Errorf("failed to count remaining extensions: %w", err)
    }

    // Compute base end time by rolling back current end_at by existing extensions
    extensionDuration := time.Duration(antiSnipingMinutes) * time.Minute
    baseEndAt := endAt.Add(-time.Duration(extCount) * extensionDuration)
    newEndAt := baseEndAt.Add(time.Duration(remaining) * extensionDuration)

    // Update auction end time and extension count to reflect remaining extensions
    _, err = tx.Exec(ctx, `UPDATE auctions SET end_at = $1, extensions_count = $2, updated_at = NOW() WHERE id = $3`, newEndAt, remaining, auctionID)
    return err
}
