// Package users provides user profile and address management services
package users

import (
	"context"

	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"
	"encore.app/svc/notifications"
)

// Database instance for the users service
var db = sqldb.Named("coredb")

//encore:service
type Service struct {
	repo *Repository
}

// initService initializes the users service
func initService() (*Service, error) {
	repo := NewRepository(db)

	return &Service{
		repo: repo,
	}, nil
}

// GetUserProfile retrieves a user's profile information
func (s *Service) GetUserProfile(ctx context.Context, userID int64) (*UserProfileResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &UserProfileResponse{
		ID:              user.ID,
		Name:            user.Name,
		Email:           user.Email,
		Phone:           user.Phone,
		CityID:          user.CityID,
		Role:            user.Role,
		State:           user.State,
		EmailVerifiedAt: user.EmailVerifiedAt,
		CreatedAt:       user.CreatedAt,
		UpdatedAt:       user.UpdatedAt,
	}, nil
}

// UpdateUserProfile updates a user's profile information
func (s *Service) UpdateUserProfile(ctx context.Context, userID int64, req *UpdateProfileRequest) (*UpdateProfileResponse, error) {
	// Get current user to check permissions
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Check if user is active
	if user.State != "active" {
		return nil, ErrUserInactive
	}

	// Validate city if provided
	if req.CityID != nil {
		exists, err := s.repo.CityExists(ctx, *req.CityID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrInvalidCity
		}
	}

	// Update user profile
	updatedUser, err := s.repo.UpdateUser(ctx, userID, req)
	if err != nil {
		return nil, err
	}

	return &UpdateProfileResponse{
		User: UserProfileResponse{
			ID:              updatedUser.ID,
			Name:            updatedUser.Name,
			Email:           updatedUser.Email,
			Phone:           updatedUser.Phone,
			CityID:          updatedUser.CityID,
			Role:            updatedUser.Role,
			State:           updatedUser.State,
			EmailVerifiedAt: updatedUser.EmailVerifiedAt,
			CreatedAt:       updatedUser.CreatedAt,
			UpdatedAt:       updatedUser.UpdatedAt,
		},
		Message: "Profile updated successfully.",
	}, nil
}

// ProcessVerificationRequest creates a new verification request
func (s *Service) ProcessVerificationRequest(ctx context.Context, userID int64, req *VerificationRequestInput) (*VerificationRequestOutput, error) {
	// Get current user to check permissions
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Check if user is active
	if user.State != "active" {
		return nil, ErrUserInactive
	}

	// Check if user already has verified role
	if user.Role == "verified" || user.Role == "admin" {
		return nil, ErrAlreadyVerified
	}

	// Check if user has pending verification request
	hasPending, err := s.repo.HasPendingVerificationRequest(ctx, userID)
	if err != nil {
		return nil, err
	}
	if hasPending {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "لديك طلب توثيق معلق بالفعل.",
		}
	}

	// Create verification request
	verificationReq, err := s.repo.CreateVerificationRequest(ctx, userID, req.Note)
	if err != nil {
		return nil, err
	}

	// Notify admins about the new verification/upgrade request (internal + email)
	// Best-effort: failures here should not fail the main request
	if admins, aerr := s.repo.ListAdminUsers(ctx); aerr == nil {
		// Load requester details for payload
		if requester, uerr := s.repo.GetUserByID(ctx, userID); uerr == nil {
			for _, admin := range admins {
				payload := map[string]any{
					"user_name":  requester.Name,
					"user_email": requester.Email,
					"request_id": verificationReq.ID,
					"note":       req.Note,
					// For email templates
					"language": "ar",
					"name":     admin.Name,
					"email":    admin.Email,
				}
				_, _ = notifications.EnqueueInternal(ctx, admin.ID, "verification_requested_admin", payload)
				_, _ = notifications.EnqueueEmail(ctx, admin.ID, "verification_requested_admin", payload)
			}
		}
	}

	return &VerificationRequestOutput{
		Request: VerificationRequest{
			ID:         verificationReq.ID,
			UserID:     verificationReq.UserID,
			Note:       verificationReq.Note,
			Status:     verificationReq.Status,
			ReviewedBy: verificationReq.ReviewedBy,
			ReviewedAt: verificationReq.ReviewedAt,
			CreatedAt:  verificationReq.CreatedAt,
		},
		Message: MsgVerificationRequested,
	}, nil
}

// ProcessVerificationApproval approves a verification request (Admin only)
func (s *Service) ProcessVerificationApproval(ctx context.Context, requestID, adminUserID int64, req *ReviewVerificationRequest) (*ReviewVerificationResponse, error) {
	// Check if admin user has admin role
	adminUser, err := s.repo.GetUserByID(ctx, adminUserID)
	if err != nil {
		return nil, err
	}
	if adminUser.Role != "admin" {

		return nil, ErrInsufficientPermissions
	}

	// Get verification request
	verificationReq, err := s.repo.GetVerificationRequestByID(ctx, requestID)
	if err != nil {
		return nil, err
	}

	// Check if request is pending
	if verificationReq.Status != "pending" {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "طلب التوثيق ليس في حالة انتظار.",
		}
	}

	// Approve verification request and update user role
	err = s.repo.ApproveVerificationRequest(ctx, requestID, adminUserID, verificationReq.UserID)
	if err != nil {
		return nil, err
	}

	// Get updated verification request
	updatedReq, err := s.repo.GetVerificationRequestByID(ctx, requestID)
	if err != nil {
		return nil, err
	}

	// Notify user (internal inbox + email) about approval
	if user, uerr := s.repo.GetUserByID(ctx, verificationReq.UserID); uerr == nil {
		payload := map[string]any{
			"email":    user.Email,
			"name":     user.Name,
			"user_name": user.Name,
			"language": "ar",
		}
		// Internal notification (inbox)
		_, _ = notifications.EnqueueInternal(ctx, user.ID, "verification_approved", payload)
		// Email notification
		_, _ = notifications.EnqueueEmail(ctx, user.ID, "verification_approved", payload)
	}

	return &ReviewVerificationResponse{
		Request: VerificationRequest{
			ID:         updatedReq.ID,
			UserID:     updatedReq.UserID,
			Note:       updatedReq.Note,
			Status:     updatedReq.Status,
			ReviewedBy: updatedReq.ReviewedBy,
			ReviewedAt: updatedReq.ReviewedAt,
			CreatedAt:  updatedReq.CreatedAt,
		},
		Message: "Verification request approved and user role upgraded.",
	}, nil
}

// ProcessVerificationRejection rejects a verification request (Admin only)
func (s *Service) ProcessVerificationRejection(ctx context.Context, requestID, adminUserID int64, req *ReviewVerificationRequest) (*ReviewVerificationResponse, error) {
	// Check if admin user has admin role
	adminUser, err := s.repo.GetUserByID(ctx, adminUserID)
	if err != nil {
		return nil, err
	}
	if adminUser.Role != "admin" {
		return nil, ErrInsufficientPermissions
	}

	// Get verification request
	verificationReq, err := s.repo.GetVerificationRequestByID(ctx, requestID)
	if err != nil {
		return nil, err
	}

	// Check if request is pending
	if verificationReq.Status != "pending" {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "طلب التوثيق ليس في حالة انتظار.",
		}
	}

	// Reject verification request
	err = s.repo.RejectVerificationRequest(ctx, requestID, adminUserID)
	if err != nil {
		return nil, err
	}

	// Get updated verification request
	updatedReq, err := s.repo.GetVerificationRequestByID(ctx, requestID)
	if err != nil {
		return nil, err
	}

	return &ReviewVerificationResponse{
		Request: VerificationRequest{
			ID:         updatedReq.ID,
			UserID:     updatedReq.UserID,
			Note:       updatedReq.Note,
			Status:     updatedReq.Status,
			ReviewedBy: updatedReq.ReviewedBy,
			ReviewedAt: updatedReq.ReviewedAt,
			CreatedAt:  updatedReq.CreatedAt,
		},
		Message: "Verification request rejected.",
	}, nil
}

// GetAddressesForUser retrieves all addresses for a user
func (s *Service) GetAddressesForUser(ctx context.Context, userID int64) (*ListAddressesResponse, error) {
	// Get current user to check permissions
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Check if user is active
	if user.State != "active" {
		return nil, ErrUserInactive
	}

	addresses, err := s.repo.GetUserAddresses(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &ListAddressesResponse{
		Addresses: addresses,
		Total:     len(addresses),
	}, nil
}

// ProcessAddressCreation creates a new address for a user (requires email verification)
func (s *Service) ProcessAddressCreation(ctx context.Context, userID int64, req *AddressInput) (*AddressOutput, error) {
	// Get current user to check permissions
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Check if user is active
	if user.State != "active" {
		return nil, ErrUserInactive
	}

	// Check if email is verified (required for address creation)
	if user.EmailVerifiedAt == nil {
		return nil, &errs.Error{
			Code:    errs.PermissionDenied,
			Message: "فعِّل بريدك الإلكتروني لإضافة العناوين.",
		}
	}

	// Validate city
	exists, err := s.repo.CityExists(ctx, req.CityID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "المدينة المحددة غير صالحة.",
		}
	}

	// Create address
	address, err := s.repo.CreateAddress(ctx, userID, req)
	if err != nil {
		return nil, err
	}

	return &AddressOutput{
		Address: *address,
		Message: "Address created successfully.",
	}, nil
}

// ProcessAddressUpdate updates an existing address for a user (requires email verification)
func (s *Service) ProcessAddressUpdate(ctx context.Context, userID, addressID int64, req *UpdateAddressRequest) (*UpdateAddressResponse, error) {
	// Get current user to check permissions
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Check if user is active
	if user.State != "active" {
		return nil, ErrUserInactive
	}

	// Check if email is verified (required for address modification)
	if user.EmailVerifiedAt == nil {
		return nil, &errs.Error{
			Code:    errs.PermissionDenied,
			Message: "فعِّل بريدك الإلكتروني لتعديل العناوين.",
		}
	}

	// Check if address belongs to user
	address, err := s.repo.GetAddressByID(ctx, addressID)
	if err != nil {
		return nil, err
	}
	if address.UserID != userID {
		return nil, &errs.Error{
			Code:    errs.PermissionDenied,
			Message: "ليس لديك صلاحية لتعديل هذا العنوان.",
		}
	}

	// Validate city if provided
	if req.CityID != nil {
		exists, err := s.repo.CityExists(ctx, *req.CityID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrInvalidCity
		}
	}

	// Update address
	updatedAddress, err := s.repo.UpdateAddress(ctx, addressID, req)
	if err != nil {
		return nil, err
	}

	var message string
	if req.ArchivedAt != nil {
		message = "Address archived successfully."
	} else {
		message = "Address updated successfully."
	}

	return &UpdateAddressResponse{
		Address: *updatedAddress,
		Message: message,
	}, nil
}
