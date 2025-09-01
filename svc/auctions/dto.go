package auctions

import (
	"time"
)

// CreateAuctionDTO represents the data transfer object for creating an auction
type CreateAuctionDTO struct {
	ProductID             int64     `json:"product_id" validate:"required,gt=0"`
	StartPrice            float64   `json:"start_price" validate:"required,gte=0"`
	BidStep               int       `json:"bid_step" validate:"required,gte=1"`
	ReservePrice          *float64  `json:"reserve_price,omitempty" validate:"omitempty,gte=0"`
	StartAt               time.Time `json:"start_at" validate:"required"`
	EndAt                 time.Time `json:"end_at" validate:"required"`
	AntiSnipingMinutes    *int      `json:"anti_sniping_minutes,omitempty" validate:"omitempty,gte=0,lte=60"`
	MaxExtensionsOverride *int      `json:"max_extensions_override,omitempty" validate:"omitempty,gte=0"`
}

// PlaceBidDTO represents the data transfer object for placing a bid
type PlaceBidDTO struct {
	Amount float64 `json:"amount" validate:"required,gt=0"`
}

// CancelAuctionDTO represents the data transfer object for canceling an auction
type CancelAuctionDTO struct {
	Reason string `json:"reason" validate:"required,min=5,max=500"`
}

// RemoveBidDTO represents the data transfer object for removing a bid
type RemoveBidDTO struct {
	Reason string `json:"reason" validate:"required,min=5,max=500"`
}

// AuctionListFiltersDTO represents filters for listing auctions
type AuctionListFiltersDTO struct {
	Status     string `json:"status,omitempty"`
	EndingSoon bool   `json:"ending_soon,omitempty"`
	Query      string `json:"q,omitempty"`
	Page       int    `json:"page,omitempty" validate:"omitempty,gte=1"`
	Limit      int    `json:"limit,omitempty" validate:"omitempty,gte=1,lte=100"`
	Sort       string `json:"sort,omitempty"`
}

// BidListFiltersDTO represents filters for listing bids
type BidListFiltersDTO struct {
	Page  int `json:"page,omitempty" validate:"omitempty,gte=1"`
	Limit int `json:"limit,omitempty" validate:"omitempty,gte=1,lte=100"`
}

// AuctionResponseDTO represents the response data for an auction
type AuctionResponseDTO struct {
	ID                    int64         `json:"id"`
	ProductID             int64         `json:"product_id"`
	ProductTitle          string        `json:"product_title"`
	ProductSlug           string        `json:"product_slug"`
	StartPrice            float64       `json:"start_price"`
	BidStep               int           `json:"bid_step"`
	ReservePrice          *float64      `json:"reserve_price,omitempty"`
	StartAt               time.Time     `json:"start_at"`
	EndAt                 time.Time     `json:"end_at"`
	AntiSnipingMinutes    int           `json:"anti_sniping_minutes"`
	Status                AuctionStatus `json:"status"`
	ExtensionsCount       int           `json:"extensions_count"`
	MaxExtensionsOverride *int          `json:"max_extensions_override,omitempty"`
	CurrentPrice          float64       `json:"current_price"`
	BidsCount             int           `json:"bids_count"`
	HighestBidder         *string       `json:"highest_bidder,omitempty"`
	HighestBidderCity     *string       `json:"highest_bidder_city,omitempty"`
	ReserveMet            *bool         `json:"reserve_met,omitempty"`
	TimeRemaining         *int64        `json:"time_remaining_seconds,omitempty"`
	CreatedAt             time.Time     `json:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
}

// BidResponseDTO represents the response data for a bid
type BidResponseDTO struct {
	ID                 int64     `json:"id"`
	AuctionID          int64     `json:"auction_id"`
	UserID             int64     `json:"user_id"`
	Amount             float64   `json:"amount"`
	BidderNameSnapshot string    `json:"bidder_name"`
	BidderCityName     *string   `json:"bidder_city,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// AuctionListResponseDTO represents the response for auction list
type AuctionListResponseDTO struct {
	Auctions   []*AuctionResponseDTO `json:"auctions"`
	Total      int                   `json:"total"`
	Page       int                   `json:"page"`
	Limit      int                   `json:"limit"`
	TotalPages int                   `json:"total_pages"`
}

// BidListResponseDTO represents the response for bid list
type BidListResponseDTO struct {
	Bids       []*BidResponseDTO `json:"bids"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
	TotalPages int               `json:"total_pages"`
}

// AuctionEventDTO represents real-time auction events
type AuctionEventDTO struct {
	Type      string      `json:"type"`
	AuctionID int64       `json:"auction_id"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// BidPlacedEventDTO represents data for bid_placed event
type BidPlacedEventDTO struct {
	BidID        int64   `json:"bid_id"`
	Amount       float64 `json:"amount"`
	BidderName   string  `json:"bidder_name"`
	BidderCity   *string `json:"bidder_city,omitempty"`
	CurrentPrice float64 `json:"current_price"`
	BidsCount    int     `json:"bids_count"`
}

// OutbidEventDTO represents data for outbid event
type OutbidEventDTO struct {
	NewAmount    float64 `json:"new_amount"`
	NewBidder    string  `json:"new_bidder"`
	CurrentPrice float64 `json:"current_price"`
}

// ExtendedEventDTO represents data for extended event
type ExtendedEventDTO struct {
	OldEndAt        time.Time `json:"old_end_at"`
	NewEndAt        time.Time `json:"new_end_at"`
	ExtensionsCount int       `json:"extensions_count"`
	ExtendedByBidID int64     `json:"extended_by_bid_id"`
}

// EndedEventDTO represents data for ended event
type EndedEventDTO struct {
	WinnerBidID *int64  `json:"winner_bid_id,omitempty"`
	WinnerName  *string `json:"winner_name,omitempty"`
	WinnerCity  *string `json:"winner_city,omitempty"`
	FinalPrice  float64 `json:"final_price"`
	ReserveMet  bool    `json:"reserve_met"`
	BidsCount   int     `json:"bids_count"`
}

// BidRemovedEventDTO represents data for bid_removed event
type BidRemovedEventDTO struct {
	RemovedBidID int64  `json:"removed_bid_id"`
	Reason       string `json:"reason"`
	RemovedBy    string `json:"removed_by"`
}

// PriceRecomputedEventDTO represents data for price_recomputed event
type PriceRecomputedEventDTO struct {
	NewCurrentPrice float64 `json:"new_current_price"`
	ExtensionsCount int     `json:"extensions_count"`
	Reason          string  `json:"reason"`
}

// ErrorResponseDTO represents error response
type ErrorResponseDTO struct {
	Code          string      `json:"code"`
	Message       string      `json:"message"`
	CorrelationID string      `json:"correlation_id,omitempty"`
	Details       interface{} `json:"details,omitempty"`
}

// SuccessResponseDTO represents success response
type SuccessResponseDTO struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ToAuctionResponse converts AuctionWithDetails to AuctionResponseDTO
func ToAuctionResponse(auction *AuctionWithDetails) *AuctionResponseDTO {
	return &AuctionResponseDTO{
		ID:                    auction.ID,
		ProductID:             auction.ProductID,
		ProductTitle:          auction.ProductTitle,
		ProductSlug:           auction.ProductSlug,
		StartPrice:            auction.StartPrice,
		BidStep:               auction.BidStep,
		ReservePrice:          auction.ReservePrice,
		StartAt:               auction.StartAt,
		EndAt:                 auction.EndAt,
		AntiSnipingMinutes:    auction.AntiSnipingMinutes,
		Status:                auction.Status,
		ExtensionsCount:       auction.ExtensionsCount,
		MaxExtensionsOverride: auction.MaxExtensionsOverride,
		CurrentPrice:          auction.CurrentPrice,
		BidsCount:             auction.BidsCount,
		HighestBidder:         auction.HighestBidder,
		HighestBidderCity:     auction.HighestBidderCity,
		ReserveMet:            auction.ReserveMet,
		TimeRemaining:         auction.TimeRemaining,
		CreatedAt:             auction.CreatedAt,
		UpdatedAt:             auction.UpdatedAt,
	}
}

// ToAuctionListResponse converts auction list to AuctionListResponseDTO
func ToAuctionListResponse(auctions []*AuctionWithDetails, total, page, limit int) *AuctionListResponseDTO {
	auctionDTOs := make([]*AuctionResponseDTO, len(auctions))
	for i, auction := range auctions {
		auctionDTOs[i] = ToAuctionResponse(auction)
	}

	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	return &AuctionListResponseDTO{
		Auctions:   auctionDTOs,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}
}

// ToBidResponse converts BidWithDetails to BidResponseDTO
func ToBidResponse(bid *BidWithDetails) *BidResponseDTO {
	return &BidResponseDTO{
		ID:                 bid.ID,
		AuctionID:          bid.AuctionID,
		UserID:             bid.UserID,
		Amount:             bid.Amount,
		BidderNameSnapshot: bid.BidderNameSnapshot,
		BidderCityName:     bid.BidderCityName,
		CreatedAt:          bid.CreatedAt,
	}
}

// ToBidListResponse converts bid list to BidListResponseDTO
func ToBidListResponse(bids []*BidWithDetails, total, page, limit int) *BidListResponseDTO {
	bidDTOs := make([]*BidResponseDTO, len(bids))
	for i, bid := range bids {
		bidDTOs[i] = ToBidResponse(bid)
	}

	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	return &BidListResponseDTO{
		Bids:       bidDTOs,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}
}

// ToCreateAuctionRequest converts CreateAuctionDTO to CreateAuctionRequest
func ToCreateAuctionRequest(dto *CreateAuctionDTO) *CreateAuctionRequest {
	return &CreateAuctionRequest{
		ProductID:             dto.ProductID,
		StartPrice:            dto.StartPrice,
		BidStep:               dto.BidStep,
		ReservePrice:          dto.ReservePrice,
		StartAt:               dto.StartAt,
		EndAt:                 dto.EndAt,
		AntiSnipingMinutes:    dto.AntiSnipingMinutes,
		MaxExtensionsOverride: dto.MaxExtensionsOverride,
	}
}

// ToPlaceBidRequest converts PlaceBidDTO to PlaceBidRequest
func ToPlaceBidRequest(dto *PlaceBidDTO) *PlaceBidRequest {
	return &PlaceBidRequest{
		Amount: dto.Amount,
	}
}

// ToCancelAuctionRequest converts CancelAuctionDTO to CancelAuctionRequest
func ToCancelAuctionRequest(dto *CancelAuctionDTO) *CancelAuctionRequest {
	return &CancelAuctionRequest{
		Reason: dto.Reason,
	}
}

// ToRemoveBidRequest converts RemoveBidDTO to RemoveBidRequest
func ToRemoveBidRequest(dto *RemoveBidDTO) *RemoveBidRequest {
	return &RemoveBidRequest{
		Reason: dto.Reason,
	}
}

// ToAuctionFilters converts AuctionListFiltersDTO to AuctionFilters
func ToAuctionFilters(dto *AuctionListFiltersDTO) *AuctionFilters {
	filters := &AuctionFilters{
		EndingSoon: dto.EndingSoon,
		Query:      dto.Query,
		Page:       dto.Page,
		Limit:      dto.Limit,
		Sort:       dto.Sort,
	}

	if dto.Status != "" {
		status := AuctionStatus(dto.Status)
		filters.Status = &status
	}

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

	return filters
}

// ToBidFilters converts BidListFiltersDTO to BidFilters
func ToBidFilters(dto *BidListFiltersDTO, auctionID int64) *BidFilters {
	filters := &BidFilters{
		AuctionID: auctionID,
		Page:      dto.Page,
		Limit:     dto.Limit,
	}

	// Set defaults
	if filters.Page <= 0 {
		filters.Page = 1
	}
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}

	return filters
}

// GetReserveStatusResponse represents the response for reserve status
type GetReserveStatusResponse struct {
	AuctionID       int64    `json:"auction_id"`
	HasReserve      bool     `json:"has_reserve"`
	ReservePrice    *float64 `json:"reserve_price,omitempty"`
	HighestBid      *float64 `json:"highest_bid,omitempty"`
	ReserveMet      bool     `json:"reserve_met"`
	AmountToReserve *float64 `json:"amount_to_reserve,omitempty"`
}

// ProcessAuctionEndResponse represents the response for auction end processing
type ProcessAuctionEndResponse struct {
	AuctionID    int64           `json:"auction_id"`
	Outcome      AuctionOutcome  `json:"outcome"`
	WinnerBid    *BidResponseDTO `json:"winner_bid,omitempty"`
	HighestBid   *BidResponseDTO `json:"highest_bid,omitempty"`
	ReservePrice *float64        `json:"reserve_price,omitempty"`
	OrderID      *int64          `json:"order_id,omitempty"`
	Message      string          `json:"message"`
	EndedAt      time.Time       `json:"ended_at"`
}

// AuctionResponse represents auction data in API responses
type AuctionResponse struct {
	ID                    int64     `json:"id"`
	ProductID             int64     `json:"product_id"`
	StartPrice            float64   `json:"start_price"`
	BidStep               float64   `json:"bid_step"`
	ReservePrice          *float64  `json:"reserve_price,omitempty"`
	CurrentPrice          *float64  `json:"current_price,omitempty"`
	BidsCount             int       `json:"bids_count"`
	HighestBidder         *string   `json:"highest_bidder,omitempty"`
	ReserveMet            *bool     `json:"reserve_met,omitempty"`
	StartAt               time.Time `json:"start_at"`
	EndAt                 time.Time `json:"end_at"`
	AntiSnipingMinutes    *int      `json:"anti_sniping_minutes,omitempty"`
	Status                string    `json:"status"`
	ExtensionsCount       int       `json:"extensions_count"`
	MaxExtensionsOverride *int      `json:"max_extensions_override,omitempty"`
	TimeRemaining         *int64    `json:"time_remaining,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// AuctionListResponse represents paginated auction list
type AuctionListResponse struct {
	Auctions []*AuctionResponse `json:"auctions"`
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	Limit    int                `json:"limit"`
}

// AuctionDetailResponse represents detailed auction with bids
type AuctionDetailResponse struct {
	Auction       *AuctionResponse       `json:"auction"`
	Bids          []*BidResponse         `json:"bids"`
	BidCount      int                    `json:"bid_count"`
	ReserveStatus *ReserveStatusResponse `json:"reserve_status,omitempty"`
}

// BidResponse represents bid data in API responses
type BidResponse struct {
	ID                 int64     `json:"id"`
	AuctionID          int64     `json:"auction_id"`
	UserID             int64     `json:"user_id"`
	Amount             float64   `json:"amount"`
	BidderNameSnapshot string    `json:"bidder_name"`
	BidderCityName     *string   `json:"bidder_city,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// MarkWinnerUnpaidDTO represents request to mark winner as unpaid
type MarkWinnerUnpaidDTO struct {
	Reason string `json:"reason" validate:"required,min=5,max=500"`
}

// ReserveStatusResponse represents reserve price status response
type ReserveStatusResponse struct {
	HasReserve      bool     `json:"has_reserve"`
	ReserveMet      bool     `json:"reserve_met"`
	AmountToReserve *float64 `json:"amount_to_reserve,omitempty"`
}

// ToSimpleAuctionResponse converts Auction to AuctionResponse
func ToSimpleAuctionResponse(auction *Auction) *AuctionResponse {
	response := &AuctionResponse{
		ID:                    auction.ID,
		ProductID:             auction.ProductID,
		StartPrice:            auction.StartPrice,
		BidStep:               float64(auction.BidStep),
		ReservePrice:          auction.ReservePrice,
		CurrentPrice:          auction.CurrentPrice,
		BidsCount:             auction.BidsCount,
		HighestBidder:         nil,
		ReserveMet:            nil,
		StartAt:               auction.StartAt,
		EndAt:                 auction.EndAt,
		AntiSnipingMinutes:    &auction.AntiSnipingMinutes,
		Status:                string(auction.Status),
		ExtensionsCount:       auction.ExtensionsCount,
		MaxExtensionsOverride: auction.MaxExtensionsOverride,
		TimeRemaining:         auction.TimeRemaining,
		CreatedAt:             auction.CreatedAt,
		UpdatedAt:             auction.UpdatedAt,
	}
	return response
}

// ToRichAuctionResponse converts AuctionWithDetails to AuctionResponse with rich data
func ToRichAuctionResponse(auction *AuctionWithDetails) *AuctionResponse {
	response := &AuctionResponse{
		ID:                    auction.ID,
		ProductID:             auction.ProductID,
		StartPrice:            auction.StartPrice,
		BidStep:               float64(auction.BidStep),
		ReservePrice:          auction.ReservePrice,
		CurrentPrice:          &auction.CurrentPrice,
		BidsCount:             auction.BidsCount,
		HighestBidder:         auction.HighestBidder,
		StartAt:               auction.StartAt,
		EndAt:                 auction.EndAt,
		AntiSnipingMinutes:    &auction.AntiSnipingMinutes,
		Status:                string(auction.Status),
		ExtensionsCount:       auction.ExtensionsCount,
		MaxExtensionsOverride: auction.MaxExtensionsOverride,
		TimeRemaining:         auction.TimeRemaining,
		CreatedAt:             auction.CreatedAt,
		UpdatedAt:             auction.UpdatedAt,
	}
	if auction.ReservePrice != nil {
		rm := auction.CurrentPrice >= *auction.ReservePrice
		response.ReserveMet = &rm
	}
	if response.TimeRemaining == nil {
		now := time.Now().UTC()
		if auction.EndAt.After(now) && string(auction.Status) == string(AuctionStatusLive) {
			secs := int64(auction.EndAt.Sub(now).Seconds())
			response.TimeRemaining = &secs
		}
	}
	return response
}

// ToSimpleBidResponse converts Bid to BidResponse
func ToSimpleBidResponse(bid *Bid) *BidResponse {
	return &BidResponse{
		ID:                 bid.ID,
		AuctionID:          bid.AuctionID,
		UserID:             bid.UserID,
		Amount:             bid.Amount,
		BidderNameSnapshot: bid.BidderNameSnapshot,
		BidderCityName:     nil, // Will be populated from BidWithDetails if available
		CreatedAt:          bid.CreatedAt,
	}
}
