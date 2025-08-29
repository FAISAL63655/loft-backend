package authn

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestNewJWTManager(t *testing.T) {
	accessSecret := "test-access-secret"
	refreshSecret := "test-refresh-secret"

	manager := NewJWTManager(accessSecret, refreshSecret)

	if manager == nil {
		t.Fatal("NewJWTManager returned nil")
	}

	if string(manager.accessSecret) != accessSecret {
		t.Errorf("Access secret mismatch: got %s, want %s", string(manager.accessSecret), accessSecret)
	}

	if string(manager.refreshSecret) != refreshSecret {
		t.Errorf("Refresh secret mismatch: got %s, want %s", string(manager.refreshSecret), refreshSecret)
	}
}

func TestGenerateTokens(t *testing.T) {
	manager := NewJWTManager("test-access-secret", "test-refresh-secret")

	userID := int64(123)
	role := "verified"
	email := "test@example.com"

	tokenPair, err := manager.GenerateTokens(userID, role, email)
	if err != nil {
		t.Fatalf("GenerateTokens failed: %v", err)
	}

	if tokenPair == nil {
		t.Fatal("GenerateTokens returned nil token pair")
	}

	if tokenPair.AccessToken == "" {
		t.Error("Access token is empty")
	}

	if tokenPair.RefreshToken == "" {
		t.Error("Refresh token is empty")
	}

	if tokenPair.TokenType != "Bearer" {
		t.Errorf("Token type mismatch: got %s, want Bearer", tokenPair.TokenType)
	}

	if tokenPair.ExpiresAt.Before(time.Now()) {
		t.Error("Token expiration time is in the past")
	}

	// Verify access token can be validated
	claims, err := manager.ValidateAccessToken(tokenPair.AccessToken)
	if err != nil {
		t.Fatalf("Failed to validate access token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("User ID mismatch: got %d, want %d", claims.UserID, userID)
	}

	if claims.Role != role {
		t.Errorf("Role mismatch: got %s, want %s", claims.Role, role)
	}

	if claims.Email != email {
		t.Errorf("Email mismatch: got %s, want %s", claims.Email, email)
	}
}

func TestValidateAccessToken(t *testing.T) {
	manager := NewJWTManager("test-access-secret", "test-refresh-secret")

	userID := int64(123)
	role := "verified"
	email := "test@example.com"

	tokenPair, err := manager.GenerateTokens(userID, role, email)
	if err != nil {
		t.Fatalf("GenerateTokens failed: %v", err)
	}

	tests := []struct {
		name        string
		token       string
		expectError error
	}{
		{
			name:        "valid token",
			token:       tokenPair.AccessToken,
			expectError: nil,
		},
		{
			name:        "empty token",
			token:       "",
			expectError: ErrInvalidToken,
		},
		{
			name:        "invalid token format",
			token:       "invalid.token.format",
			expectError: ErrInvalidToken,
		},
		{
			name:        "token with wrong secret",
			token:       generateTokenWithWrongSecret(t, userID, role, email),
			expectError: ErrInvalidSignature,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := manager.ValidateAccessToken(tt.token)

			if tt.expectError != nil {
				if err == nil {
					t.Errorf("Expected error %v, got nil", tt.expectError)
				} else if !strings.Contains(err.Error(), tt.expectError.Error()) {
					t.Errorf("Expected error containing %v, got %v", tt.expectError, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if claims.UserID != userID {
				t.Errorf("User ID mismatch: got %d, want %d", claims.UserID, userID)
			}
		})
	}
}

func TestValidateRefreshToken(t *testing.T) {
	manager := NewJWTManager("test-access-secret", "test-refresh-secret")

	userID := int64(123)
	role := "verified"
	email := "test@example.com"

	tokenPair, err := manager.GenerateTokens(userID, role, email)
	if err != nil {
		t.Fatalf("GenerateTokens failed: %v", err)
	}

	// Test valid refresh token
	claims, err := manager.ValidateRefreshToken(tokenPair.RefreshToken)
	if err != nil {
		t.Fatalf("Failed to validate refresh token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("User ID mismatch: got %d, want %d", claims.UserID, userID)
	}

	// Test invalid refresh token
	_, err = manager.ValidateRefreshToken("invalid.token")
	if err == nil {
		t.Error("Expected error for invalid refresh token")
	}
}

func TestRefreshTokens(t *testing.T) {
	manager := NewJWTManager("test-access-secret", "test-refresh-secret")

	userID := int64(123)
	role := "verified"
	email := "test@example.com"

	// Generate initial tokens
	initialTokens, err := manager.GenerateTokens(userID, role, email)
	if err != nil {
		t.Fatalf("GenerateTokens failed: %v", err)
	}

	// Wait a moment to ensure different issued times
	time.Sleep(time.Millisecond * 10)

	// Refresh tokens
	newTokens, err := manager.RefreshTokens(initialTokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshTokens failed: %v", err)
	}

	// Verify new tokens are different
	if newTokens.AccessToken == initialTokens.AccessToken {
		t.Error("New access token should be different from initial token")
	}

	if newTokens.RefreshToken == initialTokens.RefreshToken {
		t.Error("New refresh token should be different from initial token")
	}

	// Verify new tokens are valid
	claims, err := manager.ValidateAccessToken(newTokens.AccessToken)
	if err != nil {
		t.Fatalf("Failed to validate new access token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("User ID mismatch in new token: got %d, want %d", claims.UserID, userID)
	}

	// Test refresh with invalid token
	_, err = manager.RefreshTokens("invalid.token")
	if err == nil {
		t.Error("Expected error when refreshing with invalid token")
	}
}

func TestExtractUserID(t *testing.T) {
	manager := NewJWTManager("test-access-secret", "test-refresh-secret")

	userID := int64(123)
	role := "verified"
	email := "test@example.com"

	tokenPair, err := manager.GenerateTokens(userID, role, email)
	if err != nil {
		t.Fatalf("GenerateTokens failed: %v", err)
	}

	extractedUserID, err := manager.ExtractUserID(tokenPair.AccessToken)
	if err != nil {
		t.Fatalf("ExtractUserID failed: %v", err)
	}

	if extractedUserID != userID {
		t.Errorf("User ID mismatch: got %d, want %d", extractedUserID, userID)
	}

	// Test with invalid token
	_, err = manager.ExtractUserID("invalid.token")
	if err == nil {
		t.Error("Expected error for invalid token")
	}
}

func TestExtractUserRole(t *testing.T) {
	manager := NewJWTManager("test-access-secret", "test-refresh-secret")

	userID := int64(123)
	role := "verified"
	email := "test@example.com"

	tokenPair, err := manager.GenerateTokens(userID, role, email)
	if err != nil {
		t.Fatalf("GenerateTokens failed: %v", err)
	}

	extractedRole, err := manager.ExtractUserRole(tokenPair.AccessToken)
	if err != nil {
		t.Fatalf("ExtractUserRole failed: %v", err)
	}

	if extractedRole != role {
		t.Errorf("Role mismatch: got %s, want %s", extractedRole, role)
	}

	// Test with invalid token
	_, err = manager.ExtractUserRole("invalid.token")
	if err == nil {
		t.Error("Expected error for invalid token")
	}
}

func TestGenerateSecureSecret(t *testing.T) {
	secret1, err := GenerateSecureSecret()
	if err != nil {
		t.Fatalf("GenerateSecureSecret failed: %v", err)
	}

	if len(secret1) != 64 { // 32 bytes = 64 hex characters
		t.Errorf("Secret length mismatch: got %d, want 64", len(secret1))
	}

	secret2, err := GenerateSecureSecret()
	if err != nil {
		t.Fatalf("GenerateSecureSecret failed: %v", err)
	}

	if secret1 == secret2 {
		t.Error("Generated secrets should be unique")
	}
}

func TestIsTokenExpired(t *testing.T) {
	manager := NewJWTManager("test-access-secret", "test-refresh-secret")

	userID := int64(123)
	role := "verified"
	email := "test@example.com"

	tokenPair, err := manager.GenerateTokens(userID, role, email)
	if err != nil {
		t.Fatalf("GenerateTokens failed: %v", err)
	}

	// Test valid token (should not be expired)
	if IsTokenExpired(tokenPair.AccessToken) {
		t.Error("Valid token should not be expired")
	}

	// Test invalid token format
	if !IsTokenExpired("invalid.token") {
		t.Error("Invalid token should be considered expired")
	}

	// Test empty token
	if !IsTokenExpired("") {
		t.Error("Empty token should be considered expired")
	}
}

func TestGetTokenExpirationTime(t *testing.T) {
	manager := NewJWTManager("test-access-secret", "test-refresh-secret")

	userID := int64(123)
	role := "verified"
	email := "test@example.com"

	tokenPair, err := manager.GenerateTokens(userID, role, email)
	if err != nil {
		t.Fatalf("GenerateTokens failed: %v", err)
	}

	expirationTime, err := GetTokenExpirationTime(tokenPair.AccessToken)
	if err != nil {
		t.Fatalf("GetTokenExpirationTime failed: %v", err)
	}

	if expirationTime.Before(time.Now()) {
		t.Error("Token expiration time should be in the future")
	}

	// Test invalid token
	_, err = GetTokenExpirationTime("invalid.token")
	if err == nil {
		t.Error("Expected error for invalid token")
	}
}

func TestTokenExpiration(t *testing.T) {
	// Create a manager with very short token duration for testing
	manager := NewJWTManager("test-access-secret", "test-refresh-secret")

	// Create a token with custom short expiration
	now := time.Now()
	claims := &CustomClaims{
		UserID: 123,
		Role:   "verified",
		Email:  "test@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(-time.Second)), // Already expired
			IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)),
			Issuer:    "loft-dughairi",
			Subject:   "user:123",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	expiredTokenString, err := token.SignedString(manager.accessSecret)
	if err != nil {
		t.Fatalf("Failed to create expired token: %v", err)
	}

	// Test validation of expired token
	_, err = manager.ValidateAccessToken(expiredTokenString)
	if err != ErrExpiredToken {
		t.Errorf("Expected ErrExpiredToken, got %v", err)
	}
}

// Helper function to generate a token with wrong secret for testing
func generateTokenWithWrongSecret(t *testing.T, userID int64, role, email string) string {
	wrongManager := NewJWTManager("wrong-secret", "wrong-refresh-secret")
	tokenPair, err := wrongManager.GenerateTokens(userID, role, email)
	if err != nil {
		t.Fatalf("Failed to generate token with wrong secret: %v", err)
	}
	return tokenPair.AccessToken
}

func BenchmarkGenerateTokens(b *testing.B) {
	manager := NewJWTManager("test-access-secret", "test-refresh-secret")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.GenerateTokens(123, "verified", "test@example.com")
		if err != nil {
			b.Fatalf("GenerateTokens failed: %v", err)
		}
	}
}

func BenchmarkValidateAccessToken(b *testing.B) {
	manager := NewJWTManager("test-access-secret", "test-refresh-secret")
	tokenPair, err := manager.GenerateTokens(123, "verified", "test@example.com")
	if err != nil {
		b.Fatalf("GenerateTokens failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.ValidateAccessToken(tokenPair.AccessToken)
		if err != nil {
			b.Fatalf("ValidateAccessToken failed: %v", err)
		}
	}
}
