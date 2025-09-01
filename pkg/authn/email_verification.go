package authn

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// Email verification constants
const (
	// VerificationCodeLength is the length of the verification code (6 digits)
	VerificationCodeLength = 6
	// VerificationCodeExpiry is the expiration time for verification codes (15 minutes)
	VerificationCodeExpiry = 15 * time.Minute
	// MaxResendAttempts is the maximum number of resend attempts per hour
	MaxResendAttempts = 3
	// ResendCooldown is the minimum time between resend attempts
	ResendCooldown = 2 * time.Minute
)

// Email verification errors
var (
	ErrCodeExpired       = errors.New("verification code has expired")
	ErrCodeInvalid       = errors.New("invalid verification code")
	ErrCodeAlreadyUsed   = errors.New("verification code has already been used")
	ErrTooManyAttempts   = errors.New("too many verification attempts")
	ErrResendTooSoon     = errors.New("please wait before requesting another code")
	ErrMaxResendsReached = errors.New("maximum resend attempts reached for this hour")
)

// VerificationCode represents an email verification code
type VerificationCode struct {
	Code      string     `json:"code"`
	Email     string     `json:"email"`
	UserID    int64      `json:"user_id"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	Attempts  int        `json:"attempts"`
}

// VerificationManager handles email verification operations
type VerificationManager struct {
	codes         map[string]*VerificationCode // In production, use Redis or database
	resendTracker map[string][]time.Time       // Track resend attempts per email
	mutex         sync.RWMutex                 // Protects concurrent access to maps
}

// NewVerificationManager creates a new verification manager
func NewVerificationManager() *VerificationManager {
	return &VerificationManager{
		codes:         make(map[string]*VerificationCode),
		resendTracker: make(map[string][]time.Time),
	}
}

// GenerateVerificationCode generates a new 6-digit verification code
func GenerateVerificationCode() (string, error) {
	code := ""
	for i := 0; i < VerificationCodeLength; i++ {
		digit, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("failed to generate verification code: %w", err)
		}
		code += digit.String()
	}
	return code, nil
}

// CreateVerificationCode creates a new verification code for the user
func (vm *VerificationManager) CreateVerificationCode(userID int64, email string) (*VerificationCode, error) {
	// Check resend limits (needs read lock)
	if err := vm.checkResendLimits(email); err != nil {
		return nil, err
	}

	// Generate new code
	code, err := GenerateVerificationCode()
	if err != nil {
		return nil, err
	}

	// Create verification code record
	verificationCode := &VerificationCode{
		Code:      code,
		Email:     email,
		UserID:    userID,
		ExpiresAt: time.Now().UTC().Add(VerificationCodeExpiry),
		CreatedAt: time.Now().UTC(),
		Attempts:  0,
	}

	// Store the code and track resend attempt (needs write lock)
	vm.mutex.Lock()
	vm.codes[email] = verificationCode
	vm.trackResendAttemptLocked(email)
	vm.mutex.Unlock()

	return verificationCode, nil
}

// VerifyCode verifies the provided code for the email
func (vm *VerificationManager) VerifyCode(email, code string) (*VerificationCode, error) {
	vm.mutex.Lock()
	defer vm.mutex.Unlock()

	// Get stored code
	storedCode, exists := vm.codes[email]
	if !exists {
		return nil, ErrCodeInvalid
	}

	// Check if code has been used
	if storedCode.UsedAt != nil {
		return nil, ErrCodeAlreadyUsed
	}

	// Check if code has expired
	if time.Now().UTC().After(storedCode.ExpiresAt) {
		return nil, ErrCodeExpired
	}

	// Increment attempts
	storedCode.Attempts++

	// Check for too many attempts (prevent brute force)
	if storedCode.Attempts > 5 {
		return nil, ErrTooManyAttempts
	}

	// Verify the code
	if storedCode.Code != code {
		return nil, ErrCodeInvalid
	}

	// Mark as used
	now := time.Now()
	storedCode.UsedAt = &now

	return storedCode, nil
}

// IsCodeValid checks if a code exists and is still valid (not expired or used)
func (vm *VerificationManager) IsCodeValid(email string) bool {
	vm.mutex.RLock()
	defer vm.mutex.RUnlock()

	storedCode, exists := vm.codes[email]
	if !exists {
		return false
	}

	// Check if already used
	if storedCode.UsedAt != nil {
		return false
	}

	// Check if expired
	if time.Now().UTC().After(storedCode.ExpiresAt) {
		return false
	}

	return true
}

// GetCodeInfo returns information about the verification code for an email
func (vm *VerificationManager) GetCodeInfo(email string) (*VerificationCode, bool) {
	vm.mutex.RLock()
	defer vm.mutex.RUnlock()

	code, exists := vm.codes[email]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent external modification
	codeCopy := *code
	return &codeCopy, true
}

// CleanupExpiredCodes removes expired verification codes
func (vm *VerificationManager) CleanupExpiredCodes() int {
	vm.mutex.Lock()
	defer vm.mutex.Unlock()

	now := time.Now().UTC()
	cleaned := 0

	for email, code := range vm.codes {
		if now.After(code.ExpiresAt) {
			delete(vm.codes, email)
			cleaned++
		}
	}

	// Also cleanup old resend tracking
	for email, attempts := range vm.resendTracker {
		var validAttempts []time.Time
		for _, attempt := range attempts {
			if now.Sub(attempt) < time.Hour {
				validAttempts = append(validAttempts, attempt)
			}
		}

		if len(validAttempts) == 0 {
			delete(vm.resendTracker, email)
		} else {
			vm.resendTracker[email] = validAttempts
		}
	}

	return cleaned
}

// checkResendLimits checks if the user can request another verification code
func (vm *VerificationManager) checkResendLimits(email string) error {
	vm.mutex.RLock()
	defer vm.mutex.RUnlock()

	now := time.Now().UTC()
	attempts, exists := vm.resendTracker[email]

	if !exists {
		return nil // First attempt, allow it
	}

	// Filter attempts within the last hour
	var recentAttempts []time.Time
	for _, attempt := range attempts {
		if now.Sub(attempt) < time.Hour {
			recentAttempts = append(recentAttempts, attempt)
		}
	}

	// Check maximum attempts per hour
	if len(recentAttempts) >= MaxResendAttempts {
		return ErrMaxResendsReached
	}

	// Check cooldown period (last attempt must be at least 2 minutes ago)
	if len(recentAttempts) > 0 {
		lastAttempt := recentAttempts[len(recentAttempts)-1]
		if now.Sub(lastAttempt) < ResendCooldown {
			return ErrResendTooSoon
		}
	}

	return nil
}

// trackResendAttempt records a resend attempt for rate limiting
func (vm *VerificationManager) trackResendAttempt(email string) {
	vm.mutex.Lock()
	defer vm.mutex.Unlock()
	vm.trackResendAttemptLocked(email)
}

// trackResendAttemptLocked records a resend attempt (assumes lock is held)
func (vm *VerificationManager) trackResendAttemptLocked(email string) {
	now := time.Now().UTC()

	if attempts, exists := vm.resendTracker[email]; exists {
		vm.resendTracker[email] = append(attempts, now)
	} else {
		vm.resendTracker[email] = []time.Time{now}
	}
}

// GetResendCooldownRemaining returns the remaining cooldown time before next resend
func (vm *VerificationManager) GetResendCooldownRemaining(email string) time.Duration {
	vm.mutex.RLock()
	defer vm.mutex.RUnlock()

	attempts, exists := vm.resendTracker[email]
	if !exists || len(attempts) == 0 {
		return 0
	}

	now := time.Now().UTC()
	lastAttempt := attempts[len(attempts)-1]
	elapsed := now.Sub(lastAttempt)

	if elapsed >= ResendCooldown {
		return 0
	}

	return ResendCooldown - elapsed
}

// GetRemainingResendAttempts returns the number of remaining resend attempts for the hour
func (vm *VerificationManager) GetRemainingResendAttempts(email string) int {
	vm.mutex.RLock()
	defer vm.mutex.RUnlock()

	now := time.Now().UTC()
	attempts, exists := vm.resendTracker[email]

	if !exists {
		return MaxResendAttempts
	}

	// Count recent attempts (within last hour)
	recentCount := 0
	for _, attempt := range attempts {
		if now.Sub(attempt) < time.Hour {
			recentCount++
		}
	}

	remaining := MaxResendAttempts - recentCount
	if remaining < 0 {
		return 0
	}
	return remaining
}

// InvalidateCode invalidates a verification code (useful for cleanup or security)
func (vm *VerificationManager) InvalidateCode(email string) bool {
	vm.mutex.Lock()
	defer vm.mutex.Unlock()

	if _, exists := vm.codes[email]; exists {
		delete(vm.codes, email)
		return true
	}
	return false
}
