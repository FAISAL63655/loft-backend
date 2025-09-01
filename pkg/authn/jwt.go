package authn

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Token durations
const (
	// AccessTokenDuration is the lifetime of access tokens (20 minutes)
	AccessTokenDuration = 20 * time.Minute
	// RefreshTokenDuration is the lifetime of refresh tokens (60 days)
	RefreshTokenDuration = 60 * 24 * time.Hour
)

// JWT errors
var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrInvalidClaims    = errors.New("invalid token claims")
	ErrTokenNotFound    = errors.New("token not found")
	ErrInvalidSignature = errors.New("invalid token signature")
)

// CustomClaims represents the custom JWT claims for our application
type CustomClaims struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// TokenPair represents a pair of access and refresh tokens
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// JWTManager handles JWT token operations
type JWTManager struct {
	accessSecret  []byte
	refreshSecret []byte
}

// NewJWTManager creates a new JWT manager with the provided secrets
func NewJWTManager(accessSecret, refreshSecret string) *JWTManager {
	return &JWTManager{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
	}
}

// GenerateTokens creates a new access and refresh token pair for the user
func (j *JWTManager) GenerateTokens(userID int64, role, email string) (*TokenPair, error) {
	now := time.Now().UTC()

	// Generate access token
	accessClaims := &CustomClaims{
		UserID: userID,
		Role:   role,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenDuration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "loft-dughairi",
			Subject:   fmt.Sprintf("user:%d", userID),
			ID:        uuid.New().String(), // Unique token ID
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(j.accessSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	// Generate refresh token
	refreshClaims := &CustomClaims{
		UserID: userID,
		Role:   role,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(RefreshTokenDuration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "loft-dughairi",
			Subject:   fmt.Sprintf("user:%d", userID),
			ID:        uuid.New().String(), // Unique token ID
		},
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString(j.refreshSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresAt:    now.Add(AccessTokenDuration),
		TokenType:    "Bearer",
	}, nil
}

// ValidateAccessToken validates an access token and returns the claims
func (j *JWTManager) ValidateAccessToken(tokenString string) (*CustomClaims, error) {
	return j.validateToken(tokenString, j.accessSecret)
}

// ValidateRefreshToken validates a refresh token and returns the claims
func (j *JWTManager) ValidateRefreshToken(tokenString string) (*CustomClaims, error) {
	return j.validateToken(tokenString, j.refreshSecret)
}

// validateToken validates a token with the given secret
func (j *JWTManager) validateToken(tokenString string, secret []byte) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		if errors.Is(err, jwt.ErrSignatureInvalid) {
			return nil, ErrInvalidSignature
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidClaims
	}
	// تشديد التحقق: التأكد من المُصدِر
	const expectedIssuer = "loft-dughairi"
	if claims.Issuer != expectedIssuer {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// RefreshTokens generates new tokens using a valid refresh token
func (j *JWTManager) RefreshTokens(refreshTokenString string) (*TokenPair, error) {
	// Validate the refresh token
	claims, err := j.ValidateRefreshToken(refreshTokenString)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// Generate new token pair
	return j.GenerateTokens(claims.UserID, claims.Role, claims.Email)
}

// ExtractUserID extracts the user ID from a valid access token
func (j *JWTManager) ExtractUserID(tokenString string) (int64, error) {
	claims, err := j.ValidateAccessToken(tokenString)
	if err != nil {
		return 0, err
	}
	return claims.UserID, nil
}

// ExtractUserRole extracts the user role from a valid access token
func (j *JWTManager) ExtractUserRole(tokenString string) (string, error) {
	claims, err := j.ValidateAccessToken(tokenString)
	if err != nil {
		return "", err
	}
	return claims.Role, nil
}

// GenerateSecureSecret generates a cryptographically secure secret for JWT signing
func GenerateSecureSecret() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure secret: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// IsTokenExpired checks if a token is expired by decoding payload without verifying signature
func IsTokenExpired(tokenString string) bool {
	exp, err := getTokenExpUnverified(tokenString)
	if err != nil {
		return true
	}
	return time.Now().UTC().After(exp)
}

// GetTokenExpirationTime returns the expiration time by decoding payload without verifying signature
func GetTokenExpirationTime(tokenString string) (time.Time, error) {
	return getTokenExpUnverified(tokenString)
}

// getTokenExpUnverified decodes JWT payload (base64url) and extracts exp without signature verification
func getTokenExpUnverified(tokenString string) (time.Time, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return time.Time{}, ErrInvalidToken
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, ErrInvalidToken
	}
	var p struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(b, &p); err != nil || p.Exp == 0 {
		return time.Time{}, ErrInvalidClaims
	}
	return time.Unix(p.Exp, 0), nil
}
