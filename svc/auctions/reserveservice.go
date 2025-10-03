package auctions

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"encore.app/pkg/audit"
	"encore.app/pkg/errs"
	"encore.app/svc/notifications"
	"encore.dev/storage/sqldb"
)

// ReserveService handles reserve price logic for auctions
type ReserveService struct {
	db   *sqldb.Database
	repo *Repository
}

// NewReserveService creates a new reserve service
func NewReserveService(db *sqldb.Database) *ReserveService {
	return &ReserveService{
		db:   db,
		repo: NewRepository(db),
	}
}

// CheckReservePrice checks if the highest bid meets the reserve price
func (s *ReserveService) CheckReservePrice(ctx context.Context, auctionID int64) (bool, error) {
	// Get auction details
	auction, err := s.repo.GetAuction(ctx, auctionID)
	if err != nil {
		return false, fmt.Errorf("failed to get auction: %w", err)
	}

	// If no reserve price is set, always pass
	if auction.ReservePrice == nil {
		return true, nil
	}

	// Get highest bid
	highestBid, err := s.getHighestBidAmount(ctx, auctionID)
	if err != nil {
		return false, fmt.Errorf("failed to get highest bid: %w", err)
	}

	// Check if highest bid meets or exceeds reserve price
	return highestBid >= *auction.ReservePrice, nil
}

// ProcessAuctionEnd processes auction ending with reserve price logic
func (s *ReserveService) ProcessAuctionEnd(ctx context.Context, auctionID int64) (*AuctionEndResult, error) {
	// Start transaction for atomic processing
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Get auction with lock
	auction, err := s.getAuctionForProcessing(ctx, tx, auctionID)
	if err != nil {
		return nil, err
	}

	// Validate auction can be ended
	if err := s.validateAuctionForEnding(auction); err != nil {
		return nil, err
	}

	// Get highest bid
	highestBid, err := s.getHighestBidWithDetails(ctx, auctionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get highest bid: %w", err)
	}

	// Determine auction outcome
	result := &AuctionEndResult{
		AuctionID: auctionID,
		EndedAt:   time.Now().UTC(),
	}

	if highestBid == nil {
		// No bids - auction ends without winner
		result.Outcome = AuctionOutcomeNoBids
		result.Message = "انتهى المزاد بدون مزايدات"

		// Update auction status
		if err := s.repo.UpdateAuctionStatus(ctx, tx, auctionID, AuctionStatusEnded); err != nil {
			return nil, fmt.Errorf("failed to update auction status: %w", err)
		}

		// Update product status back to available
		if err := s.updateProductStatus(ctx, tx, auction.ProductID, "available"); err != nil {
			return nil, fmt.Errorf("failed to update product status: %w", err)
		}

	} else {
		// Check reserve price if set
		reserveMet := true
		if auction.ReservePrice != nil {
			reserveMet = highestBid.Amount >= *auction.ReservePrice
		}

		if reserveMet {
			// Reserve price met or no reserve - auction has winner
			result.Outcome = AuctionOutcomeWinner
			result.WinnerBid = highestBid
			result.Message = fmt.Sprintf("انتهى المزاد بفوز المزايد بمبلغ %.2f ر.س", highestBid.Amount)

			// Update auction status
			if err := s.repo.UpdateAuctionStatus(ctx, tx, auctionID, AuctionStatusEnded); err != nil {
				return nil, fmt.Errorf("failed to update auction status: %w", err)
			}

			// Update product status to auction_hold
			if err := s.updateProductStatus(ctx, tx, auction.ProductID, "auction_hold"); err != nil {
				return nil, fmt.Errorf("failed to update product status: %w", err)
			}

			// Create order for winner
			orderID, err := s.createWinnerOrder(ctx, tx, auction, highestBid)
			if err != nil {
				return nil, fmt.Errorf("failed to create winner order: %w", err)
			}
			result.OrderID = &orderID

		} else {
			// Reserve price not met - auction ends without winner
			result.Outcome = AuctionOutcomeReserveNotMet
			result.HighestBid = highestBid
			result.ReservePrice = auction.ReservePrice
			result.Message = "لم يتحقق سعر الاحتياطي"

			// Update auction status
			if err := s.repo.UpdateAuctionStatus(ctx, tx, auctionID, AuctionStatusEnded); err != nil {
				return nil, fmt.Errorf("failed to update auction status: %w", err)
			}

			// Update product status back to available
			if err := s.updateProductStatus(ctx, tx, auction.ProductID, "available"); err != nil {
				return nil, fmt.Errorf("failed to update product status: %w", err)
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Broadcast auction ended event
	realtimeService := GetRealtimeService()
	if realtimeService != nil {
		if err := realtimeService.BroadcastEnded(ctx, auctionID, result); err != nil {
			fmt.Printf("Failed to broadcast auction ended event: %v\n", err)
		}
	}

	// Log audit entry for auction end
	auditEntry := audit.Entry{
		EntityType: "auction",
		EntityID:   fmt.Sprintf("%d", auctionID),
		Action:     "ended",
		Meta: map[string]interface{}{
			"outcome":      result.Outcome,
			"product_id":   auction.ProductID,
			"end_reason":   result.Outcome,
			"message":      result.Message,
			"reserve_met":  auction.ReservePrice == nil || (result.WinnerBid != nil && result.WinnerBid.Amount >= *auction.ReservePrice),
			"bids_present": highestBid != nil,
		},
	}
	
	if result.WinnerBid != nil {
		auditEntry.Meta.(map[string]interface{})["winner_user_id"] = result.WinnerBid.UserID
		auditEntry.Meta.(map[string]interface{})["winning_amount"] = result.WinnerBid.Amount
	}
	
	if result.OrderID != nil {
		auditEntry.Meta.(map[string]interface{})["order_id"] = *result.OrderID
	}
	
	if _, err := audit.Log(ctx, s.db, auditEntry); err != nil {
		fmt.Printf("Failed to log audit entry for auction end: %v\n", err)
	}

	// Send notifications based on outcome
	go s.sendAuctionEndNotifications(ctx, auction, result)

	return result, nil
}

// ValidateReservePrice validates reserve price for auction creation/update
func (s *ReserveService) ValidateReservePrice(reservePrice *float64, startPrice float64) error {
	if reservePrice == nil {
		return nil // No reserve price is valid
	}

	if *reservePrice < 0 {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "سعر الاحتياطي لا يمكن أن يكون سالباً",
		}
	}

	if *reservePrice < startPrice {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "سعر الاحتياطي يجب أن يكون أكبر من أو يساوي سعر البداية",
		}
	}

	return nil
}

// GetReserveStatus gets reserve price status for an auction
func (s *ReserveService) GetReserveStatus(ctx context.Context, auctionID int64) (*ReserveStatus, error) {
	// Get auction details
	auction, err := s.repo.GetAuction(ctx, auctionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get auction: %w", err)
	}

	status := &ReserveStatus{
		AuctionID:    auctionID,
		HasReserve:   auction.ReservePrice != nil,
		ReservePrice: auction.ReservePrice,
	}

	if auction.ReservePrice != nil {
		// Get highest bid
		highestBid, err := s.getHighestBidAmount(ctx, auctionID)
		if err != nil {
			return nil, fmt.Errorf("failed to get highest bid: %w", err)
		}

		status.HighestBid = &highestBid
		status.ReserveMet = highestBid >= *auction.ReservePrice

		if !status.ReserveMet {
			remaining := *auction.ReservePrice - highestBid
			status.AmountToReserve = &remaining
		}
	}

	return status, nil
}

// Helper methods

func (s *ReserveService) getAuctionForProcessing(ctx context.Context, tx *sqldb.Tx, auctionID int64) (*Auction, error) {
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

func (s *ReserveService) validateAuctionForEnding(auction *Auction) error {
	if auction.Status != AuctionStatusLive {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "لا يمكن إنهاء مزاد غير نشط",
		}
	}

	if auction.EndAt.After(time.Now().UTC()) {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "لم يحن وقت إنهاء المزاد بعد",
		}
	}

	return nil
}

func (s *ReserveService) getHighestBidAmount(ctx context.Context, auctionID int64) (float64, error) {
	var amount sql.NullFloat64
	query := `SELECT MAX(amount) FROM bids WHERE auction_id = $1`
	err := s.db.QueryRow(ctx, query, auctionID).Scan(&amount)
	if err != nil {
		return 0, fmt.Errorf("failed to get highest bid amount: %w", err)
	}

	if amount.Valid {
		return amount.Float64, nil
	}

	return 0, nil // No bids
}

func (s *ReserveService) getHighestBidWithDetails(ctx context.Context, auctionID int64) (*BidWithDetails, error) {
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

	err := s.db.QueryRow(ctx, query, auctionID).Scan(
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
		if err == sql.ErrNoRows {
			return nil, nil // No bids
		}
		return nil, fmt.Errorf("failed to get highest bid: %w", err)
	}

	if bidderCityName.Valid {
		bid.BidderCityName = &bidderCityName.String
	}

	return bid, nil
}

func (s *ReserveService) updateProductStatus(ctx context.Context, tx *sqldb.Tx, productID int64, status string) error {
	query := `UPDATE products SET status = $1, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id = $2`
	_, err := tx.Exec(ctx, query, status, productID)
	return err
}

func (s *ReserveService) createWinnerOrder(ctx context.Context, tx *sqldb.Tx, auction *Auction, winnerBid *BidWithDetails) (int64, error) {
	// Get product details for pricing
	var unitPriceGross float64
	query := `SELECT price_net FROM products WHERE id = $1`
	err := tx.QueryRow(ctx, query, auction.ProductID).Scan(&unitPriceGross)
	if err != nil {
		return 0, fmt.Errorf("failed to get product price: %w", err)
	}

	// For auctions, the winning bid amount is the final price
	unitPriceGross = winnerBid.Amount

	// Get VAT rate from system settings
	vatRate, err := s.getVATRate(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get VAT rate: %w", err)
	}

	// Calculate totals (auction price is already gross)
	subtotalGross := winnerBid.Amount

	// Determine shipping fee with free shipping threshold
	shippingFeeGross, err := s.getShippingFee(ctx, winnerBid.UserID)
	if err != nil {
		return 0, fmt.Errorf("failed to get shipping fee: %w", err)
	}

	// Read free shipping threshold
	var thresholdStr string
	thrQuery := `SELECT value FROM system_settings WHERE key = 'shipping.free_shipping_threshold'`
	_ = s.db.QueryRow(ctx, thrQuery).Scan(&thresholdStr)
	if thresholdStr != "" {
		var freeThreshold float64
		if _, err := fmt.Sscanf(thresholdStr, "%f", &freeThreshold); err == nil {
			if subtotalGross >= freeThreshold {
				shippingFeeGross = 0
			}
		}
	}

	// Recompute VAT amount (extracted from gross)
	vatAmount := subtotalGross * vatRate / (1 + vatRate)
	grandTotal := subtotalGross + shippingFeeGross

	// Create order
	orderQuery := `
		INSERT INTO orders (user_id, source, status, subtotal_gross, vat_amount, shipping_fee_gross, grand_total)
		VALUES ($1, 'auction', 'pending_payment', $2, $3, $4, $5)
		RETURNING id`

	var orderID int64
	err = tx.QueryRow(ctx, orderQuery, winnerBid.UserID, subtotalGross, vatAmount, shippingFeeGross, grandTotal).Scan(&orderID)
	if err != nil {
		return 0, fmt.Errorf("failed to create order: %w", err)
	}

	// Create order item
	itemQuery := `
		INSERT INTO order_items (order_id, product_id, qty, unit_price_gross, line_total_gross)
		VALUES ($1, $2, 1, $3, $4)`

	_, err = tx.Exec(ctx, itemQuery, orderID, auction.ProductID, unitPriceGross, unitPriceGross)
	if err != nil {
		return 0, fmt.Errorf("failed to create order item: %w", err)
	}

	return orderID, nil
}

func (s *ReserveService) getVATRate(ctx context.Context) (float64, error) {
	var vatRateStr string
	query := `SELECT value FROM system_settings WHERE key = 'vat.rate'`
	err := s.db.QueryRow(ctx, query).Scan(&vatRateStr)
	if err != nil {
		return 0.15, nil // Default 15% VAT
	}

	var vatRate float64
	if _, err := fmt.Sscanf(vatRateStr, "%f", &vatRate); err != nil {
		return 0.15, nil // Default fallback
	}

	return vatRate, nil
}

func (s *ReserveService) getShippingFee(ctx context.Context, userID int64) (float64, error) {
	// Get user's city
	var cityID sql.NullInt64
	userQuery := `SELECT city_id FROM users WHERE id = $1`
	err := s.db.QueryRow(ctx, userQuery, userID).Scan(&cityID)
	if err != nil {
		return 0, fmt.Errorf("failed to get user city: %w", err)
	}

	if !cityID.Valid {
		// No city set, use default shipping fee
		var defaultFeeStr string
		query := `SELECT value FROM system_settings WHERE key = 'shipping.default_fee_net'`
		err := s.db.QueryRow(ctx, query).Scan(&defaultFeeStr)
		if err != nil {
			return 25.0, nil // Default fallback
		}

		var defaultFee float64
		if _, err := fmt.Sscanf(defaultFeeStr, "%f", &defaultFee); err != nil {
			return 25.0, nil
		}

		// Convert net to gross
		vatRate, _ := s.getVATRate(ctx)
		return defaultFee * (1 + vatRate), nil
	}

	// Get city shipping fee
	var shippingFeeNet float64
	cityQuery := `SELECT shipping_fee_net FROM cities WHERE id = $1`
	err = s.db.QueryRow(ctx, cityQuery, cityID.Int64).Scan(&shippingFeeNet)
	if err != nil {
		return 25.0, nil // Default fallback
	}

	// Convert net to gross
	vatRate, _ := s.getVATRate(ctx)
	return shippingFeeNet * (1 + vatRate), nil
}

// sendAuctionEndNotifications sends notifications when an auction ends
func (s *ReserveService) sendAuctionEndNotifications(ctx context.Context, auction *Auction, result *AuctionEndResult) {
	// Get product title for context
	var productTitle string
	if err := s.db.QueryRow(ctx, "SELECT title FROM products WHERE id = $1", auction.ProductID).Scan(&productTitle); err != nil {
		productTitle = fmt.Sprintf("المزاد #%d", auction.ID)
	}

	basePayload := map[string]interface{}{
		"auction_id":    fmt.Sprint(auction.ID),
		"product_title": productTitle,
		"outcome":       result.Outcome,
		"message":       result.Message,
		"language":      "ar",
	}

	switch result.Outcome {
	case AuctionOutcomeWinner:
		if result.WinnerBid != nil {
			// Notify winner
			winnerPayload := make(map[string]interface{})
			for k, v := range basePayload {
				winnerPayload[k] = v
			}
			winnerPayload["winning_amount"] = fmt.Sprintf("%.2f", result.WinnerBid.Amount)
			winnerPayload["is_winner"] = true

			// Lookup winner email/name for email delivery
			var winnerEmail, winnerName string
			_ = s.db.QueryRow(ctx, "SELECT email, name FROM users WHERE id = $1", result.WinnerBid.UserID).Scan(&winnerEmail, &winnerName)
			if winnerEmail != "" {
				winnerPayload["email"] = winnerEmail
				if winnerName != "" { winnerPayload["name"] = winnerName }
			}
			
			if _, err := notifications.EnqueueInternal(ctx, result.WinnerBid.UserID, "auction_ended_winner", winnerPayload); err != nil {
				fmt.Printf("Failed to send winner internal notification: %v\n", err)
			}
			if _, err := notifications.EnqueueEmail(ctx, result.WinnerBid.UserID, "auction_ended_winner", winnerPayload); err != nil {
				fmt.Printf("Failed to send winner email notification: %v\n", err)
			}

			// Notify other bidders they lost
			s.notifyOtherBidders(ctx, auction.ID, result.WinnerBid.UserID, basePayload)
		}

	case AuctionOutcomeReserveNotMet:
		if result.HighestBid != nil {
			// Notify highest bidder that reserve wasn't met
			bidderPayload := make(map[string]interface{})
			for k, v := range basePayload {
				bidderPayload[k] = v
			}
			bidderPayload["highest_bid"] = fmt.Sprintf("%.2f", result.HighestBid.Amount)
			if result.ReservePrice != nil {
				bidderPayload["reserve_price"] = fmt.Sprintf("%.2f", *result.ReservePrice)
			}
			// Lookup bidder email/name
			var hbEmail, hbName string
			_ = s.db.QueryRow(ctx, "SELECT email, name FROM users WHERE id = $1", result.HighestBid.UserID).Scan(&hbEmail, &hbName)
			if hbEmail != "" {
				bidderPayload["email"] = hbEmail
				if hbName != "" { bidderPayload["name"] = hbName }
			}
			
			if _, err := notifications.EnqueueInternal(ctx, result.HighestBid.UserID, "auction_ended_reserve_not_met", bidderPayload); err != nil {
				fmt.Printf("Failed to send reserve not met internal notification: %v\n", err)
			}
			if _, err := notifications.EnqueueEmail(ctx, result.HighestBid.UserID, "auction_ended_reserve_not_met", bidderPayload); err != nil {
				fmt.Printf("Failed to send reserve not met email notification: %v\n", err)
			}

			// Notify other bidders
			s.notifyOtherBidders(ctx, auction.ID, result.HighestBid.UserID, basePayload)
		}

	case AuctionOutcomeNoBids:
		// No specific user notifications needed for no bids scenario
		fmt.Printf("Auction %d ended with no bids\n", auction.ID)
	}

	// Notify auction watchers (users who viewed/watched but didn't bid)
	s.notifyAuctionWatchers(ctx, auction.ID, basePayload)
}

// notifyOtherBidders notifies all bidders except the winner/highest bidder
func (s *ReserveService) notifyOtherBidders(ctx context.Context, auctionID, excludeUserID int64, basePayload map[string]interface{}) {
    query := `
        SELECT DISTINCT b.user_id, u.email, u.name
        FROM bids b
        JOIN users u ON u.id = b.user_id
        WHERE b.auction_id = $1 AND b.user_id != $2`
    
    rows, err := s.db.Query(ctx, query, auctionID, excludeUserID)
    if err != nil {
        fmt.Printf("Failed to get other bidders: %v\n", err)
        return
    }
    defer rows.Close()

    for rows.Next() {
        var userID int64
        var email, name string
        if err := rows.Scan(&userID, &email, &name); err != nil {
            continue
        }

        // Clone payload and add recipient email/name
        payload := make(map[string]interface{})
        for k, v := range basePayload { payload[k] = v }
        if email != "" { payload["email"] = email }
        if name != "" { payload["name"] = name }

        if _, err := notifications.EnqueueInternal(ctx, userID, "auction_ended_lost", payload); err != nil {
            fmt.Printf("Failed to send lost internal notification to user %d: %v\n", userID, err)
        }
        if email != "" {
            if _, err := notifications.EnqueueEmail(ctx, userID, "auction_ended_lost", payload); err != nil {
                fmt.Printf("Failed to send lost email notification to user %d: %v\n", userID, err)
            }
        }
    }
}

// notifyAuctionWatchers notifies users who might be interested in the auction outcome
func (s *ReserveService) notifyAuctionWatchers(ctx context.Context, auctionID int64, basePayload map[string]interface{}) {
	// For now, this is a placeholder - in a full implementation, you might track
	// users who viewed the auction page, added to watchlist, etc.
	// We'll skip this for now to avoid complexity
	fmt.Printf("Auction %d ended - watchers notification placeholder\n", auctionID)
}
