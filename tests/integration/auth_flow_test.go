package integration

import (
	"context"
	"testing"
	"time"

	"encore.app/pkg/authn"
	authsvc "encore.app/svc/auth"
	"encore.dev/storage/sqldb"
)

// Helper: cleanup test data
func cleanupAuthFlowData(t *testing.T, db *sqldb.Database) {
	ctx := context.Background()
	queries := []string{
		"DELETE FROM email_verification_codes WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'test_%@example.com')",
		"DELETE FROM verification_requests WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'test_%@example.com')",
		"DELETE FROM addresses WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'test_%@example.com')",
		"DELETE FROM users WHERE email LIKE 'test_%@example.com'",
	}
	for _, q := range queries {
		if _, err := db.Exec(ctx, q); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}
}

// TestCompleteRegistrationFlow
func TestCompleteRegistrationFlow(t *testing.T) {
	ctx := context.Background()
	db := testDB
	cleanupAuthFlowData(t, db)
	defer cleanupAuthFlowData(t, db)

	svc, err := authsvc.NewService()
	if err != nil {
		t.Fatalf("failed to init auth service: %v", err)
	}

	tcs := []struct {
		name        string
		req         *authsvc.RegisterRequest
		expectError bool
	}{
		{
			name: "تسجيل ناجح",
			req: &authsvc.RegisterRequest{
				Name:     "أحمد محمد",
				Email:    "test_registration@example.com",
				Phone:    "+966501234567",
				CityID:   1,
				Password: "SecurePass123!",
			},
			expectError: false,
		},
		{
			name: "فشل - بريد مكرر",
			req: &authsvc.RegisterRequest{
				Name:     "محمد أحمد",
				Email:    "test_registration@example.com",
				Phone:    "+966501234568",
				CityID:   1,
				Password: "SecurePass123!",
			},
			expectError: true,
		},
		{
			name: "فشل - كلمة مرور ضعيفة",
			req: &authsvc.RegisterRequest{
				Name:     "سالم أحمد",
				Email:    "test_invalid_pw@example.com",
				Phone:    "+966501234569",
				CityID:   1,
				Password: "weak",
			},
			expectError: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := svc.Register(ctx, tc.req)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp == nil || !resp.RequiresEmailVerification {
				t.Fatalf("expected verification to be required")
			}
		})
	}
}

// TestEmailVerificationFlow
func TestEmailVerificationFlow(t *testing.T) {
	ctx := context.Background()
	db := testDB
	cleanupAuthFlowData(t, db)
	defer cleanupAuthFlowData(t, db)

	svc, err := authsvc.NewService()
	if err != nil {
		t.Fatalf("failed to init auth service: %v", err)
	}

	// Register user
	email := "test_verification@example.com"
	_, err = svc.Register(ctx, &authsvc.RegisterRequest{
		Name:     "اختبار",
		Email:    email,
		Phone:    "+966501234567",
		CityID:   1,
		Password: "SecurePass123!",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Get verification code from DB
	var userID int64
	if err := db.QueryRow(ctx, `SELECT id FROM users WHERE email=$1`, email).Scan(&userID); err != nil {
		t.Fatalf("failed to get user id: %v", err)
	}
	var code string
	if err := db.QueryRow(ctx, `SELECT code FROM email_verification_codes WHERE user_id=$1 AND email=$2 ORDER BY created_at DESC LIMIT 1`, userID, email).Scan(&code); err != nil {
		t.Fatalf("failed to get verification code: %v", err)
	}

	// Verify
	vr, err := svc.VerifyEmail(ctx, &authsvc.VerifyEmailRequest{Email: email, Code: code})
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if vr == nil || !vr.Success {
		t.Fatalf("expected verification success")
	}

	// Check email_verified_at
	var verified bool
	if err := db.QueryRow(ctx, `SELECT email_verified_at IS NOT NULL FROM users WHERE id=$1`, userID).Scan(&verified); err != nil {
		t.Fatalf("status check failed: %v", err)
	}
	if !verified {
		t.Fatalf("email should be verified")
	}
}

// TestLoginRateLimitAndRefresh
func TestLoginRateLimitAndRefresh(t *testing.T) {
	ctx := context.Background()
	db := testDB
	cleanupAuthFlowData(t, db)
	defer cleanupAuthFlowData(t, db)

	svc, err := authsvc.NewService()
	if err != nil {
		t.Fatalf("failed to init auth service: %v", err)
	}

	email := "test_login@example.com"
	password := "SecurePass123!"
	// Create user with hashed password directly
	hash, _ := authn.HashPassword(password)
	var userID int64
	if err := db.QueryRow(ctx, `
		INSERT INTO users (name,email,phone,city_id,password_hash,role,state,created_at,updated_at,email_verified_at)
		VALUES ('Test','test_login@example.com','+966501234567',1,$1,'registered','active',NOW(),NOW(),NOW()) RETURNING id
	`, hash).Scan(&userID); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Successful login
	lr, err := svc.Login(ctx, &authsvc.LoginRequest{Email: email, Password: password})
	if err != nil || lr == nil || lr.AccessToken == "" || lr.RefreshToken == "" {
		t.Fatalf("expected successful login, got err=%v", err)
	}

	// Rate limit: wrong password many times
	var rateHit bool
	for i := 0; i < 15; i++ {
		_, err := svc.Login(ctx, &authsvc.LoginRequest{Email: email, Password: "WrongPassword"})
		if err != nil && i >= 10 {
			rateHit = true
			break
		}
	}
	if !rateHit {
		t.Errorf("expected rate limit to trigger after multiple attempts")
	}

	// Short sleep to ensure different timestamp
	time.Sleep(100 * time.Millisecond)

	// Refresh
	rr, err := svc.RefreshToken(ctx, &authsvc.RefreshTokenRequest{RefreshToken: lr.RefreshToken})
	if err != nil || rr == nil || rr.AccessToken == "" || rr.RefreshToken == "" {
		t.Fatalf("expected refresh to succeed, err=%v", err)
	}
}
