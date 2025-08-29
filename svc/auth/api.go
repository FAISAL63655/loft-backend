// Package auth provides authentication and authorization services
package auth

import (
	"context"
	"strconv"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
)

// Register creates a new user account
//
//encore:api public method=POST path=/auth/register
func (s *Service) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	return s.RegisterUser(ctx, req)
}

// Login authenticates a user and returns JWT tokens
//
//encore:api public method=POST path=/auth/login
func (s *Service) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	return s.LoginUser(ctx, req)
}

// VerifyEmail verifies a user's email address
//
//encore:api public method=POST path=/auth/verify-email
func (s *Service) VerifyEmail(ctx context.Context, req *VerifyEmailRequest) (*VerifyEmailResponse, error) {
	return s.VerifyUserEmail(ctx, req)
}

// RefreshToken generates new access and refresh tokens
//
//encore:api public method=POST path=/auth/refresh
func (s *Service) RefreshToken(ctx context.Context, req *RefreshTokenRequest) (*RefreshTokenResponse, error) {
	return s.RefreshUserToken(ctx, req)
}

// Logout invalidates the user's session and tokens
//
//encore:api auth method=POST path=/auth/logout
func (s *Service) Logout(ctx context.Context) (*LogoutResponse, error) {
	// Get user ID from auth context
	userID, _ := auth.UserID()

	// Convert auth.UID (string) to int64
	userIDInt64, err := strconv.ParseInt(string(userID), 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.Internal,
			Message: "Invalid user ID format.",
		}
	}

	return s.LogoutUser(ctx, userIDInt64)
}

// ResendVerification resends the email verification code
//
//encore:api public method=POST path=/auth/resend-verification
func (s *Service) ResendVerification(ctx context.Context, req *ResendVerificationRequest) (*ResendVerificationResponse, error) {
	return s.ResendUserVerification(ctx, req)
}
