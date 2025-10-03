// Package users provides user profile and address management services
package users

import (
    "context"
    "database/sql"
    "fmt"
    "strings"
    "time"

    "encore.dev/beta/errs"
    "encore.dev/storage/sqldb"
)

// Repository handles database operations for users
type Repository struct {
    db *sqldb.Database
}


// ListAdminUsers returns active admin users (id, name, email)
func (r *Repository) ListAdminUsers(ctx context.Context) ([]User, error) {
    rows, err := r.db.Query(ctx, `
        SELECT id, COALESCE(name,''), COALESCE(email,'')
        FROM users
        WHERE role = 'admin' AND state = 'active'
    `)
    if err != nil {
        return nil, &errs.Error{Code: errs.Internal, Message: "تعذر جلب المدراء"}
    }
    defer rows.Close()
    var res []User
    for rows.Next() {
        var u User
        if err := rows.Scan(&u.ID, &u.Name, &u.Email); err != nil {
            return nil, &errs.Error{Code: errs.Internal, Message: "تعذر قراءة بيانات المدير"}
        }
        res = append(res, u)
    }
    if err := rows.Err(); err != nil {
        return nil, &errs.Error{Code: errs.Internal, Message: "تعذر إتمام قراءة المدراء"}
    }
    return res, nil
}

// NewRepository creates a new users repository
func NewRepository(db *sqldb.Database) *Repository {
    return &Repository{db: db}
}

// User represents a user entity from the database
type User struct {
    ID              int64
    Name            string
    Email           string
    Phone           string
    CityID          int64
    PasswordHash    string
    Role            string
    State           string
    EmailVerifiedAt *time.Time
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

// VerificationRequestDB represents a verification request entity from the database
type VerificationRequestDB struct {
    ID         int64
    UserID     int64
    Note       string
    Status     string
    ReviewedBy *int64
    ReviewedAt *time.Time
    CreatedAt  time.Time
}

// GetUserByID retrieves a user by ID
func (r *Repository) GetUserByID(ctx context.Context, userID int64) (*User, error) {
    query := `
        SELECT id, name, email, COALESCE(phone, '') as phone, COALESCE(city_id, 0) as city_id,
               password_hash, role, state, email_verified_at, created_at, updated_at
        FROM users WHERE id = $1`

    var user User
    err := r.db.QueryRow(ctx, query, userID).Scan(
        &user.ID, &user.Name, &user.Email, &user.Phone, &user.CityID,
        &user.PasswordHash, &user.Role, &user.State, &user.EmailVerifiedAt,
        &user.CreatedAt, &user.UpdatedAt,
    )
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, &errs.Error{Code: errs.NotFound, Message: "المستخدم غير موجود."}
        }
        return nil, &errs.Error{Code: errs.Internal, Message: "خطأ في جلب بيانات المستخدم."}
    }
    return &user, nil
}

// UpdateUser updates user profile information
func (r *Repository) UpdateUser(ctx context.Context, userID int64, req *UpdateProfileRequest) (*User, error) {
    set := []string{}
    args := []interface{}{}
    idx := 1
    if req.Name != nil {
        set = append(set, fmt.Sprintf("name = $%d", idx))
        args = append(args, *req.Name)
        idx++
    }
    if req.Phone != nil {
        set = append(set, fmt.Sprintf("phone = $%d", idx))
        args = append(args, *req.Phone)
        idx++
    }
    if req.CityID != nil {
        set = append(set, fmt.Sprintf("city_id = $%d", idx))
        args = append(args, *req.CityID)
        idx++
    }
    if len(set) == 0 {
        return r.GetUserByID(ctx, userID)
    }
    set = append(set, "updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')")
    args = append(args, userID)

    query := `UPDATE users SET ` + strings.Join(set, ", ") + ` WHERE id = $` + fmt.Sprintf("%d", idx) + `
              RETURNING id, name, email, COALESCE(phone, '') as phone, COALESCE(city_id, 0) as city_id,
                        password_hash, role, state, email_verified_at, created_at, updated_at`
    var user User
    if err := r.db.QueryRow(ctx, query, args...).Scan(
        &user.ID, &user.Name, &user.Email, &user.Phone, &user.CityID,
        &user.PasswordHash, &user.Role, &user.State, &user.EmailVerifiedAt,
        &user.CreatedAt, &user.UpdatedAt,
    ); err != nil {
        return nil, &errs.Error{Code: errs.Internal, Message: "فشل تحديث الملف الشخصي."}
    }
    return &user, nil
}

// CityExists checks if a city exists
func (r *Repository) CityExists(ctx context.Context, cityID int64) (bool, error) {
    var exists bool
    if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cities WHERE id = $1 AND enabled = true)`, cityID).Scan(&exists); err != nil {
        return false, &errs.Error{Code: errs.Internal, Message: "خطأ في التحقق من المدينة."}
    }
    return exists, nil
}

// HasPendingVerificationRequest checks if user has a pending verification request
func (r *Repository) HasPendingVerificationRequest(ctx context.Context, userID int64) (bool, error) {
    var exists bool
    if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM verification_requests WHERE user_id = $1 AND status = 'pending')`, userID).Scan(&exists); err != nil {
        return false, &errs.Error{Code: errs.Internal, Message: "خطأ في التحقق من طلبات التحقق."}
    }
    return exists, nil
}

// CreateVerificationRequest creates a new verification request
func (r *Repository) CreateVerificationRequest(ctx context.Context, userID int64, note string) (*VerificationRequestDB, error) {
    var req VerificationRequestDB
    err := r.db.QueryRow(ctx, `
        INSERT INTO verification_requests (user_id, note, status, created_at)
        VALUES ($1, $2, 'pending', (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'))
        RETURNING id, user_id, note, status, reviewed_by, reviewed_at, created_at`,
        userID, note,
    ).Scan(&req.ID, &req.UserID, &req.Note, &req.Status, &req.ReviewedBy, &req.ReviewedAt, &req.CreatedAt)
    if err != nil {
        return nil, &errs.Error{Code: errs.Internal, Message: "فشل إنشاء طلب التحقق."}
    }
    return &req, nil
}

// GetVerificationRequestByID retrieves a verification request by ID
func (r *Repository) GetVerificationRequestByID(ctx context.Context, requestID int64) (*VerificationRequestDB, error) {
    var req VerificationRequestDB
    err := r.db.QueryRow(ctx, `
        SELECT id, user_id, note, status, reviewed_by, reviewed_at, created_at
        FROM verification_requests WHERE id = $1`, requestID,
    ).Scan(&req.ID, &req.UserID, &req.Note, &req.Status, &req.ReviewedBy, &req.ReviewedAt, &req.CreatedAt)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, &errs.Error{Code: errs.NotFound, Message: "طلب التحقق غير موجود."}
        }
        return nil, &errs.Error{Code: errs.Internal, Message: "خطأ في قراءة طلب التحقق."}
    }
    return &req, nil
}

// ApproveVerificationRequest approves a verification request and updates user role
func (r *Repository) ApproveVerificationRequest(ctx context.Context, requestID, adminUserID, targetUserID int64) error {
    tx, err := r.db.Begin(ctx)
    if err != nil { return &errs.Error{Code: errs.Internal, Message: "تعذر بدء المعاملة."} }
    defer tx.Rollback()

    if _, err := tx.Exec(ctx, `
        UPDATE verification_requests
        SET status = 'approved', reviewed_by = $1, reviewed_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
        WHERE id = $2`, adminUserID, requestID); err != nil {
        return &errs.Error{Code: errs.Internal, Message: "تعذر تحديث طلب التحقق."}
    }

    if _, err := tx.Exec(ctx, `
        UPDATE users SET role = 'verified', updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
        WHERE id = $1`, targetUserID); err != nil {
        return &errs.Error{Code: errs.Internal, Message: "تعذر ترقية دور المستخدم."}
    }

    if err := tx.Commit(); err != nil { return &errs.Error{Code: errs.Internal, Message: "خطأ في المعاملة."} }
    return nil
}

// RejectVerificationRequest rejects a verification request
func (r *Repository) RejectVerificationRequest(ctx context.Context, requestID, adminUserID int64) error {
    if _, err := r.db.Exec(ctx, `
        UPDATE verification_requests
        SET status = 'rejected', reviewed_by = $1, reviewed_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
        WHERE id = $2`, adminUserID, requestID); err != nil {
        return &errs.Error{Code: errs.Internal, Message: "تعذر رفض طلب التحقق."}
    }
    return nil
}

// GetUserAddresses retrieves all addresses for a user
func (r *Repository) GetUserAddresses(ctx context.Context, userID int64) ([]Address, error) {
    rows, err := r.db.Query(ctx, `
        SELECT id, user_id, city_id, label, line1, line2, is_default, archived_at, created_at, updated_at
        FROM addresses WHERE user_id = $1
        ORDER BY is_default DESC, created_at DESC`, userID)
    if err != nil { return nil, &errs.Error{Code: errs.Internal, Message: "تعذر جلب العناوين."} }
    defer rows.Close()

    var res []Address
    for rows.Next() {
        var a Address
        if err := rows.Scan(&a.ID, &a.UserID, &a.CityID, &a.Label, &a.Line1, &a.Line2, &a.IsDefault, &a.ArchivedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
            return nil, &errs.Error{Code: errs.Internal, Message: "تعذر قراءة العنوان."}
        }
        res = append(res, a)
    }
    if err := rows.Err(); err != nil { return nil, &errs.Error{Code: errs.Internal, Message: "خطأ في قراءة العناوين."} }
    return res, nil
}

// GetAddressByID retrieves an address by ID
func (r *Repository) GetAddressByID(ctx context.Context, addressID int64) (*Address, error) {
    var a Address
    err := r.db.QueryRow(ctx, `
        SELECT id, user_id, city_id, label, line1, line2, is_default, archived_at, created_at, updated_at
        FROM addresses WHERE id = $1`, addressID,
    ).Scan(&a.ID, &a.UserID, &a.CityID, &a.Label, &a.Line1, &a.Line2, &a.IsDefault, &a.ArchivedAt, &a.CreatedAt, &a.UpdatedAt)
    if err != nil {
        if err == sql.ErrNoRows { return nil, &errs.Error{Code: errs.NotFound, Message: "العنوان غير موجود."} }
        return nil, &errs.Error{Code: errs.Internal, Message: "خطأ في جلب العنوان."}
    }
    return &a, nil
}

// CreateAddress creates a new address for a user
func (r *Repository) CreateAddress(ctx context.Context, userID int64, req *AddressInput) (*Address, error) {
    isDefault := false
    if req.IsDefault != nil { isDefault = *req.IsDefault }

    var a Address
    err := r.db.QueryRow(ctx, `
        INSERT INTO addresses (user_id, city_id, label, line1, line2, is_default, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,(CURRENT_TIMESTAMP AT TIME ZONE 'UTC'),(CURRENT_TIMESTAMP AT TIME ZONE 'UTC'))
        RETURNING id, user_id, city_id, label, line1, line2, is_default, archived_at, created_at, updated_at`,
        userID, req.CityID, req.Label, req.Line1, req.Line2, isDefault,
    ).Scan(&a.ID, &a.UserID, &a.CityID, &a.Label, &a.Line1, &a.Line2, &a.IsDefault, &a.ArchivedAt, &a.CreatedAt, &a.UpdatedAt)
    if err != nil {
        if strings.Contains(err.Error(), "uq_default_address_per_user") {
            return nil, &errs.Error{Code: errs.InvalidArgument, Message: "لدى المستخدم عنوان افتراضي بالفعل."}
        }
        return nil, &errs.Error{Code: errs.Internal, Message: "تعذر إنشاء العنوان."}
    }
    return &a, nil
}

// UpdateAddress updates an existing address
func (r *Repository) UpdateAddress(ctx context.Context, addressID int64, req *UpdateAddressRequest) (*Address, error) {
    set := []string{}
    args := []interface{}{}
    idx := 1
    if req.CityID != nil { set = append(set, fmt.Sprintf("city_id = $%d", idx)); args = append(args, *req.CityID); idx++ }
    if req.Label != nil { set = append(set, fmt.Sprintf("label = $%d", idx)); args = append(args, *req.Label); idx++ }
    if req.Line1 != nil { set = append(set, fmt.Sprintf("line1 = $%d", idx)); args = append(args, *req.Line1); idx++ }
    if req.Line2 != nil { set = append(set, fmt.Sprintf("line2 = $%d", idx)); args = append(args, *req.Line2); idx++ }
    if req.IsDefault != nil { set = append(set, fmt.Sprintf("is_default = $%d", idx)); args = append(args, *req.IsDefault); idx++ }
    if req.ArchivedAt != nil { set = append(set, fmt.Sprintf("archived_at = $%d", idx)); args = append(args, *req.ArchivedAt); idx++ }
    set = append(set, "updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')")
    args = append(args, addressID)

    query := `UPDATE addresses SET ` + strings.Join(set, ", ") + ` WHERE id = $` + fmt.Sprintf("%d", idx) + `
              RETURNING id, user_id, city_id, label, line1, line2, is_default, archived_at, created_at, updated_at`
    var a Address
    if err := r.db.QueryRow(ctx, query, args...).Scan(&a.ID, &a.UserID, &a.CityID, &a.Label, &a.Line1, &a.Line2, &a.IsDefault, &a.ArchivedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
        return nil, &errs.Error{Code: errs.Internal, Message: "تعذر تحديث العنوان."}
    }
    return &a, nil
}

