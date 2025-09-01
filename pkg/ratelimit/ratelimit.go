// Package ratelimit provides rate limiting functionality for authentication endpoints
package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// Rate limiting errors
var (
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	ErrInvalidKey        = errors.New("invalid rate limit key")
)

// RateLimitConfig defines the configuration for rate limiting
type RateLimitConfig struct {
	MaxAttempts int           `json:"max_attempts"`
	Window      time.Duration `json:"window"`
	BlockTime   time.Duration `json:"block_time,omitempty"` // Optional blocking time after limit exceeded
}

// AttemptRecord tracks attempts for a specific key
type AttemptRecord struct {
	Key       string     `json:"key"`
	Count     int        `json:"count"`
	FirstSeen time.Time  `json:"first_seen"`
	LastSeen  time.Time  `json:"last_seen"`
	BlockedAt *time.Time `json:"blocked_at,omitempty"`
}

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	config         RateLimitConfig
	storage        Storage
	cleanupTicker  *time.Ticker
	cleanupStop    chan struct{}
	cleanupEnabled bool
}

// NewRateLimiter creates a new rate limiter with the given configuration using memory storage
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	return NewRateLimiterWithStorage(config, NewMemoryStorage())
}

// NewRateLimiterWithStorage creates a new rate limiter with custom storage backend
func NewRateLimiterWithStorage(config RateLimitConfig, storage Storage) *RateLimiter {
	rl := &RateLimiter{
		config:         config,
		storage:        storage,
		cleanupStop:    make(chan struct{}),
		cleanupEnabled: false,
	}

	return rl
}

// EnableAutoCleanup starts automatic cleanup of expired records
func (rl *RateLimiter) EnableAutoCleanup(interval time.Duration) {
	if rl.cleanupEnabled {
		return
	}

	rl.cleanupEnabled = true
	rl.cleanupTicker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-rl.cleanupTicker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := rl.storage.CleanupExpired(ctx, rl.config.Window); err != nil {
					log.Printf("RateLimiter storage error in CleanupExpired: %v", err)
				}
				cancel()
			case <-rl.cleanupStop:
				return
			}
		}
	}()
}

// hashSensitiveKey creates a hash of sensitive key for safe logging
// This prevents exposure of IPs, emails, or user IDs in log files
func hashSensitiveKey(key string) string {
	if key == "" {
		return "empty-key"
	}

	// Create SHA256 hash
	hash := sha256.Sum256([]byte(key))

	// Return first 8 characters of hex for readability
	// This provides enough uniqueness for debugging while maintaining privacy
	return hex.EncodeToString(hash[:4])
}

// DisableAutoCleanup stops automatic cleanup
func (rl *RateLimiter) DisableAutoCleanup() {
	if !rl.cleanupEnabled {
		return
	}

	rl.cleanupEnabled = false
	if rl.cleanupTicker != nil {
		rl.cleanupTicker.Stop()
	}
	close(rl.cleanupStop)
	rl.cleanupStop = make(chan struct{})
}

// IsAllowed checks if the request is allowed based on the rate limit
func (rl *RateLimiter) IsAllowed(key string) bool {
	if key == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC()
	record, err := rl.storage.GetRecord(ctx, key)
	if err != nil {
		// Log storage error for monitoring and debugging
		log.Printf("RateLimiter storage error in GetRecord for key hash %s: %v", hashSensitiveKey(key), err)
		// On storage error, allow the request to avoid blocking legitimate users
		// In production, you might want to handle this differently based on your requirements
		return true
	}

	if record == nil {
		// First attempt for this key
		newRecord := &AttemptRecord{
			Key:       key,
			Count:     1,
			FirstSeen: now,
			LastSeen:  now,
		}
		if err := rl.storage.SetRecord(ctx, key, newRecord); err != nil {
			log.Printf("RateLimiter storage error in SetRecord for key hash %s: %v", hashSensitiveKey(key), err)
		}
		return true
	}

	// Check if blocked and block time hasn't expired
	if record.BlockedAt != nil && rl.config.BlockTime > 0 {
		if now.Sub(*record.BlockedAt) < rl.config.BlockTime {
			return false
		}
		// Block time expired, reset the record
		record.BlockedAt = nil
		record.Count = 0
		record.FirstSeen = now
	}

	// Check if window has expired
	if now.Sub(record.FirstSeen) >= rl.config.Window {
		// Reset the window
		record.Count = 1
		record.FirstSeen = now
		record.LastSeen = now
		if err := rl.storage.SetRecord(ctx, key, record); err != nil {
			log.Printf("RateLimiter storage error in SetRecord for key hash %s: %v", hashSensitiveKey(key), err)
		}
		return true
	}

	// Within the window, check if limit exceeded
	if record.Count >= rl.config.MaxAttempts {
		// Block the key if block time is configured
		if rl.config.BlockTime > 0 && record.BlockedAt == nil {
			record.BlockedAt = &now
		}
		if err := rl.storage.SetRecord(ctx, key, record); err != nil {
			log.Printf("RateLimiter storage error in SetRecord for key hash %s: %v", hashSensitiveKey(key), err)
		}
		return false
	}

	// Increment count and allow
	record.Count++
	record.LastSeen = now
	if err := rl.storage.SetRecord(ctx, key, record); err != nil {
		log.Printf("RateLimiter storage error in SetRecord for key %s: %v", key, err)
	}
	return true
}

// RecordAttempt records an attempt for the given key and returns if it's allowed
func (rl *RateLimiter) RecordAttempt(key string) error {
	if !rl.IsAllowed(key) {
		return ErrRateLimitExceeded
	}
	return nil
}

// GetRemainingAttempts returns the number of remaining attempts for the key
func (rl *RateLimiter) GetRemainingAttempts(key string) int {
	if key == "" {
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	record, err := rl.storage.GetRecord(ctx, key)
	if err != nil || record == nil {
		return rl.config.MaxAttempts
	}

	now := time.Now().UTC()

	// Check if blocked
	if record.BlockedAt != nil && rl.config.BlockTime > 0 {
		if now.Sub(*record.BlockedAt) < rl.config.BlockTime {
			return 0
		}
	}

	// Check if window expired
	if now.Sub(record.FirstSeen) >= rl.config.Window {
		return rl.config.MaxAttempts
	}

	remaining := rl.config.MaxAttempts - record.Count
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetTimeUntilReset returns the time until the rate limit resets for the key
func (rl *RateLimiter) GetTimeUntilReset(key string) time.Duration {
	if key == "" {
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	record, err := rl.storage.GetRecord(ctx, key)
	if err != nil || record == nil {
		return 0
	}

	now := time.Now().UTC()

	// If blocked, return time until block expires
	if record.BlockedAt != nil && rl.config.BlockTime > 0 {
		blockExpiry := record.BlockedAt.Add(rl.config.BlockTime)
		if now.Before(blockExpiry) {
			return blockExpiry.Sub(now)
		}
	}

	// Return time until window expires
	windowExpiry := record.FirstSeen.Add(rl.config.Window)
	if now.Before(windowExpiry) {
		return windowExpiry.Sub(now)
	}

	return 0
}

// IsBlocked checks if the key is currently blocked
func (rl *RateLimiter) IsBlocked(key string) bool {
	if key == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	record, err := rl.storage.GetRecord(ctx, key)
	if err != nil || record == nil {
		return false
	}

	if record.BlockedAt == nil || rl.config.BlockTime == 0 {
		return false
	}

	now := time.Now().UTC()
	return now.Sub(*record.BlockedAt) < rl.config.BlockTime
}

// GetBlockTimeRemaining returns the remaining block time for the key
func (rl *RateLimiter) GetBlockTimeRemaining(key string) time.Duration {
	if key == "" {
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	record, err := rl.storage.GetRecord(ctx, key)
	if err != nil || record == nil || record.BlockedAt == nil || rl.config.BlockTime == 0 {
		return 0
	}

	now := time.Now().UTC()
	blockExpiry := record.BlockedAt.Add(rl.config.BlockTime)

	if now.Before(blockExpiry) {
		return blockExpiry.Sub(now)
	}

	return 0
}

// Reset clears the rate limit for the given key
func (rl *RateLimiter) Reset(key string) {
	if key == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rl.storage.DeleteRecord(ctx, key); err != nil {
		log.Printf("RateLimiter storage error in DeleteRecord for key hash %s: %v", hashSensitiveKey(key), err)
	}
}

// ResetAll clears all rate limits
func (rl *RateLimiter) ResetAll() {
	// For memory storage, we can close and recreate
	if memStorage, ok := rl.storage.(*MemoryStorage); ok {
		_ = memStorage.Close()
		rl.storage = NewMemoryStorage()
	} else {
		// For other storage types, we would need a ClearAll method
		// This is a limitation that could be addressed by adding ClearAll to Storage interface
	}
}

// GetAttemptInfo returns information about attempts for the key
func (rl *RateLimiter) GetAttemptInfo(key string) (*AttemptRecord, bool) {
	if key == "" {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	record, err := rl.storage.GetRecord(ctx, key)
	if err != nil || record == nil {
		return nil, false
	}

	// Storage already returns a copy, so we can return it directly
	return record, true
}

// CleanupExpiredRecords removes expired records to prevent memory leaks
func (rl *RateLimiter) CleanupExpiredRecords() int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use storage's cleanup method
	err := rl.storage.CleanupExpired(ctx, rl.config.Window)
	if err != nil {
		return 0
	}

	// For memory storage, we can get the count from stats
	stats, err := rl.storage.GetStats(ctx)
	if err != nil {
		return 0
	}

	// Return total records as an approximation
	if totalRecords, ok := stats["total_records"].(int); ok {
		return totalRecords
	}

	return 0
}

// GetStats returns statistics about the rate limiter
func (rl *RateLimiter) GetStats() map[string]interface{} {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	storageStats, err := rl.storage.GetStats(ctx)
	if err != nil {
		// Return basic config stats on error
		return map[string]interface{}{
			"max_attempts": rl.config.MaxAttempts,
			"window":       rl.config.Window.String(),
			"block_time":   rl.config.BlockTime.String(),
			"error":        err.Error(),
		}
	}

	// Add configuration to storage stats
	storageStats["max_attempts"] = rl.config.MaxAttempts
	storageStats["window"] = rl.config.Window.String()
	storageStats["block_time"] = rl.config.BlockTime.String()
	storageStats["cleanup_enabled"] = rl.cleanupEnabled

	return storageStats
}

// Close closes the rate limiter and cleans up resources
func (rl *RateLimiter) Close() error {
	// Stop auto cleanup if enabled
	rl.DisableAutoCleanup()

	// Close storage
	if rl.storage != nil {
		return rl.storage.Close()
	}

	return nil
}

// Predefined rate limiter configurations for common use cases
var (
	// Production Rate Limits (default)

	// LoginRateLimit: 10 attempts per 10 minutes
	LoginRateLimit = RateLimitConfig{
		MaxAttempts: 10,
		Window:      10 * time.Minute,
		BlockTime:   5 * time.Minute, // Block for 5 minutes after limit exceeded
	}

	// RegistrationRateLimit: 5 attempts per hour
	RegistrationRateLimit = RateLimitConfig{
		MaxAttempts: 5,
		Window:      time.Hour,
		BlockTime:   30 * time.Minute,
	}

	// EmailVerificationRateLimit: 3 attempts per hour
	EmailVerificationRateLimit = RateLimitConfig{
		MaxAttempts: 3,
		Window:      time.Hour,
		BlockTime:   15 * time.Minute,
	}

	// PasswordResetRateLimit: 5 attempts per hour
	PasswordResetRateLimit = RateLimitConfig{
		MaxAttempts: 5,
		Window:      time.Hour,
		BlockTime:   30 * time.Minute,
	}

	// Development Rate Limits (faster for testing)

	// LoginRateLimitDev: 10 attempts per 10 seconds (for development/testing)
	LoginRateLimitDev = RateLimitConfig{
		MaxAttempts: 10,
		Window:      10 * time.Second,
		BlockTime:   30 * time.Second, // Short block for dev
	}

	// RegistrationRateLimitDev: 5 attempts per 5 minutes
	RegistrationRateLimitDev = RateLimitConfig{
		MaxAttempts: 5,
		Window:      5 * time.Minute,
		BlockTime:   2 * time.Minute,
	}

	// EmailVerificationRateLimitDev: 3 attempts per 5 minutes
	EmailVerificationRateLimitDev = RateLimitConfig{
		MaxAttempts: 3,
		Window:      5 * time.Minute,
		BlockTime:   1 * time.Minute,
	}

	// PasswordResetRateLimitDev: 5 attempts per 10 minutes
	PasswordResetRateLimitDev = RateLimitConfig{
		MaxAttempts: 5,
		Window:      10 * time.Minute,
		BlockTime:   2 * time.Minute,
	}
)

// GetRateLimitConfig returns appropriate rate limit config based on environment
func GetRateLimitConfig(configType string, isDevelopment bool) RateLimitConfig {
	switch configType {
	case "login":
		if isDevelopment {
			return LoginRateLimitDev
		}
		return LoginRateLimit

	case "registration":
		if isDevelopment {
			return RegistrationRateLimitDev
		}
		return RegistrationRateLimit

	case "email_verification":
		if isDevelopment {
			return EmailVerificationRateLimitDev
		}
		return EmailVerificationRateLimit

	case "password_reset":
		if isDevelopment {
			return PasswordResetRateLimitDev
		}
		return PasswordResetRateLimit

	default:
		// Default to login rate limit
		if isDevelopment {
			return LoginRateLimitDev
		}
		return LoginRateLimit
	}
}

// GenerateKey generates a rate limit key based on the provided components
// Uses base64 encoding to prevent key collision when components contain ":"
func GenerateKey(components ...string) string {
	if len(components) == 0 {
		return ""
	}

	// Encode each component to prevent collision
	encodedComponents := make([]string, len(components))
	for i, component := range components {
		encodedComponents[i] = base64.URLEncoding.EncodeToString([]byte(component))
	}

	return strings.Join(encodedComponents, ":")
}

// GenerateSimpleKey generates a simple key without encoding (for backward compatibility)
func GenerateSimpleKey(components ...string) string {
	if len(components) == 0 {
		return ""
	}

	return strings.Join(components, ":")
}

// GenerateIPKey generates a rate limit key for IP-based limiting
func GenerateIPKey(action, ip string) string {
	return GenerateKey("ip", action, ip)
}

// GenerateUserKey generates a rate limit key for user-based limiting
func GenerateUserKey(action string, userID int64) string {
	return GenerateKey("user", action, fmt.Sprintf("%d", userID))
}

// GenerateEmailKey generates a rate limit key for email-based limiting
func GenerateEmailKey(action, email string) string {
	return GenerateKey("email", action, email)
}
