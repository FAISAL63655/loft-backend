// Package auth provides authentication and authorization services
package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"encore.app/pkg/authn"
	"encore.app/pkg/errs"
	encore "encore.dev"
	"encore.dev/beta/auth"
)

// Register creates a new user account
//
//encore:api public method=POST path=/auth/register
func (s *Service) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	return s.RegisterUser(ctx, req)
}

// StartPhone begins phone verification by generating and (for now) returning a 4-digit OTP
//
//encore:api public method=POST path=/auth/phone/start
func (s *Service) StartPhone(ctx context.Context, req *StartPhoneRequest) (*StartPhoneResponse, error) {
	return s.StartPhoneRegistration(ctx, req)
}

// VerifyPhone verifies the OTP and returns a short-lived token to be used in registration
//
//encore:api public method=POST path=/auth/phone/verify
func (s *Service) VerifyPhoneAPI(ctx context.Context, req *VerifyPhoneRequest) (*VerifyPhoneResponse, error) {
	return s.VerifyPhone(ctx, req)
}

// Login authenticates a user and sets HttpOnly refresh cookie; returns access token in JSON
//
//encore:api public raw method=POST path=/auth/login
func (s *Service) LoginRaw(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", "invalid request body")
		return
	}

	resp, err := s.LoginUser(ctx, &req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", err.Error())
		return
	}

	// Set refresh token as HttpOnly cookie
	setRefreshCookie(w, resp.RefreshToken)

	// Optionally set server session cookie (disabled by default)
	// if sessionCookieEnabled() {
	//   sid, _, _ := s.sessionManager.CreateSession(resp.User.ID, resp.User.Role, resp.User.Email, resp.AccessToken, resp.RefreshToken, getClientIP(ctx), getUserAgent(ctx))
	//   s.sessionManager.SetSessionCookie(w, sid)
	// }

	// Do not return refresh token in body
	resp.RefreshToken = ""
	writeJSON(w, resp)
}

// VerifyEmail verifies a user's email address
//
//encore:api public method=POST path=/auth/verify-email
func (s *Service) VerifyEmail(ctx context.Context, req *VerifyEmailRequest) (*VerifyEmailResponse, error) {
	return s.VerifyUserEmail(ctx, req)
}

// RefreshToken generates new access/refresh tokens and rotates refresh cookie
//
//encore:api public raw method=POST path=/auth/refresh
func (s *Service) RefreshRaw(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Read refresh token from cookie
	cookie, err := r.Cookie(refreshCookieName())
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "missing refresh cookie")
		return
	}

	resp, err := s.RefreshUserToken(ctx, &RefreshTokenRequest{RefreshToken: cookie.Value})
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", err.Error())
		return
	}

	// Rotate refresh cookie
	setRefreshCookie(w, resp.RefreshToken)
	resp.RefreshToken = ""
	writeJSON(w, resp)
}

// Logout invalidates the user's session and tokens
//
//encore:api auth method=POST path=/auth/logout
func (s *Service) Logout(ctx context.Context) (*LogoutResponse, error) {
	// Get user ID from auth context
	userID, ok := auth.UserID()
	if !ok {
		return nil, errs.E(ctx, "AUTH_UNAUTHENTICATED", "المستخدم غير مصادق.")
	}

	// Convert auth.UID (string) to int64
	userIDInt64, err := strconv.ParseInt(string(userID), 10, 64)
	if err != nil {
		return nil, errs.E(ctx, "AUTH_AUTH_ID_INVALID", "معرّف المستخدم غير صالح.")
	}

	return s.LogoutUser(ctx, userIDInt64)
}

// ResendVerification resends the email verification code
//
//encore:api public method=POST path=/auth/resend-verification
func (s *Service) ResendVerification(ctx context.Context, req *ResendVerificationRequest) (*ResendVerificationResponse, error) {
	return s.ResendUserVerification(ctx, req)
}

// RequestPasswordReset initiates password reset flow by sending reset email
//
//encore:api public method=POST path=/auth/forgot-password
func (s *Service) RequestPasswordReset(ctx context.Context, req *RequestPasswordResetRequest) (*RequestPasswordResetResponse, error) {
	return s.RequestPasswordResetFlow(ctx, req)
}

// ResetPassword completes password reset using the token from email
//
//encore:api public method=POST path=/auth/reset-password
func (s *Service) ResetPassword(ctx context.Context, req *ResetPasswordRequest) (*ResetPasswordResponse, error) {
	return s.ResetUserPassword(ctx, req)
}

// --- helpers ---

func refreshCookieName() string { return "refresh_token" }

func setRefreshCookie(w http.ResponseWriter, token string) {
	secure := true
	sameSite := http.SameSiteNoneMode // للسماح بـ cross-domain cookies (Vercel → Encore Cloud)

	if encore.Meta().Environment.Type == encore.EnvDevelopment && encore.Meta().Environment.Cloud == encore.CloudLocal {
		secure = false
		sameSite = http.SameSiteLaxMode // في التطوير المحلي نستخدم Lax
	}

	cookie := &http.Cookie{
		Name:     refreshCookieName(),
		Value:    token,
		Path:     "/",
		MaxAge:   int(authn.RefreshTokenDuration.Seconds()),
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	}
	http.SetCookie(w, cookie)
}

func sessionCookieEnabled() bool { return false }

// minimal JSON helpers (avoid external deps)
func decodeJSON(r *http.Request, v interface{}) error { return json.NewDecoder(r.Body).Decode(v) }
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    code,
		"message": message,
	})
}
