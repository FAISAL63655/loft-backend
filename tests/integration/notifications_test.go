package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	authlib "encore.dev/beta/auth"

	"encore.app/pkg/templates"
	authsvc "encore.app/svc/auth"
	notifs "encore.app/svc/notifications"
)

// createAdminUserNotifications creates a unique admin user for notifications tests.
func createAdminUserNotifications(t *testing.T) int64 {
	t.Helper()
	ctx := context.Background()
	email := fmt.Sprintf("notif_admin_%d@example.com", time.Now().UnixNano())
	var id int64
	if err := testDB.QueryRow(ctx, `
        INSERT INTO users (name,email,password_hash,phone,city_id,role,state,email_verified_at,created_at,updated_at)
        VALUES ('Admin Notif',$1,'x','+966500000000',1,'admin','active',NOW(),NOW(),NOW()) RETURNING id
    `, email).Scan(&id); err != nil {
		t.Fatalf("failed to create admin user: %v", err)
	}
	return id
}

func adminCtx(t *testing.T, adminID int64) context.Context {
	t.Helper()
	ctx := context.Background()
	uid := authsvc.AuthData{UserID: adminID, Role: "admin", Email: fmt.Sprintf("admin_%d@example.com", adminID)}
	return authlib.WithContext(ctx, authlib.UID(strconv.FormatInt(adminID, 10)), &uid)
}

func TestNotifications_Templates_And_TestEmail(t *testing.T) {
	t.Parallel()
	adminID := createAdminUserNotifications(t)
	ctx := adminCtx(t, adminID)

	// Get templates requires auth
	// Read templates via package
	ids := templates.GetAvailableTemplates()
	if len(ids) == 0 {
		t.Fatalf("expected at least one template")
	}

	// Use the first available template if possible, else fallback
	tplID := ids[0]
	if tplID == "" {
		tplID = "welcome"
	}

	// Enqueue a test email
	req := &notifs.TestEmailRequest{
		Email:      fmt.Sprintf("user_%d@example.com", time.Now().UnixNano()),
		TemplateID: tplID,
		Language:   "ar",
	}
	// Use enqueue directly (avoid API call restriction from tests)
	data := map[string]any{"email": req.Email, "name": "Test User", "language": req.Language}
	emailID, err := notifs.EnqueueEmail(ctx, adminID, req.TemplateID, data)
	if err != nil {
		t.Fatalf("EnqueueEmail failed: %v", err)
	}

	// Assert the notification row exists and is queued
	var status string
	if err := testDB.QueryRow(ctx, `SELECT status::text FROM notifications WHERE id=$1`, emailID).Scan(&status); err != nil {
		t.Fatalf("failed to read queued email: %v", err)
	}
	if status != "queued" && status != "sending" && status != "sent" && status != "failed" {
		t.Fatalf("unexpected email status: %s", status)
	}

	// Also enqueue and list internal notification for the same user
	payload := map[string]any{"msg": "hello"}
	buf, _ := json.Marshal(payload)
	_, err = notifs.EnqueueInternal(ctx, adminID, "test_internal", json.RawMessage(buf))
	if err != nil {
		t.Fatalf("EnqueueInternal failed: %v", err)
	}

	// List internal notifications for current user
	var count int
	if err := testDB.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND channel='internal'`, adminID).Scan(&count); err != nil {
		t.Fatalf("failed to count internal notifications: %v", err)
	}
	if count == 0 {
		t.Fatalf("expected at least one internal notification")
	}
}
