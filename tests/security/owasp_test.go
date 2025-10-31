package security

import (
	"context"
	"strconv"
	"testing"
	"time"

	authlib "encore.dev/beta/auth"

	auctionssvc "encore.app/svc/auctions"
	authsvc "encore.app/svc/auth"
)

func uidCtx(userID int64, role string) context.Context {
	data := authsvc.AuthData{UserID: userID, Role: role, Email: role + "@example.com"}
	return authlib.WithContext(context.Background(), authlib.UID(strconv.FormatInt(userID, 10)), &data)
}

// Broken Access Control: ensure non-verified (registered) user cannot place bids
func TestOWASP_BrokenAccessControl_UnverifiedCannotBid(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// seed registered (not verified) user
	var regID int64
	if err := testDB.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role,state,created_at,updated_at) VALUES($1,$2,'x','registered','active',NOW(),NOW()) RETURNING id`, "Reg User", "reg_user_"+strconv.FormatInt(time.Now().UnixNano(), 10)+"@example.com").Scan(&regID); err != nil {
		t.Fatalf("seed registered: %v", err)
	}
	// product & auction
	var pid int64
	if err := testDB.QueryRow(ctx, `INSERT INTO products(type,title,slug,description,price_net,status,created_at,updated_at) VALUES('pigeon',$1,$2,'d',1000,'available',NOW(),NOW()) RETURNING id`, "Sec Pigeon "+strconv.FormatInt(time.Now().UnixNano(), 10), "sec-"+strconv.FormatInt(time.Now().UnixNano(), 10)).Scan(&pid); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if _, err := testDB.Exec(ctx, `INSERT INTO pigeons(product_id,ring_number,sex) VALUES($1,$2,'unknown')`, pid, "SEC-"+strconv.FormatInt(time.Now().UnixNano(), 10)); err != nil {
		t.Fatalf("seed pigeon: %v", err)
	}

	svc := auctionssvc.NewService(testDB, nil) // nil storage for tests
	auc, err := svc.CreateAuction(context.Background(), &auctionssvc.CreateAuctionRequest{ProductID: pid, StartPrice: 1000, BidStep: 10, StartAt: time.Now().Add(-time.Minute), EndAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatalf("create auction: %v", err)
	}
	if _, err := testDB.Exec(ctx, `UPDATE auctions SET status='live' WHERE id=$1`, auc.ID); err != nil {
		t.Fatalf("activate auction: %v", err)
	}

	// attempt bid as registered (unverified)
	bidSvc := auctionssvc.NewBidService(testDB)
	if _, err := bidSvc.PlaceBid(uidCtx(regID, "registered"), auc.ID, regID, 1010); err == nil {
		t.Fatalf("expected forbidden for unverified user bid")
	}
}

// Identification & Authentication Failures: ensure auth is required for PlaceBid
func TestOWASP_AuthRequired_PlaceBid(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// seed admin & product & live auction
	var adminID int64
	_ = testDB.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role,state,created_at,updated_at) VALUES($1,$2,'x','admin','active',NOW(),NOW()) RETURNING id`, "Sec Admin2", "sec_admin2_"+strconv.FormatInt(time.Now().UnixNano(), 10)+"@example.com").Scan(&adminID)
	var pid int64
	_ = testDB.QueryRow(ctx, `INSERT INTO products(type,title,slug,description,price_net,status,created_at,updated_at) VALUES('pigeon',$1,$2,'d',1000,'available',NOW(),NOW()) RETURNING id`, "Sec2 Pigeon "+strconv.FormatInt(time.Now().UnixNano(), 10), "sec2-"+strconv.FormatInt(time.Now().UnixNano(), 10)).Scan(&pid)
	_, _ = testDB.Exec(ctx, `INSERT INTO pigeons(product_id,ring_number,sex) VALUES($1,$2,'unknown')`, pid, "SE2-"+strconv.FormatInt(time.Now().UnixNano(), 10))

	svc := auctionssvc.NewService(testDB, nil) // nil storage for tests
	auc, _ := svc.CreateAuction(uidCtx(adminID, "admin"), &auctionssvc.CreateAuctionRequest{ProductID: pid, StartPrice: 1000, BidStep: 10, StartAt: time.Now().Add(-time.Minute), EndAt: time.Now().Add(time.Hour)})
	_, _ = testDB.Exec(ctx, `UPDATE auctions SET status='live' WHERE id=$1`, auc.ID)

	// call PlaceBid without proper user (no auth context data)
	bidSvc := auctionssvc.NewBidService(testDB)
	if _, err := bidSvc.PlaceBid(context.Background(), auc.ID, 0, 1010); err == nil {
		t.Fatalf("expected error when placing bid without auth context/user")
	}
}

// Injection: attempt SQL-injection-like payloads in titles/labels should be safely handled (no crash, constraints may reject)
func TestOWASP_Injection_SQLLikePayloads(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	payload := "test'); DROP TABLE users; --"
	var pid int64
	if err := testDB.QueryRow(ctx, `INSERT INTO products(type,title,slug,description,price_net,status,created_at,updated_at) VALUES('pigeon',$1,$2,'d',1000,'available',NOW(),NOW()) RETURNING id`, payload, "inj-"+strconv.FormatInt(time.Now().UnixNano(), 10)).Scan(&pid); err != nil {
		t.Fatalf("injection product insert failed unexpectedly: %v", err)
	}
	if _, err := testDB.Exec(ctx, `INSERT INTO pigeons(product_id,ring_number,sex) VALUES($1,$2,'unknown')`, pid, "INJ-"+strconv.FormatInt(time.Now().UnixNano(), 10)); err != nil {
		t.Fatalf("injection pigeon insert failed: %v", err)
	}
}

// Cryptographic Failures: ensure passwords are stored hashed when using RegisterUser
func TestOWASP_Crypto_PasswordHashedOnRegister(t *testing.T) {
	t.Parallel()
	s, err := authsvc.NewService()
	if err != nil {
		t.Fatalf("auth svc: %v", err)
	}
	email := "crypto_user_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com"
	req := &authsvc.RegisterRequest{Name: "Crypto User", Email: email, Password: "Aa1!aaaa", CityID: 1, Phone: "+966555555555"}
	if _, err := s.RegisterUser(context.Background(), req); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	var hash string
	if err := testDB.QueryRow(context.Background(), `SELECT password_hash FROM users WHERE email=$1`, email).Scan(&hash); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	if len(hash) < 20 { // bcrypt/argon hashes are long
		t.Fatalf("password not hashed: %q", hash)
	}
}
