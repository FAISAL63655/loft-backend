package auctions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"encore.app/pkg/audit"
	"encore.app/pkg/config"
	"encore.app/pkg/errs"
	"encore.app/svc/notifications"
	"encore.app/svc/orders/order_mgmt"
	"encore.dev/storage/sqldb"
)

// Service handles auction business logic
type Service struct {
	db               *sqldb.Database
	repo             *Repository
	reserveService   *ReserveService
	rateLimitService *RateLimitService
	bidMgmtService   *BidManagementService
}

// NewService creates a new auction service
func NewService(db *sqldb.Database) *Service {
	// Initialize realtime service
	InitRealtimeService(db)

	service := &Service{
		db:               db,
		repo:             NewRepository(db),
		reserveService:   NewReserveService(db),
		rateLimitService: NewRateLimitService(db),
		bidMgmtService:   NewBidManagementService(db),
	}

	// Set the service instance for API endpoints
	SetService(service)

	return service
}

// CreateAuction creates a new auction (Admin only)
func (s *Service) CreateAuction(ctx context.Context, req *CreateAuctionRequest) (*Auction, error) {
	// Validate that the product exists and is available
	if err := s.validateProductForAuction(ctx, req.ProductID); err != nil {
		return nil, err
	}

	// Validate auction parameters
	if err := s.validateAuctionRequest(req); err != nil {
		return nil, err
	}

	// Get minimum bid step from system settings
	minBidStep, err := s.getMinBidStep(ctx)
	if err != nil {
		return nil, err
	}

	if req.BidStep < minBidStep {
		return nil, errs.E(ctx, "AUC_BID_STEP_TOO_LOW", fmt.Sprintf("خطوة المزايدة يجب أن تكون على الأقل %d", minBidStep))
	}

	// Validate reserve price
	if err := s.reserveService.ValidateReservePrice(req.ReservePrice, req.StartPrice); err != nil {
		return nil, err
	}

	// Set default anti-sniping minutes if not provided (read from system settings, treated as minutes)
	antiSnipingMinutes := 10
	if gm := config.GetGlobalManager(); gm != nil {
		if settings := gm.GetSettings(); settings != nil {
			if settings.AuctionsAntiSnipingMinutes > 0 {
				antiSnipingMinutes = settings.AuctionsAntiSnipingMinutes
			}
		}
	}
	if req.AntiSnipingMinutes != nil {
		antiSnipingMinutes = *req.AntiSnipingMinutes
	}

	auction := &Auction{
		ProductID:             req.ProductID,
		StartPrice:            req.StartPrice,
		BidStep:               req.BidStep,
		ReservePrice:          req.ReservePrice,
		StartAt:               req.StartAt,
		EndAt:                 req.EndAt,
		AntiSnipingMinutes:    antiSnipingMinutes,
		Status:                AuctionStatusDraft,
		ExtensionsCount:       0,
		MaxExtensionsOverride: req.MaxExtensionsOverride,
	}

	// Determine initial status
	now := time.Now().UTC()
	var needsProductUpdate bool
	if auction.StartAt.After(now) {
		auction.Status = AuctionStatusScheduled
	} else if auction.EndAt.After(now) {
		auction.Status = AuctionStatusLive
		needsProductUpdate = true // Mark that we need to update product status after auction creation
	} else {
		return nil, errs.E(ctx, "AUC_INVALID_TIME_WINDOW", "لا يمكن إنشاء مزاد منتهي الصلاحية")
	}

	// Create the auction first
	createdAuction, err := s.repo.CreateAuction(ctx, auction)
	if err != nil {
		// Check for unique constraint violation (active auction for same product)
		if strings.Contains(err.Error(), "uq_auction_active_product") {
			return nil, errs.E(ctx, "AUC_NEW_FORBIDDEN_STATE", "يوجد مزاد نشط بالفعل لهذا المنتج")
		}
		return nil, errs.EDetails(ctx, "AUC_CREATE_FAILED", "فشل إنشاء المزاد", map[string]any{"product_id": req.ProductID})
	}

	// Now update product status if auction is live (after auction exists)
	if needsProductUpdate {
		if err := s.updateProductStatus(ctx, req.ProductID, "in_auction"); err != nil {
			// If product update fails, we should rollback the auction creation
			// For now, log the error but don't fail the auction creation
			fmt.Printf("Warning: Failed to update product status to in_auction: %v\n", err)
		}
	}

	// Send audit notification
	s.sendAuditNotification(ctx, "AUC.CREATED", createdAuction.ID, map[string]interface{}{
		"product_id":  createdAuction.ProductID,
		"start_price": createdAuction.StartPrice,
		"status":      createdAuction.Status,
		"start_at":    createdAuction.StartAt,
		"end_at":      createdAuction.EndAt,
	})

	return createdAuction, nil
}

// GetAuction retrieves an auction by ID with details
func (s *Service) GetAuction(ctx context.Context, auctionID int64) (*AuctionWithDetails, error) {
	auction, err := s.repo.GetAuctionWithDetails(ctx, auctionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &errs.Error{
				Code:    errs.NotFound,
				Message: "المزاد غير موجود",
			}
		}
		return nil, fmt.Errorf("failed to get auction: %w", err)
	}

	// Calculate time remaining if auction is live
	if auction.Status == AuctionStatusLive {
		remaining := auction.EndAt.Sub(time.Now().UTC())
		if remaining > 0 {
			seconds := int64(remaining.Seconds())
			auction.TimeRemaining = &seconds
		}
	}

	// Check if reserve price is met (if reserve exists)
	if auction.ReservePrice != nil {
		met := auction.CurrentPrice >= *auction.ReservePrice
		auction.ReserveMet = &met
	}

	return auction, nil
}

// ListAuctions retrieves auctions with filters
func (s *Service) ListAuctions(ctx context.Context, filters *AuctionFilters) ([]*AuctionWithDetails, int, error) {
	// Set defaults
	if filters.Page <= 0 {
		filters.Page = 1
	}
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}
	if filters.Sort == "" {
		filters.Sort = "ending_soon"
	}

	auctions, total, err := s.repo.ListAuctionsWithDetails(ctx, filters)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list auctions: %w", err)
	}

	// Calculate time remaining for live auctions
	now := time.Now().UTC()
	for _, auction := range auctions {
		if auction.Status == AuctionStatusLive {
			remaining := auction.EndAt.Sub(now)
			if remaining > 0 {
				seconds := int64(remaining.Seconds())
				auction.TimeRemaining = &seconds
			}
		}

		// Check if reserve price is met
		if auction.ReservePrice != nil {
			met := auction.CurrentPrice >= *auction.ReservePrice
			auction.ReserveMet = &met
		}
	}

	return auctions, total, nil
}

// CancelAuction cancels an auction (Admin only)
func (s *Service) CancelAuction(ctx context.Context, auctionID int64, reason string) error {
	// Get auction
	auction, err := s.repo.GetAuction(ctx, auctionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &errs.Error{
				Code:    errs.NotFound,
				Message: "المزاد غير موجود",
			}
		}
		return fmt.Errorf("failed to get auction: %w", err)
	}

	// Check if auction can be cancelled
	if auction.Status != AuctionStatusScheduled && auction.Status != AuctionStatusLive {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "لا يمكن إلغاء هذا المزاد في حالته الحالية",
		}
	}

	// Start transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Update auction status
	if err := s.repo.UpdateAuctionStatus(ctx, tx, auctionID, AuctionStatusCancelled); err != nil {
		return fmt.Errorf("failed to update auction status: %w", err)
	}

	// Update product status back to available
	if err := s.updateProductStatusTx(ctx, tx, auction.ProductID, "available"); err != nil {
		return fmt.Errorf("failed to update product status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Audit log
	_, _ = audit.LogAction(ctx, s.db, "AUC.CANCELLED", "auction", fmt.Sprint(auctionID), map[string]interface{}{
		"reason":     reason,
		"product_id": auction.ProductID,
	}, audit.InferActorFromAuth())

	// Send notifications to bidders
	go s.sendAuctionCancellationNotifications(ctx, auction, reason)

	// Broadcast cancellation event
	realtimeService := GetRealtimeService()
	if realtimeService != nil {
		// Use BroadcastEnded with custom result for cancellation
		cancelResult := &AuctionEndResult{
			AuctionID: auctionID,
			Outcome:   "cancelled",
			Message:   "تم إلغاء المزاد",
			EndedAt:   time.Now().UTC(),
		}
		if err := realtimeService.BroadcastEnded(ctx, auctionID, cancelResult); err != nil {
			fmt.Printf("Failed to broadcast auction cancellation: %v\n", err)
		}
	}

	return nil
}

// MarkWinnerUnpaid marks auction winner as unpaid (Admin only)
func (s *Service) MarkWinnerUnpaid(ctx context.Context, auctionID int64) error {
	// Get auction
	auction, err := s.repo.GetAuction(ctx, auctionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &errs.Error{
				Code:    errs.NotFound,
				Message: "المزاد غير موجود",
			}
		}
		return fmt.Errorf("failed to get auction: %w", err)
	}

	// Check if auction is ended
	if auction.Status != AuctionStatusEnded {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "يمكن وسم الفائز كغير مسدد فقط للمزادات المنتهية",
		}
	}

	// Start transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Update auction status
	if err := s.repo.UpdateAuctionStatus(ctx, tx, auctionID, AuctionStatusWinnerUnpaid); err != nil {
		return fmt.Errorf("failed to update auction status: %w", err)
	}

	// Update product status from auction_hold back to available
	if err := s.updateProductStatusTx(ctx, tx, auction.ProductID, "available"); err != nil {
		return fmt.Errorf("failed to update product status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Audit log
	_, _ = audit.LogAction(ctx, s.db, "AUC.WINNER_UNPAID", "auction", fmt.Sprint(auctionID), map[string]interface{}{
		"product_id": auction.ProductID,
	}, audit.InferActorFromAuth())

	// Send notification to winner
	go s.sendWinnerUnpaidNotification(ctx, auction)

	// Broadcast event
	realtimeService := GetRealtimeService()
	if realtimeService != nil {
		// Create custom event for winner unpaid status
		unpaidResult := &AuctionEndResult{
			AuctionID: auctionID,
			Outcome:   "winner_unpaid",
			Message:   "الفائز لم يدفع في الوقت المحدد",
			EndedAt:   time.Now().UTC(),
		}
		if err := realtimeService.BroadcastEnded(ctx, auctionID, unpaidResult); err != nil {
			fmt.Printf("Failed to broadcast winner unpaid event: %v\n", err)
		}
	}

	return nil
}
//
//encore:api private
func TickAuctions(ctx context.Context) error {
    s := GetService()
    fmt.Printf("[AUCTION_TICK] Tick started at %s\n", time.Now().UTC().Format(time.RFC3339))

    // Prevent overlapping ticks using a Postgres advisory lock
    // This helps avoid "Step is still running" and ensures only one tick runs at a time.
    var got bool
    if err := s.db.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", int64(424242)).Scan(&got); err != nil {
        fmt.Printf("[AUCTION_TICK] failed to acquire advisory lock: %v\n", err)
        return err
    }
    if !got {
        fmt.Printf("[AUCTION_TICK] skipped: another tick is running\n")
        return nil
    }
    defer func() {
        _, _ = s.db.Exec(ctx, "SELECT pg_advisory_unlock($1)", int64(424242))
    }()

	// Close ended auctions
	    toClose, err := s.repo.GetAuctionsToClose(ctx)
    if err == nil {
        fmt.Printf("[AUCTION_TICK] toClose=%d\n", len(toClose))
        for _, a := range toClose {
			// Re-fetch details to get current price and reserve
			det, derr := s.repo.GetAuctionWithDetails(ctx, a.ID)
			if derr != nil {
				_ = s.CancelAuction(ctx, a.ID, "auto_close_time_elapsed")
				continue
			}
			// Decide winner: at least one bid and reserve met (if reserve set)
			winnerExists := det.BidsCount > 0
			reserveOk := det.ReservePrice == nil || det.CurrentPrice >= *det.ReservePrice
			if winnerExists && reserveOk {
				// Mark auction ended and product hold
				tx, txerr := s.db.Begin(ctx)
				if txerr != nil {
					continue
				}
				_ = s.repo.UpdateAuctionStatus(ctx, tx, a.ID, AuctionStatusEnded)
				_ = s.updateProductStatusTx(ctx, tx, det.ProductID, "auction_hold")
				_ = tx.Commit()
				fmt.Printf("[AUCTION_TICK] Closed auction %d with winner\n", a.ID)
				// Lookup winner user id (last highest bid)
				var winnerUserID int64
				_ = s.db.QueryRow(ctx, `SELECT user_id FROM bids WHERE auction_id=$1 ORDER BY amount DESC, created_at DESC LIMIT 1`, det.ID).Scan(&winnerUserID)
				if winnerUserID != 0 {
					orderResp, orderErr := order_mgmt.CreateAuctionWinnerOrder(ctx, &order_mgmt.CreateAuctionWinnerParams{
						AuctionID:          det.ID,
						ProductID:          det.ProductID,
						WinnerUserID:       winnerUserID,
						WinningAmountGross: det.CurrentPrice,
					})

					// Send notifications to winner (internal + email)
					var productTitle string
					_ = s.db.QueryRow(ctx, `SELECT title FROM products WHERE id=$1`, det.ProductID).Scan(&productTitle)
					if productTitle == "" { productTitle = fmt.Sprintf("المزاد #%d", det.ID) }
					var email, name string
					_ = s.db.QueryRow(ctx, `SELECT email, name FROM users WHERE id=$1`, winnerUserID).Scan(&email, &name)
					
					// Get invoice number
					var invoiceNumber string
					if orderErr == nil && orderResp != nil {
						_ = s.db.QueryRow(ctx, `SELECT number FROM invoices WHERE id=$1`, orderResp.InvoiceID).Scan(&invoiceNumber)
					}
					
					payload := map[string]interface{}{
						"auction_id":     fmt.Sprint(det.ID),
						"product_title":  productTitle,
						"outcome":        "winner",
						"message":        "انتهى المزاد بفوزك - يرجى الدفع خلال 48 ساعة",
						"winning_amount": fmt.Sprintf("%.2f", det.CurrentPrice),
						"is_winner":      true,
						"language":       "ar",
					}
					if orderErr == nil && orderResp != nil {
						payload["order_id"] = fmt.Sprint(orderResp.OrderID)
						payload["invoice_id"] = fmt.Sprint(orderResp.InvoiceID)
						payload["invoice_number"] = invoiceNumber
						payload["payment_url"] = fmt.Sprintf("https://dughairiloft.com/checkout/%d", orderResp.OrderID)
					}
					if email != "" { 
						payload["email"] = email 
						payload["name"] = name
						payload["Name"] = name // For template compatibility
					}
					if name != "" { payload["name"] = name }
					_, _ = notifications.EnqueueInternal(ctx, winnerUserID, "auction_ended_winner", payload)
					if email != "" { _, _ = notifications.EnqueueEmail(ctx, winnerUserID, "auction_ended_winner", payload) }
				}
			} else {
				// No winner: end auction (not cancelled) and return product to available
				reason := "reserve_not_met"
				if !winnerExists {
					reason = "no_bids"
				}
				tx, txerr := s.db.Begin(ctx)
				if txerr != nil {
					continue
				}
				_ = s.repo.UpdateAuctionStatus(ctx, tx, a.ID, AuctionStatusEnded)
				_ = s.updateProductStatusTx(ctx, tx, det.ProductID, "available")
				_ = tx.Commit()
				fmt.Printf("[AUCTION_TICK] Closed auction %d without winner (%s)\n", a.ID, reason)
				// Audit end without winner
				s.sendAuditNotification(ctx, "AUC.ENDED_NO_WINNER", a.ID, map[string]interface{}{
					"product_id":  det.ProductID,
					"reason":      reason,
					"reserve_met": reserveOk,
					"bids_count":  det.BidsCount,
				})

				// If there were bids but reserve not met, notify highest bidder
				if winnerExists && !reserveOk {
					var hbUserID int64
					var hbAmount float64
					_ = s.db.QueryRow(ctx, `SELECT user_id, amount FROM bids WHERE auction_id=$1 ORDER BY amount DESC, created_at DESC LIMIT 1`, det.ID).Scan(&hbUserID, &hbAmount)
					if hbUserID != 0 {
						var productTitle string
						_ = s.db.QueryRow(ctx, `SELECT title FROM products WHERE id=$1`, det.ProductID).Scan(&productTitle)
						if productTitle == "" { productTitle = fmt.Sprintf("المزاد #%d", det.ID) }
						var email, name string
						_ = s.db.QueryRow(ctx, `SELECT email, name FROM users WHERE id=$1`, hbUserID).Scan(&email, &name)
						payload := map[string]interface{}{
							"auction_id":    fmt.Sprint(det.ID),
							"product_title": productTitle,
							"outcome":       "reserve_not_met",
							"message":       "لم يتحقق سعر الاحتياطي",
							"highest_bid":   fmt.Sprintf("%.2f", hbAmount),
							"language":      "ar",
						}
						if email != "" { payload["email"] = email }
						if name != "" { payload["name"] = name }
						_, _ = notifications.EnqueueInternal(ctx, hbUserID, "auction_ended_reserve_not_met", payload)
						if email != "" { _, _ = notifications.EnqueueEmail(ctx, hbUserID, "auction_ended_reserve_not_met", payload) }
					}
				}
			}
		}
	}

	// Start scheduled auctions whose start_at has passed
	    toStart, err := s.repo.GetAuctionsToStart(ctx)
    if err == nil {
        fmt.Printf("[AUCTION_TICK] toStart=%d\n", len(toStart))
        for _, a := range toStart {
			// Flip to live and mark product in_auction atomically
			tx, txerr := s.db.Begin(ctx)
			if txerr != nil {
				continue
			}
			// Update auction status
			_ = s.repo.UpdateAuctionStatus(ctx, tx, a.ID, AuctionStatusLive)
			// Update product status
			_ = s.updateProductStatusTx(ctx, tx, a.ProductID, "in_auction")
			_ = tx.Commit()
			fmt.Printf("[AUCTION_TICK] Started auction %d (product %d)\n", a.ID, a.ProductID)

			// Audit start event (best-effort)
			s.sendAuditNotification(ctx, "AUC.STARTED", a.ID, map[string]interface{}{
				"product_id": a.ProductID,
				"start_at":   a.StartAt,
				"end_at":     a.EndAt,
			})
		}
	}

	return nil
}

// validateProductForAuction validates that a product can be auctioned
func (s *Service) validateProductForAuction(ctx context.Context, productID int64) error {
	var productType, status string
	query := `SELECT type, status FROM products WHERE id = $1`
	err := s.db.QueryRow(ctx, query, productID).Scan(&productType, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.E(ctx, "AUC_PRODUCT_NOT_FOUND", "المنتج غير موجود")
		}
		return errs.E(ctx, "AUC_PRODUCT_READ_FAILED", "فشل الحصول على المنتج")
	}

	// Only pigeons can be auctioned
	if productType != "pigeon" {
		return errs.E(ctx, "AUC_TYPE_NOT_PIGEON", "يمكن مزاد الحمام فقط")
	}

	// Product must be available
	if status != "available" {
		return errs.E(ctx, "AUC_PRODUCT_NOT_AVAILABLE", "المنتج غير متاح للمزاد")
	}

	return nil
}

// validateAuctionRequest validates auction creation request
func (s *Service) validateAuctionRequest(req *CreateAuctionRequest) error {
	if req.StartPrice < 0 {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "سعر البداية يجب أن يكون أكبر من أو يساوي الصفر",
		}
	}

	if req.BidStep < 1 {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "خطوة المزايدة يجب أن تكون أكبر من الصفر",
		}
	}

	if req.ReservePrice != nil && *req.ReservePrice < req.StartPrice {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "سعر الاحتياطي يجب أن يكون أكبر من أو يساوي سعر البداية",
		}
	}

	if !req.EndAt.After(req.StartAt) {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "وقت انتهاء المزاد يجب أن يكون بعد وقت البداية",
		}
	}

	if req.AntiSnipingMinutes != nil && (*req.AntiSnipingMinutes < 0 || *req.AntiSnipingMinutes > 60) {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "دقائق Anti-Sniping يجب أن تكون بين 0 و 60",
		}
	}

	if req.MaxExtensionsOverride != nil && *req.MaxExtensionsOverride < 0 {
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "الحد الأقصى للتمديدات يجب أن يكون أكبر من أو يساوي الصفر",
		}
	}

	return nil
}

// getMinBidStep gets minimum bid step from system settings
func (s *Service) getMinBidStep(ctx context.Context) (int, error) {
	var value string
	query := `SELECT value FROM system_settings WHERE key = 'bids.min_step_global'`
	err := s.db.QueryRow(ctx, query).Scan(&value)
	if err != nil {
		return 5, nil // default fallback
	}

	var minStep int
	if _, err := fmt.Sscanf(value, "%d", &minStep); err != nil {
		return 5, nil // default fallback
	}

	return minStep, nil
}

// updateProductStatus updates product status
func (s *Service) updateProductStatus(ctx context.Context, productID int64, status string) error {
	query := `UPDATE products SET status = $1, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id = $2`
	_, err := s.db.Exec(ctx, query, status, productID)
	return err
}

// updateProductStatusTx updates product status within a transaction
func (s *Service) updateProductStatusTx(ctx context.Context, tx *sqldb.Tx, productID int64, status string) error {
	query := `UPDATE products SET status = $1, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id = $2`
	_, err := tx.Exec(ctx, query, status, productID)
	return err
}

// sendAuditNotification sends an audit notification for auction events
func (s *Service) sendAuditNotification(ctx context.Context, eventType string, auctionID int64, details map[string]interface{}) {
	// Enhanced audit notification with proper logging and optional admin notification
	fmt.Printf("[AUDIT] %s - Auction %d: %+v\n", eventType, auctionID, details)
	
	// For critical events, also send notification to administrators
	criticalEvents := map[string]bool{
		"AUC.CREATED":         true,
		"AUC.CANCELLED":       true,
		"AUC.ENDED_NO_WINNER": true,
		"AUC.WINNER_UNPAID":   true,
	}
	
	if criticalEvents[eventType] {
		// Send notification to admin users (role = 'admin')
		go s.sendAdminAuditNotification(ctx, eventType, auctionID, details)
	}
}

// sendAuctionCancellationNotifications sends notifications when an auction is cancelled
func (s *Service) sendAuctionCancellationNotifications(ctx context.Context, auction *Auction, reason string) {
	// Get product title
	var productTitle string
	if err := s.db.QueryRow(ctx, "SELECT title FROM products WHERE id = $1", auction.ProductID).Scan(&productTitle); err != nil {
		productTitle = fmt.Sprintf("المزاد #%d", auction.ID)
	}

	basePayload := map[string]interface{}{
		"auction_id":    fmt.Sprint(auction.ID),
		"product_title": productTitle,
		"reason":        reason,
		"message":       "تم إلغاء المزاد",
		"language":      "ar",
	}

	// Get all bidders for this auction
	query := `SELECT DISTINCT user_id FROM bids WHERE auction_id = $1`
	rows, err := s.db.Query(ctx, query, auction.ID)
	if err != nil {
		fmt.Printf("Failed to get bidders for cancellation notification: %v\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			continue
		}

		// Send notifications to each bidder
		if _, err := notifications.EnqueueInternal(ctx, userID, "auction_cancelled", basePayload); err != nil {
			fmt.Printf("Failed to send cancellation internal notification to user %d: %v\n", userID, err)
		}
		if _, err := notifications.EnqueueEmail(ctx, userID, "auction_cancelled", basePayload); err != nil {
			fmt.Printf("Failed to send cancellation email notification to user %d: %v\n", userID, err)
		}
	}
}

// sendWinnerUnpaidNotification sends notification when winner fails to pay
func (s *Service) sendWinnerUnpaidNotification(ctx context.Context, auction *Auction) {
	// Get winner bid details
	query := `
		SELECT user_id, amount, bidder_name_snapshot 
		FROM bids 
		WHERE auction_id = $1 
		ORDER BY amount DESC, created_at DESC 
		LIMIT 1`
	
	var winnerUserID int64
	var winningAmount float64
	var winnerName string
	
	err := s.db.QueryRow(ctx, query, auction.ID).Scan(&winnerUserID, &winningAmount, &winnerName)
	if err != nil {
		fmt.Printf("Failed to get winner details for unpaid notification: %v\n", err)
		return
	}

	// Get product title
	var productTitle string
	if err := s.db.QueryRow(ctx, "SELECT title FROM products WHERE id = $1", auction.ProductID).Scan(&productTitle); err != nil {
		productTitle = fmt.Sprintf("المزاد #%d", auction.ID)
	}

	payload := map[string]interface{}{
		"auction_id":     fmt.Sprint(auction.ID),
		"product_title":  productTitle,
		"winning_amount": fmt.Sprintf("%.2f", winningAmount),
		"winner_name":    winnerName,
		"message":        "انتهت مهلة الدفع وتم إعادة المنتج للبيع",
		"language":       "ar",
	}

	// Send notification to the unpaid winner
	if _, err := notifications.EnqueueInternal(ctx, winnerUserID, "auction_winner_unpaid", payload); err != nil {
		fmt.Printf("Failed to send unpaid winner internal notification: %v\n", err)
	}
	if _, err := notifications.EnqueueEmail(ctx, winnerUserID, "auction_winner_unpaid", payload); err != nil {
		fmt.Printf("Failed to send unpaid winner email notification: %v\n", err)
	}
}

// sendAdminAuditNotification sends audit notifications to admin users
func (s *Service) sendAdminAuditNotification(ctx context.Context, eventType string, auctionID int64, details map[string]interface{}) {
	// Get all admin users
	query := `SELECT id FROM users WHERE role = 'admin' AND state = 'active'`
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		fmt.Printf("Failed to get admin users for audit notification: %v\n", err)
		return
	}
	defer rows.Close()

	payload := map[string]interface{}{
		"event_type":  eventType,
		"auction_id":  fmt.Sprint(auctionID),
		"details":     details,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"language":    "ar",
	}

	for rows.Next() {
		var adminUserID int64
		if err := rows.Scan(&adminUserID); err != nil {
			continue
		}

		// Send internal notification only (no email spam for audit events)
		if _, err := notifications.EnqueueInternal(ctx, adminUserID, "auction_audit_event", payload); err != nil {
			fmt.Printf("Failed to send audit notification to admin %d: %v\n", adminUserID, err)
		}
	}
}
