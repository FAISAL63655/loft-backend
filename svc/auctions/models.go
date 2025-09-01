package auctions

import (
	"time"
)

// AuctionStatus represents the status of an auction
type AuctionStatus string

const (
	AuctionStatusDraft        AuctionStatus = "draft"
	AuctionStatusScheduled    AuctionStatus = "scheduled"
	AuctionStatusLive         AuctionStatus = "live"
	AuctionStatusEnded        AuctionStatus = "ended"
	AuctionStatusCancelled    AuctionStatus = "cancelled"
	AuctionStatusWinnerUnpaid AuctionStatus = "winner_unpaid"
)

// Auction represents an auction for a pigeon
type Auction struct {
	ID                    int64         `json:"id"`
	ProductID             int64         `json:"product_id"`
	StartPrice            float64       `json:"start_price"`
	BidStep               int           `json:"bid_step"`
	ReservePrice          *float64      `json:"reserve_price,omitempty"`
	StartAt               time.Time     `json:"start_at"`
	EndAt                 time.Time     `json:"end_at"`
	AntiSnipingMinutes    int           `json:"anti_sniping_minutes"`
	Status                AuctionStatus `json:"status"`
	ExtensionsCount       int           `json:"extensions_count"`
	MaxExtensionsOverride *int          `json:"max_extensions_override,omitempty"`
	// Additional fields for list responses
	CurrentPrice  *float64  `json:"current_price,omitempty"`
	BidsCount     int       `json:"bids_count"`
	TimeRemaining *int64    `json:"time_remaining,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Bid represents a bid on an auction
type Bid struct {
	ID                   int64     `json:"id"`
	AuctionID            int64     `json:"auction_id"`
	UserID               int64     `json:"user_id"`
	Amount               float64   `json:"amount"`
	BidderNameSnapshot   string    `json:"bidder_name_snapshot"`
	BidderCityIDSnapshot *int64    `json:"bidder_city_id_snapshot,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

// AuctionExtension represents an extension of auction end time due to anti-sniping
type AuctionExtension struct {
	ID              int64     `json:"id"`
	AuctionID       int64     `json:"auction_id"`
	ExtendedByBidID int64     `json:"extended_by_bid_id"`
	OldEndAt        time.Time `json:"old_end_at"`
	NewEndAt        time.Time `json:"new_end_at"`
	CreatedAt       time.Time `json:"created_at"`
}

// AuctionWithDetails represents an auction with additional details
type AuctionWithDetails struct {
	Auction
	ProductTitle      string  `json:"product_title"`
	ProductSlug       string  `json:"product_slug"`
	CurrentPrice      float64 `json:"current_price"`
	BidsCount         int     `json:"bids_count"`
	HighestBidder     *string `json:"highest_bidder,omitempty"`
	HighestBidderCity *string `json:"highest_bidder_city,omitempty"`
	ReserveMet        *bool   `json:"reserve_met,omitempty"`
	TimeRemaining     *int64  `json:"time_remaining_seconds,omitempty"` // seconds remaining, null if ended
}

// BidWithDetails represents a bid with additional details
type BidWithDetails struct {
	Bid
	BidderCityName *string `json:"bidder_city_name,omitempty"`
}

// CreateAuctionRequest represents the request to create a new auction
type CreateAuctionRequest struct {
	ProductID             int64     `json:"product_id"`
	StartPrice            float64   `json:"start_price"`
	BidStep               int       `json:"bid_step"`
	ReservePrice          *float64  `json:"reserve_price,omitempty"`
	StartAt               time.Time `json:"start_at"`
	EndAt                 time.Time `json:"end_at"`
	AntiSnipingMinutes    *int      `json:"anti_sniping_minutes,omitempty"`
	MaxExtensionsOverride *int      `json:"max_extensions_override,omitempty"`
}

// PlaceBidRequest represents the request to place a bid
type PlaceBidRequest struct {
	Amount float64 `json:"amount"`
}

// CancelAuctionRequest represents the request to cancel an auction
type CancelAuctionRequest struct {
	Reason string `json:"reason"`
}

// RemoveBidRequest represents the request to remove a bid (admin only)
type RemoveBidRequest struct {
	Reason string `json:"reason"`
}

// AuctionFilters represents filters for auction queries
type AuctionFilters struct {
	Status     *AuctionStatus `json:"status,omitempty"`
	EndingSoon bool           `json:"ending_soon,omitempty"`
	Query      string         `json:"q,omitempty"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	Sort       string         `json:"sort"`
}

// BidFilters represents filters for bid queries
type BidFilters struct {
	AuctionID int64 `json:"auction_id"`
	Page      int   `json:"page"`
	Limit     int   `json:"limit"`
}

// BidPlacedEventData represents data for bid_placed event
type BidPlacedEventData struct {
	BidID        int64   `json:"bid_id"`
	Amount       float64 `json:"amount"`
	BidderName   string  `json:"bidder_name"`
	BidderCity   *string `json:"bidder_city,omitempty"`
	CurrentPrice float64 `json:"current_price"`
	BidsCount    int     `json:"bids_count"`
}

// OutbidEventData represents data for outbid event
type OutbidEventData struct {
	NewAmount    float64 `json:"new_amount"`
	NewBidder    string  `json:"new_bidder"`
	CurrentPrice float64 `json:"current_price"`
}

// ExtendedEventData represents data for extended event
type ExtendedEventData struct {
	OldEndAt        time.Time `json:"old_end_at"`
	NewEndAt        time.Time `json:"new_end_at"`
	ExtensionsCount int       `json:"extensions_count"`
	ExtendedByBidID int64     `json:"extended_by_bid_id"`
}

// EndedEventData represents data for ended event
type EndedEventData struct {
	WinnerBidID *int64  `json:"winner_bid_id,omitempty"`
	WinnerName  *string `json:"winner_name,omitempty"`
	WinnerCity  *string `json:"winner_city,omitempty"`
	FinalPrice  float64 `json:"final_price"`
	ReserveMet  bool    `json:"reserve_met"`
	BidsCount   int     `json:"bids_count"`
}

// BidRemovedEventData represents data for bid_removed event
type BidRemovedEventData struct {
	RemovedBidID int64  `json:"removed_bid_id"`
	Reason       string `json:"reason"`
	RemovedBy    string `json:"removed_by"`
}

// PriceRecomputedEventData represents data for price_recomputed event
type PriceRecomputedEventData struct {
	NewCurrentPrice float64 `json:"new_current_price"`
	ExtensionsCount int     `json:"extensions_count"`
	Reason          string  `json:"reason"`
}

// AuctionOutcome represents the outcome of an ended auction
type AuctionOutcome string

const (
	AuctionOutcomeWinner        AuctionOutcome = "winner"
	AuctionOutcomeNoBids        AuctionOutcome = "no_bids"
	AuctionOutcomeReserveNotMet AuctionOutcome = "reserve_not_met"
)

// AuctionEndResult represents the result of processing an auction end
type AuctionEndResult struct {
	AuctionID    int64           `json:"auction_id"`
	Outcome      AuctionOutcome  `json:"outcome"`
	WinnerBid    *BidWithDetails `json:"winner_bid,omitempty"`
	HighestBid   *BidWithDetails `json:"highest_bid,omitempty"`
	ReservePrice *float64        `json:"reserve_price,omitempty"`
	OrderID      *int64          `json:"order_id,omitempty"`
	Message      string          `json:"message"`
	EndedAt      time.Time       `json:"ended_at"`
}

// ReserveStatus represents the reserve price status of an auction
type ReserveStatus struct {
	AuctionID       int64    `json:"auction_id"`
	HasReserve      bool     `json:"has_reserve"`
	ReservePrice    *float64 `json:"reserve_price,omitempty"`
	HighestBid      *float64 `json:"highest_bid,omitempty"`
	ReserveMet      bool     `json:"reserve_met"`
	AmountToReserve *float64 `json:"amount_to_reserve,omitempty"`
}

// RateLimitStatus represents current rate limiting status for a user
type RateLimitStatus struct {
	UserID              int64     `json:"user_id"`
	BidsPerMinute       int       `json:"bids_per_minute"`
	CurrentBidCount     int       `json:"current_bid_count"`
	BidsRemaining       int       `json:"bids_remaining"`
	PaymentInitPer5Min  int       `json:"payment_init_per_5min"`
	CurrentPaymentCount int       `json:"current_payment_count"`
	PaymentRemaining    int       `json:"payment_remaining"`
	ResetTime           time.Time `json:"reset_time"`
}
