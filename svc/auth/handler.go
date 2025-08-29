// Package auth provides authentication and authorization services
package auth

import (
	"context"
	"strconv"
	"strings"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
)

// AuthData represents the authentication data passed to authenticated endpoints
type AuthData struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
	Email  string `json:"email"`
}

// AuthHandler validates JWT tokens and returns user authentication data
//
//encore:authhandler
func (s *Service) AuthHandler(ctx context.Context, token string) (auth.UID, *AuthData, error) {
	// Remove "Bearer " prefix if present
	token = strings.TrimPrefix(token, "Bearer ")

	// Validate the access token
	claims, err := s.jwtManager.ValidateAccessToken(token)
	if err != nil {
		return "", nil, &errs.Error{
			Code:    errs.Unauthenticated,
			Message: "Invalid or expired access token.",
		}
	}

	// Check if user still exists and is active
	exists, err := s.repo.UserExistsByID(ctx, claims.UserID)
	if err != nil || !exists {
		return "", nil, &errs.Error{
			Code:    errs.Unauthenticated,
			Message: "User account not found or inactive.",
		}
	}

	// Convert user ID to string for auth.UID
	userIDStr := strconv.FormatInt(claims.UserID, 10)

	// Return authentication data
	authData := &AuthData{
		UserID: claims.UserID,
		Role:   claims.Role,
		Email:  claims.Email,
	}

	return auth.UID(userIDStr), authData, nil
}
