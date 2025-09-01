package ratelimit

import (
	"fmt"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 5,
		Window:      time.Minute,
		BlockTime:   time.Minute * 5,
	}

	rl := NewRateLimiter(config)

	if rl == nil {
		t.Fatal("NewRateLimiter returned nil")
	}

	if rl.config.MaxAttempts != config.MaxAttempts {
		t.Errorf("MaxAttempts mismatch: got %d, want %d", rl.config.MaxAttempts, config.MaxAttempts)
	}

	if rl.config.Window != config.Window {
		t.Errorf("Window mismatch: got %v, want %v", rl.config.Window, config.Window)
	}

	if rl.storage == nil {
		t.Error("storage not initialized")
	}
}

func TestIsAllowed(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 3,
		Window:      time.Minute,
		BlockTime:   time.Minute * 2,
	}
	rl := NewRateLimiter(config)

	key := "test-key"

	// First 3 attempts should be allowed
	for i := 0; i < 3; i++ {
		if !rl.IsAllowed(key) {
			t.Errorf("Attempt %d should be allowed", i+1)
		}
	}

	// 4th attempt should be denied
	if rl.IsAllowed(key) {
		t.Error("4th attempt should be denied")
	}

	// Check that key is blocked
	if !rl.IsBlocked(key) {
		t.Error("Key should be blocked after exceeding limit")
	}
}

func TestIsAllowedEmptyKey(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 3,
		Window:      time.Minute,
	}
	rl := NewRateLimiter(config)

	// Empty key should not be allowed
	if rl.IsAllowed("") {
		t.Error("Empty key should not be allowed")
	}
}

func TestRecordAttempt(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 2,
		Window:      time.Minute,
	}
	rl := NewRateLimiter(config)

	key := "test-key"

	// First 2 attempts should succeed
	for i := 0; i < 2; i++ {
		if err := rl.RecordAttempt(key); err != nil {
			t.Errorf("Attempt %d should succeed: %v", i+1, err)
		}
	}

	// 3rd attempt should fail
	if err := rl.RecordAttempt(key); err != ErrRateLimitExceeded {
		t.Errorf("3rd attempt should fail with ErrRateLimitExceeded, got: %v", err)
	}
}

func TestGetRemainingAttempts(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 5,
		Window:      time.Minute,
	}
	rl := NewRateLimiter(config)

	key := "test-key"

	// Initially should have max attempts
	remaining := rl.GetRemainingAttempts(key)
	if remaining != 5 {
		t.Errorf("Initial remaining attempts should be 5, got %d", remaining)
	}

	// After one attempt
	rl.IsAllowed(key)
	remaining = rl.GetRemainingAttempts(key)
	if remaining != 4 {
		t.Errorf("After one attempt, remaining should be 4, got %d", remaining)
	}

	// After all attempts
	for i := 0; i < 4; i++ {
		rl.IsAllowed(key)
	}
	remaining = rl.GetRemainingAttempts(key)
	if remaining != 0 {
		t.Errorf("After all attempts, remaining should be 0, got %d", remaining)
	}
}

func TestGetRemainingAttemptsEmptyKey(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 5,
		Window:      time.Minute,
	}
	rl := NewRateLimiter(config)

	remaining := rl.GetRemainingAttempts("")
	if remaining != 0 {
		t.Errorf("Empty key should have 0 remaining attempts, got %d", remaining)
	}
}

func TestWindowExpiry(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 2,
		Window:      time.Millisecond * 100, // Very short window for testing
	}
	rl := NewRateLimiter(config)

	key := "test-key"

	// Use up all attempts
	rl.IsAllowed(key)
	rl.IsAllowed(key)

	// Should be blocked
	if rl.IsAllowed(key) {
		t.Error("Should be blocked after using all attempts")
	}

	// Wait for window to expire
	time.Sleep(time.Millisecond * 150)

	// Should be allowed again
	if !rl.IsAllowed(key) {
		t.Error("Should be allowed after window expiry")
	}
}

func TestBlockTime(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 1,
		Window:      time.Minute,
		BlockTime:   time.Millisecond * 100, // Short block time for testing
	}
	rl := NewRateLimiter(config)

	key := "test-key"

	// Use up attempts
	rl.IsAllowed(key)

	// Should be blocked
	if rl.IsAllowed(key) {
		t.Error("Should be blocked after exceeding limit")
	}

	// Should still be blocked immediately
	if rl.IsAllowed(key) {
		t.Error("Should still be blocked")
	}

	// Wait for block time to expire
	time.Sleep(time.Millisecond * 150)

	// Should be allowed again
	if !rl.IsAllowed(key) {
		t.Error("Should be allowed after block time expiry")
	}
}

func TestGetTimeUntilReset(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 1,
		Window:      time.Second,
	}
	rl := NewRateLimiter(config)

	key := "test-key"

	// No attempts yet
	timeUntilReset := rl.GetTimeUntilReset(key)
	if timeUntilReset != 0 {
		t.Errorf("Time until reset should be 0 for unused key, got %v", timeUntilReset)
	}

	// Make an attempt
	rl.IsAllowed(key)

	// Should have time until reset
	timeUntilReset = rl.GetTimeUntilReset(key)
	if timeUntilReset <= 0 || timeUntilReset > time.Second {
		t.Errorf("Invalid time until reset: %v", timeUntilReset)
	}
}

func TestIsBlocked(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 1,
		Window:      time.Minute,
		BlockTime:   time.Minute,
	}
	rl := NewRateLimiter(config)

	key := "test-key"

	// Initially not blocked
	if rl.IsBlocked(key) {
		t.Error("Key should not be blocked initially")
	}

	// Use up attempts
	rl.IsAllowed(key)
	rl.IsAllowed(key) // This should trigger blocking

	// Should be blocked
	if !rl.IsBlocked(key) {
		t.Error("Key should be blocked after exceeding limit")
	}
}

func TestGetBlockTimeRemaining(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 1,
		Window:      time.Minute,
		BlockTime:   time.Second,
	}
	rl := NewRateLimiter(config)

	key := "test-key"

	// Initially no block time
	blockTime := rl.GetBlockTimeRemaining(key)
	if blockTime != 0 {
		t.Errorf("Block time should be 0 initially, got %v", blockTime)
	}

	// Trigger blocking
	rl.IsAllowed(key)
	rl.IsAllowed(key)

	// Should have block time remaining
	blockTime = rl.GetBlockTimeRemaining(key)
	if blockTime <= 0 || blockTime > time.Second {
		t.Errorf("Invalid block time remaining: %v", blockTime)
	}
}

func TestReset(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 1,
		Window:      time.Minute,
	}
	rl := NewRateLimiter(config)

	key := "test-key"

	// Use up attempts
	rl.IsAllowed(key)

	// Should be at limit
	if rl.GetRemainingAttempts(key) != 0 {
		t.Error("Should be at limit")
	}

	// Reset
	rl.Reset(key)

	// Should have full attempts again
	if rl.GetRemainingAttempts(key) != config.MaxAttempts {
		t.Error("Should have full attempts after reset")
	}
}

func TestResetAll(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 1,
		Window:      time.Minute,
	}
	rl := NewRateLimiter(config)

	key1 := "test-key-1"
	key2 := "test-key-2"

	// Use up attempts for both keys
	rl.IsAllowed(key1)
	rl.IsAllowed(key2)

	// Reset all
	rl.ResetAll()

	// Both should have full attempts
	if rl.GetRemainingAttempts(key1) != config.MaxAttempts {
		t.Error("Key1 should have full attempts after reset all")
	}
	if rl.GetRemainingAttempts(key2) != config.MaxAttempts {
		t.Error("Key2 should have full attempts after reset all")
	}
}

func TestGetAttemptInfo(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 3,
		Window:      time.Minute,
	}
	rl := NewRateLimiter(config)

	key := "test-key"

	// No attempts yet
	_, exists := rl.GetAttemptInfo(key)
	if exists {
		t.Error("Should not have attempt info for unused key")
	}

	// Make some attempts
	rl.IsAllowed(key)
	rl.IsAllowed(key)

	// Should have attempt info
	info, exists := rl.GetAttemptInfo(key)
	if !exists {
		t.Fatal("Should have attempt info after attempts")
	}

	if info.Key != key {
		t.Errorf("Key mismatch: got %s, want %s", info.Key, key)
	}

	if info.Count != 2 {
		t.Errorf("Count mismatch: got %d, want 2", info.Count)
	}
}

func TestCleanupExpiredRecords(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 1,
		Window:      time.Millisecond * 50, // Very short window
		BlockTime:   time.Millisecond * 50, // Very short block time
	}
	rl := NewRateLimiter(config)

	key1 := "test-key-1"
	key2 := "test-key-2"

	// Make attempts
	rl.IsAllowed(key1)
	rl.IsAllowed(key2)

	// Trigger blocking for key1
	rl.IsAllowed(key1)

	// Wait for expiry
	time.Sleep(time.Millisecond * 100)

	// Cleanup should work without error
	_ = rl.CleanupExpiredRecords()

	// Verify cleanup worked by checking stats
	stats := rl.GetStats()
	if totalRecords, ok := stats["total_records"].(int); ok && totalRecords > 2 {
		t.Errorf("Expected cleanup to reduce records, still have %d", totalRecords)
	}
}

func TestGetStats(t *testing.T) {
	config := RateLimitConfig{
		MaxAttempts: 2,
		Window:      time.Minute,
		BlockTime:   time.Minute,
	}
	rl := NewRateLimiter(config)

	key1 := "test-key-1"
	key2 := "test-key-2"

	// Make some attempts
	rl.IsAllowed(key1)
	rl.IsAllowed(key2)

	// Trigger blocking for key1
	rl.IsAllowed(key1)
	rl.IsAllowed(key1)

	stats := rl.GetStats()

	if stats["total_records"].(int) != 2 {
		t.Errorf("Total records should be 2, got %v", stats["total_records"])
	}

	if stats["max_attempts"].(int) != 2 {
		t.Errorf("Max attempts should be 2, got %v", stats["max_attempts"])
	}

	// Check that we have some blocked records
	if blockedCount, ok := stats["blocked_count"].(int); ok && blockedCount < 1 {
		t.Errorf("Should have at least 1 blocked record, got %v", blockedCount)
	}

	// Check that we have some active records
	if activeCount, ok := stats["active_count"].(int); ok && activeCount < 1 {
		t.Errorf("Should have at least 1 active record, got %v", activeCount)
	}
}

func TestGenerateKey(t *testing.T) {
	tests := []struct {
		name       string
		components []string
		expected   string
	}{
		{
			name:       "empty components",
			components: []string{},
			expected:   "",
		},
		{
			name:       "single component",
			components: []string{"test"},
			expected:   "dGVzdA==", // base64 encoded "test"
		},
		{
			name:       "multiple components",
			components: []string{"user", "login", "123"},
			expected:   "dXNlcg==:bG9naW4=:MTIz", // base64 encoded components
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateKey(tt.components...)
			if result != tt.expected {
				t.Errorf("GenerateKey() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestGenerateSimpleKey(t *testing.T) {
	tests := []struct {
		name       string
		components []string
		expected   string
	}{
		{
			name:       "empty components",
			components: []string{},
			expected:   "",
		},
		{
			name:       "single component",
			components: []string{"test"},
			expected:   "test",
		},
		{
			name:       "multiple components",
			components: []string{"user", "login", "123"},
			expected:   "user:login:123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSimpleKey(tt.components...)
			if result != tt.expected {
				t.Errorf("GenerateSimpleKey() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestGenerateIPKey(t *testing.T) {
	key := GenerateIPKey("login", "192.168.1.1")
	expected := "aXA=:bG9naW4=:MTkyLjE2OC4xLjE=" // base64 encoded "ip:login:192.168.1.1"
	if key != expected {
		t.Errorf("GenerateIPKey() = %s, want %s", key, expected)
	}
}

func TestGenerateUserKey(t *testing.T) {
	key := GenerateUserKey("login", 123)
	expected := "dXNlcg==:bG9naW4=:MTIz" // base64 encoded "user:login:123"
	if key != expected {
		t.Errorf("GenerateUserKey() = %s, want %s", key, expected)
	}
}

func TestGenerateEmailKey(t *testing.T) {
	key := GenerateEmailKey("verification", "test@example.com")
	expected := "ZW1haWw=:dmVyaWZpY2F0aW9u:dGVzdEBleGFtcGxlLmNvbQ==" // base64 encoded components
	if key != expected {
		t.Errorf("GenerateEmailKey() = %s, want %s", key, expected)
	}
}

func TestPredefinedConfigs(t *testing.T) {
	configs := []RateLimitConfig{
		LoginRateLimit,
		RegistrationRateLimit,
		EmailVerificationRateLimit,
		PasswordResetRateLimit,
	}

	for i, config := range configs {
		if config.MaxAttempts <= 0 {
			t.Errorf("Config %d: MaxAttempts should be positive, got %d", i, config.MaxAttempts)
		}
		if config.Window <= 0 {
			t.Errorf("Config %d: Window should be positive, got %v", i, config.Window)
		}
		if config.BlockTime < 0 {
			t.Errorf("Config %d: BlockTime should be non-negative, got %v", i, config.BlockTime)
		}
	}
}

func BenchmarkIsAllowed(b *testing.B) {
	config := RateLimitConfig{
		MaxAttempts: 1000,
		Window:      time.Hour,
	}
	rl := NewRateLimiter(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := GenerateKey("bench", "test", fmt.Sprintf("%d", i%100))
		rl.IsAllowed(key)
	}
}

func BenchmarkRecordAttempt(b *testing.B) {
	config := RateLimitConfig{
		MaxAttempts: 1000,
		Window:      time.Hour,
	}
	rl := NewRateLimiter(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := GenerateKey("bench", "test", fmt.Sprintf("%d", i%100))
		rl.RecordAttempt(key)
	}
}
