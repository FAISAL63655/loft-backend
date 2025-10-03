package pages

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"encore.app/pkg/errs"
	"encore.dev/beta/auth"
	"encore.dev/storage/sqldb"
)

var db = sqldb.Named("coredb")

//encore:service
type Service struct{}

// ===== Models =====

type PageResponse struct {
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Format    string `json:"format"` // html | markdown
	UpdatedAt string `json:"updated_at"`
}

type UpdatePageRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Format  string `json:"format"` // html | markdown
}

// Named response type for list endpoints (Encore requires named structs)
type AdminListResponse struct {
	Items []PageResponse `json:"items"`
}

var allowedSlugs = map[string]bool{
	"terms":   true,
	"privacy": true,
	"cookies": true,
	"refund":  true,
}

func ensureAdmin(ctx context.Context) error {
	uidStr, ok := auth.UserID()
	if !ok {
		return errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	// Simple role check against users table
	var role string
	id64, err := strconv.ParseInt(string(uidStr), 10, 64)
	if err != nil {
		return errs.New(errs.Forbidden, "فشل التحقق من الصلاحيات")
	}
	if err := db.QueryRow(ctx, `SELECT role FROM users WHERE id = $1 AND state='active'`, id64).Scan(&role); err != nil {
		return errs.New(errs.Forbidden, "فشل التحقق من الصلاحيات")
	}
	if role != "admin" {
		return errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}
	return nil
}

// ===== Public Endpoints =====

//encore:api public method=GET path=/pages/:slug
func (s *Service) GetPage(ctx context.Context, slug string) (*PageResponse, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !allowedSlugs[slug] {
		return nil, errs.New(errs.NotFound, "الصفحة غير موجودة")
	}
	var resp PageResponse
	if err := db.QueryRow(ctx, `
		SELECT slug, title, content, format::text,
		       to_char(updated_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM site_pages WHERE slug=$1
	`, slug).Scan(&resp.Slug, &resp.Title, &resp.Content, &resp.Format, &resp.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, errs.New(errs.NotFound, "الصفحة غير موجودة")
		}
		return nil, errs.New(errs.Internal, "تعذر جلب الصفحة")
	}
	return &resp, nil
}

// ===== Admin Endpoints =====

//encore:api auth method=GET path=/admin/pages
func (s *Service) AdminList(ctx context.Context) (*AdminListResponse, error) {
	if err := ensureAdmin(ctx); err != nil {
		return nil, err
	}
	rows, err := db.Stdlib().QueryContext(ctx, `
		SELECT slug, title, content, format::text,
		       to_char(updated_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM site_pages
		WHERE slug IN ('terms','privacy','cookies','refund')
		ORDER BY slug
	`)
	if err != nil {
		return nil, errs.New(errs.Internal, "تعذر جلب الصفحات")
	}
	defer rows.Close()
	var items []PageResponse
	for rows.Next() {
		var it PageResponse
		if err := rows.Scan(&it.Slug, &it.Title, &it.Content, &it.Format, &it.UpdatedAt); err != nil {
			return nil, errs.New(errs.Internal, "تعذر قراءة البيانات")
		}
		items = append(items, it)
	}
	return &AdminListResponse{Items: items}, nil
}

//encore:api auth method=PUT path=/admin/pages/:slug
func (s *Service) AdminUpsert(ctx context.Context, slug string, req *UpdatePageRequest) (*PageResponse, error) {
	if err := ensureAdmin(ctx); err != nil {
		return nil, err
	}
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !allowedSlugs[slug] {
		return nil, errs.New(errs.InvalidArgument, "صفحة غير مسموح بها")
	}
	if req == nil || strings.TrimSpace(req.Title) == "" {
		return nil, errs.New(errs.InvalidArgument, "العنوان مطلوب")
	}
	content := req.Content
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format != "markdown" {
		format = "html"
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO site_pages (slug, title, content, format)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (slug) DO UPDATE SET title=EXCLUDED.title, content=EXCLUDED.content, format=EXCLUDED.format, updated_at=NOW()
	`, slug, strings.TrimSpace(req.Title), content, format); err != nil {
		return nil, errs.New(errs.Internal, "فشل حفظ الصفحة")
	}
	return s.GetPage(ctx, slug)
}
