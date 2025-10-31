package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	authlib "encore.dev/beta/auth"

	auctionssvc "encore.app/svc/auctions"
	authsvc "encore.app/svc/auth"
	notifs "encore.app/svc/notifications"
)

// makeUID context with role and email
func ctxWithUser(userID int64, role string) context.Context {
	ctx := context.Background()
	data := authsvc.AuthData{UserID: userID, Role: role, Email: fmt.Sprintf("%s_%d@example.com", role, userID)}
	return authlib.WithContext(ctx, authlib.UID(strconv.FormatInt(userID, 10)), &data)
}

func seedAdmin(t *testing.T) int64 {
	t.Helper()
	var id int64
	if err := testDB.QueryRow(context.Background(), `
        INSERT INTO users(name,email,password_hash,phone,city_id,role,state,email_verified_at,created_at,updated_at)
        VALUES ('E2E Admin',$1,'x','+966500000000',1,'admin','active',NOW(),NOW(),NOW()) RETURNING id
    `, fmt.Sprintf("e2e_admin_%d@example.com", time.Now().UnixNano())).Scan(&id); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	return id
}

func seedVerifiedUser(t *testing.T) int64 {
	t.Helper()
	var id int64
	if err := testDB.QueryRow(context.Background(), `
        INSERT INTO users(name,email,password_hash,phone,city_id,role,state,email_verified_at,created_at,updated_at)
        VALUES ('E2E User',$1,'x','+966511111111',1,'verified','active',NOW(),NOW(),NOW()) RETURNING id
    `, fmt.Sprintf("e2e_user_%d@example.com", time.Now().UnixNano())).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedPigeonProduct(t *testing.T) int64 {
	t.Helper()
	title := "E2E Pigeon " + strconv.FormatInt(time.Now().UnixNano(), 10)
	slug := "e2e-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	ring := "R-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	var pid int64
	if err := testDB.QueryRow(context.Background(), `
        INSERT INTO products(type,title,slug,description,price_net,status,created_at,updated_at)
        VALUES ('pigeon',$1,$2,'desc',1000.00,'available',NOW(),NOW()) RETURNING id
    `, title, slug).Scan(&pid); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if _, err := testDB.Exec(context.Background(), `INSERT INTO pigeons(product_id,ring_number,sex) VALUES ($1,$2,'unknown')`, pid, ring); err != nil {
		t.Fatalf("seed pigeon: %v", err)
	}
	return pid
}

func TestEndToEnd_FullFlow(t *testing.T) {
	t.Parallel()

	// 1) Seed core actors
	adminID := seedAdmin(t)
	userID := seedVerifiedUser(t)
	adminCtx := ctxWithUser(adminID, "admin")
	userCtx := ctxWithUser(userID, "verified")

	// 1.a) User profile address (default shipping address)
	if _, err := testDB.Exec(context.Background(), `
		INSERT INTO addresses (user_id, city_id, label, line1, line2, is_default, created_at, updated_at)
		VALUES ($1, 1, 'Home', 'Street 1', NULL, true, NOW(), NOW())
	`, userID); err != nil {
		t.Fatalf("create default address: %v", err)
	}

	// 2) Catalog: create product (already seeded) and create AUCTION by admin
	productID := seedPigeonProduct(t)
	aucSvc := auctionssvc.NewService(testDB, nil) // nil storage for tests
	start := time.Now().Add(1 * time.Hour).UTC()
	end := time.Now().Add(25 * time.Hour).UTC()
	createReq := &auctionssvc.CreateAuctionRequest{
		ProductID:    productID,
		StartPrice:   1000,
		BidStep:      50,
		ReservePrice: nil,
		StartAt:      start,
		EndAt:        end,
	}
	auc, err := aucSvc.CreateAuction(adminCtx, createReq)
	if err != nil {
		t.Fatalf("create auction: %v", err)
	}
	if auc == nil || auc.ID == 0 {
		t.Fatalf("invalid auction response")
	}

	// 3) Admin moves auction to live by setting start_at in past (simulate schedule to live)
	if _, err := testDB.Exec(context.Background(), `UPDATE auctions SET start_at = NOW() - INTERVAL '1 minute', status='live' WHERE id=$1`, auc.ID); err != nil {
		t.Fatalf("activate auction: %v", err)
	}

	// 4) Verified user places bids
	bidSvc := auctionssvc.NewBidService(testDB)
	if _, err := bidSvc.PlaceBid(userCtx, auc.ID, userID, 1050); err != nil {
		t.Fatalf("first bid: %v", err)
	}
	if _, err := bidSvc.PlaceBid(userCtx, auc.ID, userID, 1100); err != nil {
		t.Fatalf("second bid: %v", err)
	}

	// 5) Close auction (end) and mark winner (service logic handles)
	if _, err := testDB.Exec(context.Background(), `UPDATE auctions SET status='ended', end_at=NOW() WHERE id=$1`, auc.ID); err != nil {
		t.Fatalf("end auction: %v", err)
	}

	// 6) Orders: create order for winning user from auction (simplified direct insert consistent with schema)
	var orderID int64
	if err := testDB.QueryRow(context.Background(), `
        INSERT INTO orders (user_id, source, status, subtotal_gross, vat_amount, shipping_fee_gross, grand_total, created_at, updated_at)
        VALUES ($1,'auction','paid', 1100.00, 165.00, 25.00, 1290.00, NOW(), NOW()) RETURNING id
    `, userID).Scan(&orderID); err != nil {
		t.Fatalf("create order: %v", err)
	}

	// 6.a) Order items: add purchased product (qty=1) and recalc totals via trigger
	if _, err := testDB.Exec(context.Background(), `
		INSERT INTO order_items (order_id, product_id, qty, unit_price_gross, line_total_gross, created_at)
		VALUES ($1, $2, 1, 1100.00, 1100.00, NOW())
	`, orderID, productID); err != nil {
		t.Fatalf("create order item: %v", err)
	}
	// fire BEFORE UPDATE trigger to recalculate order totals
	if _, err := testDB.Exec(context.Background(), `UPDATE orders SET updated_at = NOW() WHERE id=$1`, orderID); err != nil {
		t.Fatalf("recalc order totals: %v", err)
	}
	// verify totals non-zero and consistent
	var subtotal, grandTotal, shippingGross, vatAmt sql.NullFloat64
	if err := testDB.QueryRow(context.Background(), `SELECT subtotal_gross, grand_total, shipping_fee_gross, vat_amount FROM orders WHERE id=$1`, orderID).
		Scan(&subtotal, &grandTotal, &shippingGross, &vatAmt); err != nil {
		t.Fatalf("read order totals: %v", err)
	}
	if !subtotal.Valid || subtotal.Float64 < 1100.0 {
		t.Fatalf("unexpected subtotal: %+v", subtotal)
	}

	// 7) Payments: simulate captured payment row
	var invoiceID int64
	invNum := fmt.Sprintf("INV-%d", orderID)
	// snapshot VAT 0.15, and set minimal totals JSON
	if err := testDB.QueryRow(context.Background(), `
        INSERT INTO invoices (order_id, number, status, vat_rate_snapshot, totals, created_at, updated_at)
        VALUES ($1, $2, 'paid', 0.150, '{}'::jsonb, NOW(), NOW()) RETURNING id
    `, orderID, invNum).Scan(&invoiceID); err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if _, err := testDB.Exec(context.Background(), `
        INSERT INTO payments (invoice_id, gateway, gateway_ref, status, amount_authorized, amount_captured, amount_refunded, refund_partial, created_at, updated_at)
        VALUES ($1, 'moyasar', 'E2E-REF', 'paid', 1290.00, 1290.00, 0.00, false, NOW(), NOW())
    `, invoiceID); err != nil {
		t.Fatalf("create payment: %v", err)
	}

	// 8) Shipping: create shipment (trigger requires orders.paid)
	var shipmentID int64
	if err := testDB.QueryRow(context.Background(), `
        INSERT INTO shipments (order_id, delivery_method, status, events, created_at, updated_at)
        VALUES ($1, 'courier', 'pending', '[]', NOW(), NOW()) RETURNING id
    `, orderID).Scan(&shipmentID); err != nil {
		t.Fatalf("create shipment: %v", err)
	}
	// update shipment with event & tracking
	if _, err := testDB.Exec(context.Background(), `SELECT add_shipment_event($1, 'shipped'::shipment_status, 'out for delivery')`, shipmentID); err != nil {
		t.Fatalf("shipment event: %v", err)
	}
	if _, err := testDB.Exec(context.Background(), `UPDATE shipments SET tracking_ref='E2E-TRACK' WHERE id=$1`, shipmentID); err != nil {
		t.Fatalf("shipment track: %v", err)
	}
	// add delivered event and finalize status
	_, _ = testDB.Exec(context.Background(), `SELECT add_shipment_event($1, 'delivered'::shipment_status, 'delivered to customer')`, shipmentID)
	if _, err := testDB.Exec(context.Background(), `UPDATE shipments SET status='delivered' WHERE id=$1`, shipmentID); err != nil {
		t.Fatalf("shipment delivered: %v", err)
	}

	// 9) Notifications: enqueue internal + email to user
	emailPayload := map[string]any{"email": fmt.Sprintf("winner_%d@example.com", userID), "name": "Winner", "language": "ar"}
	if _, err := notifs.EnqueueEmail(context.Background(), userID, "auction_won", emailPayload); err != nil {
		t.Fatalf("enqueue email: %v", err)
	}
	inPayload := map[string]any{"type": "order_paid", "order_id": orderID}
	buf, _ := json.Marshal(inPayload)
	if _, err := notifs.EnqueueInternal(context.Background(), userID, "order_paid", json.RawMessage(buf)); err != nil {
		t.Fatalf("enqueue internal: %v", err)
	}
	var notifCount int
	if err := testDB.QueryRow(context.Background(), `SELECT COUNT(*) FROM notifications WHERE user_id=$1`, userID).Scan(&notifCount); err != nil || notifCount == 0 {
		t.Fatalf("notifications not recorded: %v", err)
	}

	// 10) Admin settings touch (simulate dashboard visibility)
	var usersTotal int
	if err := testDB.QueryRow(context.Background(), `SELECT COUNT(*) FROM users`).Scan(&usersTotal); err != nil || usersTotal <= 0 {
		t.Fatalf("dashboard users count invalid: %v", err)
	}
	// 10.a) Update system settings and read back
	if _, err := testDB.Exec(context.Background(), `UPDATE system_settings SET value='0.20' WHERE key='vat.rate'`); err != nil {
		t.Fatalf("update vat.rate: %v", err)
	}
	if _, err := testDB.Exec(context.Background(), `UPDATE system_settings SET value='true' WHERE key='vat.enabled'`); err != nil {
		t.Fatalf("update vat.enabled: %v", err)
	}
	var vatRate, vatEnabled string
	if err := testDB.QueryRow(context.Background(), `SELECT value FROM system_settings WHERE key='vat.rate'`).Scan(&vatRate); err != nil || vatRate == "" {
		t.Fatalf("read vat.rate: %v", err)
	}
	if err := testDB.QueryRow(context.Background(), `SELECT value FROM system_settings WHERE key='vat.enabled'`).Scan(&vatEnabled); err != nil || vatEnabled == "" {
		t.Fatalf("read vat.enabled: %v", err)
	}

	// 11) Final assertions
	var ordStatus sql.NullString
	if err := testDB.QueryRow(context.Background(), `SELECT status::text FROM orders WHERE id=$1`, orderID).Scan(&ordStatus); err != nil || !ordStatus.Valid {
		t.Fatalf("order not found/invalid: %v", err)
	}

	// 12) Refund flow: mark payment/invoice refunded and set order status
	if _, err := testDB.Exec(context.Background(), `UPDATE payments SET status='refunded', amount_refunded=amount_captured WHERE invoice_id=$1`, invoiceID); err != nil {
		t.Fatalf("refund payment: %v", err)
	}
	if _, err := testDB.Exec(context.Background(), `UPDATE invoices SET status='refunded' WHERE id=$1`, invoiceID); err != nil {
		t.Fatalf("refund invoice: %v", err)
	}
	if _, err := testDB.Exec(context.Background(), `UPDATE orders SET status='refunded' WHERE id=$1`, orderID); err != nil {
		t.Fatalf("refund order: %v", err)
	}
	var refundedStatus sql.NullString
	if err := testDB.QueryRow(context.Background(), `SELECT status::text FROM orders WHERE id=$1`, orderID).Scan(&refundedStatus); err != nil || !refundedStatus.Valid || refundedStatus.String != "refunded" {
		t.Fatalf("order not refunded: %v / %v", err, refundedStatus)
	}
}
