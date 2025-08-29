// Package auth provides authentication and authorization services
package auth

import (
	"context"
	"time"

	"encore.dev/storage/sqldb"
)

// Database connection
var db = sqldb.Named("coredb")

// Repository handles database operations for authentication
type Repository struct{}

// NewRepository creates a new authentication repository
func NewRepository() *Repository {
	return &Repository{}
}

// CreateUser creates a new user in the database
func (r *Repository) CreateUser(ctx context.Context, email, passwordHash, name string) (int64, error) {
	var userID int64
	err := db.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, role, state, name, created_at, updated_at)
		VALUES ($1, $2, 'unverified', 'active', $3, NOW(), NOW())
		RETURNING id
	`, email, passwordHash, name).Scan(&userID)

	return userID, err
}

// GetUserByEmail retrieves a user by email address
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := db.QueryRow(ctx, `
		SELECT id, email, password_hash, role, state, COALESCE(name, '') as name
		FROM users 
		WHERE email = $1 AND state = 'active'
	`, email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.State, &user.Name)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUserByID retrieves a user by ID
func (r *Repository) GetUserByID(ctx context.Context, userID int64) (*User, error) {
	var user User
	err := db.QueryRow(ctx, `
		SELECT id, email, password_hash, role, state, COALESCE(name, '') as name
		FROM users 
		WHERE id = $1 AND state = 'active'
	`, userID).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.State, &user.Name)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

// UserExists checks if a user with the given email exists
func (r *Repository) UserExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)
	`, email).Scan(&exists)

	return exists, err
}

// UserExistsByID checks if a user with the given ID exists and is active
func (r *Repository) UserExistsByID(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND state = 'active')
	`, userID).Scan(&exists)

	return exists, err
}

// UpdateUserVerificationStatus updates the user's verification status
func (r *Repository) UpdateUserVerificationStatus(ctx context.Context, userID int64, email string) error {
	_, err := db.Exec(ctx, `
		UPDATE users 
		SET role = 'verified', email_verified_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND email = $2
	`, userID, email)

	return err
}

// CreateVerificationRequest stores a verification request in the database
func (r *Repository) CreateVerificationRequest(ctx context.Context, userID int64, email, code string, expiresAt time.Time) error {
	_, err := db.Exec(ctx, `
		INSERT INTO verification_requests (user_id, email, code, expires_at, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`, userID, email, code, expiresAt)

	return err
}

// MarkVerificationRequestUsed marks a verification request as used
func (r *Repository) MarkVerificationRequestUsed(ctx context.Context, userID int64, email, code string) error {
	_, err := db.Exec(ctx, `
		UPDATE verification_requests 
		SET used_at = NOW()
		WHERE user_id = $1 AND email = $2 AND code = $3
	`, userID, email, code)

	return err
}

// UpdateUserLastLogin updates the user's last login timestamp
func (r *Repository) UpdateUserLastLogin(ctx context.Context, userID int64) error {
	_, err := db.Exec(ctx, `
		UPDATE users 
		SET last_login_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, userID)

	return err
}
