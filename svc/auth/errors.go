// Package auth provides authentication and authorization services
package auth

import "encore.dev/beta/errs"

// Authentication error codes and messages
var (
	// ErrUserAlreadyExists indicates that a user with the given email already exists
	ErrUserAlreadyExists = &errs.Error{
		Code:    errs.AlreadyExists,
		Message: "User with this email already exists.",
	}

	// ErrInvalidCredentials indicates invalid login credentials
	ErrInvalidCredentials = &errs.Error{
		Code:    errs.Unauthenticated,
		Message: "Invalid email or password.",
	}

	// ErrUserNotFound indicates that the user was not found
	ErrUserNotFound = &errs.Error{
		Code:    errs.NotFound,
		Message: "User not found.",
	}

	// ErrUserInactive indicates that the user account is inactive
	ErrUserInactive = &errs.Error{
		Code:    errs.Unauthenticated,
		Message: "User account not found or inactive.",
	}

	// ErrWeakPassword indicates that the password doesn't meet security requirements
	ErrWeakPassword = &errs.Error{
		Code:    errs.InvalidArgument,
		Message: "Password must be at least 8 characters long and contain both letters and numbers.",
	}

	// ErrInvalidVerificationCode indicates an invalid verification code
	ErrInvalidVerificationCode = &errs.Error{
		Code:    errs.InvalidArgument,
		Message: "Invalid verification code.",
	}

	// ErrVerificationCodeExpired indicates that the verification code has expired
	ErrVerificationCodeExpired = &errs.Error{
		Code:    errs.DeadlineExceeded,
		Message: "Verification code has expired. Please request a new one.",
	}

	// ErrVerificationCodeUsed indicates that the verification code has already been used
	ErrVerificationCodeUsed = &errs.Error{
		Code:    errs.FailedPrecondition,
		Message: "Verification code has already been used.",
	}

	// ErrEmailAlreadyVerified indicates that the email is already verified
	ErrEmailAlreadyVerified = &errs.Error{
		Code:    errs.FailedPrecondition,
		Message: "Email is already verified.",
	}

	// ErrInvalidRefreshToken indicates an invalid or expired refresh token
	ErrInvalidRefreshToken = &errs.Error{
		Code:    errs.Unauthenticated,
		Message: "Invalid or expired refresh token.",
	}

	// ErrRateLimitExceeded indicates that rate limit has been exceeded
	ErrRateLimitExceeded = &errs.Error{
		Code:    errs.ResourceExhausted,
		Message: "Too many attempts. Please try again later.",
	}

	// ErrInternalError indicates an internal server error
	ErrInternalError = &errs.Error{
		Code:    errs.Internal,
		Message: "An internal error occurred. Please try again later.",
	}
)

// NewRateLimitError creates a rate limit error with a custom message
func NewRateLimitError(message string) *errs.Error {
	return &errs.Error{
		Code:    errs.ResourceExhausted,
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
