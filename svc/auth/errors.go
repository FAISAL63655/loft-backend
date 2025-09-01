// Package auth provides authentication and authorization services
package auth

import "encore.app/pkg/errs"

// Authentication error codes (AUTH domain)
const (
	AuthEmailTaken             = "AUTH_EMAIL_TAKEN"
	AuthInvalidCredentials     = "AUTH_INVALID_CREDENTIALS"
	AuthUserNotFound           = "AUTH_USER_NOT_FOUND"
	AuthUserInactive           = "AUTH_USER_INACTIVE"
	AuthWeakPassword           = "AUTH_WEAK_PASSWORD"
	AuthInvalidVerificationCode = "AUTH_INVALID_VERIFICATION_CODE"
	AuthVerificationCodeExpired = "AUTH_VERIFICATION_CODE_EXPIRED"
	AuthVerificationCodeUsed   = "AUTH_VERIFICATION_CODE_USED"
	AuthEmailAlreadyVerified   = "AUTH_EMAIL_ALREADY_VERIFIED"
	AuthInvalidRefreshToken    = "AUTH_INVALID_REFRESH_TOKEN"
	AuthRateLimitExceeded      = "AUTH_RATE_LIMIT_EXCEEDED"
	AuthTokenExpired           = "AUTH_TOKEN_EXPIRED"
	AuthUnauthenticated        = "AUTH_UNAUTHENTICATED"
	AuthForbidden              = "AUTH_FORBIDDEN"
	AuthEmailVerifyRequired    = "AUTH_EMAIL_VERIFY_REQUIRED"
)

// Authentication error messages
var (
	// ErrUserAlreadyExists indicates that a user with the given email already exists
	ErrUserAlreadyExists = &errs.Error{
		Code:    AuthEmailTaken,
		Message: "مستخدم بهذا البريد الإلكتروني موجود بالفعل",
	}

	// ErrInvalidCredentials indicates invalid login credentials
	ErrInvalidCredentials = &errs.Error{
		Code:    AuthInvalidCredentials,
		Message: "البريد الإلكتروني أو كلمة المرور غير صحيحة",
	}

	// ErrUserNotFound indicates that the user was not found
	ErrUserNotFound = &errs.Error{
		Code:    AuthUserNotFound,
		Message: "المستخدم غير موجود",
	}

	// ErrUserInactive indicates that the user account is inactive
	ErrUserInactive = &errs.Error{
		Code:    AuthUserInactive,
		Message: "حساب المستخدم غير موجود أو غير نشط",
	}

	// ErrWeakPassword indicates that the password doesn't meet security requirements
	ErrWeakPassword = &errs.Error{
		Code:    AuthWeakPassword,
		Message: "كلمة المرور يجب أن تكون 8 أحرف على الأقل وتحتوي على أحرف وأرقام",
	}

	// ErrInvalidVerificationCode indicates an invalid verification code
	ErrInvalidVerificationCode = &errs.Error{
		Code:    AuthInvalidVerificationCode,
		Message: "رمز التحقق غير صالح",
	}

	// ErrVerificationCodeExpired indicates that the verification code has expired
	ErrVerificationCodeExpired = &errs.Error{
		Code:    AuthVerificationCodeExpired,
		Message: "رمز التحقق منتهي الصلاحية. يرجى طلب رمز جديد",
	}

	// ErrVerificationCodeUsed indicates that the verification code has already been used
	ErrVerificationCodeUsed = &errs.Error{
		Code:    AuthVerificationCodeUsed,
		Message: "رمز التحقق مُستخدم بالفعل",
	}

	// ErrEmailAlreadyVerified indicates that the email is already verified
	ErrEmailAlreadyVerified = &errs.Error{
		Code:    AuthEmailAlreadyVerified,
		Message: "البريد الإلكتروني مُفعّل بالفعل",
	}

	// ErrInvalidRefreshToken indicates an invalid or expired refresh token
	ErrInvalidRefreshToken = &errs.Error{
		Code:    AuthInvalidRefreshToken,
		Message: "رمز التحديث غير صالح أو منتهي الصلاحية",
	}

	// ErrRateLimitExceeded indicates that rate limit has been exceeded
	ErrRateLimitExceeded = &errs.Error{
		Code:    AuthRateLimitExceeded,
		Message: "محاولات كثيرة جداً. يرجى المحاولة لاحقاً",
	}

	// ErrInternalError indicates an internal server error
	ErrInternalError = &errs.Error{
		Code:    errs.Internal,
		Message: "حدث خطأ داخلي. يرجى المحاولة لاحقاً",
	}
)

// NewRateLimitError creates a rate limit error with a custom message
func NewRateLimitError(message string) *errs.Error {
	return &errs.Error{
		Code:    AuthRateLimitExceeded,
		Message: message,
	}
}

// NewInternalError creates an internal error with a custom message
func NewInternalError(message string) *errs.Error {
	return &errs.Error{
		Code:    errs.Internal,
		Message: message,
	}
}

// NewValidationError creates a validation error with a custom message
func NewValidationError(message string) *errs.Error {
	return &errs.Error{
		Code:    errs.ValidationFailed,
		Message: message,
	}
}
