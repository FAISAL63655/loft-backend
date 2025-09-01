// Package httpx tests for HTTP utilities
package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name           string
		remoteAddr     string
		headers        map[string]string
		expectedIP     string
		description    string
	}{
		{
			name:        "Direct connection",
			remoteAddr:  "192.168.1.100:12345",
			headers:     map[string]string{},
			expectedIP:  "192.168.1.100",
			description: "Should return direct IP from RemoteAddr",
		},
		{
			name:       "X-Forwarded-For from trusted proxy",
			remoteAddr: "10.0.0.1:80",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.1, 10.0.0.1",
			},
			expectedIP:  "203.0.113.1",
			description: "Should return client IP from X-Forwarded-For",
		},
		{
			name:       "Cloudflare CF-Connecting-IP",
			remoteAddr: "10.0.0.1:80",
			headers: map[string]string{
				"CF-Connecting-IP": "203.0.113.2",
			},
			expectedIP:  "203.0.113.2",
			description: "Should return IP from CF-Connecting-IP header",
		},
		{
			name:       "X-Real-IP header",
			remoteAddr: "10.0.0.1:80",
			headers: map[string]string{
				"X-Real-IP": "203.0.113.3",
			},
			expectedIP:  "203.0.113.3",
			description: "Should return IP from X-Real-IP header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			result := GetClientIP(req)
			if result != tt.expectedIP {
				t.Errorf("GetClientIP() = %v, want %v. %s", result, tt.expectedIP, tt.description)
			}
		})
	}
}

func TestGetUserAgent(t *testing.T) {
	tests := []struct {
		name        string
		userAgent   string
		expected    string
		description string
	}{
		{
			name:        "Normal user agent",
			userAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			expected:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			description: "Should return normal user agent unchanged",
		},
		{
			name:        "Long user agent",
			userAgent:   strings.Repeat("A", 600),
			expected:    strings.Repeat("A", 500),
			description: "Should truncate user agent to 500 characters",
		},
		{
			name:        "Empty user agent",
			userAgent:   "",
			expected:    "",
			description: "Should return empty string for empty user agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.userAgent != "" {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			result := GetUserAgent(req)
			if result != tt.expected {
				t.Errorf("GetUserAgent() = %v, want %v. %s", result, tt.expected, tt.description)
			}
		})
	}
}

func TestIsOriginAllowed(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		allowedOrigins []string
		expected       bool
		description    string
	}{
		{
			name:           "Exact match",
			origin:         "https://example.com",
			allowedOrigins: []string{"https://example.com", "https://other.com"},
			expected:       true,
			description:    "Should allow exact origin match",
		},
		{
			name:           "Wildcard allow all",
			origin:         "https://any-domain.com",
			allowedOrigins: []string{"*"},
			expected:       true,
			description:    "Should allow any origin with wildcard",
		},
		{
			name:           "Subdomain wildcard",
			origin:         "https://api.example.com",
			allowedOrigins: []string{"*.example.com"},
			expected:       true,
			description:    "Should allow subdomain with wildcard",
		},
		{
			name:           "Root domain rejected with wildcard",
			origin:         "https://example.com",
			allowedOrigins: []string{"*.example.com"},
			expected:       false,
			description:    "Should reject root domain with subdomain wildcard",
		},
		{
			name:           "Not in allowed list",
			origin:         "https://malicious.com",
			allowedOrigins: []string{"https://example.com"},
			expected:       false,
			description:    "Should reject origin not in allowed list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsOriginAllowed(tt.origin, tt.allowedOrigins)
			if result != tt.expected {
				t.Errorf("IsOriginAllowed() = %v, want %v. %s", result, tt.expected, tt.description)
			}
		})
	}
}

func TestHTTPHeadersMiddleware(t *testing.T) {
	// Create a test handler that checks context values
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		
		// Check if headers were stored in context
		clientIP := GetHeaderFromContext(ctx, ContextKeyClientIP)
		userAgent := GetHeaderFromContext(ctx, ContextKeyUserAgent)
		
		w.Header().Set("X-Test-Client-IP", clientIP)
		w.Header().Set("X-Test-User-Agent", userAgent)
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with our middleware
	handler := HTTPHeadersMiddleware(testHandler)

	// Create test request
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("User-Agent", "Test-Agent/1.0")
	
	// Create response recorder
	rr := httptest.NewRecorder()

	// Execute request
	handler.ServeHTTP(rr, req)

	// Check if middleware stored values correctly
	if rr.Header().Get("X-Test-Client-IP") == "" {
		t.Error("Middleware did not store client IP in context")
	}
	
	if rr.Header().Get("X-Test-User-Agent") != "Test-Agent/1.0" {
		t.Errorf("Middleware did not store user agent correctly, got: %s", rr.Header().Get("X-Test-User-Agent"))
	}
}

func TestExtractHeaderFromContext(t *testing.T) {
	// Create request with headers
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("User-Agent", "Test-Agent/1.0")
	req.Header.Set("X-Custom-Header", "custom-value")
	
	// Store request in context
	ctx := WithHTTPRequest(context.Background(), req)
	
	// Test header extraction
	userAgent := ExtractHeaderFromContext(ctx, "User-Agent")
	if userAgent != "Test-Agent/1.0" {
		t.Errorf("ExtractHeaderFromContext() = %v, want %v", userAgent, "Test-Agent/1.0")
	}
	
	customHeader := ExtractHeaderFromContext(ctx, "X-Custom-Header")
	if customHeader != "custom-value" {
		t.Errorf("ExtractHeaderFromContext() = %v, want %v", customHeader, "custom-value")
	}
	
	// Test non-existent header
	nonExistent := ExtractHeaderFromContext(ctx, "Non-Existent")
	if nonExistent != "" {
		t.Errorf("ExtractHeaderFromContext() should return empty string for non-existent header, got: %v", nonExistent)
	}
}

func TestGetClientIPFromContext(t *testing.T) {
	// Test with context that has IP stored
	ctx := context.WithValue(context.Background(), ContextKeyClientIP, "203.0.113.1")
	
	result := GetClientIPFromMiddleware(ctx)
	if result != "203.0.113.1" {
		t.Errorf("GetClientIPFromMiddleware() = %v, want %v", result, "203.0.113.1")
	}
	
	// Test with empty context
	emptyCtx := context.Background()
	result = GetClientIPFromMiddleware(emptyCtx)
	if result != "" {
		t.Errorf("GetClientIPFromMiddleware() should return empty string for empty context, got: %v", result)
	}
}

func TestIsValidIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"192.168.1.1", true},
		{"203.0.113.1", true},
		{"2001:db8::1", true},
		{"invalid-ip", false},
		{"256.256.256.256", false},
		{"", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			result := isValidIP(tt.ip)
			if result != tt.expected {
				t.Errorf("isValidIP(%v) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"127.0.0.1", true},
		{"203.0.113.1", false},
		{"8.8.8.8", false},
		{"invalid-ip", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			result := isPrivateIP(tt.ip)
			if result != tt.expected {
				t.Errorf("isPrivateIP(%v) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}
