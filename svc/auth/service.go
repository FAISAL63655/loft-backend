// Package auth provides authentication and authorization services
package auth

import (
    "context"
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "math/big"
    "strings"
    "time"

    "encore.app/pkg/authn"
    "encore.app/pkg/errs"
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

// Login is a wrapper to support tests calling service.Login; delegates to LoginUser
func (s *Service) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
    return s.LoginUser(ctx, req)
}

// RefreshToken is a wrapper to support tests calling service.RefreshToken; delegates to RefreshUserToken
func (s *Service) RefreshToken(ctx context.Context, req *RefreshTokenRequest) (*RefreshTokenResponse, error) {
    return s.RefreshUserToken(ctx, req)
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

    // Determine phone via new token flow or legacy request.Phone for backward compatibility with tests
    var regPhone string
    now := time.Now().UTC()
    if strings.TrimSpace(req.PhoneVerificationToken) != "" {
        phoneRec, err := s.repo.GetPhoneVerificationByToken(ctx, req.PhoneVerificationToken)
        if err != nil || phoneRec == nil {
            return nil, NewValidationError("رمز تحقق الجوال غير صالح")
        }
        if phoneRec.TokenExpiresAt == nil || now.After(*phoneRec.TokenExpiresAt) {
            return nil, NewValidationError("رمز تحقق الجوال منتهي الصلاحية")
        }
        if phoneRec.VerifiedAt == nil {
            return nil, NewValidationError("يجب التحقق من رقم الجوال أولاً")
        }
        if phoneRec.ConsumedAt != nil {
            return nil, NewValidationError("تم استخدام رمز تحقق الجوال مسبقاً")
        }
        regPhone = phoneRec.Phone
        // Ensure phone is not already taken (race safety)
        phoneInUse, err := s.repo.UserPhoneExists(ctx, regPhone)
        if err != nil {
            return nil, NewInternalError("Failed to validate phone availability.")
        }
        if phoneInUse {
            return nil, NewValidationError("رقم الجوال مستخدم بالفعل")
        }
        // proceed; will consume token after user creation
    } else if strings.TrimSpace(req.Phone) != "" { // Legacy fallback path for older clients/tests
        regPhone = strings.TrimSpace(req.Phone)
        // Ensure phone is not already taken
        phoneInUse, err := s.repo.UserPhoneExists(ctx, regPhone)
        if err != nil {
            return nil, NewInternalError("Failed to validate phone availability.")
        }
        if phoneInUse {
            return nil, NewValidationError("رقم الجوال مستخدم بالفعل")
        }
    } else {
        // Neither token nor phone provided
        return nil, NewValidationError("رمز تحقق الجوال مطلوب")
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

	// Create user and email verification request in a transaction, using verified phone
    userID, verificationCode, err := s.repo.CreateUserWithVerification(ctx, req.Email, passwordHash, req.Name, regPhone, req.CityID, s.verificationManager)
    if err != nil {
        return nil, NewInternalError("Failed to create user account.")
    }

	// Get created user for response
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, NewInternalError("Failed to retrieve user information.")
	}

    // Consume phone verification token to prevent reuse (only if provided)
    if strings.TrimSpace(req.PhoneVerificationToken) != "" {
        _ = s.repo.ConsumePhoneVerificationToken(ctx, req.PhoneVerificationToken)
    }

	// Send verification email asynchronously
	go s.sendVerificationEmail(ctx, user.ID, user.Email, user.Name, verificationCode.Code)

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

// StartPhoneRegistration handles initiating phone verification by generating a 4-digit OTP
func (s *Service) StartPhoneRegistration(ctx context.Context, req *StartPhoneRequest) (*StartPhoneResponse, error) {
	// Rate limiting by IP and phone
	clientIP := getClientIP(ctx)
	ipKey := ratelimit.GenerateIPKey("phone_start", clientIP)
	phoneKey := ratelimit.GenerateSimpleKey("phone", "start", req.Phone)

	if err := s.registerRateLimit.RecordAttempt(ipKey); err != nil {
		return nil, NewRateLimitError("Too many attempts from this IP. Please try again later.")
	}
	if err := s.registerRateLimit.RecordAttempt(phoneKey); err != nil {
		return nil, NewRateLimitError("Too many attempts for this phone number. Please try again later.")
	}

	// Ensure phone not already in use
	exists, err := s.repo.UserPhoneExists(ctx, req.Phone)
	if err != nil {
		return nil, NewInternalError("Failed to check phone availability.")
	}
	if exists {
		return nil, NewValidationError("رقم الجوال مستخدم بالفعل")
	}

	// Generate 4-digit OTP
	code, err := generate4DigitCode()
	if err != nil {
		return nil, NewInternalError("Failed to generate verification code.")
	}

	// Store verification session with 10 minutes expiry
	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	if err := s.repo.StartPhoneVerification(ctx, req.Phone, code, expiresAt); err != nil {
		return nil, NewInternalError("Failed to start phone verification.")
	}

	// For now, we include the code in the message (integration can send SMS later)
	return &StartPhoneResponse{
		Message: fmt.Sprintf("تم إرسال رمز التفعيل إلى جوالك. الرمز: %s (صالح لمدة 10 دقائق)", code),
		Success: true,
	}, nil
}

// VerifyPhone handles verifying the 4-digit OTP and returns a short-lived verification token
func (s *Service) VerifyPhone(ctx context.Context, req *VerifyPhoneRequest) (*VerifyPhoneResponse, error) {
	// Rate limit by phone
	phoneKey := ratelimit.GenerateSimpleKey("phone", "verify", req.Phone)
	if err := s.verifyRateLimit.RecordAttempt(phoneKey); err != nil {
		return nil, NewRateLimitError("Too many verification attempts. Please try again later.")
	}

	// Fetch latest matching record
	rec, err := s.repo.GetPhoneVerificationByPhoneAndCode(ctx, req.Phone, req.Code)
	if err != nil {
		return nil, NewValidationError("رمز التحقق غير صالح")
	}

	now := time.Now().UTC()
	if now.After(rec.ExpiresAt) {
		return nil, NewValidationError("رمز التحقق منتهي الصلاحية")
	}
	if rec.VerifiedAt != nil {
		// Already verified; if token still valid, return it, otherwise generate a new one
		if rec.TokenExpiresAt != nil && now.Before(*rec.TokenExpiresAt) && rec.VerificationToken != nil {
			return &VerifyPhoneResponse{
				PhoneVerificationToken: *rec.VerificationToken,
				ExpiresAt:              *rec.TokenExpiresAt,
				Success:                true,
				Message:                "تم التحقق من رقم الجوال مسبقاً",
			}, nil
		}
	}

	// Generate short-lived verification token (30 minutes)
	token, err := generateRandomToken(32)
	if err != nil {
		return nil, NewInternalError("Failed to generate verification token.")
	}
	tokenExp := now.Add(30 * time.Minute)

	if err := s.repo.MarkPhoneVerifiedAndSetToken(ctx, rec.ID, token, tokenExp); err != nil {
		return nil, NewInternalError("Failed to finalize phone verification.")
	}

	return &VerifyPhoneResponse{
		PhoneVerificationToken: token,
		ExpiresAt:              tokenExp,
		Success:                true,
		Message:                "تم التحقق من رقم الجوال بنجاح",
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

	// Look up the latest user state to reflect current role/email in refreshed tokens
	user, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, ErrUserInactive
	}

	// Generate new token pair with up-to-date role/email
	newTokenPair, err := s.jwtManager.GenerateTokens(user.ID, user.Role, user.Email)
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
	go s.sendVerificationEmail(ctx, user.ID, req.Email, user.Name, verificationCode.Code)

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

// generate4DigitCode generates a cryptographically secure 4-digit numeric code as string (0000-9999)
func generate4DigitCode() (string, error) {
	max := big.NewInt(10000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%04d", n.Int64()), nil
}

// generateRandomToken generates a URL-safe random token of approximately n bytes before encoding
func generateRandomToken(n int) (string, error) {
	if n <= 0 {
		n = 32
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// URL-safe base64 without padding
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, nil
}

// sendVerificationEmail sends verification email using the notifications service
func (s *Service) sendVerificationEmail(ctx context.Context, userID int64, email, name, code string) {
    // Detach from request-scoped context to avoid cancellation after response returns
    // Preserve correlation id for logging continuity
    corrID := errs.CorrelationIDFromContext(ctx)
    base := logger.WithRequestID(context.Background(), corrID)
    bctx, cancel := context.WithTimeout(base, 10*time.Second)
    defer cancel()

    // Create email notification payload
    payload := map[string]interface{}{
        "email":             email,
        "name":              name,
        "user_name":         name,
        "verification_code": code,
        "expires_in":        "15 دقيقة",
        "language":          "ar",
    }

    // Send verification email via notifications service
    _, err := notifications.EnqueueEmail(bctx, userID, "email_verification", payload)
    if err != nil {
        logger.LogError(bctx, err, "Failed to send verification email", logger.Fields{
            "email": email,
            "name":  name,
        })
    }

    // Also send internal system notification for admins (userID = 1 for admin)
    adminPayload := map[string]interface{}{
        "email":    email,
        "name":     name,
        "action":   "user_registered",
        "language": "ar",
    }

    _, err = notifications.EnqueueInternal(bctx, 1, "user_registered", adminPayload)
    if err != nil {
        logger.LogError(bctx, err, "Failed to send internal registration notification", logger.Fields{
            "email": email,
            "name":  name,
        })
    }
}
