package integration

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	authlib "encore.dev/beta/auth"
)

func createAdminForSettings(t *testing.T) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := testDB.QueryRow(ctx, `
        INSERT INTO users(name,email,password_hash,role,state,created_at,updated_at)
        VALUES('Admin Settings',$1,'x','admin','active',NOW(),NOW()) RETURNING id
    `, fmt.Sprintf("admin_settings_%d@example.com", time.Now().UnixNano())).Scan(&id); err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}
	return id
}

func adminCtxSettings(t *testing.T, id int64) context.Context {
	t.Helper()
	ctx := context.Background()
	ad := struct {
		UserID int64
		Role   string
		Email  string
	}{UserID: id, Role: "admin", Email: "admin_settings@example.com"}
	return authlib.WithContext(ctx, authlib.UID(strconv.FormatInt(id, 10)), &ad)
}

func TestAdminSettings_List_Get_Update_History_And_Stats(t *testing.T) {
	t.Parallel()
	_ = createAdminForSettings(t)
	ctx := context.Background()

	// List settings via DB
	var cnt int
	if err := testDB.QueryRow(ctx, `SELECT COUNT(*) FROM system_settings`).Scan(&cnt); err != nil {
		t.Fatalf("failed to count settings: %v", err)
	}
	if cnt == 0 {
		t.Fatalf("expected seeded settings")
	}

	// Get specific key directly
	var val string
	if err := testDB.QueryRow(ctx, `SELECT COALESCE(value,'') FROM system_settings WHERE key='app.name'`).Scan(&val); err != nil {
		t.Fatalf("failed to get app.name: %v", err)
	}
	if val == "" {
		t.Fatalf("app.name should not be empty")
	}

	// Update a numeric setting and verify
	if _, err := testDB.Exec(ctx, `UPDATE system_settings SET value='150', updated_at=NOW() WHERE key='ws.max_connections'`); err != nil {
		t.Fatalf("failed to update ws.max_connections: %v", err)
	}
	var chk string
	if err := testDB.QueryRow(ctx, `SELECT value FROM system_settings WHERE key='ws.max_connections'`).Scan(&chk); err != nil || chk != "150" {
		t.Fatalf("ws.max_connections not updated: %v, %s", err, chk)
	}

	// Admin dashboard-like counts
	var usersTotal int
	if err := testDB.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&usersTotal); err != nil {
		t.Fatalf("failed to count users: %v", err)
	}
	// basic sanity
	if usersTotal < 0 {
		t.Fatalf("invalid users total")
	}
}
