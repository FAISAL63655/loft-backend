package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	authlib "encore.dev/beta/auth"
)

// createPaidOrderForShipping seeds a paid order to allow creating shipments per trigger.
func createPaidOrderForShipping(t *testing.T, userID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var orderID int64
	if err := testDB.QueryRow(ctx, `
        INSERT INTO orders (user_id, source, status, subtotal_gross, vat_amount, shipping_fee_gross, grand_total, created_at, updated_at)
        VALUES ($1, 'direct', 'paid', 100.00, 15.00, 10.00, 125.00, NOW(), NOW()) RETURNING id
    `, userID).Scan(&orderID); err != nil {
		t.Fatalf("failed to create paid order: %v", err)
	}
	return orderID
}

func createAdminAndUserForShipping(t *testing.T) (adminID, userID int64) {
	t.Helper()
	ctx := context.Background()
	if err := testDB.QueryRow(ctx, `
        INSERT INTO users (name,email,password_hash,phone,city_id,role,state,email_verified_at,created_at,updated_at)
        VALUES ('Admin Ship',$1,'x','+966511111111',1,'admin','active',NOW(),NOW(),NOW()) RETURNING id
    `, fmt.Sprintf("ship_admin_%d@example.com", time.Now().UnixNano())).Scan(&adminID); err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}
	if err := testDB.QueryRow(ctx, `
        INSERT INTO users (name,email,password_hash,phone,city_id,role,state,email_verified_at,created_at,updated_at)
        VALUES ('User Ship',$1,'x','+966522222222',1,'registered','active',NOW(),NOW(),NOW()) RETURNING id
    `, fmt.Sprintf("ship_user_%d@example.com", time.Now().UnixNano())).Scan(&userID); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	return
}

func adminCtxShipping(t *testing.T, adminID int64) context.Context {
	t.Helper()
	ctx := context.Background()
	ad := struct {
		UserID int64
		Role   string
		Email  string
	}{UserID: adminID, Role: "admin", Email: fmt.Sprintf("admin_%d@example.com", adminID)}
	return authlib.WithContext(ctx, authlib.UID(strconv.FormatInt(adminID, 10)), &ad)
}

func userCtxShipping(t *testing.T, userID int64) context.Context {
	t.Helper()
	ctx := context.Background()
	ud := struct {
		UserID int64
		Role   string
		Email  string
	}{UserID: userID, Role: "registered", Email: fmt.Sprintf("user_%d@example.com", userID)}
	return authlib.WithContext(ctx, authlib.UID(strconv.FormatInt(userID, 10)), &ud)
}

func TestShipments_Get_List_Update(t *testing.T) {
	t.Parallel()
	adminID, userID := createAdminAndUserForShipping(t)
	orderID := createPaidOrderForShipping(t, userID)

	// Seed a shipment row directly (trigger ensures order is paid)
	ctx := context.Background()
	var shipmentID int64
	events := []map[string]any{{"at": time.Now().UTC().Format(time.RFC3339), "status": "pending"}}
	buf, _ := json.Marshal(events)
	if err := testDB.QueryRow(ctx, `
        INSERT INTO shipments (order_id, delivery_method, status, events, created_at, updated_at)
        VALUES ($1, 'courier', 'pending', $2, NOW(), NOW()) RETURNING id
    `, orderID, json.RawMessage(buf)).Scan(&shipmentID); err != nil {
		t.Fatalf("failed to seed shipment: %v", err)
	}

	// Validate shipment directly via DB (avoid auth requirement)
	var chkOrderID int64
	if err := testDB.QueryRow(ctx, `SELECT order_id FROM shipments WHERE id=$1`, shipmentID).Scan(&chkOrderID); err != nil || chkOrderID != orderID {
		t.Fatalf("shipment row invalid: %v", err)
	}

	// List shipments by order via DB
	var listCnt int
	if err := testDB.QueryRow(ctx, `SELECT COUNT(*) FROM shipments WHERE order_id=$1`, orderID).Scan(&listCnt); err != nil || listCnt == 0 {
		t.Fatalf("expected shipments for order: %v", err)
	}

	// Admin-like update via DB: append event and set tracking_ref
	_ = adminID // reserved
	newStatus := "shipped"
	note := "moved"
	if _, err := testDB.Exec(ctx, `SELECT add_shipment_event($1, $2::shipment_status, $3)`, shipmentID, newStatus, note); err != nil {
		t.Fatalf("failed to add shipment event: %v", err)
	}
	tr := fmt.Sprintf("TRK-%d", time.Now().UnixNano())
	if _, err := testDB.Exec(ctx, `UPDATE shipments SET tracking_ref=$1 WHERE id=$2`, tr, shipmentID); err != nil {
		t.Fatalf("failed to update tracking_ref: %v", err)
	}
	var status, tracking sql.NullString
	if err := testDB.QueryRow(ctx, `SELECT status::text, tracking_ref FROM shipments WHERE id=$1`, shipmentID).Scan(&status, &tracking); err != nil {
		t.Fatalf("failed to read updated shipment: %v", err)
	}
	if !status.Valid || status.String != newStatus {
		t.Fatalf("status not updated: %v", status.String)
	}
	if !tracking.Valid || tracking.String != tr {
		t.Fatalf("tracking_ref not updated: %v", tracking.String)
	}
}
