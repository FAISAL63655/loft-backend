// Package httpx middleware for extracting and storing HTTP headers in context
package httpx

import (
	"context"
	"net/http"
	"strings"
)

// ContextKey represents the type for context keys to avoid collisions
type ContextKey string

const (
	// ContextKeyClientIP stores the client IP in context
	ContextKeyClientIP ContextKey = "client_ip"
	// ContextKeyUserAgent stores the User-Agent in context  
	ContextKeyUserAgent ContextKey = "user_agent"
	// ContextKeyXForwardedFor stores the X-Forwarded-For header in context
	ContextKeyXForwardedFor ContextKey = "x_forwarded_for"
	// ContextKeyXRealIP stores the X-Real-IP header in context
	ContextKeyXRealIP ContextKey = "x_real_ip"
	// ContextKeyCFConnectingIP stores the CF-Connecting-IP header in context
	ContextKeyCFConnectingIP ContextKey = "cf_connecting_ip"
	// ContextKeyOrigin stores the Origin header in context
	ContextKeyOrigin ContextKey = "origin"
)

// HTTPHeadersMiddleware extracts important HTTP headers and stores them in context
// This is a standard HTTP middleware compatible with Encore's architecture
func HTTPHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract headers from the HTTP request and store in context
		ctx := extractHTTPHeaders(r.Context(), r)
		
		// Continue with the updated context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractHTTPHeaders extracts relevant HTTP headers and stores them in context
func extractHTTPHeaders(ctx context.Context, httpReq *http.Request) context.Context {
	if httpReq == nil {
		return ctx
	}

	// Extract and store Client IP
	clientIP := GetClientIP(httpReq)
	if clientIP != "" {
		ctx = context.WithValue(ctx, ContextKeyClientIP, clientIP)
	}

	// Extract and store User-Agent
	userAgent := GetUserAgent(httpReq)
	if userAgent != "" {
		ctx = context.WithValue(ctx, ContextKeyUserAgent, userAgent)
	}

	// Extract and store X-Forwarded-For
	if xff := httpReq.Header.Get("X-Forwarded-For"); xff != "" {
		ctx = context.WithValue(ctx, ContextKeyXForwardedFor, strings.TrimSpace(xff))
	}

	// Extract and store X-Real-IP
	if xri := httpReq.Header.Get("X-Real-IP"); xri != "" {
		ctx = context.WithValue(ctx, ContextKeyXRealIP, strings.TrimSpace(xri))
	}

	// Extract and store CF-Connecting-IP (Cloudflare)
	if cfIP := httpReq.Header.Get("CF-Connecting-IP"); cfIP != "" {
		ctx = context.WithValue(ctx, ContextKeyCFConnectingIP, strings.TrimSpace(cfIP))
	}

	// Extract and store Origin
	origin := GetOrigin(httpReq)
	if origin != "" {
		ctx = context.WithValue(ctx, ContextKeyOrigin, origin)
	}

	return ctx
}

// GetHeaderFromContext retrieves a header value from context
// This provides a unified way to access headers stored by the middleware
func GetHeaderFromContext(ctx context.Context, key ContextKey) string {
	if ctx == nil {
		return ""
	}

	if value, ok := ctx.Value(key).(string); ok {
		return value
	}

	return ""
}

// GetClientIPFromMiddleware gets client IP stored by middleware
func GetClientIPFromMiddleware(ctx context.Context) string {
	return GetHeaderFromContext(ctx, ContextKeyClientIP)
}

// GetUserAgentFromMiddleware gets User-Agent stored by middleware
func GetUserAgentFromMiddleware(ctx context.Context) string {
	return GetHeaderFromContext(ctx, ContextKeyUserAgent)
}

// GetOriginFromMiddleware gets Origin stored by middleware
func GetOriginFromMiddleware(ctx context.Context) string {
	return GetHeaderFromContext(ctx, ContextKeyOrigin)
}

// GetXForwardedForFromMiddleware gets X-Forwarded-For stored by middleware
func GetXForwardedForFromMiddleware(ctx context.Context) string {
	return GetHeaderFromContext(ctx, ContextKeyXForwardedFor)
}

// ValidateMiddlewareSetup checks if the middleware is properly configured
// This can be used in tests or health checks
func ValidateMiddlewareSetup(ctx context.Context) bool {
	// Check if at least one header was stored (indicating middleware is working)
	return GetHeaderFromContext(ctx, ContextKeyClientIP) != "" ||
		GetHeaderFromContext(ctx, ContextKeyUserAgent) != "" ||
		GetHeaderFromContext(ctx, ContextKeyOrigin) != ""
}
