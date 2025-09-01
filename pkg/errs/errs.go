package errs

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"encore.dev"
)

// Error codes
const (
	// 400 Bad Request
	InvalidArgument    = "INVALID_ARGUMENT"
	ValidationFailed   = "VALIDATION_FAILED"
	FailedPrecondition = "FAILED_PRECONDITION"

	// 401 Unauthorized
	Unauthenticated = "UNAUTHENTICATED"
	TokenExpired    = "TOKEN_EXPIRED"

	// 403 Forbidden
	Forbidden        = "FORBIDDEN"
	PermissionDenied = "PERMISSION_DENIED"

	// 404 Not Found
	NotFound = "NOT_FOUND"

	// 409 Conflict
	Conflict      = "CONFLICT"
	AlreadyExists = "ALREADY_EXISTS"

	// 422 Unprocessable Entity
	UnprocessableEntity = "UNPROCESSABLE_ENTITY"

	// 429 Too Many Requests
	TooManyRequests   = "TOO_MANY_REQUESTS"
	ResourceExhausted = "RESOURCE_EXHAUSTED"

	// 500 Internal Server Error
	Internal      = "INTERNAL_ERROR"
	Unimplemented = "UNIMPLEMENTED"

	// 503 Service Unavailable
	ServiceUnavailable = "SERVICE_UNAVAILABLE"

	// 504 Gateway Timeout
	DeadlineExceeded = "DEADLINE_EXCEEDED"

	// Shipping domain codes (SHP)
	ShpNotFound     = "SHP_NOT_FOUND"
	ShpOrderNotPaid = "SHP_ORDER_NOT_PAID"

	// Authentication domain codes (AUTH)
	AuthEmailTaken                = "AUTH_EMAIL_TAKEN"
	AuthInvalidCredentials        = "AUTH_INVALID_CREDENTIALS"
	AuthUserNotFound              = "AUTH_USER_NOT_FOUND"
	AuthUserInactive              = "AUTH_USER_INACTIVE"
	AuthWeakPassword              = "AUTH_WEAK_PASSWORD"
	AuthInvalidVerificationCode   = "AUTH_INVALID_VERIFICATION_CODE"
	AuthVerificationCodeExpired   = "AUTH_VERIFICATION_CODE_EXPIRED"
	AuthVerificationCodeUsed      = "AUTH_VERIFICATION_CODE_USED"
	AuthEmailAlreadyVerified      = "AUTH_EMAIL_ALREADY_VERIFIED"
	AuthInvalidRefreshToken       = "AUTH_INVALID_REFRESH_TOKEN"
	AuthRateLimitExceeded         = "AUTH_RATE_LIMIT_EXCEEDED"
	AuthTokenExpired              = "AUTH_TOKEN_EXPIRED"
	AuthUnauthenticated           = "AUTH_UNAUTHENTICATED"
	AuthForbidden                 = "AUTH_FORBIDDEN"
	AuthEmailVerifyRequired       = "AUTH_EMAIL_VERIFY_REQUIRED"
	AuthEmailVerifyRequiredAtCheckout = "AUTH_EMAIL_VERIFY_REQUIRED_AT_CHECKOUT"

	// Auction/Bidding domain codes
	BidVerifiedRequired = "BID_VERIFIED_REQUIRED"

	// Notification domain codes (NOTIF)
	NotifUnauthenticated        = "NOTIF_UNAUTHENTICATED"
	NotifInvalidTemplate        = "NOTIF_INVALID_TEMPLATE"
	NotifQueueInsertFailed      = "NOTIF_QUEUE_INSERT_FAILED"
	NotifQueueQueryFailed       = "NOTIF_QUEUE_QUERY_FAILED"
	NotifListQueryFailed        = "NOTIF_LIST_QUERY_FAILED"
	NotifRetentionArchiveFailed = "NOTIF_RETENTION_ARCHIVE_FAILED"
	NotifRetentionDeleteFailed  = "NOTIF_RETENTION_DELETE_FAILED"
	NotifUpdateFailed           = "NOTIF_UPDATE_FAILED"
	NotifNotFound               = "NOTIF_NOT_FOUND"

	// Payment domain codes (PAY)
	PayMethodDisabled = "PAY_METHOD_DISABLED"
	InvNotFound       = "INV_NOT_FOUND"
	PayUnauthenticated = "PAY_UNAUTHENTICATED"
	PayInvalidRequest  = "PAY_INVALID_REQUEST"
	PaySessionExpired  = "PAY_SESSION_EXPIRED"
)

// Error represents a structured error
type Error struct {
	Code          string      `json:"code"`
	Message       string      `json:"message"`
	CorrelationID string      `json:"correlation_id,omitempty"`
	Details       interface{} `json:"details,omitempty"`
}

// Error implements the error interface
func (e *Error) Error() string {
	if e.CorrelationID != "" {
		return fmt.Sprintf("[%s] %s: %s", e.CorrelationID, e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// HTTPStatus returns the HTTP status code for the error
func (e *Error) HTTPStatus() int {
	switch e.Code {
	// Domain-specific codes
	case "AUC_NEW_FORBIDDEN_STATE":
		return http.StatusConflict
	case "PAY_IDEM_MISMATCH":
		return http.StatusConflict
	case "ORD_PIGEON_ALREADY_PENDING":
		return http.StatusConflict
	case "ORD_NOT_FOUND":
		return http.StatusNotFound
	case "AUTH_EMAIL_VERIFY_REQUIRED_AT_CHECKOUT":
		return http.StatusForbidden

	// Authentication domain mappings
	case AuthEmailTaken:
		return http.StatusConflict
	case AuthInvalidCredentials, AuthUserNotFound, AuthUserInactive:
		return http.StatusUnauthorized
	case AuthWeakPassword, AuthInvalidVerificationCode:
		return http.StatusBadRequest
	case AuthVerificationCodeExpired:
		return http.StatusGone
	case AuthVerificationCodeUsed, AuthEmailAlreadyVerified:
		return http.StatusConflict
	case AuthInvalidRefreshToken, AuthTokenExpired, AuthUnauthenticated:
		return http.StatusUnauthorized
	case AuthRateLimitExceeded:
		return http.StatusTooManyRequests
	case AuthForbidden, AuthEmailVerifyRequired:
		return http.StatusForbidden

	// Bidding domain mappings
	case BidVerifiedRequired:
		return http.StatusForbidden

	case ShpNotFound:
		return http.StatusNotFound
	case ShpOrderNotPaid:
		return http.StatusConflict

	// Notification domain mappings
	case NotifUnauthenticated:
		return http.StatusUnauthorized

	// Payment domain mappings
	case PayMethodDisabled:
		return http.StatusBadRequest
	case InvNotFound:
		return http.StatusNotFound
	case PayUnauthenticated:
		return http.StatusUnauthorized
	case PayInvalidRequest:
		return http.StatusBadRequest
	case PaySessionExpired:
		return http.StatusGone
	case NotifInvalidTemplate:
		return http.StatusBadRequest
	case NotifQueueInsertFailed, NotifQueueQueryFailed, NotifListQueryFailed, NotifRetentionArchiveFailed, NotifRetentionDeleteFailed, NotifUpdateFailed:
		return http.StatusInternalServerError
	case NotifNotFound:
		return http.StatusNotFound

	// Generic mappings
	case InvalidArgument, ValidationFailed:
		return http.StatusBadRequest
	case FailedPrecondition:
		return http.StatusBadRequest
	case Unauthenticated, TokenExpired:
		return http.StatusUnauthorized
	case Forbidden, PermissionDenied:
		return http.StatusForbidden
	case NotFound:
		return http.StatusNotFound
	case Conflict, AlreadyExists:
		return http.StatusConflict
	case UnprocessableEntity:
		return http.StatusUnprocessableEntity
	case TooManyRequests, ResourceExhausted:
		return http.StatusTooManyRequests
	case ServiceUnavailable:
		return http.StatusServiceUnavailable
	case Unimplemented:
		return http.StatusNotImplemented
	case DeadlineExceeded:
		return http.StatusGatewayTimeout
	default:
		// Heuristics for domain-prefixed codes and common terms
		lc := strings.ToLower(e.Code)
		switch {
		case strings.Contains(lc, "not_found"):
			return http.StatusNotFound
		case strings.Contains(lc, "conflict"):
			return http.StatusConflict
		case strings.Contains(lc, "unauth"):
			return http.StatusUnauthorized
		case strings.Contains(lc, "forbidden"):
			return http.StatusForbidden
		case strings.Contains(lc, "rate_limit") || strings.Contains(lc, "too_many"):
			return http.StatusTooManyRequests
		case strings.HasPrefix(strings.ToUpper(e.Code), "AUC_") ||
			strings.HasPrefix(strings.ToUpper(e.Code), "PAY_") ||
			strings.HasPrefix(strings.ToUpper(e.Code), "ORD_") ||
			strings.HasPrefix(strings.ToUpper(e.Code), "USR_") ||
			strings.HasPrefix(strings.ToUpper(e.Code), "CAT_") ||
			strings.HasPrefix(strings.ToUpper(e.Code), "INV_") ||
			strings.HasPrefix(strings.ToUpper(e.Code), "SHP_") ||
			strings.HasPrefix(strings.ToUpper(e.Code), "BID_"):
			return http.StatusBadRequest
		default:
			return http.StatusInternalServerError
		}
	}
}

// New creates a new error
func New(code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// WithDetails adds details to an error
func (e *Error) WithDetails(details interface{}) *Error {
	e.Details = details
	return e
}

// WithCorrelationID adds correlation ID to an error
func (e *Error) WithCorrelationID(correlationID string) *Error {
	e.CorrelationID = correlationID
	return e
}

// CorrelationIDFromContext returns a correlation_id tied to current request if possible,
// otherwise generates a time-based fallback.
func CorrelationIDFromContext(ctx context.Context) string {
	if ctx != nil {
		if req := encore.CurrentRequest(); req != nil {
			// Encore does not expose a canonical request id yet; use path + timestamp surrogate
			if req.Path != "" {
				return fmt.Sprintf("%s-%d", req.Path, time.Now().UnixNano())
			}
		}
	}
	return fmt.Sprintf("cid-%d", time.Now().UnixNano())
}

// E creates a domain-coded error and auto-fills correlation_id from context.
func E(ctx context.Context, code, message string) *Error {
	return New(code, message).WithCorrelationID(CorrelationIDFromContext(ctx))
}

// EDetails creates a domain-coded error with details and auto correlation_id.
func EDetails(ctx context.Context, code, message string, details interface{}) *Error {
	return (&Error{Code: code, Message: message, Details: details}).WithCorrelationID(CorrelationIDFromContext(ctx))
}

// NewConflict creates a 409 Conflict error with optional details
func NewConflict(message string, details interface{}) *Error {
	return &Error{Code: Conflict, Message: message, Details: details}
}

// MapCustomAuctionCode normalizes custom auction codes to 409 Conflict with details
// Example: MapCustomAuctionCode("AUC_NEW_FORBIDDEN_STATE", "cannot create auction", map[string]any{"product_id": 123})
func MapCustomAuctionCode(customCode, message string, details interface{}) *Error {
	return &Error{Code: Conflict, Message: message, Details: map[string]interface{}{
		"custom_code": customCode,
		"context":     details,
	}}
}
