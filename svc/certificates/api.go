package certificates

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"encore.app/pkg/authn"
	"encore.app/pkg/errs"
	"encore.app/pkg/storagegcs"
	"encore.dev/beta/auth"
	"encore.dev/storage/sqldb"
)

var db = sqldb.Named("coredb")

// GCS secrets; reuse same secret keys used by catalog service
var secrets struct {
	GCSProjectID       string //encore:secret
	GCSBucketName      string //encore:secret
	GCSCredentialsJSON string //encore:secret
}

// ======================== User Endpoints (Authenticated) ========================

//encore:api auth method=GET path=/certificates/my/print-requests
func (s *Service) MyList(ctx context.Context, q *ListQuery) (*ListResponse, error) {
    uidStr, ok := auth.UserID()
    if !ok {
        return nil, errs.E(ctx, "USR_UNAUTHENTICATED", "مطلوب تسجيل الدخول")
    }
    uid, err := strconv.ParseInt(string(uidStr), 10, 64)
    if err != nil {
        return nil, errs.E(ctx, "USR_AUTH_ID_INVALID", "معرّف المستخدم غير صالح")
    }
    page := 1
    limit := 20
    status := ""
    if q != nil {
        if q.Page > 0 {
            page = q.Page
        }
        if q.Limit > 0 && q.Limit <= 100 {
            limit = q.Limit
        }
        status = strings.TrimSpace(q.Status)
    }
    offset := (page - 1) * limit
    var rows *sql.Rows
    var dbErr error
    if status != "" {
        rows, dbErr = db.Stdlib().QueryContext(ctx, `
            SELECT id, club_name, status::text,
                   to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
            FROM certificate_requests
            WHERE user_id=$1 AND status=$2
            ORDER BY id DESC
            LIMIT $3 OFFSET $4
        `, uid, status, limit, offset)
    } else {
        rows, dbErr = db.Stdlib().QueryContext(ctx, `
            SELECT id, club_name, status::text,
                   to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
            FROM certificate_requests
            WHERE user_id=$1
            ORDER BY id DESC
            LIMIT $2 OFFSET $3
        `, uid, limit, offset)
    }
    if dbErr != nil {
        // If the migration adding user_id hasn't been applied yet, avoid crashing the UI.
        // Return empty list so the page can still render, and the admin can apply migrations.
        msg := strings.ToLower(dbErr.Error())
        if strings.Contains(msg, "column") && strings.Contains(msg, "user_id") {
            return &ListResponse{Items: []RequestItem{}}, nil
        }
        return nil, errs.E(ctx, "CERT_LIST_READ_FAILED", "تعذر جلب الطلبات")
    }
    defer rows.Close()
    var items []RequestItem
    for rows.Next() {
        var it RequestItem
        if err := rows.Scan(&it.ID, &it.ClubName, &it.Status, &it.CreatedAt); err != nil {
            return nil, errs.E(ctx, "CERT_LIST_SCAN_FAILED", "تعذر قراءة الطلبات")
        }
        items = append(items, it)
    }
    return &ListResponse{Items: items}, nil
}

//encore:api auth method=GET path=/certificates/my/print-requests/:id
func (s *Service) MyGet(ctx context.Context, id string) (*RequestDetail, error) {
    uidStr, ok := auth.UserID()
    if !ok {
        return nil, errs.E(ctx, "USR_UNAUTHENTICATED", "مطلوب تسجيل الدخول")
    }
    uid, err := strconv.ParseInt(string(uidStr), 10, 64)
    if err != nil {
        return nil, errs.E(ctx, "USR_AUTH_ID_INVALID", "معرّف المستخدم غير صالح")
    }
    reqID, err := strconv.ParseInt(id, 10, 64)
    if err != nil {
        return nil, errs.New(errs.InvalidArgument, "معرّف غير صالح")
    }
    var detail RequestDetail
    var excel sql.NullString
    var ownerID sql.NullInt64
    err = db.QueryRow(ctx, `
        SELECT id, club_name, status::text, excel_gcs_path,
               to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
               user_id
        FROM certificate_requests WHERE id=$1
    `, reqID).Scan(&detail.ID, &detail.ClubName, &detail.Status, &excel, &detail.CreatedAt, &ownerID)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, errs.New(errs.NotFound, "غير موجود")
        }
        return nil, errs.E(ctx, "CERT_READ_FAILED", "تعذر جلب الطلب")
    }
    if !ownerID.Valid || ownerID.Int64 != uid {
        return nil, errs.New(errs.Forbidden, "الوصول غير مسموح")
    }
    if excel.Valid {
        s := excel.String
        detail.ExcelPath = &s
    }
    rows, err := db.Stdlib().QueryContext(ctx, `
        SELECT id, race_name, to_char(race_date,'YYYY-MM-DD'), quantity
        FROM certificate_request_races WHERE request_id=$1 ORDER BY id ASC
    `, reqID)
    if err != nil {
        return nil, errs.E(ctx, "CERT_RACES_READ_FAILED", "تعذر جلب التفاصيل")
    }
    defer rows.Close()
    var races []RaceItem
    for rows.Next() {
        var r RaceItem
        if err := rows.Scan(&r.ID, &r.Name, &r.Date, &r.Quantity); err != nil {
            return nil, errs.E(ctx, "CERT_RACES_SCAN_FAILED", "تعذر قراءة التفاصيل")
        }
        races = append(races, r)
    }
    detail.Races = races
    return &detail, nil
}

//encore:api public method=POST path=/certificates/print-requests/:id/status-check
func (s *Service) StatusCheck(ctx context.Context, id string, req *StatusCheckRequest) (*StatusCheckResponse, error) {
    reqID, err := strconv.ParseInt(id, 10, 64)
    if err != nil {
        return nil, errs.New(errs.InvalidArgument, "معرّف غير صالح")
    }
    var ownerID sql.NullInt64
    var status, created string
    if err := db.QueryRow(ctx, `
        SELECT user_id,
               status::text,
               to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
        FROM certificate_requests WHERE id=$1
    `, reqID).Scan(&ownerID, &status, &created); err != nil {
        if err == sql.ErrNoRows {
            return nil, errs.New(errs.NotFound, "غير موجود")
        }
        return nil, errs.E(ctx, "CERT_STATUS_READ_FAILED", "تعذر جلب الحالة")
    }
    // Allow owner to view without PIN
    if uidStr, ok := auth.UserID(); ok {
        if uid, e := strconv.ParseInt(string(uidStr), 10, 64); e == nil && ownerID.Valid && ownerID.Int64 == uid {
            return &StatusCheckResponse{ID: reqID, Status: status, CreatedAt: created}, nil
        }
    }
    // Otherwise require a valid PIN
    if req == nil || strings.TrimSpace(req.Pin) == "" {
        return nil, errs.New(errs.Forbidden, "الوصول غير مسموح")
    }
    hash, err := readPINHash(ctx)
    if err != nil {
        return nil, err
    }
    if hash == "" || authn.VerifyPassword(req.Pin, hash) != nil {
        return nil, errs.New(errs.Forbidden, "الوصول غير مسموح")
    }
    return &StatusCheckResponse{ID: reqID, Status: status, CreatedAt: created}, nil
}

//encore:service
type Service struct{}
var storage *storagegcs.Client

func initService() (*Service, error) {
    // Initialize storage client (private bucket) once
    if storage == nil {
        c, err := storagegcs.NewClient(context.Background(), storagegcs.Config{
            ProjectID:      secrets.GCSProjectID,
            BucketName:     secrets.GCSBucketName,
            CredentialsKey: secrets.GCSCredentialsJSON,
            IsPublic:       false,
        })
        if err != nil {
            return nil, err
        }
        storage = c
    }
    return &Service{}, nil
}

// ======================== Secure Access (PIN) ========================

type SetPINRequest struct {
	NewPIN string `json:"new_pin"`
}

type MessageResponse struct {
    Message string `json:"message"`
}

// VerifyPINResponse is a named response type to satisfy Encore schema requirements
type VerifyPINResponse struct {
    OK bool `json:"ok"`
}

// ======================== Public Status Check ========================

type StatusCheckRequest struct {
    Pin string `json:"pin"`
}

type StatusCheckResponse struct {
    ID        int64  `json:"id"`
    Status    string `json:"status"`
    CreatedAt string `json:"created_at"`
}

//encore:api auth method=POST path=/certificates/settings/pin
func (s *Service) SetPIN(ctx context.Context, req *SetPINRequest) (*MessageResponse, error) {
	if err := ensureAdmin(ctx); err != nil {
		return nil, err
	}
	if req == nil || strings.TrimSpace(req.NewPIN) == "" {
		// Allow clearing the PIN by setting empty hash
		if _, err := db.Exec(ctx, `UPDATE system_settings SET value='' , updated_at=NOW() WHERE key='cert.print_access_pin_hash'`); err != nil {
			return nil, errs.E(ctx, "CERT_PIN_SAVE_FAILED", "فشل تحديث الرمز السري")
		}
		return &MessageResponse{Message: "تم تحديث الرمز السري"}, nil
	}
	hash, err := authn.HashPassword(req.NewPIN)
	if err != nil {
		return nil, errs.E(ctx, "CERT_PIN_HASH_FAILED", "تعذر توليد بصمة الرمز")
	}
	_, err = db.Exec(ctx, `
		INSERT INTO system_settings (key, value, description)
		VALUES ('cert.print_access_pin_hash', $1, 'Argon2id hash for certificate print requests access PIN')
		ON CONFLICT (key) DO UPDATE SET value=$1, updated_at=NOW()
	`, hash)
	if err != nil {
		return nil, errs.E(ctx, "CERT_PIN_SAVE_FAILED", "فشل حفظ الرمز السري")
	}
	return &MessageResponse{Message: "تم تحديث الرمز السري"}, nil
}

//encore:api public method=POST path=/certificates/pin/verify
func (s *Service) VerifyPIN(ctx context.Context, req *SetPINRequest) (*VerifyPINResponse, error) {
    if req == nil || strings.TrimSpace(req.NewPIN) == "" {
        return &VerifyPINResponse{OK: false}, nil
    }
    hash, err := readPINHash(ctx)
    if err != nil {
        return nil, err
    }
    if hash == "" {
        return &VerifyPINResponse{OK: false}, nil
    }
    if authn.VerifyPassword(req.NewPIN, hash) == nil {
        return &VerifyPINResponse{OK: true}, nil
    }
    return &VerifyPINResponse{OK: false}, nil
}

func readPINHash(ctx context.Context) (string, error) {
	var hash sql.NullString
	if err := db.QueryRow(ctx, `SELECT value FROM system_settings WHERE key='cert.print_access_pin_hash'`).Scan(&hash); err != nil && err != sql.ErrNoRows {
		return "", errs.E(ctx, "CERT_PIN_READ_FAILED", "تعذر قراءة إعداد الوصول")
	}
	if hash.Valid {
		return hash.String, nil
	}
	return "", nil
}

// ======================== Submit Request (Public + PIN) ========================

type RaceInput struct {
	RaceName string `json:"race_name"`
	RaceDate string `json:"race_date"` // YYYY-MM-DD
	Quantity int    `json:"quantity"`
}

type SubmitRequest struct {
	Pin      string      `json:"pin"`
	ClubName string      `json:"club_name"`
	Races    []RaceInput `json:"races"`
}

type SubmitResponse struct {
	RequestID int64  `json:"request_id"`
	Status    string `json:"status"`
}

//encore:api public raw method=POST path=/certificates/print-requests
func SubmitPrintRequest(w http.ResponseWriter, req *http.Request) {
    ctx := req.Context()
    if _, err := initService(); err != nil {
        writeError(w, errs.E(ctx, "CERT_INIT_FAILED", "فشل التهيئة"))
        return
    }
    // Require login and use the authenticated user_id for ownership
    uidStr, ok := auth.UserID()
    if !ok {
        writeError(w, errs.E(ctx, "USR_UNAUTHENTICATED", "مطلوب تسجيل الدخول"))
        return
    }
    uid, err := strconv.ParseInt(string(uidStr), 10, 64)
    if err != nil {
        writeError(w, errs.E(ctx, "USR_AUTH_ID_INVALID", "معرّف المستخدم غير صالح"))
        return
    }
	// Parse multipart up to 25MB
	if err := req.ParseMultipartForm(25 << 20); err != nil {
		writeError(w, errs.E(ctx, "CERT_PARSE_FAILED", "تعذر قراءة الطلب"))
		return
	}
	pin := strings.TrimSpace(req.FormValue("pin"))
	club := strings.TrimSpace(req.FormValue("club_name"))
	racesJSON := strings.TrimSpace(req.FormValue("races"))
	if pin == "" || club == "" || racesJSON == "" {
		writeError(w, errs.New(errs.InvalidArgument, "الرجاء تعبئة جميع الحقول"))
		return
	}
	pinHash, err := readPINHash(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	if pinHash == "" || authn.VerifyPassword(pin, pinHash) != nil {
		writeError(w, errs.New(errs.Forbidden, "الوصول غير مسموح"))
		return
	}
	var races []RaceInput
	if err := json.Unmarshal([]byte(racesJSON), &races); err != nil || len(races) == 0 {
		writeError(w, errs.New(errs.InvalidArgument, "تنسيق السباقات غير صالح"))
		return
	}
	// Optional Excel upload
	var excelGCS string
	file, header, err := req.FormFile("excel")
	if err == nil && header != nil && file != nil {
		defer file.Close()
		res, upErr := storage.Upload(ctx, file, header.Filename, storagegcs.UploadConfig{})
		if upErr != nil {
			writeError(w, errs.E(ctx, "CERT_UPLOAD_FAILED", "فشل رفع الملف"))
			return
		}
		excelGCS = res.GCSPath
	}
	// Insert in transaction
	tx, err := db.Stdlib().BeginTx(ctx, nil)
	if err != nil {
		writeError(w, errs.E(ctx, "CERT_TX_BEGIN_FAILED", "تعذر بدء العملية"))
		return
	}
	var reqID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO certificate_requests (user_id, club_name, excel_gcs_path)
		VALUES ($1, $2, $3)
		RETURNING id
	`, uid, club, sql.NullString{String: excelGCS, Valid: excelGCS != ""}).Scan(&reqID)
	if err != nil {
		_ = tx.Rollback()
		writeError(w, errs.E(ctx, "CERT_SAVE_FAILED", "فشل حفظ الطلب"))
		return
	}
	for _, r := range races {
		name := strings.TrimSpace(r.RaceName)
		if name == "" || r.Quantity <= 0 || strings.TrimSpace(r.RaceDate) == "" {
			_ = tx.Rollback()
			writeError(w, errs.New(errs.InvalidArgument, "بيانات السباق غير صالحة"))
			return
		}
		_, parseErr := time.Parse("2006-01-02", r.RaceDate)
		if parseErr != nil {
			_ = tx.Rollback()
			writeError(w, errs.New(errs.InvalidArgument, "تاريخ السباق غير صالح"))
			return
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO certificate_request_races (request_id, race_name, race_date, quantity)
			VALUES ($1, $2, $3, $4)
		`, reqID, name, r.RaceDate, r.Quantity); err != nil {
			_ = tx.Rollback()
			writeError(w, errs.E(ctx, "CERT_RACE_SAVE_FAILED", "فشل حفظ تفاصيل السباق"))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, errs.E(ctx, "CERT_TX_COMMIT_FAILED", "تعذر إتمام العملية"))
		return
	}
	writeJSON(w, http.StatusCreated, SubmitResponse{RequestID: reqID, Status: "pending"})
}

// ======================== Admin Review Endpoints ========================

type RequestItem struct {
	ID        int64  `json:"id"`
	ClubName  string `json:"club_name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type ListQuery struct {
	Status string `json:"status"`
	Page   int    `json:"page"`
	Limit  int    `json:"limit"`
}

type ListResponse struct {
	Items []RequestItem `json:"items"`
}

//encore:api auth method=GET path=/certificates/print-requests
func (s *Service) AdminList(ctx context.Context, req *ListQuery) (*ListResponse, error) {
	if err := ensureAdmin(ctx); err != nil {
		return nil, err
	}
	limit := 20
	offset := 0
	if req != nil {
		if req.Limit > 0 && req.Limit <= 100 {
			limit = req.Limit
		}
		if req.Page > 1 {
			offset = (req.Page - 1) * limit
		}
	}
	var rows *sql.Rows
	var err error
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "pending" || status == "approved" || status == "rejected" {
		rows, err = db.Stdlib().QueryContext(ctx, `
			SELECT id, club_name, status::text,
			       to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
			FROM certificate_requests
			WHERE status = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`, status, limit, offset)
	} else {
		rows, err = db.Stdlib().QueryContext(ctx, `
			SELECT id, club_name, status::text,
			       to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
			FROM certificate_requests
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
	}
	if err != nil {
		return nil, errs.E(ctx, "CERT_LIST_FAILED", "فشل جلب الطلبات")
	}
	defer rows.Close()
	items := make([]RequestItem, 0)
	for rows.Next() {
		var it RequestItem
		if err := rows.Scan(&it.ID, &it.ClubName, &it.Status, &it.CreatedAt); err != nil {
			return nil, errs.E(ctx, "CERT_LIST_READ_FAILED", "تعذر قراءة النتائج")
		}
		items = append(items, it)
	}
	return &ListResponse{Items: items}, nil
}

// Detailed view

type RaceItem struct {
	ID       int64  `json:"id"`
	Name     string `json:"race_name"`
	Date     string `json:"race_date"`
	Quantity int    `json:"quantity"`
}

type RequestDetail struct {
	ID        int64      `json:"id"`
	ClubName  string     `json:"club_name"`
	Status    string     `json:"status"`
	ExcelPath *string    `json:"excel_gcs_path,omitempty"`
	CreatedAt string     `json:"created_at"`
	Races     []RaceItem `json:"races"`
}

//encore:api auth method=GET path=/certificates/print-requests/:id
func (s *Service) AdminGet(ctx context.Context, id string) (*RequestDetail, error) {
	if err := ensureAdmin(ctx); err != nil {
		return nil, err
	}
	reqID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errs.New(errs.InvalidArgument, "معرّف غير صالح")
	}
	var detail RequestDetail
	var excel sql.NullString
	err = db.QueryRow(ctx, `
		SELECT id, club_name, status::text, excel_gcs_path,
		       to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM certificate_requests WHERE id=$1
	`, reqID).Scan(&detail.ID, &detail.ClubName, &detail.Status, &excel, &detail.CreatedAt)
	if err != nil {
		return nil, errs.E(ctx, "CERT_NOT_FOUND", "الطلب غير موجود")
	}
	if excel.Valid {
		p := excel.String
		detail.ExcelPath = &p
	}
	rows, err := db.Stdlib().QueryContext(ctx, `
		SELECT id, race_name, to_char(race_date,'YYYY-MM-DD'), quantity
		FROM certificate_request_races WHERE request_id=$1 ORDER BY id ASC
	`, reqID)
	if err != nil {
		return nil, errs.E(ctx, "CERT_RACES_READ_FAILED", "تعذر جلب التفاصيل")
	}
	defer rows.Close()
	var races []RaceItem
	for rows.Next() {
		var r RaceItem
		if err := rows.Scan(&r.ID, &r.Name, &r.Date, &r.Quantity); err != nil {
			return nil, errs.E(ctx, "CERT_RACES_SCAN_FAILED", "تعذر قراءة التفاصيل")
		}
		races = append(races, r)
	}
	detail.Races = races
	return &detail, nil
}

// Update status

type UpdateStatusRequest struct {
	Status string `json:"status"` // approved | rejected | pending
}

type UpdateStatusResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

//encore:api auth method=PATCH path=/certificates/print-requests/:id/status
func (s *Service) AdminSetStatus(ctx context.Context, id string, req *UpdateStatusRequest) (*UpdateStatusResponse, error) {
	if err := ensureAdmin(ctx); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errs.New(errs.InvalidArgument, "الطلب فارغ")
	}
	newStatus := strings.ToLower(strings.TrimSpace(req.Status))
	if newStatus != "approved" && newStatus != "rejected" && newStatus != "pending" {
		return nil, errs.New(errs.InvalidArgument, "قيمة الحالة غير صالحة")
	}
	reqID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errs.New(errs.InvalidArgument, "معرّف غير صالح")
	}
	_, err = db.Exec(ctx, `UPDATE certificate_requests SET status=$1, updated_at=NOW() WHERE id=$2`, newStatus, reqID)
	if err != nil {
		return nil, errs.E(ctx, "CERT_STATUS_UPDATE_FAILED", "فشل تحديث الحالة")
	}
	return &UpdateStatusResponse{ID: reqID, Status: newStatus}, nil
}

// ======================== Helpers ========================

func ensureAdmin(ctx context.Context) error {
	uidStr, ok := auth.UserID()
	if !ok {
		return errs.E(ctx, "USR_UNAUTHENTICATED", "مطلوب تسجيل الدخول")
	}
	uid, err := strconv.ParseInt(string(uidStr), 10, 64)
	if err != nil {
		return errs.E(ctx, "USR_AUTH_ID_INVALID", "معرّف المستخدم غير صالح")
	}
	var role string
	if err := db.QueryRow(ctx, `SELECT role FROM users WHERE id=$1 AND state='active'`, uid).Scan(&role); err != nil {
		return errs.E(ctx, "USR_PERM_CHECK_FAILED", "فشل التحقق من الصلاحيات")
	}
	if role != "admin" {
		return errs.E(ctx, "USR_FORBIDDEN_ADMIN", "يتطلب صلاحيات مدير")
	}
	return nil
}

func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	var e *errs.Error
	if pe, ok := err.(*errs.Error); ok {
		e = pe
	} else {
		e = errs.New(errs.Internal, err.Error())
	}
	w.WriteHeader(e.HTTPStatus())
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    e.Code,
		"message": e.Message,
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
