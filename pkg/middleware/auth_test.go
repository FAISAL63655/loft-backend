package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"encore.app/pkg/authn"
	"encore.app/pkg/session"
)

func TestGetUserFromContext(t *testing.T) {
	ctx := context.Background()

	// Test with no user in context
	_, ok := GetUserFromContext(ctx)
	if ok {
		t.Error("Should return false when no user in context")
	}

	// Test with user in context
	user := &UserContext{
		UserID: 123,
		Role:   "verified",
		Email:  "test@example.com",
	}
	ctx = context.WithValue(ctx, UserContextKey, user)

	retrievedUser, ok := GetUserFromContext(ctx)
	if !ok {
		t.Fatal("Should return true when user in context")
	}

	if retrievedUser.UserID != user.UserID {
		t.Errorf("User ID mismatch: got %d, want %d", retrievedUser.UserID, user.UserID)
	}

	if retrievedUser.Role != user.Role {
		t.Errorf("Role mismatch: got %s, want %s", retrievedUser.Role, user.Role)
	}

	if retrievedUser.Email != user.Email {
		t.Errorf("Email mismatch: got %s, want %s", retrievedUser.Email, user.Email)
	}
}

func TestGetUserIDFromContext(t *testing.T) {
	ctx := context.Background()

	// Test with no user in context
	_, ok := GetUserIDFromContext(ctx)
	if ok {
		t.Error("Should return false when no user in context")
	}

	// Test with user in context
	user := &UserContext{UserID: 123}
	ctx = context.WithValue(ctx, UserContextKey, user)

	userID, ok := GetUserIDFromContext(ctx)
	if !ok {
		t.Fatal("Should return true when user in context")
	}

	if userID != 123 {
		t.Errorf("User ID mismatch: got %d, want 123", userID)
	}
}

func TestGetUserRoleFromContext(t *testing.T) {
	ctx := context.Background()

	// Test with no user in context
	_, ok := GetUserRoleFromContext(ctx)
	if ok {
		t.Error("Should return false when no user in context")
	}

	// Test with user in context
	user := &UserContext{Role: "admin"}
	ctx = context.WithValue(ctx, UserContextKey, user)

	role, ok := GetUserRoleFromContext(ctx)
	if !ok {
		t.Fatal("Should return true when user in context")
	}

	if role != "admin" {
		t.Errorf("Role mismatch: got %s, want admin", role)
	}
}

func TestIsAdmin(t *testing.T) {
	ctx := context.Background()

	// Test with no user in context
	if IsAdmin(ctx) {
		t.Error("Should return false when no user in context")
	}

	// Test with non-admin user
	user := &UserContext{Role: "verified"}
	ctx = context.WithValue(ctx, UserContextKey, user)

	if IsAdmin(ctx) {
		t.Error("Should return false for non-admin user")
	}

	// Test with admin user
	user.Role = "admin"
	ctx = context.WithValue(ctx, UserContextKey, user)

	if !IsAdmin(ctx) {
		t.Error("Should return true for admin user")
	}
}

func TestIsVerified(t *testing.T) {
	ctx := context.Background()

	// Test with no user in context
	if IsVerified(ctx) {
		t.Error("Should return false when no user in context")
	}

	// Test with unverified user
	user := &UserContext{Role: "unverified"}
	ctx = context.WithValue(ctx, UserContextKey, user)

	if IsVerified(ctx) {
		t.Error("Should return false for unverified user")
	}

	// Test with verified user
	user.Role = "verified"
	ctx = context.WithValue(ctx, UserContextKey, user)

	if !IsVerified(ctx) {
		t.Error("Should return true for verified user")
	}

	// Test with admin user
	user.Role = "admin"
	ctx = context.WithValue(ctx, UserContextKey, user)

	if !IsVerified(ctx) {
		t.Error("Should return true for admin user")
	}
}

func TestHasRequiredRole(t *testing.T) {
	tests := []struct {
		name          string
		userRole      string
		requiredRoles []string
		expected      bool
	}{
		{
			name:          "user has required role",
			userRole:      "admin",
			requiredRoles: []string{"admin", "moderator"},
			expected:      true,
		},
		{
			name:          "user doesn't have required role",
			userRole:      "user",
			requiredRoles: []string{"admin", "moderator"},
			expected:      false,
		},
		{
			name:          "empty required roles",
			userRole:      "user",
			requiredRoles: []string{},
			expected:      false,
		},
		{
			name:          "single required role match",
			userRole:      "verified",
			requiredRoles: []string{"verified"},
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasRequiredRole(tt.userRole, tt.requiredRoles)
			if result != tt.expected {
				t.Errorf("hasRequiredRole() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractAuthInfo(t *testing.T) {
	tests := []struct {
		name          string
		authHeader    string
		setupSession  bool
		expectedToken string
		expectedError bool
	}{
		{
			name:          "valid bearer token",
			authHeader:    "Bearer test_token_123",
			expectedToken: "test_token_123",
			expectedError: false,
		},
		{
			name:          "invalid auth header format",
			authHeader:    "InvalidFormat test_token_123",
			expectedError: true,
		},
		{
			name:          "no auth header",
			authHeader:    "",
			expectedError: true,
		},
		{
			name:          "bearer with no token",
			authHeader:    "Bearer",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			token, sessionID, _, err := extractAuthInfo(req, nil)

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if token != tt.expectedToken {
				t.Errorf("Token mismatch: got %s, want %s", token, tt.expectedToken)
			}

			if sessionID != "" {
				t.Errorf("Session ID should be empty when using bearer token, got %s", sessionID)
			}
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	// Create JWT manager for testing
	jwtManager := authn.NewJWTManager("test-secret", "test-refresh-secret")

	// Create session manager for testing
	sessionConfig := session.DefaultSessionConfig
	sessionConfig.CleanupInterval = 0 // Disable cleanup for testing
	sessionManager := session.NewSessionManager(sessionConfig)

	// Generate test token
	tokenPair, err := jwtManager.GenerateTokens(123, "verified", "test@example.com")
	if err != nil {
		t.Fatalf("Failed to generate test token: %v", err)
	}

	tests := []struct {
		name           string
		authHeader     string
		requiredRoles  []string
		optional       bool
		expectedStatus int
	}{
		{
			name:           "valid token",
			authHeader:     "Bearer " + tokenPair.AccessToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid token",
			authHeader:     "Bearer invalid_token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "no token required auth",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "no token optional auth",
			authHeader:     "",
			optional:       true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "valid token wrong role",
			authHeader:     "Bearer " + tokenPair.AccessToken,
			requiredRoles:  []string{"admin"},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "valid token correct role",
			authHeader:     "Bearer " + tokenPair.AccessToken,
			requiredRoles:  []string{"verified"},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Create middleware
			config := AuthConfig{
				JWTManager:     jwtManager,
				SessionManager: sessionManager,
				RequiredRoles:  tt.requiredRoles,
				Optional:       tt.optional,
			}
			middleware := AuthMiddleware(config)
			wrappedHandler := middleware(handler)

			// Create request
			req := httptest.NewRequest("GET", "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			// Create response recorder
			w := httptest.NewRecorder()

			// Execute request
			wrappedHandler.ServeHTTP(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Status code mismatch: got %d, want %d", w.Code, tt.expectedStatus)
			}
		})
	}
}

func TestRequireAuth(t *testing.T) {
	jwtManager := authn.NewJWTManager("test-secret", "test-refresh-secret")
	sessionConfig := session.DefaultSessionConfig
	sessionConfig.CleanupInterval = 0
	sessionManager := session.NewSessionManager(sessionConfig)

	middleware := RequireAuth(jwtManager, sessionManager)

	// Test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(handler)

	// Test without auth
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}
}

func TestRequireRole(t *testing.T) {
	jwtManager := authn.NewJWTManager("test-secret", "test-refresh-secret")
	sessionConfig := session.DefaultSessionConfig
	sessionConfig.CleanupInterval = 0
	sessionManager := session.NewSessionManager(sessionConfig)

	// Generate token with verified role
	tokenPair, err := jwtManager.GenerateTokens(123, "verified", "test@example.com")
	if err != nil {
		t.Fatalf("Failed to generate test token: %v", err)
	}

	middleware := RequireRole(jwtManager, sessionManager, "admin")

	// Test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(handler)

	// Test with wrong role
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403, got %d", w.Code)
	}
}

func TestRequireAdmin(t *testing.T) {
	jwtManager := authn.NewJWTManager("test-secret", "test-refresh-secret")
	sessionConfig := session.DefaultSessionConfig
	sessionConfig.CleanupInterval = 0
	sessionManager := session.NewSessionManager(sessionConfig)

	// Generate admin token
	adminTokenPair, err := jwtManager.GenerateTokens(123, "admin", "admin@example.com")
	if err != nil {
		t.Fatalf("Failed to generate admin token: %v", err)
	}

	middleware := RequireAdmin(jwtManager, sessionManager)

	// Test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(handler)

	// Test with admin role
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+adminTokenPair.AccessToken)
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for admin, got %d", w.Code)
	}
}

func TestRequireVerified(t *testing.T) {
	jwtManager := authn.NewJWTManager("test-secret", "test-refresh-secret")
	sessionConfig := session.DefaultSessionConfig
	sessionConfig.CleanupInterval = 0
	sessionManager := session.NewSessionManager(sessionConfig)

	// Generate verified token
	verifiedTokenPair, err := jwtManager.GenerateTokens(123, "verified", "verified@example.com")
	if err != nil {
		t.Fatalf("Failed to generate verified token: %v", err)
	}

	middleware := RequireVerified(jwtManager, sessionManager)

	// Test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(handler)

	// Test with verified role
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+verifiedTokenPair.AccessToken)
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for verified user, got %d", w.Code)
	}
}

func TestAuthMiddleware_SessionBasedAuth(t *testing.T) {
	// Create JWT manager
	jwtManager := authn.NewJWTManager("test-access-secret", "test-refresh-secret")

	// Create session manager
	sessionConfig := session.DefaultSessionConfig
	sessionConfig.CleanupInterval = 0 // Disable cleanup for testing
	sessionManager := session.NewSessionManager(sessionConfig)

	// Generate tokens for a test user
	userID := int64(123)
	role := "verified"
	email := "test@example.com"

	tokenPair, err := jwtManager.GenerateTokens(userID, role, email)
	if err != nil {
		t.Fatalf("Failed to generate tokens: %v", err)
	}

	// Create a session with the refresh token
	sessionID, sessionData, err := sessionManager.CreateSession(userID, role, email, tokenPair.RefreshToken, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create HTTP cookie for the session
	cookie := &http.Cookie{
		Name:  sessionConfig.CookieName,
		Value: sessionID,
		Path:  "/",
	}

	// Suppress unused variable warning
	_ = sessionData

	// Create auth middleware
	config := AuthConfig{
		JWTManager:     jwtManager,
		SessionManager: sessionManager,
		Optional:       false,
	}
	middleware := AuthMiddleware(config)

	// Test handler that checks user context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := GetUserFromContext(r.Context())
		if !ok {
			t.Error("User context should be available")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if user.UserID != userID {
			t.Errorf("User ID mismatch: got %d, want %d", user.UserID, userID)
		}

		if user.Role != role {
			t.Errorf("Role mismatch: got %s, want %s", user.Role, role)
		}

		if user.Email != email {
			t.Errorf("Email mismatch: got %s, want %s", user.Email, email)
		}

		if user.SessionID != sessionID {
			t.Errorf("Session ID mismatch: got %s, want %s", user.SessionID, sessionID)
		}

		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(handler)

	// Test with session cookie (no Authorization header)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for session-based auth, got %d", w.Code)
	}
}

func TestAuthMiddleware_OptionalAuth_InvalidToken(t *testing.T) {
	// Create JWT manager
	jwtManager := authn.NewJWTManager("test-access-secret", "test-refresh-secret")

	// Create auth middleware with optional auth
	config := AuthConfig{
		JWTManager: jwtManager,
		Optional:   true,
	}
	middleware := AuthMiddleware(config)

	// Test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not reach here with invalid token, even if optional
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(handler)

	// Test with invalid token - should return 401 even with optional auth
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for invalid token even with optional auth, got %d", w.Code)
	}
}
