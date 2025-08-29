package authn

import (
	"fmt"
	"testing"
	"time"
)

func TestGenerateVerificationCode(t *testing.T) {
	code, err := GenerateVerificationCode()
	if err != nil {
		t.Fatalf("GenerateVerificationCode failed: %v", err)
	}

	if len(code) != VerificationCodeLength {
		t.Errorf("Code length mismatch: got %d, want %d", len(code), VerificationCodeLength)
	}

	// Check that code contains only digits
	for _, char := range code {
		if char < '0' || char > '9' {
			t.Errorf("Code contains non-digit character: %c", char)
		}
	}

	// Generate multiple codes to ensure uniqueness
	codes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code, err := GenerateVerificationCode()
		if err != nil {
			t.Fatalf("GenerateVerificationCode failed on iteration %d: %v", i, err)
		}

		if codes[code] {
			t.Errorf("Duplicate code generated: %s", code)
		}
		codes[code] = true
	}
}

func TestNewVerificationManager(t *testing.T) {
	vm := NewVerificationManager()

	if vm == nil {
		t.Fatal("NewVerificationManager returned nil")
	}

	if vm.codes == nil {
		t.Error("codes map not initialized")
	}

	if vm.resendTracker == nil {
		t.Error("resendTracker map not initialized")
	}
}

func TestCreateVerificationCode(t *testing.T) {
	vm := NewVerificationManager()
	userID := int64(123)
	email := "test@example.com"

	code, err := vm.CreateVerificationCode(userID, email)
	if err != nil {
		t.Fatalf("CreateVerificationCode failed: %v", err)
	}

	if code == nil {
		t.Fatal("CreateVerificationCode returned nil code")
	}

	if code.UserID != userID {
		t.Errorf("UserID mismatch: got %d, want %d", code.UserID, userID)
	}

	if code.Email != email {
		t.Errorf("Email mismatch: got %s, want %s", code.Email, email)
	}

	if len(code.Code) != VerificationCodeLength {
		t.Errorf("Code length mismatch: got %d, want %d", len(code.Code), VerificationCodeLength)
	}

	if code.ExpiresAt.Before(time.Now()) {
		t.Error("Code expiration time is in the past")
	}

	if code.UsedAt != nil {
		t.Error("New code should not be marked as used")
	}

	if code.Attempts != 0 {
		t.Errorf("New code should have 0 attempts, got %d", code.Attempts)
	}
}

func TestVerifyCode(t *testing.T) {
	tests := []struct {
		name        string
		setupCode   bool
		email       string
		code        string
		expectError error
	}{
		{
			name:        "valid code",
			setupCode:   true,
			email:       "test1@example.com",
			code:        "", // Will be set during test
			expectError: nil,
		},
		{
			name:        "invalid code",
			setupCode:   true,
			email:       "test2@example.com",
			code:        "000000",
			expectError: ErrCodeInvalid,
		},
		{
			name:        "non-existent email",
			setupCode:   false,
			email:       "nonexistent@example.com",
			code:        "123456",
			expectError: ErrCodeInvalid,
		},
		{
			name:        "empty code",
			setupCode:   true,
			email:       "test3@example.com",
			code:        "",
			expectError: ErrCodeInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := NewVerificationManager()
			userID := int64(123)

			var createdCode *VerificationCode
			if tt.setupCode {
				var err error
				createdCode, err = vm.CreateVerificationCode(userID, tt.email)
				if err != nil {
					t.Fatalf("CreateVerificationCode failed: %v", err)
				}

				// Use the actual code for valid test
				if tt.name == "valid code" {
					tt.code = createdCode.Code
				}
			}

			verifiedCode, err := vm.VerifyCode(tt.email, tt.code)

			if tt.expectError != nil {
				if err == nil {
					t.Errorf("Expected error %v, got nil", tt.expectError)
				} else if err != tt.expectError {
					t.Errorf("Expected error %v, got %v", tt.expectError, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if verifiedCode.UsedAt == nil {
				t.Error("Verified code should be marked as used")
			}
		})
	}
}

func TestVerifyCodeAlreadyUsed(t *testing.T) {
	vm := NewVerificationManager()
	userID := int64(123)
	email := "test@example.com"

	// Create and verify code
	createdCode, err := vm.CreateVerificationCode(userID, email)
	if err != nil {
		t.Fatalf("CreateVerificationCode failed: %v", err)
	}

	// First verification should succeed
	_, err = vm.VerifyCode(email, createdCode.Code)
	if err != nil {
		t.Fatalf("First verification failed: %v", err)
	}

	// Second verification should fail
	_, err = vm.VerifyCode(email, createdCode.Code)
	if err != ErrCodeAlreadyUsed {
		t.Errorf("Expected ErrCodeAlreadyUsed, got %v", err)
	}
}

func TestVerifyCodeExpired(t *testing.T) {
	vm := NewVerificationManager()
	userID := int64(123)
	email := "test@example.com"

	// Create code
	createdCode, err := vm.CreateVerificationCode(userID, email)
	if err != nil {
		t.Fatalf("CreateVerificationCode failed: %v", err)
	}

	// Manually expire the code
	vm.codes[email].ExpiresAt = time.Now().Add(-time.Minute)

	// Verification should fail
	_, err = vm.VerifyCode(email, createdCode.Code)
	if err != ErrCodeExpired {
		t.Errorf("Expected ErrCodeExpired, got %v", err)
	}
}

func TestVerifyCodeTooManyAttempts(t *testing.T) {
	vm := NewVerificationManager()
	userID := int64(123)
	email := "test@example.com"

	// Create code
	createdCode, err := vm.CreateVerificationCode(userID, email)
	if err != nil {
		t.Fatalf("CreateVerificationCode failed: %v", err)
	}

	// Make 5 failed attempts
	for i := 0; i < 5; i++ {
		_, err = vm.VerifyCode(email, "wrong_code")
		if err != ErrCodeInvalid {
			t.Errorf("Expected ErrCodeInvalid on attempt %d, got %v", i+1, err)
		}
	}

	// 6th attempt should fail with too many attempts
	_, err = vm.VerifyCode(email, createdCode.Code)
	if err != ErrTooManyAttempts {
		t.Errorf("Expected ErrTooManyAttempts, got %v", err)
	}
}

func TestIsCodeValid(t *testing.T) {
	vm := NewVerificationManager()
	userID := int64(123)
	email := "test@example.com"

	// No code exists
	if vm.IsCodeValid(email) {
		t.Error("IsCodeValid should return false for non-existent code")
	}

	// Create code
	_, err := vm.CreateVerificationCode(userID, email)
	if err != nil {
		t.Fatalf("CreateVerificationCode failed: %v", err)
	}

	// Code should be valid
	if !vm.IsCodeValid(email) {
		t.Error("IsCodeValid should return true for valid code")
	}

	// Use the code
	createdCode := vm.codes[email]
	now := time.Now()
	createdCode.UsedAt = &now

	// Code should no longer be valid
	if vm.IsCodeValid(email) {
		t.Error("IsCodeValid should return false for used code")
	}
}

func TestResendLimits(t *testing.T) {
	vm := NewVerificationManager()
	userID := int64(123)
	email := "test@example.com"

	// First 3 attempts should succeed
	for i := 0; i < MaxResendAttempts; i++ {
		_, err := vm.CreateVerificationCode(userID, email)
		if err != nil {
			t.Errorf("CreateVerificationCode failed on attempt %d: %v", i+1, err)
		}

		// Wait for cooldown
		time.Sleep(ResendCooldown + time.Millisecond*10)
	}

	// 4th attempt should fail
	_, err := vm.CreateVerificationCode(userID, email)
	if err != ErrMaxResendsReached {
		t.Errorf("Expected ErrMaxResendsReached, got %v", err)
	}
}

func TestResendCooldown(t *testing.T) {
	vm := NewVerificationManager()
	userID := int64(123)
	email := "test@example.com"

	// Create first code
	_, err := vm.CreateVerificationCode(userID, email)
	if err != nil {
		t.Fatalf("CreateVerificationCode failed: %v", err)
	}

	// Immediate second attempt should fail
	_, err = vm.CreateVerificationCode(userID, email)
	if err != ErrResendTooSoon {
		t.Errorf("Expected ErrResendTooSoon, got %v", err)
	}

	// Check cooldown remaining
	remaining := vm.GetResendCooldownRemaining(email)
	if remaining <= 0 || remaining > ResendCooldown {
		t.Errorf("Invalid cooldown remaining: %v", remaining)
	}
}

func TestGetRemainingResendAttempts(t *testing.T) {
	vm := NewVerificationManager()
	email := "test@example.com"

	// Initially should have max attempts
	remaining := vm.GetRemainingResendAttempts(email)
	if remaining != MaxResendAttempts {
		t.Errorf("Initial remaining attempts should be %d, got %d", MaxResendAttempts, remaining)
	}

	// After one attempt
	vm.trackResendAttempt(email)
	remaining = vm.GetRemainingResendAttempts(email)
	if remaining != MaxResendAttempts-1 {
		t.Errorf("After one attempt, remaining should be %d, got %d", MaxResendAttempts-1, remaining)
	}
}

func TestCleanupExpiredCodes(t *testing.T) {
	vm := NewVerificationManager()
	userID := int64(123)
	email1 := "test1@example.com"
	email2 := "test2@example.com"

	// Create two codes
	_, err := vm.CreateVerificationCode(userID, email1)
	if err != nil {
		t.Fatalf("CreateVerificationCode failed: %v", err)
	}

	_, err = vm.CreateVerificationCode(userID, email2)
	if err != nil {
		t.Fatalf("CreateVerificationCode failed: %v", err)
	}

	// Expire first code
	vm.codes[email1].ExpiresAt = time.Now().Add(-time.Minute)

	// Cleanup should remove 1 code
	cleaned := vm.CleanupExpiredCodes()
	if cleaned != 1 {
		t.Errorf("Expected 1 code cleaned, got %d", cleaned)
	}

	// First code should be gone
	if _, exists := vm.codes[email1]; exists {
		t.Error("Expired code should have been removed")
	}

	// Second code should still exist
	if _, exists := vm.codes[email2]; !exists {
		t.Error("Valid code should not have been removed")
	}
}

func TestGetCodeInfo(t *testing.T) {
	vm := NewVerificationManager()
	userID := int64(123)
	email := "test@example.com"

	// No code exists
	_, exists := vm.GetCodeInfo(email)
	if exists {
		t.Error("GetCodeInfo should return false for non-existent code")
	}

	// Create code
	originalCode, err := vm.CreateVerificationCode(userID, email)
	if err != nil {
		t.Fatalf("CreateVerificationCode failed: %v", err)
	}

	// Get code info
	codeInfo, exists := vm.GetCodeInfo(email)
	if !exists {
		t.Fatal("GetCodeInfo should return true for existing code")
	}

	if codeInfo.Code != originalCode.Code {
		t.Errorf("Code mismatch: got %s, want %s", codeInfo.Code, originalCode.Code)
	}

	if codeInfo.UserID != originalCode.UserID {
		t.Errorf("UserID mismatch: got %d, want %d", codeInfo.UserID, originalCode.UserID)
	}
}

func TestInvalidateCode(t *testing.T) {
	vm := NewVerificationManager()
	userID := int64(123)
	email := "test@example.com"

	// Invalidate non-existent code
	if vm.InvalidateCode(email) {
		t.Error("InvalidateCode should return false for non-existent code")
	}

	// Create code
	_, err := vm.CreateVerificationCode(userID, email)
	if err != nil {
		t.Fatalf("CreateVerificationCode failed: %v", err)
	}

	// Invalidate existing code
	if !vm.InvalidateCode(email) {
		t.Error("InvalidateCode should return true for existing code")
	}

	// Code should be gone
	if _, exists := vm.codes[email]; exists {
		t.Error("Code should have been invalidated")
	}
}

func BenchmarkGenerateVerificationCode(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GenerateVerificationCode()
		if err != nil {
			b.Fatalf("GenerateVerificationCode failed: %v", err)
		}
	}
}

func BenchmarkCreateVerificationCode(b *testing.B) {
	vm := NewVerificationManager()
	userID := int64(123)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		email := fmt.Sprintf("test%d@example.com", i)
		_, err := vm.CreateVerificationCode(userID, email)
		if err != nil {
			b.Fatalf("CreateVerificationCode failed: %v", err)
		}
	}
}

func BenchmarkVerifyCode(b *testing.B) {
	vm := NewVerificationManager()
	userID := int64(123)

	// Create codes for benchmarking
	codes := make([]string, b.N)
	emails := make([]string, b.N)

	for i := 0; i < b.N; i++ {
		email := fmt.Sprintf("test%d@example.com", i)
		emails[i] = email

		code, err := vm.CreateVerificationCode(userID, email)
		if err != nil {
			b.Fatalf("CreateVerificationCode failed: %v", err)
		}
		codes[i] = code.Code
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := vm.VerifyCode(emails[i], codes[i])
		if err != nil {
			b.Fatalf("VerifyCode failed: %v", err)
		}
	}
}
