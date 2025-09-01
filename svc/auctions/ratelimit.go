package auctions

import (
	"context"
	"fmt"
	"sync"
	"time"

	"encore.app/pkg/errs"
	"encore.dev/storage/sqldb"
)

// RateLimitService handles rate limiting for auction operations
type RateLimitService struct {
	db    *sqldb.Database
	cache map[string]*UserRateLimit
	mu    sync.RWMutex
}

// UserRateLimit tracks rate limiting for a user
type UserRateLimit struct {
	UserID    int64
	BidCount  int
	LastReset time.Time
	mu        sync.Mutex
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	BidsPerMinute        int
	PaymentInitPer5Min   int
	WSConnectionsPerHost int
	WSMessagesPerMinute  int
}

// NewRateLimitService creates a new rate limit service
func NewRateLimitService(db *sqldb.Database) *RateLimitService {
	service := &RateLimitService{
		db:    db,
		cache: make(map[string]*UserRateLimit),
	}

	// Start cleanup goroutine
	go service.cleanupExpiredEntries()

	return service
}

// CheckBidRateLimit checks if user can place a bid
func (s *RateLimitService) CheckBidRateLimit(ctx context.Context, userID int64) error {
	config, err := s.getRateLimitConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get rate limit config: %w", err)
	}

	// Use database-based rate limiting for accuracy
	return s.checkDatabaseRateLimit(ctx, userID, config.BidsPerMinute, "bids")
}

// CheckPaymentInitRateLimit checks payment initialization rate limit
func (s *RateLimitService) CheckPaymentInitRateLimit(ctx context.Context, userID int64) error {
	config, err := s.getRateLimitConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get rate limit config: %w", err)
	}

	// Check 5 requests per 5 minutes
	var requestCount int
	query := `
		SELECT COUNT(*) 
		FROM payment_sessions 
		WHERE user_id = $1 AND created_at > NOW() - INTERVAL '5 minutes'`

	err = s.db.QueryRow(ctx, query, userID).Scan(&requestCount)
	if err != nil {
		return fmt.Errorf("failed to check payment rate limit: %w", err)
	}

	if requestCount >= config.PaymentInitPer5Min {
		return &errs.Error{
			Code:    errs.TooManyRequests,
			Message: "تجاوزت الحد المسموح لطلبات الدفع",
		}
	}

	return nil
}

// CheckWebSocketRateLimit checks WebSocket connection rate limit
func (s *RateLimitService) CheckWebSocketRateLimit(ctx context.Context, clientIP string) error {
	config, err := s.getRateLimitConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get rate limit config: %w", err)
	}

	s.mu.RLock()
	key := fmt.Sprintf("ws_conn_%s", clientIP)
	userLimit, exists := s.cache[key]
	s.mu.RUnlock()

	if !exists {
		s.mu.Lock()
		userLimit = &UserRateLimit{
			UserID:    0, // IP-based, not user-based
			BidCount:  1,
			LastReset: time.Now(),
		}
		s.cache[key] = userLimit
		s.mu.Unlock()
		return nil
	}

	userLimit.mu.Lock()
	defer userLimit.mu.Unlock()

	// Reset counter if more than 1 minute has passed
	if time.Since(userLimit.LastReset) > time.Minute {
		userLimit.BidCount = 1
		userLimit.LastReset = time.Now()
		return nil
	}

	if userLimit.BidCount >= config.WSConnectionsPerHost {
		return &errs.Error{
			Code:    errs.TooManyRequests,
			Message: "تجاوزت الحد المسموح للاتصالات المتزامنة",
		}
	}

	userLimit.BidCount++
	return nil
}

// RecordBidAttempt records a bid attempt for rate limiting
func (s *RateLimitService) RecordBidAttempt(ctx context.Context, userID int64) error {
	// This is handled automatically by the database when a bid is inserted
	// But we can use this for additional tracking if needed
	return nil
}

// GetRateLimitStatus returns current rate limit status for a user
func (s *RateLimitService) GetRateLimitStatus(ctx context.Context, userID int64) (*RateLimitStatus, error) {
	config, err := s.getRateLimitConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit config: %w", err)
	}

	// Get current bid count in last minute
	var bidCount int
	query := `
		SELECT COUNT(*) 
		FROM bids 
		WHERE user_id = $1 AND created_at > NOW() - INTERVAL '1 minute'`

	err = s.db.QueryRow(ctx, query, userID).Scan(&bidCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get bid count: %w", err)
	}

	// Get payment init count in last 5 minutes
	var paymentCount int
	paymentQuery := `
		SELECT COUNT(*) 
		FROM payment_sessions 
		WHERE user_id = $1 AND created_at > NOW() - INTERVAL '5 minutes'`

	err = s.db.QueryRow(ctx, paymentQuery, userID).Scan(&paymentCount)
	if err != nil {
		// Payment sessions table might not exist yet, ignore error
		paymentCount = 0
	}

	status := &RateLimitStatus{
		UserID:              userID,
		BidsPerMinute:       config.BidsPerMinute,
		CurrentBidCount:     bidCount,
		BidsRemaining:       max(0, config.BidsPerMinute-bidCount),
		PaymentInitPer5Min:  config.PaymentInitPer5Min,
		CurrentPaymentCount: paymentCount,
		PaymentRemaining:    max(0, config.PaymentInitPer5Min-paymentCount),
		ResetTime:           time.Now().Add(time.Minute),
	}

	return status, nil
}

// Private methods

func (s *RateLimitService) getRateLimitConfig(ctx context.Context) (*RateLimitConfig, error) {
	config := &RateLimitConfig{
		BidsPerMinute:        60, // Default values
		PaymentInitPer5Min:   5,
		WSConnectionsPerHost: 120,
		WSMessagesPerMinute:  30,
	}

	// Get values from system settings
	settings := map[string]*int{
		"bids.rate_limit_per_minute":   &config.BidsPerMinute,
		"payments.rate_limit_per_5min": &config.PaymentInitPer5Min,
		"ws.max_connections_per_host":  &config.WSConnectionsPerHost,
		"ws.msgs_per_minute":           &config.WSMessagesPerMinute,
	}

	for key, target := range settings {
		var valueStr string
		query := `SELECT value FROM system_settings WHERE key = $1`
		err := s.db.QueryRow(ctx, query, key).Scan(&valueStr)
		if err == nil {
			var value int
			if _, err := fmt.Sscanf(valueStr, "%d", &value); err == nil {
				*target = value
			}
		}
	}

	return config, nil
}

func (s *RateLimitService) checkDatabaseRateLimit(ctx context.Context, userID int64, limit int, operation string) error {
	var count int
	var query string

	switch operation {
	case "bids":
		query = `
			SELECT COUNT(*) 
			FROM bids 
			WHERE user_id = $1 AND created_at > NOW() - INTERVAL '1 minute'`
	default:
		return fmt.Errorf("unknown operation: %s", operation)
	}

	err := s.db.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check rate limit: %w", err)
	}

	if count >= limit {
		return &errs.Error{
			Code:    errs.TooManyRequests,
			Message: "تجاوزت الحد المسموح للطلبات",
		}
	}

	return nil
}

func (s *RateLimitService) cleanupExpiredEntries() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for key, userLimit := range s.cache {
				userLimit.mu.Lock()
				if now.Sub(userLimit.LastReset) > 10*time.Minute {
					delete(s.cache, key)
				}
				userLimit.mu.Unlock()
			}
			s.mu.Unlock()
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
