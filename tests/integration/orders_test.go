package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"encore.app/pkg/authn"
	"encore.dev/storage/sqldb"
)

// TestCreateOrder: إنشاء طلب واختبار حالته وقيمته
func TestCreateOrder(t *testing.T) {
	ctx := context.Background()
	db := testDB

	// تنظيف البيانات القديمة
	cleanupOrderTestData(t, db)
	defer cleanupOrderTestData(t, db)

	// إنشاء مزاد منتهي مع فائز ثم إنشاء طلب pending_payment
	_, winnerID, productID := createCompletedAuctionForOrders(t, db)

	orderID, _, grand := createTestOrderForOrders(t, db, winnerID, productID)
	if orderID == 0 || grand <= 0 {
		t.Fatalf("expected order created with total > 0")
	}

	// تحقق من الحالة من قاعدة البيانات
	var status string
	if err := db.QueryRow(ctx, `SELECT status::text FROM orders WHERE id=$1`, orderID).Scan(&status); err != nil {
		t.Fatalf("failed to read order status: %v", err)
	}
	if status != "pending_payment" {
		t.Errorf("expected status pending_payment got %s", status)
	}

	// تحقق من المشتري
	var buyer int64
	if err := db.QueryRow(ctx, `SELECT user_id FROM orders WHERE id=$1`, orderID).Scan(&buyer); err == nil {
		if buyer != winnerID {
			t.Errorf("expected buyer to equal winner, got %d != %d", buyer, winnerID)
		}
	}
}

// TestOrderStatusUpdate: تحديث حالة الطلب والتحقق منها
func TestOrderStatusUpdate(t *testing.T) {
	ctx := context.Background()
	db := testDB

	cleanupOrderTestData(t, db)
	defer cleanupOrderTestData(t, db)

	_, winnerID, productID := createCompletedAuctionForOrders(t, db)
	orderID, _, _ := createTestOrderForOrders(t, db, winnerID, productID)

	// تحديث الحالة إلى paid
	if _, err := db.Exec(ctx, `UPDATE orders SET status='paid', updated_at=NOW() WHERE id=$1`, orderID); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}
	var status string
	if err := db.QueryRow(ctx, `SELECT status::text FROM orders WHERE id=$1`, orderID).Scan(&status); err != nil {
		t.Fatalf("failed to read status: %v", err)
	}
	if status != "paid" {
		t.Errorf("expected paid, got %s", status)
	}
	// الغاء الطلب
	if _, err := db.Exec(ctx, `UPDATE orders SET status='cancelled', updated_at=NOW() WHERE id=$1`, orderID); err != nil {
		t.Fatalf("failed to set cancelled: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT status::text FROM orders WHERE id=$1`, orderID).Scan(&status); err != nil || status != "cancelled" {
		t.Errorf("expected cancelled, got %s err=%v", status, err)
	}
}

// TestRefundPayment: إنشاء دفع واسترداده والتحقق من القيم من قاعدة البيانات
func TestRefundPayment(t *testing.T) {
	ctx := context.Background()
	db := testDB

	cleanupOrderTestData(t, db)
	defer cleanupOrderTestData(t, db)

	_, winnerID, productID := createCompletedAuctionForOrders(t, db)
	orderID, _, amount := createTestOrderForOrders(t, db, winnerID, productID)
	// إنشاء فاتورة ومدفوعات
	invoiceID := createInvoiceForOrder(t, db, orderID)
	paymentID := createPaymentForInvoice(t, db, invoiceID, amount)
	_ = paymentID

	// استرداد جزئي
	refund := amount / 2
	if _, err := db.Exec(ctx, `UPDATE payments SET amount_refunded=$1, refund_partial=true, updated_at=NOW() WHERE invoice_id=$2`, refund, invoiceID); err != nil {
		t.Fatalf("failed partial refund: %v", err)
	}
	var gotRefunded float64
	if err := db.QueryRow(ctx, `SELECT amount_refunded FROM payments WHERE invoice_id=$1`, invoiceID).Scan(&gotRefunded); err != nil || gotRefunded <= 0 {
		t.Fatalf("expected refunded > 0, got %f err=%v", gotRefunded, err)
	}
	// استرداد كامل
	if _, err := db.Exec(ctx, `UPDATE payments SET amount_refunded=$1, refund_partial=false, status='refunded', updated_at=NOW() WHERE invoice_id=$2`, amount, invoiceID); err != nil {
		t.Fatalf("failed full refund: %v", err)
	}
	var pstatus string
	if err := db.QueryRow(ctx, `SELECT status::text FROM payments WHERE invoice_id=$1`, invoiceID).Scan(&pstatus); err != nil || pstatus != "refunded" {
		t.Errorf("expected payment status refunded, got %s err=%v", pstatus, err)
	}
}

// Helpers (kept in-file)

func cleanupOrderTestData(t *testing.T, db *sqldb.Database) {
	ctx := context.Background()
	queries := []string{
		"DELETE FROM payments",
		"DELETE FROM invoices",
		"DELETE FROM order_items",
		"DELETE FROM orders",
		"DELETE FROM bids",
		"DELETE FROM auctions",
		"DELETE FROM pigeons WHERE product_id IN (SELECT id FROM products WHERE title LIKE 'Test Order%')",
		"DELETE FROM products WHERE title LIKE 'Test Order%' OR title='Test Pigeon'",
		"DELETE FROM addresses WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'test_%@example.com' OR email LIKE '%_payment@example.com' OR email LIKE '%_user@example.com')",
		"DELETE FROM users WHERE email LIKE 'test_%@example.com' OR email LIKE '%_payment@example.com' OR email LIKE '%_user@example.com'",
	}
	for _, q := range queries {
		_, _ = db.Exec(ctx, q)
	}
}

func createCompletedAuctionForOrders(t *testing.T, db *sqldb.Database) (int64, int64, int64) {
	ctx := context.Background()
	adminEmail := fmt.Sprintf("orders_admin_%d@example.com", time.Now().UnixNano())
	_ = createOrdersAdminWithEmail(t, db, adminEmail)
	winnerID := createOrdersUser(t, db, "test_winner@example.com", "SecurePass123!", true)

	// create product+pigeon
	slug := fmt.Sprintf("test-order-pigeon-%d", time.Now().UnixNano())
	var productID int64
	if err := db.QueryRow(ctx, `INSERT INTO products (type, title, slug, price_net, status) VALUES ('pigeon', 'Test Order Pigeon', $1, 1000.00, 'available') RETURNING id`, slug).Scan(&productID); err != nil {
		t.Fatalf("Failed to create product: %v", err)
	}
	ring := fmt.Sprintf("RO-%d", time.Now().UnixNano())
	if _, err := db.Exec(ctx, `INSERT INTO pigeons (product_id, ring_number, sex) VALUES ($1, $2, 'male')`, productID, ring); err != nil {
		t.Fatalf("Failed to create pigeon: %v", err)
	}

	// create LIVE auction first to allow bidding
	var auctionID int64
	if err := db.QueryRow(ctx, `
		INSERT INTO auctions (
			product_id, start_price, bid_step, reserve_price, start_at, end_at, status, created_at, updated_at
		) VALUES (
			$1, 1000.00, 100, NULL, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '1 hour', 'live', NOW(), NOW()
		) RETURNING id
	`, productID).Scan(&auctionID); err != nil {
		t.Fatalf("Failed to create live auction: %v", err)
	}
	// place winning bid while live
	if _, err := db.Exec(ctx, `INSERT INTO bids (auction_id, user_id, amount, bidder_name_snapshot, bidder_city_id_snapshot, created_at) VALUES ($1, $2, $3, 'Winner', 1, NOW())`, auctionID, winnerID, 1500.00); err != nil {
		t.Fatalf("Failed to create winning bid: %v", err)
	}
	// mark auction ended
	if _, err := db.Exec(ctx, `UPDATE auctions SET status='ended', end_at=NOW() - INTERVAL '1 hour', updated_at=NOW() WHERE id=$1`, auctionID); err != nil {
		t.Fatalf("Failed to end auction: %v", err)
	}
	return auctionID, winnerID, productID
}

func createTestOrderForOrders(t *testing.T, db *sqldb.Database, buyerID int64, productID int64) (int64, int64, float64) {
	ctx := context.Background()
	var orderID int64
	amount := 1500.00

	// Insert order with pending_payment status
	if err := db.QueryRow(ctx, `
		INSERT INTO orders (user_id, source, status, subtotal_gross, vat_amount, shipping_fee_gross, grand_total)
		VALUES ($1, 'auction', 'pending_payment', 0, 0, 0, 0) RETURNING id
	`, buyerID).Scan(&orderID); err != nil {
		t.Fatalf("Failed to create order: %v", err)
	}
	// Insert order item for the product
	if _, err := db.Exec(ctx, `
		INSERT INTO order_items (order_id, product_id, qty, unit_price_gross, line_total_gross)
		VALUES ($1, $2, 1, $3, $3)
	`, orderID, productID, amount); err != nil {
		t.Fatalf("Failed to insert order item: %v", err)
	}
	// Trigger totals recalculation via update
	if _, err := db.Exec(ctx, `UPDATE orders SET updated_at=NOW() WHERE id=$1`, orderID); err != nil {
		t.Fatalf("Failed to update order totals: %v", err)
	}
	var grand float64
	if err := db.QueryRow(ctx, `SELECT grand_total FROM orders WHERE id=$1`, orderID).Scan(&grand); err != nil {
		t.Fatalf("Failed to read grand_total: %v", err)
	}
	return orderID, buyerID, grand
}

func createInvoiceForOrder(t *testing.T, db *sqldb.Database, orderID int64) int64 {
	ctx := context.Background()
	var year int
	_ = db.QueryRow(ctx, `SELECT EXTRACT(YEAR FROM NOW())::INT`).Scan(&year)
	var number string
	_ = db.QueryRow(ctx, `SELECT next_invoice_number($1)`, year).Scan(&number)
	var invoiceID int64
	if err := db.QueryRow(ctx, `
		INSERT INTO invoices (order_id, number, status, vat_rate_snapshot, totals)
		VALUES ($1, $2, 'paid', 0.150, '{}'::jsonb) RETURNING id
	`, orderID, number).Scan(&invoiceID); err != nil {
		t.Fatalf("Failed to create invoice: %v", err)
	}
	return invoiceID
}

func createPaymentForInvoice(t *testing.T, db *sqldb.Database, invoiceID int64, amount float64) int64 {
	ctx := context.Background()
	gwRef := fmt.Sprintf("TXN-%d", time.Now().UnixNano())
	var paymentID int64
	if err := db.QueryRow(ctx, `
		INSERT INTO payments (invoice_id, gateway, gateway_ref, status, amount_authorized, amount_captured, amount_refunded, refund_partial, currency, raw_response)
		VALUES ($1, 'moyasar', $2, 'paid', $3, $3, 0, false, 'SAR', '{}'::jsonb) RETURNING id
	`, invoiceID, gwRef, amount).Scan(&paymentID); err != nil {
		t.Fatalf("Failed to create payment: %v", err)
	}
	return paymentID
}

func createOrdersAdminWithEmail(t *testing.T, db *sqldb.Database, email string) int64 {
    ctx := context.Background()
    hash, _ := authn.HashPassword("AdminPass123!")
    var id int64
    if err := db.QueryRow(ctx, `INSERT INTO users (name,email,password_hash,phone,city_id,role,state,email_verified_at,created_at,updated_at) VALUES ('Admin',$1,$2,$3,1,'admin','active',NOW(),NOW(),NOW()) RETURNING id`, email, hash, uniqueTestPhone()).Scan(&id); err != nil {
        t.Fatalf("failed to create admin: %v", err)
    }
    return id
}

func createOrdersAdmin(t *testing.T, db *sqldb.Database) int64 {
	return createOrdersAdminWithEmail(t, db, fmt.Sprintf("orders_admin_%d@example.com", time.Now().UnixNano()))
}

func createOrdersUser(t *testing.T, db *sqldb.Database, email, password string, verified bool) int64 {
    ctx := context.Background()
    hash, _ := authn.HashPassword(password)
    var id int64
    var verifiedAt interface{}
    role := "registered"
    if verified {
        verifiedAt = time.Now()
        role = "verified"
    } else {
        verifiedAt = nil
    }
    if err := db.QueryRow(ctx, `INSERT INTO users (name,email,password_hash,phone,city_id,role,state,email_verified_at,created_at,updated_at) VALUES ('Test User',$1,$2,$3,1,$4,'active',$5,NOW(),NOW()) RETURNING id`, email, hash, uniqueTestPhone(), role, verifiedAt).Scan(&id); err != nil {
        t.Fatalf("failed to create user: %v", err)
    }
    return id
}
