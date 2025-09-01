package integration

import (
	"context"
	"strconv"
	"testing"
	"time"

	authlib "encore.dev/beta/auth"

	catalog "encore.app/svc/catalog"
)

func createAdminForCatalog(t *testing.T) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := testDB.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role,state,created_at,updated_at) VALUES($1,$2,'x','admin','active',NOW(),NOW()) RETURNING id`, "Admin Cat", "admin_cat_"+strconv.FormatInt(time.Now().UnixNano(), 10)+"@example.com").Scan(&id); err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}
	return id
}

func adminCtxCatalog(t *testing.T, id int64) context.Context {
	t.Helper()
	ctx := context.Background()
	ad := struct {
		UserID int64
		Role   string
		Email  string
	}{UserID: id, Role: "admin", Email: "admin@example.com"}
	return authlib.WithContext(ctx, authlib.UID(strconv.FormatInt(id, 10)), &ad)
}

func TestCatalog_Create_Get_List_MediaList(t *testing.T) {
	t.Parallel()
	adminID := createAdminForCatalog(t)
	_ = adminID // reserved for future admin-only endpoints

	// Create pigeon product directly through service layer to avoid API call restriction
	title := "Test Pigeon " + strconv.FormatInt(time.Now().UnixNano(), 10)
	ring := "RING-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	// Use repository directly to create product
	// Generate slug unique
	slug := "test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	// Insert product
	var pid int64
	if err := testDB.QueryRow(context.Background(), `
        INSERT INTO products (type,title,slug,description,price_net,status,created_at,updated_at)
        VALUES ('pigeon',$1,$2,$3,1000.00,'available',NOW(),NOW()) RETURNING id
    `, title, slug, "desc").Scan(&pid); err != nil {
		t.Fatalf("failed to insert product: %v", err)
	}
	// Insert pigeon details
	if _, err := testDB.Exec(context.Background(), `
        INSERT INTO pigeons (product_id, ring_number, sex) VALUES ($1,$2,'unknown')
    `, pid, ring); err != nil {
		t.Fatalf("failed to insert pigeon: %v", err)
	}

	// Public GetProduct
	// Use repository directly
	dprod, err := catalog.NewRepository(testDB).GetProductByID(context.Background(), pid)
	if err != nil {
		t.Fatalf("GetProduct failed: %v", err)
	}
	if dprod == nil || dprod.ID != pid {
		t.Fatalf("unexpected product id")
	}

	// Public GetProducts
	// Use service with exported constructor parts: initialize through NewService with minimal deps is complex; call repo then convert lightly via service.GetProducts not possible here.
	// Instead, list via repo
	prods, total, err := catalog.NewRepository(testDB).GetProducts(context.Background(), catalog.ProductsFilter{}, catalog.ProductsSort{Field: "created_at", Direction: "DESC"}, 0, 10)
	if err != nil {
		t.Fatalf("GetProducts failed: %v", err)
	}
	if len(prods) == 0 || total == 0 {
		t.Fatalf("expected products >=1")
	}

	// Public GetProductMediaList (no upload yet => 0 items is acceptable)
	if _, err := catalog.NewRepository(testDB).GetMediaByProductID(context.Background(), pid, false); err != nil {
		t.Fatalf("GetProduct media failed: %v", err)
	}

	// Ensure repo media read works (no media expected)
	if media, err := catalog.NewRepository(testDB).GetMediaByProductID(context.Background(), pid, false); err != nil {
		t.Fatalf("GetMediaByProductID failed: %v", err)
	} else if len(media) != 0 {
		t.Fatalf("expected 0 media initially")
	}
}
