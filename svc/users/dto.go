// Package users provides user profile and address management services
package users

import "time"

// UserProfileResponse represents user profile information
type UserProfileResponse struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Email           string     `json:"email"`
	Phone           string     `json:"phone"`
	CityID          int64      `json:"city_id"`
	Role            string     `json:"role"`
	State           string     `json:"state"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// UpdateProfileRequest represents the profile update request
type UpdateProfileRequest struct {
	Name   *string `json:"name,omitempty" validate:"omitempty,min=2,max=100"`
	Phone  *string `json:"phone,omitempty" validate:"omitempty"`
	CityID *int64  `json:"city_id,omitempty" validate:"omitempty,min=1"`
}

// UpdateProfileResponse represents the profile update response
type UpdateProfileResponse struct {
	User    UserProfileResponse `json:"user"`
	Message string              `json:"message"`
}

// VerificationRequest represents a verification request
type VerificationRequest struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	Note       string     `json:"note"`
	Status     string     `json:"status"`
	ReviewedBy *int64     `json:"reviewed_by"`
	ReviewedAt *time.Time `json:"reviewed_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// VerificationRequestDetail represents a detailed verification request with admin_reason
type VerificationRequestDetail struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	Note        string     `json:"note"`
	Status      string     `json:"status"` // pending, approved, rejected
	AdminReason *string    `json:"admin_reason"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// VerificationRequestInput represents the verification request creation
type VerificationRequestInput struct {
	Note string `json:"note" validate:"required,min=10,max=500"`
}

// VerificationRequestOutput represents the verification request creation response
type VerificationRequestOutput struct {
	Request VerificationRequest `json:"request"`
	Message string              `json:"message"`
}

// ReviewVerificationRequest represents the admin verification review request
type ReviewVerificationRequest struct {
	Reason string `json:"reason,omitempty" validate:"omitempty,max=500"`
}

// ReviewVerificationResponse represents the admin verification review response
type ReviewVerificationResponse struct {
	Request VerificationRequest `json:"request"`
	Message string              `json:"message"`
}

// ListVerificationRequestsQuery represents list filters for verification requests (admin)
type ListVerificationRequestsQuery struct {
	Status string `json:"status,omitempty"` // optional: pending/approved/rejected; default: pending
	Page   int    `json:"page,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// VerificationRequestListItem represents a verification request joined with basic user info (admin)
type VerificationRequestListItem struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	UserName   string     `json:"user_name"`
	UserEmail  string     `json:"user_email"`
	UserPhone  string     `json:"user_phone"`
	Note       string     `json:"note"`
	Status     string     `json:"status"`
	ReviewedBy *int64     `json:"reviewed_by"`
	ReviewedAt *time.Time `json:"reviewed_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ListVerificationRequestsResponse represents the list response for admin
type ListVerificationRequestsResponse struct {
	Items []VerificationRequestListItem `json:"items"`
	Total int                           `json:"total"`
	Page  int                           `json:"page"`
	Limit int                           `json:"limit"`
}

// Address represents a user address
type Address struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	CityID     int64      `json:"city_id"`
	Label      string     `json:"label"`
	Line1      string     `json:"line1"`
	Line2      *string    `json:"line2"`
	IsDefault  bool       `json:"is_default"`
	ArchivedAt *time.Time `json:"archived_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// AddressInput represents the address creation request
type AddressInput struct {
	CityID    int64   `json:"city_id" validate:"required,min=1"`
	Label     string  `json:"label" validate:"required,min=2,max=50"`
	Line1     string  `json:"line1" validate:"required,min=5,max=200"`
	Line2     *string `json:"line2,omitempty" validate:"omitempty,max=200"`
	IsDefault *bool   `json:"is_default,omitempty"`
}

// AddressOutput represents the address creation response
type AddressOutput struct {
	Address Address `json:"address"`
	Message string  `json:"message"`
}

// UpdateAddressRequest represents the address update request
type UpdateAddressRequest struct {
	CityID     *int64     `json:"city_id,omitempty" validate:"omitempty,min=1"`
	Label      *string    `json:"label,omitempty" validate:"omitempty,min=2,max=50"`
	Line1      *string    `json:"line1,omitempty" validate:"omitempty,min=5,max=200"`
	Line2      *string    `json:"line2,omitempty" validate:"omitempty,max=200"`
	IsDefault  *bool      `json:"is_default,omitempty"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

// UpdateAddressResponse represents the address update response
type UpdateAddressResponse struct {
	Address Address `json:"address"`
	Message string  `json:"message"`
}

// ListAddressesResponse represents the addresses list response
type ListAddressesResponse struct {
	Addresses []Address `json:"addresses"`
	Total     int       `json:"total"`
}
