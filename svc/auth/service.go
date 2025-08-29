// Package auth provides authentication and authorization services
package auth

import (
	"context"
	"fmt"
	"time"

	"encore.app/pkg/authn"
	"encore.app/pkg/rate_limit"
	"encore.app/pkg/session"
)

// Secrets configuration
var secrets struct {
	JWTAccessSecret  string
	JWTRefreshSecret string
}

// Service represents the authentication service
//
//encore:service
type Service struct {
	repo                *Repository
	jwtManager          *authn.JWTManager
	sessionManager      *session.SessionManager
	verificationManager *authn.VerificationManager
	loginRateLimit      *rate_limit.RateLimiter
	registerRateLimit   *rate_limit.RateLimiter
	verifyRateLimit     *rate_limit.RateLimiter
}

// Initialize the authentication service
func initService() (*Service, error) {
	// Initialize repository
	repo := NewRepository()

	// Initialize JWT manager with secure secrets from Encore secrets
	jwtManager := authn.NewJWTManager(secrets.JWTAccessSecret, secrets.JWTRefreshSecret)

	// Initialize session manager
	sessionConfig := session.ProductionSessionConfig
	sessionManager := session.NewSessionManager(sessionConfig)

	// Initialize verification manager
	verificationManager := authn.NewVerificationManager()

	// Initialize rate limiters
	loginRateLimit := rate_limit.NewRateLimiter(rate_limit.LoginRateLimit)
	registerRateLimit := rate_limit.NewRateLimiter(rate_limit.RegistrationRateLimit)
	verifyRateLimit := rate_limit.NewRateLimiter(rate_limit.EmailVerificationRateLimit)

	return &Service{
		repo:                repo,
		jwtManager:          jwtManager,
		sessionManager:      sessionManager,
		verificationManager: verificationManager,
		loginRateLimit:      loginRateLimit,
		registerRateLimit:   registerRateLimit,
		verifyRateLimit:     verifyRateLimit,
	}, nil
}

// RegisterUser handles user registration business logic
func (s *Service) RegisterUser(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	// Rate limiting by IP
	clientIP := getClientIP(ctx)
	rateLimitKey := rate_limit.GenerateIPKey("register", clientIP)

	if err := s.registerRateLimit.RecordAttempt(rateLimitKey); err != nil {
		return nil, NewRateLimitError("Too many registration attempts. Please try again later.")
	}

	// Validate password strength
	if !authn.IsValidPassword(req.Password) {
		return nil, ErrWeakPassword
	}

	// Check if user already exists
	exists, err := s.repo.UserExists(ctx, req.Email)
	if err != nil {
		return nil, NewInternalError("Failed to check user existence.")
	}
	if exists {
		return nil, ErrUserAlreadyExists
	}

	// Hash password
	passwordHash, err := authn.HashPassword(req.Password)
	if err != nil {
		return nil, NewInternalError("Failed to process password.")
	}

	// Create user
	userID, err := s.repo.CreateUser(ctx, req.Email, passwordHash, req.Name)
	if err != nil {
		return nil, NewInternalError("Failed to create user account.")
	}

	// Generate verification code
	verificationCode, err := s.verificationManager.CreateVerificationCode(userID, req.Email)
	if err != nil {
		return nil, NewInternalError("Failed to generate verification code.")
	}

	// Store verification request
	err = s.repo.CreateVerificationRequest(ctx, userID, req.Email, verificationCode.Code, verificationCode.ExpiresAt)
	if err != nil {
		return nil, NewInternalError("Failed to store verification request.")
	}

	// TODO: Send verification email
	// For now, we'll just return the code in development

	return &RegisterResponse{
		Message: fmt.Sprintf("User registered successfully. Verification code: %s (expires in 15 minutes)", verificationCode.Code),
		UserID:  userID,
	}, nil
}

// LoginUser handles user authentication business logic
func (s *Service) LoginUser(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	// Rate limiting by IP and email
	clientIP := getClientIP(ctx)
	ipRateLimitKey := rate_limit.GenerateIPKey("login", clientIP)
	emailRateLimitKey := rate_limit.GenerateEmailKey("login", req.Email)

	// Check both IP and email rate limits
	if err := s.loginRateLimit.RecordAttempt(ipRateLimitKey); err != nil {
		return nil, NewRateLimitError("Too many login attempts from this IP. Please try again later.")
	}

	if err := s.loginRateLimit.RecordAttempt(emailRateLimitKey); err != nil {
		return nil, NewRateLimitError("Too many login attempts for this email. Please try again later.")
	}

	// Get user from database
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	// Verify password
	if err := authn.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		return nil, ErrInvalidCredentials
	}

	// Generate JWT tokens
	tokenPair, err := s.jwtManager.GenerateTokens(user.ID, user.Role, user.Email)
	if err != nil {
		return nil, NewInternalError("Failed to generate authentication tokens.")
	}

	// Create session
	userAgent := getUserAgent(ctx)
	sessionID, _, err := s.sessionManager.CreateSession(
		user.ID, user.Role, user.Email, tokenPair.RefreshToken, clientIP, userAgent)
	if err != nil {
		return nil, NewInternalError("Failed to create session.")
	}

	// Update last login
	if err := s.repo.UpdateUserLastLogin(ctx, user.ID); err != nil {
		// Log error but don't fail the request
		fmt.Printf("Failed to update last login for user %d: %v\n", user.ID, err)
	}

	// Set session cookie (this would be done in HTTP middleware in a real implementation)
	_ = sessionID

	return &LoginResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt,
		TokenType:    tokenPair.TokenType,
		User: UserInfo{
			ID:    user.ID,
			Email: user.Email,
			Role:  user.Role,
			Name:  user.Name,
		},
	}, nil
}

// VerifyUserEmail handles email verification business logic
func (s *Service) VerifyUserEmail(ctx context.Context, req *VerifyEmailRequest) (*VerifyEmailResponse, error) {
	// Rate limiting by email
	rateLimitKey := rate_limit.GenerateEmailKey("verify", req.Email)

	if err := s.verifyRateLimit.RecordAttempt(rateLimitKey); err != nil {
		return nil, NewRateLimitError("Too many verification attempts. Please try again later.")
	}

	// Verify the code
	verificationCode, err := s.verificationManager.VerifyCode(req.Email, req.Code)
	if err != nil {
		switch err {
		case authn.ErrCodeExpired:
			return nil, ErrVerificationCodeExpired
		case authn.ErrCodeInvalid:
			return nil, ErrInvalidVerificationCode
		case authn.ErrCodeAlreadyUsed:
			return nil, ErrVerificationCodeUsed
		case authn.ErrTooManyAttempts:
			return nil, NewRateLimitError("Too many verification attempts. Please request a new code.")
		default:
			return nil, NewInternalError("Failed to verify code.")
		}
	}

	// Update user verification status
	err = s.repo.UpdateUserVerificationStatus(ctx, verificationCode.UserID, req.Email)
	if err != nil {
		return nil, NewInternalError("Failed to update user verification status.")
	}

	// Mark verification request as used
	err = s.repo.MarkVerificationRequestUsed(ctx, verificationCode.UserID, req.Email, req.Code)
	if err != nil {
		// Log error but don't fail the request
		fmt.Printf("Failed to mark verification request as used: %v\n", err)
	}

	return &VerifyEmailResponse{
		Message: "Email verified successfully.",
		Success: true,
	}, nil
}

// RefreshUserToken handles token refresh business logic
func (s *Service) RefreshUserToken(ctx context.Context, req *RefreshTokenRequest) (*RefreshTokenResponse, error) {
	// Validate refresh token
	claims, err := s.jwtManager.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}

	// Check if user still exists and is active
	exists, err := s.repo.UserExistsByID(ctx, claims.UserID)
	if err != nil || !exists {
		return nil, ErrUserInactive
	}

	// Generate new token pair
	newTokenPair, err := s.jwtManager.GenerateTokens(claims.UserID, claims.Role, claims.Email)
	if err != nil {
		return nil, NewInternalError("Failed to generate new tokens.")
	}

	return &RefreshTokenResponse{
		AccessToken:  newTokenPair.AccessToken,
		RefreshToken: newTokenPair.RefreshToken,
		ExpiresAt:    newTokenPair.ExpiresAt,
		TokenType:    newTokenPair.TokenType,
	}, nil
}

// LogoutUser handles user logout business logic
func (s *Service) LogoutUser(ctx context.Context, userID int64) (*LogoutResponse, error) {
	// Delete all user sessions
	deletedSessions := s.sessionManager.DeleteUserSessions(userID)

	// In a real implementation, you might also want to:
	// - Add tokens to a blacklist
	// - Log the logout event
	// - Clear session cookies

	fmt.Printf("Logged out user %d, deleted %d sessions\n", userID, deletedSessions)

	return &LogoutResponse{
		Message: "Logged out successfully.",
		Success: true,
	}, nil
}

// ResendUserVerification handles resending verification code business logic
func (s *Service) ResendUserVerification(ctx context.Context, req *ResendVerificationRequest) (*ResendVerificationResponse, error) {
	// Rate limiting by email
	rateLimitKey := rate_limit.GenerateEmailKey("resend", req.Email)

	if err := s.verifyRateLimit.RecordAttempt(rateLimitKey); err != nil {
		return nil, NewRateLimitError("Too many resend attempts. Please try again later.")
	}

	// Get user information
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Check if user is already verified
	if user.Role == "verified" || user.Role == "admin" {
		return nil, ErrEmailAlreadyVerified
	}

	// Generate new verification code
	verificationCode, err := s.verificationManager.CreateVerificationCode(user.ID, req.Email)
	if err != nil {
		switch err {
		case authn.ErrResendTooSoon:
			cooldown := s.verificationManager.GetResendCooldownRemaining(req.Email)
			return nil, NewRateLimitError(fmt.Sprintf("Please wait %v before requesting another code.", cooldown.Round(time.Second)))
		case authn.ErrMaxResendsReached:
			return nil, NewRateLimitError("Maximum resend attempts reached for this hour.")
		default:
			return nil, NewInternalError("Failed to generate verification code.")
		}
	}

	// Store verification request
	err = s.repo.CreateVerificationRequest(ctx, user.ID, req.Email, verificationCode.Code, verificationCode.ExpiresAt)
	if err != nil {
		return nil, NewInternalError("Failed to store verification request.")
	}

	// TODO: Send verification email
	// For now, we'll just return the code in development

	return &ResendVerificationResponse{
		Message: fmt.Sprintf("Verification code sent. Code: %s (expires in 15 minutes)", verificationCode.Code),
		Success: true,
	}, nil
}

// Helper functions

// getClientIP extracts the client IP address from the request context
func getClientIP(ctx context.Context) string {
	// In a real Encore application, you would extract this from the request
	// For now, we'll return a placeholder
	return "127.0.0.1"
}

// getUserAgent extracts the user agent from the request context
func getUserAgent(ctx context.Context) string {
	// In a real Encore application, you would extract this from the request
	// For now, we'll return a placeholder
	return "Encore-Client/1.0"
}
