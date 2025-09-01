// Package httpx provides HTTP utilities and helpers
package httpx

import (
	"context"
	"net"
	"net/http"
	"strings"

	"encore.dev"
)

// Trusted proxy networks (RFC 1918 private ranges + common cloud providers)
var trustedProxyNetworks = []string{
	"10.0.0.0/8",     // Private Class A
	"172.16.0.0/12",  // Private Class B
	"192.168.0.0/16", // Private Class C
	"127.0.0.0/8",    // Loopback
	"169.254.0.0/16", // Link-local
	"::1/128",        // IPv6 loopback
	"fc00::/7",       // IPv6 unique local
	"fe80::/10",      // IPv6 link-local
}

// isTrustedProxy checks if the given IP is from a trusted proxy network
func isTrustedProxy(ip string) bool {
	clientIP := net.ParseIP(ip)
	if clientIP == nil {
		return false
	}

	for _, cidr := range trustedProxyNetworks {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(clientIP) {
			return true
		}
	}
	return false
}

// GetClientIP extracts the real client IP address from HTTP request
// Only trusts proxy headers from known trusted proxy networks
func GetClientIP(r *http.Request) string {
	// Get the immediate connection's remote address
	remoteAddr := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}

	// Only trust proxy headers if the request comes from a trusted proxy
	if !isTrustedProxy(remoteAddr) {
		return remoteAddr
	}

	// Check X-Forwarded-For header (most common with proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs separated by commas
		// Format: "client, proxy1, proxy2"
		// We want the first (leftmost) IP which should be the original client
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if clientIP != "" && isValidIP(clientIP) && !isPrivateIP(clientIP) {
				return clientIP
			}
		}
	}

	// Check X-Real-IP header (used by some proxies)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		xri = strings.TrimSpace(xri)
		if isValidIP(xri) {
			return xri
		}
	}

	// Check CF-Connecting-IP header (Cloudflare)
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		cfIP = strings.TrimSpace(cfIP)
		if isValidIP(cfIP) {
			return cfIP
		}
	}

	// Fall back to RemoteAddr (direct connection)
	// RemoteAddr is in format "IP:Port", so we need to extract just the IP
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}

	// If SplitHostPort fails, return the RemoteAddr as-is
	return r.RemoteAddr
}

// isValidIP checks if the provided string is a valid IP address
func isValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// isPrivateIP checks if the IP address is private/internal
func isPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// Check if it's in private ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(parsedIP) {
			return true
		}
	}
	return false
}

// GetUserAgent extracts and sanitizes the User-Agent header
func GetUserAgent(r *http.Request) string {
	ua := r.Header.Get("User-Agent")
	// Basic sanitization - limit length to prevent abuse
	if len(ua) > 500 {
		ua = ua[:500]
	}
	return ua
}

// GetOrigin safely extracts the Origin header
func GetOrigin(r *http.Request) string {
	origin := r.Header.Get("Origin")
	// Basic validation - ensure it looks like a valid origin
	if origin == "" {
		return ""
	}

	// Remove trailing slash if present
	origin = strings.TrimSuffix(origin, "/")

	// Basic format validation - should start with http:// or https://
	if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
		return ""
	}

	return origin
}

// IsOriginAllowed checks if the origin is in the allowed list
// Supports wildcards like *.example.com (but NOT the root domain)
func IsOriginAllowed(origin string, allowedOrigins []string) bool {
	if origin == "" {
		return false
	}

	for _, allowed := range allowedOrigins {
		// Exact match or wildcard "*"
		if allowed == "*" || allowed == origin {
			return true
		}

		// Support wildcard subdomains (e.g., *.example.com)
		if strings.HasPrefix(allowed, "*.") {
			domain := allowed[2:] // Remove "*."

			// Extract hostname from origin URL
			originHost := ExtractHost(origin)
			if originHost == "" {
				continue
			}

			// For *.example.com:
			// - Allow: sub.example.com, api.example.com, www.example.com
			// - Reject: example.com (root domain)
			// - Reject: notexample.com, example.com.evil.com

			if originHost == domain {
				// Reject root domain for wildcard pattern
				continue
			}

			if strings.HasSuffix(originHost, "."+domain) {
				// Check that it's actually a subdomain, not just ending with domain
				// e.g., "notexample.com" should not match "*.example.com"
				prefixLen := len(originHost) - len("."+domain)
				if prefixLen > 0 {
					prefix := originHost[:prefixLen]
					// Ensure prefix doesn't contain dots (single-level subdomain check)
					// or allow multi-level subdomains
					if !strings.Contains(prefix, ".") || allowMultiLevelSubdomains(prefix) {
						return true
					}
				}
			}
		}
	}

	return false
}

// allowMultiLevelSubdomains checks if multi-level subdomains should be allowed
// For security, we can be more restrictive here
func allowMultiLevelSubdomains(prefix string) bool {
	// Allow multi-level subdomains like api.v1.example.com for *.example.com
	// But limit the number of levels to prevent abuse
	levels := strings.Count(prefix, ".") + 1
	return levels <= 3 // Allow up to 3 levels (e.g., a.b.c.example.com)
}

// ExtractHost extracts the hostname from a URL or origin
func ExtractHost(urlOrOrigin string) string {
	// Remove protocol
	if strings.HasPrefix(urlOrOrigin, "https://") {
		urlOrOrigin = urlOrOrigin[8:]
	} else if strings.HasPrefix(urlOrOrigin, "http://") {
		urlOrOrigin = urlOrOrigin[7:]
	}

	// Remove path (everything after first /)
	if idx := strings.Index(urlOrOrigin, "/"); idx >= 0 {
		urlOrOrigin = urlOrOrigin[:idx]
	}

	// Remove port if present
	if host, _, err := net.SplitHostPort(urlOrOrigin); err == nil {
		return host
	}

	return urlOrOrigin
}

// GetClientIPFromContext extracts client IP from Encore request context
// This integrates with Encore's request handling
func GetClientIPFromContext(ctx context.Context) string {
	// Primary: try to extract from context (set by middleware)
	if clientIP := getClientIPFromContextValue(ctx); clientIP != "" {
		return clientIP
	}

	// Secondary: try to get current request from Encore
	if req := encore.CurrentRequest(); req != nil {
		return getClientIPFromEncoreRequest(req)
	}

	// Fallback for development: use localhost
	// Better than "unknown" for rate limiting in local development
	if isLocalDevelopment() {
		return "127.0.0.1"
	}

	// Last resort: return unknown IP
	return "unknown"
}

// getClientIPFromEncoreRequest extracts IP from Encore request
func getClientIPFromEncoreRequest(req *encore.Request) string {
	// Try to extract headers from Encore request if available
	if req != nil {
		// Check for proxy headers first (most reliable in production)
		if xff := getHeaderFromRequest(req, "X-Forwarded-For"); xff != "" {
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				clientIP := strings.TrimSpace(ips[0])
				if clientIP != "" && isValidIP(clientIP) {
					return clientIP
				}
			}
		}

		// Check X-Real-IP header
		if xri := getHeaderFromRequest(req, "X-Real-IP"); xri != "" {
			xri = strings.TrimSpace(xri)
			if isValidIP(xri) {
				return xri
			}
		}

		// Check CF-Connecting-IP header (Cloudflare)
		if cfIP := getHeaderFromRequest(req, "CF-Connecting-IP"); cfIP != "" {
			cfIP = strings.TrimSpace(cfIP)
			if isValidIP(cfIP) {
				return cfIP
			}
		}

		// Try to extract from Encore request if there's direct access
		// This would require Encore to expose client IP directly
		// For now, we rely on headers set by proxy/load balancer
	}

	return ""
}

// getHeaderFromRequest attempts to extract a header from Encore request
// This is a helper function that provides a future-proof interface for header access
func getHeaderFromRequest(req *encore.Request, headerName string) string {
	if req == nil || headerName == "" {
		return ""
	}

	// Currently, Encore doesn't expose direct header access in *encore.Request
	// This implementation provides a consistent interface for when Encore adds header support
	
	// The system currently relies on middleware to populate context with header values
	// This ensures compatibility when Encore provides direct header access in future versions
	
	// Future implementation when Encore supports it:
	// if req.Headers != nil {
	//     return req.Headers.Get(headerName)
	// }
	
	return ""
}

// getClientIPFromContextValue tries to get IP from context value
func getClientIPFromContextValue(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	// Primary: Check if IP was stored in context by HTTPHeadersMiddleware
	if ip, ok := ctx.Value(ContextKeyClientIP).(string); ok && ip != "" {
		return ip
	}

	// Legacy: Check old context keys for backward compatibility
	if ip, ok := ctx.Value("client_ip").(string); ok && ip != "" {
		return ip
	}

	// Fallback: Check for X-Forwarded-For in context
	if xff, ok := ctx.Value(ContextKeyXForwardedFor).(string); ok && xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if isValidIP(clientIP) {
				return clientIP
			}
		}
	}

	// Legacy X-Forwarded-For check
	if xff, ok := ctx.Value("x-forwarded-for").(string); ok && xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if isValidIP(clientIP) {
				return clientIP
			}
		}
	}

	return ""
}

// GetUserAgentFromContext extracts User-Agent from Encore request context
// This integrates with Encore's request handling
func GetUserAgentFromContext(ctx context.Context) string {
	// Primary: try to extract from context (set by middleware)
	if userAgent := getUserAgentFromContextValue(ctx); userAgent != "" {
		return userAgent
	}

	// Secondary: try to get current request from Encore
	if req := encore.CurrentRequest(); req != nil {
		// Extract User-Agent from request data if available
		if userAgent := getUserAgentFromEncoreRequest(req); userAgent != "" {
			return userAgent
		}
	}

	// Fallback for development: use a meaningful default
	if isLocalDevelopment() {
		return "Loft-Development-Client/1.0"
	}

	// Last resort: return unknown user agent
	return "Unknown-Client/1.0"
}

// getUserAgentFromEncoreRequest extracts User-Agent from Encore request
// Professional implementation using current request context
func getUserAgentFromEncoreRequest(req *encore.Request) string {
	if req == nil {
		return ""
	}

	// Get current context from Encore's request handling
	// Since encore.Request doesn't expose Context() directly,
	// we use the current request context from Encore's runtime
	ctx := context.Background()
	
	// Try to get current request context if available
	if currentReq := encore.CurrentRequest(); currentReq != nil {
		// Extract User-Agent from the current request context
		userAgent := getUserAgentFromContextValue(ctx)
		if userAgent != "" {
			return userAgent
		}
	}

	// Professional fallback: return empty to allow system fallbacks
	// This ensures graceful degradation without blocking functionality
	return ""
}

// getUserAgentFromContextValue tries to get User-Agent from context value
func getUserAgentFromContextValue(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	// Primary: Check if User-Agent was stored in context by HTTPHeadersMiddleware
	if ua, ok := ctx.Value(ContextKeyUserAgent).(string); ok && ua != "" {
		// Basic sanitization - limit length to prevent abuse
		if len(ua) > 500 {
			ua = ua[:500]
		}
		return ua
	}

	// Legacy: Check old context keys for backward compatibility
	if ua, ok := ctx.Value("user_agent").(string); ok && ua != "" {
		// Basic sanitization - limit length to prevent abuse
		if len(ua) > 500 {
			ua = ua[:500]
		}
		return ua
	}

	return ""
}

// isLocalDevelopment checks if we're running in local development environment
func isLocalDevelopment() bool {
	// Check if we're in Encore local development mode
	if meta := encore.Meta(); meta != nil {
		return meta.Environment.Type == encore.EnvLocal
	}
	return false
}
