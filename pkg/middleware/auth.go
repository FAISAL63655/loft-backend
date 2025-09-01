// Package middleware provides authentication and authorization middleware for Encore.go
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"encore.app/pkg/authn"
	"encore.app/pkg/session"
	"encore.dev/beta/auth"
)

// Context keys for type safety
type contextKey string

const (
	UserContextKey      contextKey = "user"
	RequestIDContextKey contextKey = "request_id"
)

// Middleware errors
var (
	ErrMissingToken      = errors.New("missing authentication token")
	ErrInvalidToken      = errors.New("invalid authentication token")
	ErrInsufficientPerms = errors.New("insufficient permissions")
	ErrSessionRequired   = errors.New("valid session required")
)

// AuthConfig defines the configuration for authentication middleware
type AuthConfig struct {
	JWTManager     *authn.JWTManager
	SessionManager *session.SessionManager
	RequiredRoles  []string
	Optional       bool // If true, authentication is optional
}

// UserContext represents the authenticated user context
type UserContext struct {
	UserID    int64             `json:"user_id"`
	Role      string            `json:"role"`
	Email     string            `json:"email"`
	SessionID string            `json:"session_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// AuthMiddleware provides JWT-based authentication middleware for Encore.go
func AuthMiddleware(config AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Authorization header or session cookie
			token, sessionID, _, err := extractAuthInfo(r, config.SessionManager)

			if err != nil {
				if config.Optional {
					// Continue without authentication
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Validate JWT access token (secure)
			claims, err := config.JWTManager.ValidateAccessToken(token)

			if err != nil {
				// For optional auth, only allow missing tokens, not invalid ones
				if config.Optional && errors.Is(err, ErrMissingToken) {
					next.ServeHTTP(w, r)
					return
				}

				// Handle specific JWT errors
				switch {
				case errors.Is(err, authn.ErrExpiredToken):
					http.Error(w, "Token expired", http.StatusUnauthorized)
				case errors.Is(err, authn.ErrInvalidSignature):
					http.Error(w, "Invalid token signature", http.StatusUnauthorized)
				default:
					http.Error(w, "Invalid token", http.StatusUnauthorized)
				}
				return
			}

			// Check role requirements
			if len(config.RequiredRoles) > 0 {
				if !hasRequiredRole(claims.Role, config.RequiredRoles) {
					http.Error(w, "Insufficient permissions", http.StatusForbidden)
					return
				}
			}

			// Create user context
			userCtx := &UserContext{
				UserID:    claims.UserID,
				Role:      claims.Role,
				Email:     claims.Email,
				SessionID: sessionID,
			}

			// Add user context to request context using typed key
			ctx := context.WithValue(r.Context(), UserContextKey, userCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuth creates middleware that requires authentication
func RequireAuth(jwtManager *authn.JWTManager, sessionManager *session.SessionManager) func(http.Handler) http.Handler {
	return AuthMiddleware(AuthConfig{
		JWTManager:     jwtManager,
		SessionManager: sessionManager,
		Optional:       false,
	})
}

// OptionalAuth creates middleware that allows optional authentication
func OptionalAuth(jwtManager *authn.JWTManager, sessionManager *session.SessionManager) func(http.Handler) http.Handler {
	return AuthMiddleware(AuthConfig{
		JWTManager:     jwtManager,
		SessionManager: sessionManager,
		Optional:       true,
	})
}

// RequireRole creates middleware that requires specific roles
func RequireRole(jwtManager *authn.JWTManager, sessionManager *session.SessionManager, roles ...string) func(http.Handler) http.Handler {
	return AuthMiddleware(AuthConfig{
		JWTManager:     jwtManager,
		SessionManager: sessionManager,
		RequiredRoles:  roles,
		Optional:       false,
	})
}

// RequireAdmin creates middleware that requires admin role
func RequireAdmin(jwtManager *authn.JWTManager, sessionManager *session.SessionManager) func(http.Handler) http.Handler {
	return RequireRole(jwtManager, sessionManager, "admin")
}

// RequireVerified creates middleware that requires verified user role
func RequireVerified(jwtManager *authn.JWTManager, sessionManager *session.SessionManager) func(http.Handler) http.Handler {
	return RequireRole(jwtManager, sessionManager, "verified", "admin")
}

// extractAuthInfo extracts authentication information from the request
func extractAuthInfo(r *http.Request, sessionManager *session.SessionManager) (token, sessionID string, isFromSession bool, err error) {
	// Try to get token from Authorization header first
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1], "", false, nil
		}
	}

	// Try to get session from cookie
	if sessionManager != nil {
		sessionID, err := sessionManager.GetSessionFromRequest(r)
		if err != nil {
			return "", "", false, ErrMissingToken
		}

		sessionData, err := sessionManager.GetSession(sessionID)
		if err != nil {
			return "", "", false, ErrSessionRequired
		}

		// Use AccessToken from session for authentication (secure)
		if sessionData.AccessToken == "" {
			return "", "", false, ErrSessionRequired
		}
		return sessionData.AccessToken, sessionID, true, nil // true = isFromSession
	}

	return "", "", false, ErrMissingToken
}

// hasRequiredRole checks if the user has one of the required roles
func hasRequiredRole(userRole string, requiredRoles []string) bool {
	for _, role := range requiredRoles {
		if userRole == role {
			return true
		}
	}
	return false
}

// GetUserFromContext extracts the user context from the request context
func GetUserFromContext(ctx context.Context) (*UserContext, bool) {
	user, ok := ctx.Value(UserContextKey).(*UserContext)
	return user, ok
}

// GetUserIDFromContext extracts the user ID from the request context
func GetUserIDFromContext(ctx context.Context) (int64, bool) {
	user, ok := GetUserFromContext(ctx)
	if !ok {
		return 0, false
	}
	return user.UserID, true
}

// GetUserRoleFromContext extracts the user role from the request context
func GetUserRoleFromContext(ctx context.Context) (string, bool) {
	user, ok := GetUserFromContext(ctx)
	if !ok {
		return "", false
	}
	return user.Role, true
}

// IsAdmin checks if the current user is an admin
func IsAdmin(ctx context.Context) bool {
	role, ok := GetUserRoleFromContext(ctx)
	return ok && role == "admin"
}

// IsVerified checks if the current user is verified or admin
func IsVerified(ctx context.Context) bool {
	role, ok := GetUserRoleFromContext(ctx)
	return ok && (role == "verified" || role == "admin")
}

// Encore.go auth handler integration
// This function can be used as an Encore auth handler

// AuthHandler is an Encore auth handler that validates JWT tokens
func AuthHandler(ctx context.Context, token string) (auth.UID, *UserContext, error) {
	// This would be configured with your JWT manager instance
	// For now, we'll return an error indicating it needs to be configured
	return "", nil, errors.New("auth handler not configured")
}

// CreateEncoreAuthHandler creates an Encore auth handler with the provided JWT manager
func CreateEncoreAuthHandler(jwtManager *authn.JWTManager) func(context.Context, string) (auth.UID, *UserContext, error) {
	return func(ctx context.Context, token string) (auth.UID, *UserContext, error) {
		claims, err := jwtManager.ValidateAccessToken(token)
		if err != nil {
			switch {
			case errors.Is(err, authn.ErrExpiredToken):
				return "", nil, errors.New("token expired")
			case errors.Is(err, authn.ErrInvalidSignature):
				return "", nil, errors.New("invalid token signature")
			default:
				return "", nil, errors.New("invalid token")
			}
		}

		userCtx := &UserContext{
			UserID: claims.UserID,
			Role:   claims.Role,
			Email:  claims.Email,
		}

		// Convert user ID to auth.UID
		uid := auth.UID(claims.Subject)

		return uid, userCtx, nil
	}
}
