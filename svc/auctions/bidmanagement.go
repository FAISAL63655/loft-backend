package auctions

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"encore.app/pkg/errs"
	"encore.app/svc/notifications"
	"encore.dev/storage/sqldb"
)

// BidManagementService handles administrative bid management
type BidManagementService struct {
	db              *sqldb.Database
	repo            *Repository
	bidRepo         *BidRepository
	realtimeService *RealtimeService
}

// NewBidManagementService creates a new bid management service
func NewBidManagementService(db *sqldb.Database) *BidManagementService {
	return &BidManagementService{
		db:              db,
		repo:            NewRepository(db),
		bidRepo:         NewBidRepository(db),
		realtimeService: GetRealtimeService(),
	}
}

// RemoveBidResponse represents the response after removing a bid
type RemoveBidResponse struct {
	Success              bool    `json:"success"`
	Message              string  `json:"message"`
	NewCurrentPrice      float64 `json:"new_current_price"`
	ExtensionsRemoved    int     `json:"extensions_removed"`
	AffectedBiddersCount int     `json:"affected_bidders_count"`
}

// BidRemovalAuditEntry represents an audit entry for bid removal
type BidRemovalAuditEntry struct {
	ID        int64     `json:"id"`
	BidID     int64     `json:"bid_id"`
	AuctionID int64     `json:"auction_id"`
	RemovedBy string    `json:"removed_by"`
	Reason    string    `json:"reason"`
	OldAmount float64   `json:"old_amount"`
	NewPrice  float64   `json:"new_price"`
	RemovedAt time.Time `json:"removed_at"`
}

// RemoveBid removes a bid with full audit trail and notifications
func (s *BidManagementService) RemoveBid(ctx context.Context, bidID int64, reason string, removedBy string) (*RemoveBidResponse, error) {
	// Start transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Get bid details before removal
	bid, err := s.getBidForRemoval(ctx, tx, bidID)
	if err != nil {
		return nil, err
	}

	// Get auction details
	auction, err := s.getAuctionForBidRemoval(ctx, tx, bid.AuctionID)
	if err != nil {
		return nil, err
	}

	// Validate bid can be removed
	if err := s.validateBidRemoval(auction, bid); err != nil {
		return nil, err
	}

	// Remove related extensions first
	extensionsRemoved, err := s.removeRelatedExtensions(ctx, tx, bidID)
	if err != nil {
		return nil, fmt.Errorf("failed to remove related extensions: %w", err)
	}

	// Remove the bid
	if err := s.removeBidFromDatabase(ctx, tx, bidID); err != nil {
		return nil, fmt.Errorf("failed to remove bid: %w", err)
	}

	// Calculate new current price
	newCurrentPrice, err := s.calculateNewCurrentPrice(ctx, tx, auction)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate new current price: %w", err)
	}

	// Create audit entry
	auditEntry := &BidRemovalAuditEntry{
		BidID:     bidID,
		AuctionID: bid.AuctionID,
		RemovedBy: removedBy,
		Reason:    reason,
		OldAmount: bid.Amount,
		NewPrice:  newCurrentPrice,
		RemovedAt: time.Now().UTC(),
	}

	if err := s.createAuditEntry(ctx, tx, auditEntry); err != nil {
		return nil, fmt.Errorf("failed to create audit entry: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Send notifications and broadcasts (after commit)
	go s.sendPostRemovalNotifications(ctx, bid, auction, reason, removedBy, newCurrentPrice)

	// Broadcast events
	if s.realtimeService != nil {
		// Broadcast bid_removed event
		if err := s.realtimeService.BroadcastBidRemoved(ctx, auction.ID, bidID, reason, removedBy); err != nil {
			fmt.Printf("Failed to broadcast bid_removed event: %v\n", err)
		}

		// Broadcast price_recomputed event
		if err := s.realtimeService.BroadcastPriceRecomputed(ctx, auction.ID, newCurrentPrice, auction.ExtensionsCount, "bid_removed"); err != nil {
			fmt.Printf("Failed to broadcast price_recomputed event: %v\n", err)
		}
	}

	// Count affected bidders (those who bid after the removed bid)
	affectedCount, err := s.countAffectedBidders(ctx, bid.AuctionID, bid.CreatedAt)
	if err != nil {
		affectedCount = 0 // Don't fail the operation for this
	}

	response := &RemoveBidResponse{
		Success:              true,
		Message:              fmt.Sprintf("تم حذف المزايدة بنجاح. السعر الجديد: %.2f ر.س", newCurrentPrice),
		NewCurrentPrice:      newCurrentPrice,
		ExtensionsRemoved:    extensionsRemoved,
		AffectedBiddersCount: affectedCount,
	}

	return response, nil
}

// BulkRemoveBids removes multiple bids in a single transaction
func (s *BidManagementService) BulkRemoveBids(ctx context.Context, bidIDs []int64, reason string, removedBy string) (*BulkRemovalResponse, error) {
	if len(bidIDs) == 0 {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "لا توجد مزايدات للحذف",
		}
	}

	// Start transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	var removedBids []*Bid
	var failedRemovals []BidRemovalFailure
	totalExtensionsRemoved := 0

	// Process each bid
	for _, bidID := range bidIDs {
		bid, err := s.getBidForRemoval(ctx, tx, bidID)
		if err != nil {
			failedRemovals = append(failedRemovals, BidRemovalFailure{
				BidID: bidID,
				Error: err.Error(),
			})
			continue
		}

		// Remove related extensions
		extensionsRemoved, err := s.removeRelatedExtensions(ctx, tx, bidID)
		if err != nil {
			failedRemovals = append(failedRemovals, BidRemovalFailure{
				BidID: bidID,
				Error: fmt.Sprintf("failed to remove extensions: %v", err),
			})
			continue
		}

		// Remove the bid
		if err := s.removeBidFromDatabase(ctx, tx, bidID); err != nil {
			failedRemovals = append(failedRemovals, BidRemovalFailure{
				BidID: bidID,
				Error: fmt.Sprintf("failed to remove bid: %v", err),
			})
			continue
		}

		removedBids = append(removedBids, bid)
		totalExtensionsRemoved += extensionsRemoved

		// Create audit entry
		auditEntry := &BidRemovalAuditEntry{
			BidID:     bidID,
			AuctionID: bid.AuctionID,
			RemovedBy: removedBy,
			Reason:    reason,
			OldAmount: bid.Amount,
			RemovedAt: time.Now().UTC(),
		}

		if err := s.createAuditEntry(ctx, tx, auditEntry); err != nil {
			fmt.Printf("Failed to create audit entry for bid %d: %v\n", bidID, err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Group by auction and broadcast events
	auctionGroups := make(map[int64][]*Bid)
	for _, bid := range removedBids {
		auctionGroups[bid.AuctionID] = append(auctionGroups[bid.AuctionID], bid)
	}

    for auctionID, bids := range auctionGroups {
        // Calculate new current price for this auction
        auction, err := s.repo.GetAuction(ctx, auctionID)
        if err != nil {
            continue
        }

        // Recompute auction end time and extensions_count based on remaining extensions
        var remaining int
        if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM auction_extensions WHERE auction_id = $1`, auctionID).Scan(&remaining); err == nil {
            extensionDuration := time.Duration(auction.AntiSnipingMinutes) * time.Minute
            baseEndAt := auction.EndAt.Add(-time.Duration(auction.ExtensionsCount) * extensionDuration)
            newEndAt := baseEndAt.Add(time.Duration(remaining) * extensionDuration)
            _, _ = s.db.Exec(ctx, `UPDATE auctions SET end_at = $1, extensions_count = $2, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id = $3`, newEndAt, remaining, auctionID)
            // reflect new extensions count for broadcasting
            auction.ExtensionsCount = remaining
        }

        newCurrentPrice, err := s.calculateNewCurrentPrice(ctx, nil, auction)
        if err != nil {
            continue
        }

		// Broadcast events for each removed bid
		for _, bid := range bids {
			if s.realtimeService != nil {
				s.realtimeService.BroadcastBidRemoved(ctx, auctionID, bid.ID, reason, removedBy)
			}
		}

		// Broadcast price recomputed once per auction
        if s.realtimeService != nil {
            s.realtimeService.BroadcastPriceRecomputed(ctx, auctionID, newCurrentPrice, auction.ExtensionsCount, "bulk_bid_removal")
        }
	}

	response := &BulkRemovalResponse{
		TotalRequested:     len(bidIDs),
		SuccessfulRemovals: len(removedBids),
		FailedRemovals:     len(failedRemovals),
		Failures:           failedRemovals,
		ExtensionsRemoved:  totalExtensionsRemoved,
		AffectedAuctions:   len(auctionGroups),
	}

	return response, nil
}

// GetBidRemovalHistory gets the history of bid removals for an auction
func (s *BidManagementService) GetBidRemovalHistory(ctx context.Context, auctionID int64) ([]*BidRemovalAuditEntry, error) {
	query := `
		SELECT id, bid_id, auction_id, removed_by, reason, old_amount, new_price, removed_at
		FROM bid_removal_audit
		WHERE auction_id = $1
		ORDER BY removed_at DESC`

	rows, err := s.db.Query(ctx, query, auctionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query bid removal history: %w", err)
	}
	defer rows.Close()

	var entries []*BidRemovalAuditEntry
	for rows.Next() {
		entry := &BidRemovalAuditEntry{}
		err := rows.Scan(
			&entry.ID,
			&entry.BidID,
			&entry.AuctionID,
			&entry.RemovedBy,
			&entry.Reason,
			&entry.OldAmount,
			&entry.NewPrice,
			&entry.RemovedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan bid removal entry: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// Private helper methods

func (s *BidManagementService) getBidForRemoval(ctx context.Context, tx *sqldb.Tx, bidID int64) (*Bid, error) {
	query := `
		SELECT id, auction_id, user_id, amount, created_at, 
			   bidder_name_snapshot, bidder_city_id_snapshot
		FROM bids 
		WHERE id = $1`

	bid := &Bid{}
	var err error
	if tx != nil {
		err = tx.QueryRow(ctx, query, bidID).Scan(
			&bid.ID,
			&bid.AuctionID,
			&bid.UserID,
			&bid.Amount,
			&bid.CreatedAt,
			&bid.BidderNameSnapshot,
			&bid.BidderCityIDSnapshot,
		)
	} else {
		err = s.db.QueryRow(ctx, query, bidID).Scan(
			&bid.ID,
			&bid.AuctionID,
			&bid.UserID,
			&bid.Amount,
			&bid.CreatedAt,
			&bid.BidderNameSnapshot,
			&bid.BidderCityIDSnapshot,
		)
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &errs.Error{
				Code:    errs.NotFound,
				Message: "المزايدة غير موجودة",
			}
		}
		return nil, fmt.Errorf("failed to get bid: %w", err)
	}

	return bid, nil
}

func (s *BidManagementService) getAuctionForBidRemoval(ctx context.Context, tx *sqldb.Tx, auctionID int64) (*Auction, error) {
	query := `
		SELECT id, product_id, start_price, bid_step, reserve_price,
			   start_at, end_at, anti_sniping_minutes, status,
			   extensions_count, max_extensions_override, created_at, updated_at
		FROM auctions 
		WHERE id = $1`

	auction := &Auction{}
	var err error
	if tx != nil {
		err = tx.QueryRow(ctx, query, auctionID).Scan(
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
	} else {
		err = s.db.QueryRow(ctx, query, auctionID).Scan(
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
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &errs.Error{
				Code:    errs.NotFound,
				Message: "المزاد غير موجود",
			}
		}
		return nil, fmt.Errorf("failed to get auction: %w", err)
	}

	return auction, nil
}

func (s *BidManagementService) validateBidRemoval(auction *Auction, bid *Bid) error {
	// Can only remove bids from live or ended auctions
	if auction.Status != AuctionStatusLive && auction.Status != AuctionStatusEnded {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "لا يمكن حذف مزايدات من مزاد غير نشط أو منتهي",
		}
	}

	return nil
}

func (s *BidManagementService) removeRelatedExtensions(ctx context.Context, tx *sqldb.Tx, bidID int64) (int, error) {
	query := `DELETE FROM auction_extensions WHERE extended_by_bid_id = $1`
	result, err := tx.Exec(ctx, query, bidID)
	if err != nil {
		return 0, err
	}

	rowsAffected := result.RowsAffected()
	return int(rowsAffected), nil
}

func (s *BidManagementService) removeBidFromDatabase(ctx context.Context, tx *sqldb.Tx, bidID int64) error {
	query := `DELETE FROM bids WHERE id = $1`
	_, err := tx.Exec(ctx, query, bidID)
	return err
}

func (s *BidManagementService) calculateNewCurrentPrice(ctx context.Context, tx *sqldb.Tx, auction *Auction) (float64, error) {
	var currentPrice sql.NullFloat64
	query := `SELECT MAX(amount) FROM bids WHERE auction_id = $1`

	var err error
	if tx != nil {
		err = tx.QueryRow(ctx, query, auction.ID).Scan(&currentPrice)
	} else {
		err = s.db.QueryRow(ctx, query, auction.ID).Scan(&currentPrice)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get current price: %w", err)
	}

	if currentPrice.Valid {
		return currentPrice.Float64, nil
	}

	return auction.StartPrice, nil
}

func (s *BidManagementService) createAuditEntry(ctx context.Context, tx *sqldb.Tx, entry *BidRemovalAuditEntry) error {
	query := `
		INSERT INTO audit_logs (entity_type, entity_id, action, actor, details, created_at)
		VALUES ($1, $2, $3, $4, $5, (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'))
		RETURNING id`

	details := fmt.Sprintf("{\"bid_id\":%d,\"old_amount\":%f,\"new_price\":%f,\"reason\":%q}", entry.BidID, entry.OldAmount, entry.NewPrice, entry.Reason)

	var id int64
	err := tx.QueryRow(ctx, query,
		"auction",       // entity_type
		entry.AuctionID, // entity_id
		"bid_removed",   // action
		entry.RemovedBy, // actor
		details,         // details (JSON string)
	).Scan(&id)

	entry.ID = id
	return err
}

func (s *BidManagementService) countAffectedBidders(ctx context.Context, auctionID int64, removedBidTime time.Time) (int, error) {
	var count int
	query := `
		SELECT COUNT(DISTINCT user_id) 
		FROM bids 
		WHERE auction_id = $1 AND created_at > $2`

	err := s.db.QueryRow(ctx, query, auctionID, removedBidTime).Scan(&count)
	return count, err
}

func (s *BidManagementService) sendPostRemovalNotifications(ctx context.Context, bid *Bid, auction *Auction, reason, removedBy string, newPrice float64) {
	// 1. Notify the bidder whose bid was removed
	s.notifyBidderRemoved(ctx, bid, auction, reason, removedBy)
	
	// 2. Notify other affected bidders (those who bid after the removed bid)
	s.notifyAffectedBidders(ctx, bid, auction, newPrice)
	
	// 3. Notify auction watchers about price change
	s.notifyAuctionWatchers(ctx, auction, newPrice, reason)
}

// notifyBidderRemoved sends notification to the bidder whose bid was removed
func (s *BidManagementService) notifyBidderRemoved(ctx context.Context, bid *Bid, auction *Auction, reason, removedBy string) {
    // Get product title for context
    var productTitle string
    if err := s.db.QueryRow(ctx, "SELECT title FROM products WHERE id = $1", auction.ProductID).Scan(&productTitle); err != nil {
        productTitle = fmt.Sprintf("المزاد #%d", auction.ID)
    }

    payload := map[string]interface{}{
        "bidder_name":  bid.BidderNameSnapshot,
        "auction_id":   fmt.Sprint(auction.ID),
        "product_title": productTitle,
        "bid_amount":   fmt.Sprintf("%.2f", bid.Amount),
        "reason":       reason,
        "removed_by":   removedBy,
        "language":     "ar",
    }
	
	// Send internal notification
	if _, err := notifications.EnqueueInternal(ctx, bid.UserID, "bid_removed", payload); err != nil {
		fmt.Printf("Failed to send internal notification to bidder %d: %v\n", bid.UserID, err)
	}
	
	// Get user email for email notification
	var userEmail, userName string
	if err := s.db.QueryRow(ctx, "SELECT email, name FROM users WHERE id = $1", bid.UserID).Scan(&userEmail, &userName); err == nil {
		payload["email"] = userEmail
		payload["name"] = userName
		
		if _, err := notifications.EnqueueEmail(ctx, bid.UserID, "bid_removed", payload); err != nil {
			fmt.Printf("Failed to send email notification to bidder %d: %v\n", bid.UserID, err)
		}
	}
}

// notifyAffectedBidders sends notifications to bidders who bid after the removed bid
func (s *BidManagementService) notifyAffectedBidders(ctx context.Context, removedBid *Bid, auction *Auction, newPrice float64) {
    // Get product title for context
    var productTitle string
    if err := s.db.QueryRow(ctx, "SELECT title FROM products WHERE id = $1", auction.ProductID).Scan(&productTitle); err != nil {
        productTitle = fmt.Sprintf("المزاد #%d", auction.ID)
    }

	// Get affected bidders (those who bid after the removed bid)
	query := `
		SELECT DISTINCT b.user_id, u.email, u.name, b.bidder_name_snapshot
		FROM bids b
		JOIN users u ON b.user_id = u.id
		WHERE b.auction_id = $1 AND b.created_at > $2 AND b.user_id != $3`
	
	rows, err := s.db.Query(ctx, query, auction.ID, removedBid.CreatedAt, removedBid.UserID)
	if err != nil {
		fmt.Printf("Failed to query affected bidders: %v\n", err)
		return
	}
	defer rows.Close()
	
	for rows.Next() {
		var userID int64
		var email, name, bidderName string
		
		if err := rows.Scan(&userID, &email, &name, &bidderName); err != nil {
			continue
		}
		
		payload := map[string]interface{}{
			"bidder_name":  bidderName,
			"auction_id":   fmt.Sprint(auction.ID),
			"product_title": productTitle,
			"new_price":    fmt.Sprintf("%.2f", newPrice),
			"email":        email,
			"name":         name,
			"language":     "ar",
		}
		
		// Send internal notification
		if _, err := notifications.EnqueueInternal(ctx, userID, "auction_price_changed", payload); err != nil {
			fmt.Printf("Failed to send internal notification to affected bidder %d: %v\n", userID, err)
		}
		
		// Send email notification
		if _, err := notifications.EnqueueEmail(ctx, userID, "auction_price_changed", payload); err != nil {
			fmt.Printf("Failed to send email notification to affected bidder %d: %v\n", userID, err)
		}
	}
}

// notifyAuctionWatchers sends notifications to users watching the auction
func (s *BidManagementService) notifyAuctionWatchers(ctx context.Context, auction *Auction, newPrice float64, reason string) {
    // Get product title for context
    var productTitle string
    if err := s.db.QueryRow(ctx, "SELECT title FROM products WHERE id = $1", auction.ProductID).Scan(&productTitle); err != nil {
        productTitle = fmt.Sprintf("المزاد #%d", auction.ID)
    }

	// Note: In a real implementation, you would have a watchers/followers table
	// For now, we'll notify recent bidders as they are likely watching
	query := `
		SELECT DISTINCT b.user_id, u.email, u.name, b.bidder_name_snapshot
		FROM bids b
		JOIN users u ON b.user_id = u.id
		WHERE b.auction_id = $1 AND b.created_at > NOW() - INTERVAL '24 hours'
		ORDER BY b.created_at DESC
		LIMIT 50`
	
	rows, err := s.db.Query(ctx, query, auction.ID)
	if err != nil {
		fmt.Printf("Failed to query auction watchers: %v\n", err)
		return
	}
	defer rows.Close()
	
	for rows.Next() {
		var userID int64
		var email, name, bidderName string
		
		if err := rows.Scan(&userID, &email, &name, &bidderName); err != nil {
			continue
		}
		
		payload := map[string]interface{}{
			"bidder_name":  bidderName,
			"auction_id":   fmt.Sprint(auction.ID),
			"product_title": productTitle,
			"new_price":    fmt.Sprintf("%.2f", newPrice),
			"reason":       reason,
			"email":        email,
			"name":         name,
			"language":     "ar",
		}
		
		// Send internal notification only (avoid spam emails)
		if _, err := notifications.EnqueueInternal(ctx, userID, "auction_admin_action", payload); err != nil {
			fmt.Printf("Failed to send internal notification to watcher %d: %v\n", userID, err)
		}
	}
}

// Supporting types

type BulkRemovalResponse struct {
	TotalRequested     int                 `json:"total_requested"`
	SuccessfulRemovals int                 `json:"successful_removals"`
	FailedRemovals     int                 `json:"failed_removals"`
	Failures           []BidRemovalFailure `json:"failures,omitempty"`
	ExtensionsRemoved  int                 `json:"extensions_removed"`
	AffectedAuctions   int                 `json:"affected_auctions"`
}

type BidRemovalFailure struct {
	BidID int64  `json:"bid_id"`
	Error string `json:"error"`
}
