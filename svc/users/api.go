// Package users provides user profile and address management services
package users

import (
	"context"
	"strconv"
	"time"

	"encore.app/pkg/errs"
	"encore.app/pkg/ratelimit"
	"encore.dev/beta/auth"
)

// isAdmin checks if the authenticated user has admin role
func isAdmin() bool {
	if d := auth.Data(); d != nil {
		// Case 1: map[string]any
		if m, ok := d.(map[string]interface{}); ok {
			if role, ok := m["role"].(string); ok {
				return role == "admin"
			}
		}
		// Case 2: struct with Role field
		if v, ok := d.(interface{ GetRole() string }); ok {
			return v.GetRole() == "admin"
		}
	}
	return false
}

// GetProfile returns the current user's profile information
//
//encore:api auth method=GET path=/me
func (s *Service) GetProfile(ctx context.Context) (*UserProfileResponse, error) {
	// Get user ID from auth context
	userID, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "المستخدم غير مصادق."}
	}

	// Convert auth.UID (string) to int64
	userIDInt64, err := strconv.ParseInt(string(userID), 10, 64)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "معرّف المستخدم غير صالح."}
	}

	return s.GetUserProfile(ctx, userIDInt64)
}

// UpdateProfile updates the current user's profile information
//
//encore:api auth method=PATCH path=/me
func (s *Service) UpdateProfile(ctx context.Context, req *UpdateProfileRequest) (*UpdateProfileResponse, error) {
	// Get user ID from auth context
	userID, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{
			Code:    errs.Unauthenticated,
			Message: "المستخدم غير مصادق.",
		}
	}

	// Convert auth.UID (string) to int64
	userIDInt64, err := strconv.ParseInt(string(userID), 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.Internal,
			Message: "معرّف المستخدم غير صالح.",
		}
	}

	return s.UpdateUserProfile(ctx, userIDInt64, req)
}

// CreateVerificationRequest creates a new verification request for the current user
//
//encore:api auth method=POST path=/verify/requests
func (s *Service) CreateVerificationRequest(ctx context.Context, req *VerificationRequestInput) (*VerificationRequestOutput, error) {
	// Get user ID from auth context
	userID, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "المستخدم غير مصادق."}
	}

	// Convert auth.UID (string) to int64
	userIDInt64, err := strconv.ParseInt(string(userID), 10, 64)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "معرّف المستخدم غير صالح."}
	}

	return s.ProcessVerificationRequest(ctx, userIDInt64, req)
}

// ApproveVerificationRequest approves a verification request (Admin only)
//
//encore:api auth method=POST path=/verify/requests/:id/approve
func (s *Service) ApproveVerificationRequest(ctx context.Context, id int64, req *ReviewVerificationRequest) (*ReviewVerificationResponse, error) {
	// Get admin user ID from auth context
	adminUserID, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{
			Code:    errs.Unauthenticated,
			Message: "المستخدم غير مصادق.",
		}
	}

	// Enforce admin-only access using role from auth.Data()
	if !isAdmin() {
		return nil, &errs.Error{
			Code:    errs.Forbidden,
			Message: "يتطلب صلاحيات مدير",
		}
	}

	// Convert auth.UID (string) to int64
	adminUserIDInt64, err := strconv.ParseInt(string(adminUserID), 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.Internal,
			Message: "معرّف المستخدم غير صالح.",
		}
	}

	return s.ProcessVerificationApproval(ctx, id, adminUserIDInt64, req)
}

// RejectVerificationRequest rejects a verification request (Admin only)
//
//encore:api auth method=POST path=/verify/requests/:id/reject
func (s *Service) RejectVerificationRequest(ctx context.Context, id int64, req *ReviewVerificationRequest) (*ReviewVerificationResponse, error) {
	// Get admin user ID from auth context
	adminUserID, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{
			Code:    errs.Unauthenticated,
			Message: "المستخدم غير مصادق.",
		}
	}

	// Enforce admin-only access using role from auth.Data()
	if !isAdmin() {
		return nil, &errs.Error{
			Code:    errs.Forbidden,
			Message: "يتطلب صلاحيات مدير",
		}
	}

	// Convert auth.UID (string) to int64
	adminUserIDInt64, err := strconv.ParseInt(string(adminUserID), 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.Internal,
			Message: "معرّف المستخدم غير صالح.",
		}
	}

	return s.ProcessVerificationRejection(ctx, id, adminUserIDInt64, req)
}

// ListAddresses returns all addresses for the current user (available to registered users)
//
//encore:api auth method=GET path=/addresses
func (s *Service) ListAddresses(ctx context.Context) (*ListAddressesResponse, error) {
	// Get user ID from auth context
	userID, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{
			Code:    errs.Unauthenticated,
			Message: "المستخدم غير مصادق.",
		}
	}

	// Convert auth.UID (string) to int64
	userIDInt64, err := strconv.ParseInt(string(userID), 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.Internal,
			Message: "معرّف المستخدم غير صالح.",
		}
	}

	return s.GetAddressesForUser(ctx, userIDInt64)
}

// CreateAddress creates a new address for the current user (requires email verification)
//
//encore:api auth method=POST path=/addresses
func (s *Service) CreateAddress(ctx context.Context, req *AddressInput) (*AddressOutput, error) {
	// Get user ID from auth context
	userID, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{
			Code:    errs.Unauthenticated,
			Message: "المستخدم غير مصادق.",
		}
	}

	// Rate limiting: 10 requests per minute per user (as per PRD)
	uid, _ := strconv.ParseInt(string(userID), 10, 64)
	rateLimitKey := ratelimit.GenerateUserKey("create_address", uid)
	addressRL := ratelimit.NewRateLimiter(ratelimit.RateLimitConfig{
		MaxAttempts: 10,
		Window:      time.Minute,
	})
	if err := addressRL.RecordAttempt(rateLimitKey); err != nil {
		return nil, &errs.Error{
			Code:    errs.TooManyRequests,
			Message: "تجاوزت حد المحاولات. حاول لاحقاً",
		}
	}

	// Convert auth.UID (string) to int64
	userIDInt64, err := strconv.ParseInt(string(userID), 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.Internal,
			Message: "معرّف المستخدم غير صالح.",
		}
	}

	return s.ProcessAddressCreation(ctx, userIDInt64, req)
}

// UpdateAddress updates an existing address for the current user (requires email verification)
//
//encore:api auth method=PATCH path=/addresses/:id
func (s *Service) UpdateAddress(ctx context.Context, id int64, req *UpdateAddressRequest) (*UpdateAddressResponse, error) {
	// Get user ID from auth context
	userID, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{
			Code:    errs.Unauthenticated,
			Message: "المستخدم غير مصادق.",
		}
	}

	// Rate limiting: 10 requests per minute per user (as per PRD)
	uid, _ := strconv.ParseInt(string(userID), 10, 64)
	rateLimitKey := ratelimit.GenerateUserKey("update_address", uid)
	addressRL := ratelimit.NewRateLimiter(ratelimit.RateLimitConfig{
		MaxAttempts: 10,
		Window:      time.Minute,
	})
	if err := addressRL.RecordAttempt(rateLimitKey); err != nil {
		return nil, &errs.Error{
			Code:    errs.TooManyRequests,
			Message: "تجاوزت حد المحاولات. حاول لاحقاً",
		}
	}

	// Convert auth.UID (string) to int64
	userIDInt64, err := strconv.ParseInt(string(userID), 10, 64)
	if err != nil {
		return nil, &errs.Error{
			Code:    errs.Internal,
			Message: "معرّف المستخدم غير صالح.",
		}
	}

	return s.ProcessAddressUpdate(ctx, userIDInt64, id, req)
}
