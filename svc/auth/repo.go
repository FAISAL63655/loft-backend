// Package auth provides authentication and authorization services
package auth

import (
	"context"
	"time"

	"encore.app/pkg/authn"
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

// CityExists checks if a city exists and is enabled
func (r *Repository) CityExists(ctx context.Context, cityID int64) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cities WHERE id = $1 AND enabled = true)`, cityID).Scan(&exists)
	return exists, err
}

// EmailVerificationCode represents a record from email_verification_codes
type EmailVerificationCode struct {
	UserID    int64
	Email     string
	Code      string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// GetEmailVerificationCode fetches the latest verification code record for the given email+code
func (r *Repository) GetEmailVerificationCode(ctx context.Context, email, code string) (*EmailVerificationCode, error) {
	var rec EmailVerificationCode
	err := db.QueryRow(ctx, `
        SELECT user_id, email, code, expires_at, used_at
        FROM email_verification_codes
        WHERE email = $1 AND code = $2
        ORDER BY created_at DESC
        LIMIT 1
    `, email, code).Scan(&rec.UserID, &rec.Email, &rec.Code, &rec.ExpiresAt, &rec.UsedAt)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// CreateUser creates a new user in the database
func (r *Repository) CreateUser(ctx context.Context, email, passwordHash, name, phone string, cityID int64) (int64, error) {
	var userID int64
	err := db.QueryRow(ctx, `
		INSERT INTO users (name, email, phone, password_hash, role, state, city_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'registered', 'active', $5, (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'))
		RETURNING id
	`, name, email, phone, passwordHash, cityID).Scan(&userID)

	return userID, err
}

// CreateUserWithVerification creates a user and verification request in a single transaction
func (r *Repository) CreateUserWithVerification(ctx context.Context, email, passwordHash, name, phone string, cityID int64, verificationManager *authn.VerificationManager) (int64, *authn.VerificationCode, error) {
	// Start a transaction
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()

	// Create user
	var userID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO users (name, email, phone, password_hash, role, state, city_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'registered', 'active', $5, (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'))
		RETURNING id
	`, name, email, phone, passwordHash, cityID).Scan(&userID)
	if err != nil {
		return 0, nil, err
	}

	// Generate verification code
	verificationCode, err := verificationManager.CreateVerificationCode(userID, email)
	if err != nil {
		return 0, nil, err
	}

	// Store verification code
	_, err = tx.Exec(ctx, `
		INSERT INTO email_verification_codes (user_id, email, code, expires_at, created_at)
		VALUES ($1, $2, $3, $4, (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'))
	`, userID, email, verificationCode.Code, verificationCode.ExpiresAt)
	if err != nil {
		return 0, nil, err
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return 0, nil, err
	}

	return userID, verificationCode, nil
}

// GetUserByEmail retrieves a user by email address
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := db.QueryRow(ctx, `
		SELECT id, name, email, COALESCE(phone, '') as phone, COALESCE(city_id, 0) as city_id, 
		       password_hash, role, state, email_verified_at
		FROM users 
		WHERE email = $1 AND state = 'active'
	`, email).Scan(&user.ID, &user.Name, &user.Email, &user.Phone, &user.CityID, &user.PasswordHash, &user.Role, &user.State, &user.EmailVerifiedAt)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUserByID retrieves a user by ID
func (r *Repository) GetUserByID(ctx context.Context, userID int64) (*User, error) {
	var user User
	err := db.QueryRow(ctx, `
		SELECT id, name, email, COALESCE(phone, '') as phone, COALESCE(city_id, 0) as city_id, 
		       password_hash, role, state, email_verified_at
		FROM users 
		WHERE id = $1 AND state = 'active'
	`, userID).Scan(&user.ID, &user.Name, &user.Email, &user.Phone, &user.CityID, &user.PasswordHash, &user.Role, &user.State, &user.EmailVerifiedAt)

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

// UpdateUserVerificationStatus updates the user's email verification status (does not change role)
func (r *Repository) UpdateUserVerificationStatus(ctx context.Context, userID int64, email string) error {
	_, err := db.Exec(ctx, `
		UPDATE users 
		SET email_verified_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		WHERE id = $1 AND email = $2
	`, userID, email)

	return err
}

// CreateVerificationRequest stores a verification code in the database
func (r *Repository) CreateVerificationRequest(ctx context.Context, userID int64, email, code string, expiresAt time.Time) error {
	_, err := db.Exec(ctx, `
		INSERT INTO email_verification_codes (user_id, email, code, expires_at, created_at)
		VALUES ($1, $2, $3, $4, (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'))
	`, userID, email, code, expiresAt)

	return err
}

// MarkVerificationRequestUsed marks a verification code as used
func (r *Repository) MarkVerificationRequestUsed(ctx context.Context, userID int64, email, code string) error {
	_, err := db.Exec(ctx, `
		UPDATE email_verification_codes 
		SET used_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		WHERE user_id = $1 AND email = $2 AND code = $3
	`, userID, email, code)

	return err
}

// UpdateUserLastLogin updates the user's last login timestamp
func (r *Repository) UpdateUserLastLogin(ctx context.Context, userID int64) error {
	_, err := db.Exec(ctx, `
		UPDATE users 
		SET last_login_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'), updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		WHERE id = $1
	`, userID)

	return err
}
