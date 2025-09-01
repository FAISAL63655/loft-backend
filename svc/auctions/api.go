package auctions

import (
	"context"
	"strconv"
	"strings"
	"time"

	"encore.app/pkg/errs"
	"encore.app/pkg/ratelimit"
	authsvc "encore.app/svc/auth"
	"encore.dev/beta/auth"
)

// Service instance for API endpoints
var auctionService *Service

// Admin actions rate limiter: protect sensitive endpoints
var adminActionsRL = ratelimit.NewRateLimiter(ratelimit.RateLimitConfig{MaxAttempts: 20, Window: time.Minute})

// Initialize the service
func init() {
	// Service will be initialized when database is available
}

// GetService returns the auction service instance
func GetService() *Service {
	if auctionService == nil {
		// This will be set when the service is properly initialized
		panic("auction service not initialized")
	}
	return auctionService
}

// SetService sets the auction service instance (for initialization)
func SetService(service *Service) {
	auctionService = service
}

// checkAdminAuth checks if the current user is an admin
func checkAdminAuth() error {
	if _, ok := auth.UserID(); !ok {
		return &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	if d := auth.Data(); d != nil {
		switch v := d.(type) {
		case *authsvc.AuthData:
			if v.Role == "admin" {
				return nil
			}
		case authsvc.AuthData:
			if v.Role == "admin" {
				return nil
			}
		case map[string]interface{}:
			if role, ok := v["role"].(string); ok && role == "admin" {
				return nil
			}
		}
	}
	return &errs.Error{Code: errs.Forbidden, Message: "يتطلب صلاحيات مدير"}
}

// CreateAuction creates a new auction (Admin only)
//
//encore:api auth method=POST path=/auctions
func CreateAuction(ctx context.Context, req *CreateAuctionDTO) (*AuctionResponse, error) {
	// Check admin authorization
	if err := checkAdminAuth(); err != nil {
		return nil, err
	}

	service := GetService()

	// Pre-check: prevent duplicate active auction for the same product to return consistent PRD code
	if active, err := service.repo.HasActiveAuctionForProduct(ctx, req.ProductID); err == nil && active {
		return nil, errs.E(ctx, "AUC_NEW_FORBIDDEN_STATE", "يوجد مزاد نشط بالفعل لهذا المنتج")
	}

	// Convert DTO to internal request
	createReq := &CreateAuctionRequest{
		ProductID:             req.ProductID,
		StartPrice:            req.StartPrice,
		BidStep:               req.BidStep,
		ReservePrice:          req.ReservePrice,
		StartAt:               req.StartAt,
		EndAt:                 req.EndAt,
		AntiSnipingMinutes:    req.AntiSnipingMinutes,
		MaxExtensionsOverride: req.MaxExtensionsOverride,
	}

	auction, err := service.CreateAuction(ctx, createReq)
	if err != nil {
		// Check for unique constraint violation and map to PRD code
		if strings.Contains(err.Error(), "uq_auction_active_product") {
			return nil, errs.E(ctx, "AUC_NEW_FORBIDDEN_STATE", "يوجد مزاد نشط بالفعل لهذا المنتج")
		}
		return nil, err
	}

	return ToSimpleAuctionResponse(auction), nil
}

// CancelAuction cancels a live or scheduled auction (Admin only)
//
//encore:api auth method=POST path=/auctions/:id/cancel
func CancelAuction(ctx context.Context, id string, req *CancelAuctionDTO) (*MessageResponse, error) {
	// Check admin authorization
	if err := checkAdminAuth(); err != nil {
		return nil, err
	}

	// Admin rate limit: cancel-auction actions per admin
	if uid, ok := auth.UserID(); ok {
		if adminID, err := strconv.ParseInt(string(uid), 10, 64); err == nil {
			if err := adminActionsRL.RecordAttempt(ratelimit.GenerateUserKey("admin_cancel_auction", adminID)); err != nil {
				return nil, &errs.Error{Code: errs.TooManyRequests, Message: "تجاوزت حد محاولات إلغاء المزاد. حاول لاحقًا"}
			}
		}
	}

	// Parse auction ID
	auctionID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "معرف المزاد غير صحيح",
		}
	}

	service := GetService()
	err = service.CancelAuction(ctx, auctionID, req.Reason)
	if err != nil {
		return nil, err
	}

	return &MessageResponse{
		Success: true,
		Message: "تم إلغاء المزاد بنجاح",
	}, nil
}

// MarkWinnerUnpaid marks the auction winner as unpaid (Admin only)
//
//encore:api auth method=POST path=/auctions/:id/mark-winner-unpaid
func MarkWinnerUnpaid(ctx context.Context, id string, req *MarkWinnerUnpaidDTO) (*MessageResponse, error) {
	// Check admin authorization
	if err := checkAdminAuth(); err != nil {
		return nil, err
	}

	// Parse auction ID
	auctionID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "معرف المزاد غير صحيح",
		}
	}

	service := GetService()
	err = service.MarkWinnerUnpaid(ctx, auctionID)
	if err != nil {
		return nil, err
	}

	return &MessageResponse{
		Success: true,
		Message: "تم تحديث حالة الفائز بنجاح",
	}, nil
}

// ListAuctions lists auctions with filtering and pagination
//
//encore:api public method=GET path=/auctions
func ListAuctions(ctx context.Context, req *AuctionListFiltersDTO) (*AuctionListResponse, error) {
	service := GetService()

	// Convert DTO to internal filters
	filters := ToAuctionFilters(req)

	// Use GetAuctionWithDetails to get rich auction data
	auctions, total, err := service.repo.ListAuctionsWithDetails(ctx, filters)
	if err != nil {
		return nil, err
	}

	// Convert to response format with rich data
	auctionResponses := make([]*AuctionResponse, len(auctions))
	for i, auction := range auctions {
		auctionResponses[i] = ToRichAuctionResponse(auction)
	}

	return &AuctionListResponse{
		Auctions: auctionResponses,
		Total:    total,
		Page:     filters.Page,
		Limit:    filters.Limit,
	}, nil
}

// GetAuction gets auction details with bid history
//
//encore:api public method=GET path=/auctions/:id
func GetAuction(ctx context.Context, id string) (*AuctionDetailResponse, error) {
	// Parse auction ID
	auctionID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "معرف المزاد غير صحيح",
		}
	}

	service := GetService()

	// Get auction with details (includes current price, bids count, etc.)
	auction, err := service.repo.GetAuctionWithDetails(ctx, auctionID)
	if err != nil {
		return nil, err
	}

	// Get bid history (limit to reasonable number for display)
	bidService := NewBidService(service.db)
	bidFilters := &BidFilters{
		Page:  1,
		Limit: 50, // Reasonable limit for display
	}
	bids, _, err := bidService.GetAuctionBids(ctx, auctionID, bidFilters)
	if err != nil {
		// Don't fail if we can't get bids, just return empty list
		bids = []*BidWithDetails{}
	}

	// Convert bids to response format
	bidResponses := make([]*BidResponse, len(bids))
	for i, bid := range bids {
		bidResponses[i] = ToSimpleBidResponse(&bid.Bid)
	}

	// Get reserve status if applicable
	var reserveStatus *ReserveStatusResponse
	if auction.ReservePrice != nil {
		reserveService := service.reserveService
		status, err := reserveService.GetReserveStatus(ctx, auctionID)
		if err == nil {
			reserveStatus = &ReserveStatusResponse{
				HasReserve:      status.HasReserve,
				ReserveMet:      status.ReserveMet,
				AmountToReserve: status.AmountToReserve,
			}
		}
	}

	return &AuctionDetailResponse{
		Auction:       ToRichAuctionResponse(auction),
		Bids:          bidResponses,
		BidCount:      auction.BidsCount, // Use the count from AuctionWithDetails
		ReserveStatus: reserveStatus,
	}, nil
}

// PlaceBid places a bid on an auction (Verified users only)
//
//encore:api auth method=POST path=/auctions/:id/bid
func PlaceBid(ctx context.Context, id string, req *PlaceBidDTO) (*BidResponse, error) {
	// Check authentication
	userID, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{
			Code:    errs.Unauthenticated,
			Message: "مطلوب تسجيل الدخول",
		}
	}

	// Parse user ID
	userIDInt, err := strconv.ParseInt(string(userID), 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.Internal,
			Message: "خطأ في معرف المستخدم",
		}
	}

	// Check if user has verified role (required for bidding per PRD)
	authData := auth.Data()
	if authData == nil {
		return nil, errs.E(ctx, errs.BidVerifiedRequired, "المزايدة تتطلب دور مفعّل (verified)")
	}

	var userRole string
	switch v := authData.(type) {
	case *authsvc.AuthData:
		userRole = v.Role
	case authsvc.AuthData:
		userRole = v.Role
	case map[string]interface{}:
		if role, ok := v["role"].(string); ok {
			userRole = role
		}
	}

	if userRole != "verified" && userRole != "admin" {
		return nil, errs.E(ctx, errs.BidVerifiedRequired, "المزايدة تتطلب دور مفعّل (verified)")
	}

	// Rate limit: protect bid spamming based on system settings
	if svc := GetService(); svc != nil && svc.rateLimitService != nil {
		if err := svc.rateLimitService.CheckBidRateLimit(ctx, userIDInt); err != nil {
			return nil, err
		}
	}

	// Parse auction ID from path
	auctionID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرف المزاد غير صحيح"}
	}

	service := GetService()
	bidService := NewBidService(service.db)

	bid, err := bidService.PlaceBid(ctx, auctionID, userIDInt, req.Amount)
	if err != nil {
		return nil, err
	}

	return ToSimpleBidResponse(bid), nil
}

// RemoveBid removes a bid (Admin only)
//
//encore:api auth method=POST path=/bids/:id/remove
func RemoveBid(ctx context.Context, id string, req *RemoveBidDTO) (*RemoveBidResponse, error) {
	// Check admin authorization
	if err := checkAdminAuth(); err != nil {
		return nil, err
	}

	// Admin rate limit: remove-bid actions per admin
	if uid, ok := auth.UserID(); ok {
		if adminID, err := strconv.ParseInt(string(uid), 10, 64); err == nil {
			if err := adminActionsRL.RecordAttempt(ratelimit.GenerateUserKey("admin_remove_bid", adminID)); err != nil {
				return nil, &errs.Error{Code: errs.TooManyRequests, Message: "تجاوزت حد محاولات إزالة المزايدة. حاول لاحقًا"}
			}
		}
	}

	// Parse bid ID
	bidID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "معرف المزايدة غير صحيح",
		}
	}

	service := GetService()
	bidMgmtService := service.bidMgmtService

	// Get admin name from auth context
	adminName := "admin" // Default fallback
	if authData := auth.Data(); authData != nil {
		if dataMap, ok := authData.(map[string]interface{}); ok {
			if name, exists := dataMap["name"]; exists {
				if nameStr, ok := name.(string); ok && nameStr != "" {
					adminName = nameStr
				}
			}
		}
	}

	response, err := bidMgmtService.RemoveBid(ctx, bidID, req.Reason, adminName)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// GetRateLimitStatus gets current rate limit status for the user
//
//encore:api auth method=GET path=/user/auction-rate-limit
func GetRateLimitStatus(ctx context.Context) (*RateLimitStatus, error) {
	// Check authentication
	userID, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{
			Code:    errs.Unauthenticated,
			Message: "مطلوب تسجيل الدخول",
		}
	}

	// Parse user ID
	userIDInt, err := strconv.ParseInt(string(userID), 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.Internal,
			Message: "خطأ في معرف المستخدم",
		}
	}

	service := GetService()
	rateLimitService := service.rateLimitService

	status, err := rateLimitService.GetRateLimitStatus(ctx, userIDInt)
	if err != nil {
		return nil, err
	}

	return status, nil
}

// GetReserveStatus gets reserve price status for an auction
//
//encore:api public method=GET path=/auctions/:id/reserve-status
func GetReserveStatus(ctx context.Context, id string) (*ReserveStatusResponse, error) {
	// Parse auction ID
	auctionID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "معرف المزاد غير صحيح",
		}
	}

	service := GetService()
	reserveService := service.reserveService

	status, err := reserveService.GetReserveStatus(ctx, auctionID)
	if err != nil {
		return nil, err
	}

	return &ReserveStatusResponse{
		HasReserve:      status.HasReserve,
		ReserveMet:      status.ReserveMet,
		AmountToReserve: status.AmountToReserve,
	}, nil
}

// ProcessAuctionEnd processes the end of an auction (Internal/Admin)
//
//encore:api auth method=POST path=/auctions/:id/process-end
func ProcessAuctionEnd(ctx context.Context, id string) (*ProcessAuctionEndResponse, error) {
	// Check admin authorization
	if err := checkAdminAuth(); err != nil {
		return nil, err
	}

	// Parse auction ID
	auctionID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "معرف المزاد غير صحيح",
		}
	}

	service := GetService()
	reserveService := service.reserveService

	result, err := reserveService.ProcessAuctionEnd(ctx, auctionID)
	if err != nil {
		return nil, err
	}

	// Convert to response format
	response := &ProcessAuctionEndResponse{
		AuctionID: result.AuctionID,
		Outcome:   result.Outcome,
		Message:   result.Message,
		EndedAt:   result.EndedAt,
	}

	if result.WinnerBid != nil {
		response.WinnerBid = ToBidResponse(result.WinnerBid)
	}

	if result.OrderID != nil {
		response.OrderID = result.OrderID
	}

	return response, nil
}

// Supporting response types

// MessageResponse represents a simple message response
type MessageResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
