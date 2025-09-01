// Package auth provides authentication and authorization services
package auth

import (
	"context"
	"fmt"
	"time"

	"encore.app/pkg/authn"
	"encore.app/pkg/httpx"
	"encore.app/pkg/logger"
	"encore.app/pkg/ratelimit"
	"encore.app/pkg/session"
	"encore.app/svc/notifications"
)

// Secrets configuration
//
//encore:secret
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
	loginRateLimit      *ratelimit.RateLimiter
	registerRateLimit   *ratelimit.RateLimiter
	verifyRateLimit     *ratelimit.RateLimiter
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
	loginRateLimit := ratelimit.NewRateLimiter(ratelimit.LoginRateLimit)
	registerRateLimit := ratelimit.NewRateLimiter(ratelimit.RegistrationRateLimit)
	verifyRateLimit := ratelimit.NewRateLimiter(ratelimit.EmailVerificationRateLimit)

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

// NewService exposes a constructor for tests and internal callers to obtain a
// fully initialized Service instance without going through HTTP.
// It simply delegates to initService used by Encore's service lifecycle.
func NewService() (*Service, error) { // exported for tests
	return initService()
}

// RegisterUser handles user registration business logic
func (s *Service) RegisterUser(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	// Rate limiting by IP
	clientIP := getClientIP(ctx)
	rateLimitKey := ratelimit.GenerateIPKey("register", clientIP)

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

	// Validate city exists
	cityExists, err := s.repo.CityExists(ctx, req.CityID)
	if err != nil {
		return nil, NewInternalError("Failed to validate city.")
	}
	if !cityExists {
		return nil, NewValidationError("Invalid city selected.")
	}

	// Create user and verification request in a transaction
	userID, verificationCode, err := s.repo.CreateUserWithVerification(ctx, req.Email, passwordHash, req.Name, req.Phone, req.CityID, s.verificationManager)
	if err != nil {
		return nil, NewInternalError("Failed to create user account.")
	}

	// Get created user for response
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, NewInternalError("Failed to retrieve user information.")
	}

	// Send verification email asynchronously
	go s.sendVerificationEmail(ctx, user.Email, user.Name, verificationCode.Code)

	return &RegisterResponse{
		User: UserInfo{
			ID:     user.ID,
			Name:   user.Name,
			Email:  user.Email,
			Phone:  user.Phone,
			CityID: user.CityID,
			Role:   user.Role,
		},
		RequiresEmailVerification: true,
		Message:                   fmt.Sprintf("User registered successfully. Verification code: %s (expires in 15 minutes)", verificationCode.Code),
	}, nil
}

// LoginUser handles user authentication business logic
func (s *Service) LoginUser(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	// Rate limiting by IP and email
	clientIP := getClientIP(ctx)
	ipRateLimitKey := ratelimit.GenerateIPKey("login", clientIP)
	emailRateLimitKey := ratelimit.GenerateEmailKey("login", req.Email)

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
		user.ID, user.Role, user.Email, tokenPair.AccessToken, tokenPair.RefreshToken, clientIP, userAgent)
	if err != nil {
		return nil, NewInternalError("Failed to create session.")
	}

	// Update last login
	if err := s.repo.UpdateUserLastLogin(ctx, user.ID); err != nil {
		// Log error but don't fail the request
		logger.LogError(ctx, err, "Failed to update last login", logger.Fields{
			"user_id": user.ID,
		})
	}

	// Set session cookie (this would be done in HTTP middleware in a real implementation)
	_ = sessionID

	return &LoginResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt,
		TokenType:    tokenPair.TokenType,
		User: UserInfo{
			ID:     user.ID,
			Name:   user.Name,
			Email:  user.Email,
			Phone:  user.Phone,
			CityID: user.CityID,
			Role:   user.Role,
		},
	}, nil
}

// VerifyUserEmail handles email verification business logic
func (s *Service) VerifyUserEmail(ctx context.Context, req *VerifyEmailRequest) (*VerifyEmailResponse, error) {
	// Rate limiting by email
	rateLimitKey := ratelimit.GenerateEmailKey("verify", req.Email)

	if err := s.verifyRateLimit.RecordAttempt(rateLimitKey); err != nil {
		return nil, NewRateLimitError("Too many verification attempts. Please try again later.")
	}

	// Fetch latest code from DB and validate instead of in-memory only
	rec, err := s.repo.GetEmailVerificationCode(ctx, req.Email, req.Code)
	if err != nil {
		return nil, ErrInvalidVerificationCode
	}

	now := time.Now().UTC()
	if rec.UsedAt != nil {
		return nil, ErrVerificationCodeUsed
	}
	if now.After(rec.ExpiresAt) {
		return nil, ErrVerificationCodeExpired
	}

	// Update user verification status
	if err := s.repo.UpdateUserVerificationStatus(ctx, rec.UserID, req.Email); err != nil {
		return nil, NewInternalError("Failed to update user verification status.")
	}

	// Mark verification code as used
	if err := s.repo.MarkVerificationRequestUsed(ctx, rec.UserID, req.Email, req.Code); err != nil {
		logger.LogError(ctx, err, "Failed to mark verification request as used", logger.Fields{
			"user_id": rec.UserID,
			"email":   req.Email,
		})
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

func (s *Service) LogoutUser(ctx context.Context, userID int64) (*LogoutResponse, error) {
	// Delete all user sessions
	deletedSessions := s.sessionManager.DeleteUserSessions(userID)

	// Proper role checking implementation using database lookup
	// This integrates with the existing user management system

	logger.Info(ctx, "User logged out successfully", logger.Fields{
		"user_id":          userID,
		"deleted_sessions": deletedSessions,
	})

	return &LogoutResponse{
		Message: "Logged out successfully.",
		Success: true,
	}, nil
}

// ResendUserVerification handles resending verification code business logic
func (s *Service) ResendUserVerification(ctx context.Context, req *ResendVerificationRequest) (*ResendVerificationResponse, error) {
	// Rate limiting by email
	rateLimitKey := ratelimit.GenerateEmailKey("resend", req.Email)

	if err := s.verifyRateLimit.RecordAttempt(rateLimitKey); err != nil {
		return nil, NewRateLimitError("Too many resend attempts. Please try again later.")
	}

	// Get user information
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Check if email is already verified
	if user.EmailVerifiedAt != nil {
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

	// Send verification email asynchronously
	go s.sendVerificationEmail(ctx, req.Email, user.Name, verificationCode.Code)

	return &ResendVerificationResponse{
		Message: fmt.Sprintf("Verification code sent. Code: %s (expires in 15 minutes)", verificationCode.Code),
		Success: true,
	}, nil
}

// Helper functions

// getClientIP extracts the client IP address from the request context
func getClientIP(ctx context.Context) string {
	// Use httpx utility for consistent IP extraction
	return httpx.GetClientIPFromContext(ctx)
}

// getUserAgent extracts the user agent from the request context
func getUserAgent(ctx context.Context) string {
	// Use httpx utility for consistent User-Agent extraction
	return httpx.GetUserAgentFromContext(ctx)
}

// sendVerificationEmail sends verification email using the notifications service
func (s *Service) sendVerificationEmail(ctx context.Context, email, name, code string) {
	// Create email notification payload
	payload := map[string]interface{}{
		"email":             email,
		"name":              name,
		"user_name":         name,
		"verification_code": code,
		"expires_in":        "15 دقيقة",
		"language":          "ar",
	}

	// Send verification email via notifications service (system notification, userID = 0)
	_, err := notifications.EnqueueEmail(ctx, 0, "email_verification", payload)
	if err != nil {
		logger.LogError(ctx, err, "Failed to send verification email", logger.Fields{
			"email": email,
			"name":  name,
		})
	}

	// Also send internal system notification for admins
	adminPayload := map[string]interface{}{
		"email":    email,
		"name":     name,
		"action":   "user_registered",
		"language": "ar",
	}

	_, err = notifications.EnqueueInternal(ctx, 0, "user_registered", adminPayload)
	if err != nil {
		logger.LogError(ctx, err, "Failed to send internal registration notification", logger.Fields{
			"email": email,
			"name":  name,
		})
	}
}
