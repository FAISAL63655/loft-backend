// Package httpx - Context helper functions for HTTP request data extraction
package httpx

import (
	"context"
	"net/http"
)

// RequestContextKey is the key type for storing HTTP request in context
type RequestContextKey string

const (
	// HTTPRequestKey stores the original HTTP request in context
	HTTPRequestKey RequestContextKey = "http_request"
)

// WithHTTPRequest stores the HTTP request in context for later retrieval
func WithHTTPRequest(ctx context.Context, req *http.Request) context.Context {
	if req == nil {
		return ctx
	}
	return context.WithValue(ctx, HTTPRequestKey, req)
}

// GetHTTPRequest retrieves the stored HTTP request from context
func GetHTTPRequest(ctx context.Context) *http.Request {
	if ctx == nil {
		return nil
	}
	
	if req, ok := ctx.Value(HTTPRequestKey).(*http.Request); ok {
		return req
	}
	
	return nil
}

// ExtractHeaderFromContext tries to get a header value from context or stored request
func ExtractHeaderFromContext(ctx context.Context, headerName string) string {
	if ctx == nil || headerName == "" {
		return ""
	}

	// Try to get from stored HTTP request first
	if req := GetHTTPRequest(ctx); req != nil {
		return req.Header.Get(headerName)
	}

	// Fallback to context values set by middleware
	switch headerName {
	case "User-Agent":
		return GetHeaderFromContext(ctx, ContextKeyUserAgent)
	case "X-Forwarded-For":
		return GetHeaderFromContext(ctx, ContextKeyXForwardedFor)
	case "X-Real-IP":
		return GetHeaderFromContext(ctx, ContextKeyXRealIP)
	case "CF-Connecting-IP":
		return GetHeaderFromContext(ctx, ContextKeyCFConnectingIP)
	case "Origin":
		return GetHeaderFromContext(ctx, ContextKeyOrigin)
	}

	return ""
}

// EnhancedHTTPMiddleware stores the HTTP request in context and extracts headers
func EnhancedHTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Store the original request in context
		ctx := WithHTTPRequest(r.Context(), r)
		
		// Extract and store headers
		ctx = extractHTTPHeaders(ctx, r)
		
		// Continue with enhanced context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
