// Package ratelimit provides rate limiting functionality for authentication endpoints
package ratelimit

import (
	"net/http"
	"strconv"
	"time"

	"encore.app/pkg/httpx"
)

// RateLimitMiddleware creates middleware that adds rate limiting headers
func RateLimitMiddleware(rateLimiter *RateLimiter, keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if key == "" {
				// If no key can be generated, proceed without rate limiting
				next.ServeHTTP(w, r)
				return
			}

			// Check if request is allowed
			allowed := rateLimiter.IsAllowed(key)

			// Get current attempt info for headers
			remaining := rateLimiter.GetRemainingAttempts(key)

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rateLimiter.config.MaxAttempts))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			if !allowed {
				// Calculate retry-after based on window or block time
				retryAfter := int(rateLimiter.config.Window.Seconds())
				if rateLimiter.config.BlockTime > 0 {
					retryAfter = int(rateLimiter.config.BlockTime.Seconds())
				}

				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().UTC().Add(time.Duration(retryAfter)*time.Second).Unix(), 10))

				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			// Set reset time header
			resetTime := time.Now().UTC().Add(rateLimiter.config.Window)
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

			next.ServeHTTP(w, r)
		})
	}
}

// IPBasedKeyFunc generates rate limit key based on IP address
func IPBasedKeyFunc(action string) func(*http.Request) string {
	return func(r *http.Request) string {
		ip := getClientIP(r)
		return GenerateIPKey(action, ip)
	}
}

// UserBasedKeyFunc generates rate limit key based on user ID from context
func UserBasedKeyFunc(action string) func(*http.Request) string {
	return func(r *http.Request) string {
		// This would need to extract user ID from request context
		// Implementation depends on your authentication middleware
		userID := getUserIDFromRequest(r)
		if userID == 0 {
			return ""
		}
		return GenerateUserKey(action, userID)
	}
}

// CombinedKeyFunc generates rate limit key based on both IP and user
func CombinedKeyFunc(action string) func(*http.Request) string {
	return func(r *http.Request) string {
		ip := getClientIP(r)
		userID := getUserIDFromRequest(r)

		if userID > 0 {
			return GenerateUserKey(action, userID)
		}
		return GenerateIPKey(action, ip)
	}
}

// Helper function to extract client IP
func getClientIP(r *http.Request) string {
	return httpx.GetClientIP(r)
}

// Helper function to extract user ID from request
// This is a placeholder - implement based on your authentication system
func getUserIDFromRequest(r *http.Request) int64 {
	// Example implementation - you would replace this with your actual logic
	// This might involve checking JWT claims, session data, etc.

	// For now, return 0 to indicate no authenticated user
	return 0
}
