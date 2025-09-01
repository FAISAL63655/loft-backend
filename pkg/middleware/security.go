package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"encore.app/pkg/httpx"
	"encore.app/pkg/logger"
	"encore.app/pkg/metrics"
)

// SecurityConfig defines the configuration for security middleware
type SecurityConfig struct {
	// CSRF protection
	CSRFTokenName   string
	CSRFCookieName  string
	CSRFExemptPaths []string

	// Security headers
	EnableHSTS               bool
	HSTSMaxAge               int
	EnableContentTypeNoSniff bool
	EnableFrameOptions       bool
	FrameOptionsValue        string
	EnableXSSProtection      bool
	ContentSecurityPolicy    string
	ReferrerPolicy           string

	// Rate limiting
	EnableRateLimit bool
	RateLimitConfig map[string]interface{}
}

// DefaultSecurityConfig provides a secure default configuration
var DefaultSecurityConfig = SecurityConfig{
	CSRFTokenName:            "csrf_token",
	CSRFCookieName:           "csrf_cookie",
	CSRFExemptPaths:          []string{"/health", "/metrics"},
	EnableHSTS:               true,
	HSTSMaxAge:               31536000, // 1 year
	EnableContentTypeNoSniff: true,
	EnableFrameOptions:       true,
	FrameOptionsValue:        "DENY",
	EnableXSSProtection:      false, // Disabled as it's deprecated and can cause issues
	ContentSecurityPolicy:    "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline';",
	ReferrerPolicy:           "strict-origin-when-cross-origin",
}

// DevelopmentSecurityConfig provides relaxed security for development
var DevelopmentSecurityConfig = SecurityConfig{
	CSRFTokenName:            "csrf_token",
	CSRFCookieName:           "csrf_cookie",
	CSRFExemptPaths:          []string{"/health", "/metrics", "/api"},
	EnableHSTS:               false, // No HTTPS in dev usually
	HSTSMaxAge:               0,
	EnableContentTypeNoSniff: true,
	EnableFrameOptions:       true,
	FrameOptionsValue:        "SAMEORIGIN", // More permissive for dev tools
	EnableXSSProtection:      false,
	ContentSecurityPolicy:    "default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline';", // Relaxed for dev
	ReferrerPolicy:           "no-referrer-when-downgrade",
}

// ProductionSecurityConfig provides strict security for production
var ProductionSecurityConfig = SecurityConfig{

	CSRFTokenName:            "csrf_token",
	CSRFCookieName:           "csrf_cookie",
	CSRFExemptPaths:          []string{"/health", "/metrics"}, // Minimal exemptions
	EnableHSTS:               true,
	HSTSMaxAge:               31536000, // 1 year
	EnableContentTypeNoSniff: true,
	EnableFrameOptions:       true,
	FrameOptionsValue:        "DENY", // Strict frame protection
	EnableXSSProtection:      false,  // Still deprecated
	// Strict CSP including Moyasar & Apple Pay domains per PRD
	ContentSecurityPolicy: "default-src 'self'; " +
		"script-src 'self'; " +
		"style-src 'self'; " +
		"img-src 'self' data: https:; " +
		"font-src 'self'; " +
		"connect-src 'self' https://api.moyasar.com https://*.moyasar.com https://apple-pay-gateway.apple.com wss:; " +
		"frame-ancestors 'none'; base-uri 'self'; object-src 'none';",
	ReferrerPolicy: "strict-origin-when-cross-origin",
}

// SecurityHeadersMiddleware adds security headers to responses
func SecurityHeadersMiddleware(config SecurityConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// HSTS (HTTP Strict Transport Security)
			if config.EnableHSTS && r.TLS != nil {
				w.Header().Set("Strict-Transport-Security",
					fmt.Sprintf("max-age=%d; includeSubDomains", config.HSTSMaxAge))
			}

			// Content Type Options
			if config.EnableContentTypeNoSniff {
				w.Header().Set("X-Content-Type-Options", "nosniff")
			}

			// Frame Options
			if config.EnableFrameOptions {
				w.Header().Set("X-Frame-Options", config.FrameOptionsValue)
			}

			// XSS Protection (deprecated but kept for compatibility)
			if config.EnableXSSProtection {
				w.Header().Set("X-XSS-Protection", "1; mode=block")
			}

			// Content Security Policy
			if config.ContentSecurityPolicy != "" {
				w.Header().Set("Content-Security-Policy", config.ContentSecurityPolicy)
			}

			// Referrer Policy
			if config.ReferrerPolicy != "" {
				w.Header().Set("Referrer-Policy", config.ReferrerPolicy)
			}

			// Modern security headers
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

			// Remove server information
			w.Header().Set("Server", "")

			next.ServeHTTP(w, r)
		})
	}
}

// CORSSettingsProvider defines interface for dynamic CORS settings
type CORSSettingsProvider func() *CORSConfig

// CORSConfig defines CORS configuration
type CORSConfig struct {
	AllowedOrigins     []string
	AllowedMethods     []string
	AllowedHeaders     []string
	ExposedHeaders     []string
	AllowCredentials   bool
	MaxAge             int
	OptionsPassthrough bool
	UseSystemSettings  bool                 // Enable dynamic loading from system_settings
	SettingsProvider   CORSSettingsProvider // Callback for dynamic settings (avoids import cycle)
}

// GetDynamicSettings gets CORS settings from provider or fallback to static
func (c *CORSConfig) GetDynamicSettings() *CORSConfig {
	if !c.UseSystemSettings {
		return c
	}

	// Try to get settings from provider (avoids import cycle)
	if c.SettingsProvider != nil {
		if dynamicConfig := c.SettingsProvider(); dynamicConfig != nil {
			return dynamicConfig
		}
	}

	// Fallback to static config
	return c
}

// DefaultCORSConfig provides a default CORS configuration
var DefaultCORSConfig = CORSConfig{
	AllowedOrigins:    []string{"*"},
	AllowedMethods:    []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	AllowedHeaders:    []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
	ExposedHeaders:    []string{"Link"},
	AllowCredentials:  false,
	MaxAge:            300,
	UseSystemSettings: false,
}

// NewDynamicCORSConfig creates CORS configuration with system_settings integration
func NewDynamicCORSConfig(settingsProvider CORSSettingsProvider) CORSConfig {
	return CORSConfig{
		// Static fallback values (used when system_settings is unavailable)
		AllowedOrigins:    []string{"*"},
		AllowedMethods:    []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:    []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:    []string{"Link", "X-Total-Count", "X-Request-ID"},
		AllowCredentials:  false,
		MaxAge:            86400, // 24 hours (will be overridden by system_settings)
		UseSystemSettings: true,  // Enable dynamic loading
		SettingsProvider:  settingsProvider,
	}
}

// CORSMiddleware handles Cross-Origin Resource Sharing with dynamic settings
func CORSMiddleware(config CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := httpx.GetOrigin(r)

			// Get dynamic CORS settings
			settings := config.GetDynamicSettings()
			if settings == nil {
				// Fallback to static config if dynamic loading fails
				settings = &config
			}

			// Set Vary headers for proper caching
			w.Header().Add("Vary", "Origin")
			if r.Method == "OPTIONS" {
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
			}

			// Check if origin is allowed using improved logic
			if origin != "" && httpx.IsOriginAllowed(origin, settings.AllowedOrigins) {
				// Handle wildcard vs specific origin for credentials
				if settings.AllowCredentials && origin != "" {
					// When credentials are allowed, we must specify the exact origin
					w.Header().Set("Access-Control-Allow-Origin", origin)
				} else if len(settings.AllowedOrigins) == 1 && settings.AllowedOrigins[0] == "*" {
					// Only use wildcard when credentials are not allowed
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
			}

			// Set other CORS headers
			if len(settings.AllowedMethods) > 0 {
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(settings.AllowedMethods, ", "))
			}

			if len(settings.AllowedHeaders) > 0 {
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(settings.AllowedHeaders, ", "))
			}

			if len(settings.ExposedHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", strings.Join(settings.ExposedHeaders, ", "))
			}

			if settings.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if settings.MaxAge > 0 {
				w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", settings.MaxAge))
			}

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				if settings.OptionsPassthrough {
					next.ServeHTTP(w, r)
				} else {
					w.WriteHeader(http.StatusNoContent)
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Note: isOriginAllowed has been moved to pkg/httpx for better reusability

// CSRFMiddleware provides CSRF protection
func CSRFMiddleware(config SecurityConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip CSRF check for exempt paths
			for _, path := range config.CSRFExemptPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Skip CSRF check if using Bearer token authentication
			// CSRF attacks don't apply when using Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" && strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				next.ServeHTTP(w, r)
				return
			}

			// Skip CSRF check for safe methods
			if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
				// Generate and set CSRF token for safe methods
				token, err := generateCSRFToken()
				if err == nil {
					setCSRFCookie(w, config.CSRFCookieName, token)
					w.Header().Set("X-CSRF-Token", token)
				}
				next.ServeHTTP(w, r)
				return
			}

			// Validate CSRF token for unsafe methods (only when using cookie-based auth)
			if !validateCSRFToken(r, config) {
				http.Error(w, "CSRF token validation failed", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// generateCSRFToken generates a random CSRF token
func generateCSRFToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// setCSRFCookie sets the CSRF token in a cookie
func setCSRFCookie(w http.ResponseWriter, cookieName, token string) {
	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // JavaScript needs to read this for AJAX requests
		Secure:   true,  // Should be true in production with HTTPS
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour),
	}
	http.SetCookie(w, cookie)
}

// validateCSRFToken validates the CSRF token from the request
func validateCSRFToken(r *http.Request, config SecurityConfig) bool {
	// Get token from header
	headerToken := r.Header.Get("X-CSRF-Token")
	if headerToken == "" {
		// Try to get from form data
		headerToken = r.FormValue(config.CSRFTokenName)
	}

	// Get token from cookie
	cookie, err := r.Cookie(config.CSRFCookieName)
	if err != nil {
		return false
	}

	// Compare tokens using constant-time comparison to prevent timing attacks
	if headerToken == "" || len(headerToken) != len(cookie.Value) {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(headerToken), []byte(cookie.Value)) == 1
}

// LoggingMiddleware logs HTTP requests
func LoggingMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now().UTC()

			// Create a response writer wrapper to capture status code
			wrapper := &responseWriterWrapper{ResponseWriter: w, statusCode: 200}

			next.ServeHTTP(wrapper, r)

			duration := time.Since(start)

			// Structured logging for HTTP requests
			requestID := fmt.Sprintf("req_%d", time.Now().UTC().UnixNano())
			ctx := logger.WithRequestID(r.Context(), requestID)

			logger.Info(ctx, "HTTP request completed", logger.Fields{
				"method":      r.Method,
				"path":        r.URL.Path,
				"status_code": wrapper.statusCode,
				"duration_ms": duration.Milliseconds(),
				"client_ip":   httpx.GetClientIP(r),
				"user_agent":  httpx.GetUserAgent(r),
			})

			// Prometheus metrics
			metrics.ObserveHTTPRequest(r.Method, r.URL.Path, fmt.Sprintf("%d", wrapper.statusCode), start)
		})
	}
}

// responseWriterWrapper wraps http.ResponseWriter to capture status code
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// RecoveryMiddleware recovers from panics and returns a 500 error
func RecoveryMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					// Log the panic (in production, use structured logging)
					logger.LogPanic(context.Background(), err)

					// Return 500 error
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate request ID
			requestID, err := generateRequestID()
			if err != nil {
				requestID = fmt.Sprintf("req_%d", time.Now().UTC().UnixNano())
			}

			// Add to response header
			w.Header().Set("X-Request-ID", requestID)

			// Add to request context
			ctx := r.Context()
			ctx = context.WithValue(ctx, RequestIDContextKey, requestID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// generateRequestID generates a unique request ID
func generateRequestID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("req_%x", bytes), nil
}

// GetRequestIDFromContext extracts the request ID from context
func GetRequestIDFromContext(ctx context.Context) (string, bool) {
	requestID, ok := ctx.Value(RequestIDContextKey).(string)
	return requestID, ok
}
