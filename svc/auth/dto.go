// Package auth provides authentication and authorization services
package auth

import "time"

// RegisterRequest represents the user registration request
type RegisterRequest struct {
	Name     string `json:"name" validate:"required"`
	Email    string `json:"email" validate:"required,email"`
	Phone    string `json:"phone" validate:"required"` // إجباري
	CityID   int64  `json:"city_id" validate:"required,min=1"`
	Password string `json:"password" validate:"required,min=8"`
}

// RegisterResponse represents the user registration response
type RegisterResponse struct {
	User                      UserInfo `json:"user"`
	RequiresEmailVerification bool     `json:"requires_email_verification"`
	Message                   string   `json:"message,omitempty"`
}

// LoginRequest represents the user login request
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse represents the user login response
type LoginResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	User         UserInfo  `json:"user"`
}

// UserInfo represents user information
type UserInfo struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Phone  string `json:"phone"`
	CityID int64  `json:"city_id"`
	Role   string `json:"role"`
}

// VerifyEmailRequest represents the email verification request
type VerifyEmailRequest struct {
	Email string `json:"email" validate:"required,email"`
	Code  string `json:"code" validate:"required,len=6"`
}

// VerifyEmailResponse represents the email verification response
type VerifyEmailResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

// RefreshTokenRequest represents the token refresh request
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// RefreshTokenResponse represents the token refresh response
type RefreshTokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// LogoutResponse represents the logout response
type LogoutResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

// ResendVerificationRequest represents the resend verification request
type ResendVerificationRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ResendVerificationResponse represents the resend verification response
type ResendVerificationResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

// User represents a user entity from the database
type User struct {
	ID              int64
	Name            string
	Email           string
	Phone           string
	CityID          int64
	PasswordHash    string
	Role            string
	State           string
	EmailVerifiedAt *time.Time
}
