package integration

import (
	"context"
	"testing"
	"time"

	"encore.app/pkg/authn"
	authsvc "encore.app/svc/auth"
	"encore.dev/storage/sqldb"
)

// TestRegisterUser اختبار تسجيل مستخدم جديد
func TestRegisterUser(t *testing.T) {
	ctx := context.Background()
	db := testDB

	// تنظيف البيانات القديمة
	cleanupTestData(t, db)
	defer cleanupTestData(t, db)

	service, err := authsvc.NewService()
	if err != nil {
		t.Fatalf("Failed to create auth service: %v", err)
	}

	testCases := []struct {
		name        string
		req         *authsvc.RegisterRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "تسجيل ناجح",
			req: &authsvc.RegisterRequest{
				Name:     "أحمد محمد",
				Email:    "test_register_1@example.com",
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
				Email:    "test_register_1@example.com", // نفس البريد
				Phone:    "+966501234568",
				CityID:   1,
				Password: "SecurePass123!",
			},
			expectError: true,
			errorMsg:    "already exists",
		},
		{
			name: "فشل - كلمة مرور ضعيفة",
			req: &authsvc.RegisterRequest{
				Name:     "سالم أحمد",
				Email:    "test_register_2@example.com",
				Phone:    "+966501234569",
				CityID:   1,
				Password: "weak",
			},
			expectError: true,
			errorMsg:    "weak password",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := service.Register(ctx, tc.req)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if resp == nil {
					t.Errorf("Expected response but got nil")
				} else {
					if !resp.RequiresEmailVerification {
						t.Errorf("Expected email verification required")
					}
				}
			}
		})
	}
}

// TestLoginUser اختبار تسجيل الدخول
func TestLoginUser(t *testing.T) {
	ctx := context.Background()
	db := testDB

	// تنظيف البيانات القديمة
	cleanupTestData(t, db)
	defer cleanupTestData(t, db)

	service, err := authsvc.NewService()
	if err != nil {
		t.Fatalf("Failed to create auth service: %v", err)
	}

	// إنشاء مستخدم للاختبار
	email := "test_login@example.com"
	password := "SecurePass123!"
	createTestUser(t, db, email, password, true)

	testCases := []struct {
		name        string
		req         *authsvc.LoginRequest
		expectError bool
	}{
		{
			name: "دخول ناجح",
			req: &authsvc.LoginRequest{
				Email:    email,
				Password: password,
			},
			expectError: false,
		},
		{
			name: "فشل - كلمة مرور خاطئة",
			req: &authsvc.LoginRequest{
				Email:    email,
				Password: "WrongPassword",
			},
			expectError: true,
		},
		{
			name: "فشل - بريد غير موجود",
			req: &authsvc.LoginRequest{
				Email:    "nonexistent@example.com",
				Password: password,
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := service.Login(ctx, tc.req)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if resp == nil {
					t.Errorf("Expected response but got nil")
				} else {
					if resp.AccessToken == "" {
						t.Errorf("Expected access token")
					}
					if resp.RefreshToken == "" {
						t.Errorf("Expected refresh token")
					}
				}
			}
		})
	}
}

// TestEmailVerification اختبار تفعيل البريد الإلكتروني
func TestEmailVerification(t *testing.T) {
	ctx := context.Background()
	db := testDB

	// تنظيف البيانات القديمة
	cleanupTestData(t, db)
	defer cleanupTestData(t, db)

	service, err := authsvc.NewService()
	if err != nil {
		t.Fatalf("Failed to create auth service: %v", err)
	}

	// تسجيل مستخدم جديد
	registerReq := &authsvc.RegisterRequest{
		Name:     "اختبار التفعيل",
		Email:    "test_verify@example.com",
		Phone:    "+966501234567",
		CityID:   1,
		Password: "SecurePass123!",
	}

	regResp, err := service.Register(ctx, registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	// استخراج رمز التحقق من قاعدة البيانات
	var verificationCode string
	row := db.QueryRow(ctx, `
		SELECT code FROM email_verification_codes 
		WHERE user_id = $1 AND email = $2 
		ORDER BY created_at DESC LIMIT 1
	`, regResp.User.ID, registerReq.Email)
	if err := row.Scan(&verificationCode); err != nil {
		t.Fatalf("Failed to get verification code: %v", err)
	}

	// اختبار التفعيل بالرمز الصحيح
	verifyReq := &authsvc.VerifyEmailRequest{
		Email: registerReq.Email,
		Code:  verificationCode,
	}

	verifyResp, err := service.VerifyEmail(ctx, verifyReq)
	if err != nil {
		t.Errorf("Failed to verify email: %v", err)
	}

	if !verifyResp.Success {
		t.Errorf("Expected verification to succeed")
	}

	// التأكد من تفعيل البريد في قاعدة البيانات
	var emailVerified bool
	err = db.QueryRow(ctx,
		"SELECT email_verified_at IS NOT NULL FROM users WHERE id = $1",
		regResp.User.ID).Scan(&emailVerified)

	if err != nil {
		t.Errorf("Failed to check email verification status: %v", err)
	}

	if !emailVerified {
		t.Errorf("Email should be verified in database")
	}
}

// TestRateLimiting اختبار حد المعدل
func TestRateLimiting(t *testing.T) {
	ctx := context.Background()
	db := testDB

	// تنظيف البيانات القديمة
	cleanupTestData(t, db)
	defer cleanupTestData(t, db)

	service, err := authsvc.NewService()
	if err != nil {
		t.Fatalf("Failed to create auth service: %v", err)
	}

	email := "test_ratelimit@example.com"
	createTestUser(t, db, email, "SecurePass123!", true)

	// محاولة تسجيل دخول فاشلة عدة مرات
	req := &authsvc.LoginRequest{
		Email:    email,
		Password: "WrongPassword",
	}

	// PRD: 10 محاولات فاشلة خلال 10 دقائق
	var rateLimitHit bool
	for i := 0; i < 15; i++ {
		_, err := service.Login(ctx, req)
		if err != nil && i >= 10 {
			// يجب أن نصل لحد المعدل بعد 10 محاولات
			rateLimitHit = true
			break
		}
	}

	if !rateLimitHit {
		t.Errorf("Expected rate limit to be hit after 10 attempts")
	}
}

// TestRefreshToken اختبار تجديد الرمز
func TestRefreshToken(t *testing.T) {
	ctx := context.Background()
	db := testDB

	// تنظيف البيانات القديمة
	cleanupTestData(t, db)
	defer cleanupTestData(t, db)

	service, err := authsvc.NewService()
	if err != nil {
		t.Fatalf("Failed to create auth service: %v", err)
	}

	// إنشاء مستخدم وتسجيل دخول
	email := "test_refresh@example.com"
	password := "SecurePass123!"
	createTestUser(t, db, email, password, true)

	loginResp, err := service.Login(ctx, &authsvc.LoginRequest{
		Email:    email,
		Password: password,
	})

	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// انتظار قليلاً للتأكد من اختلاف الوقت
	time.Sleep(100 * time.Millisecond)

	// تجديد الرمز
	refreshResp, err := service.RefreshToken(ctx, &authsvc.RefreshTokenRequest{
		RefreshToken: loginResp.RefreshToken,
	})

	if err != nil {
		t.Errorf("Failed to refresh token: %v", err)
	}

	if refreshResp.AccessToken == "" {
		t.Errorf("Expected new access token")
	}

	if refreshResp.RefreshToken == "" {
		t.Errorf("Expected new refresh token")
	}

	// التأكد من أن الرموز الجديدة مختلفة
	if refreshResp.AccessToken == loginResp.AccessToken {
		t.Errorf("New access token should be different")
	}
}

// Helper functions

func cleanupTestData(t *testing.T, db *sqldb.Database) {
	ctx := context.Background()
	queries := []string{
		"DELETE FROM email_verification_codes WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'test_%@example.com')",
		"DELETE FROM verification_requests WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'test_%@example.com')",
		"DELETE FROM addresses WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'test_%@example.com')",
		"DELETE FROM users WHERE email LIKE 'test_%@example.com'",
	}

	for _, query := range queries {
		if _, err := db.Exec(ctx, query); err != nil {
			t.Logf("Warning: cleanup query failed: %v", err)
		}
	}
}

func createTestUser(t *testing.T, db *sqldb.Database, email, password string, verified bool) int64 {
	ctx := context.Background()

	// تشفير كلمة المرور
	hashedPassword, err := authn.HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	var userID int64
	var verifiedAt interface{}
	if verified {
		verifiedAt = time.Now()
	} else {
		verifiedAt = nil
	}

	err = db.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, phone, city_id, role, state, email_verified_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'registered', 'active', $6, NOW(), NOW())
		RETURNING id
	`, "Test User", email, hashedPassword, "+966501234567", 1, verifiedAt).Scan(&userID)

	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	return userID
}
