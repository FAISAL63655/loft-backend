package integration

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	authlib "encore.dev/beta/auth"

	users "encore.app/svc/users"
)

func createActiveUserVerifiedEmail(t *testing.T) int64 {
	t.Helper()
	ctx := context.Background()
	email := fmt.Sprintf("user_profile_%d@example.com", time.Now().UnixNano())
	var id int64
	if err := testDB.QueryRow(ctx, `
        INSERT INTO users (name,email,password_hash,phone,city_id,role,state,email_verified_at,created_at,updated_at)
        VALUES ('User Profile',$1,'x','+966533333333',1,'registered','active',NOW(),NOW(),NOW()) RETURNING id
    `, email).Scan(&id); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	return id
}

func userCtxUsers(t *testing.T, userID int64) context.Context {
	t.Helper()
	ctx := context.Background()
	ad := struct {
		UserID int64
		Role   string
		Email  string
	}{UserID: userID, Role: "registered", Email: fmt.Sprintf("u_%d@example.com", userID)}
	return authlib.WithContext(ctx, authlib.UID(strconv.FormatInt(userID, 10)), &ad)
}

func TestUsers_Profile_And_Addresses(t *testing.T) {
	t.Parallel()
	userID := createActiveUserVerifiedEmail(t)
	// Get profile via repo to avoid auth requirement
	if u, err := users.NewRepository(testDB).GetUserByID(context.Background(), userID); err != nil || u == nil || u.ID != userID {
		t.Fatalf("GetUserByID failed: %v", err)
	}

	// Update profile city to an existing city
	// Pick city id 1
	if _, err := testDB.Exec(context.Background(), `UPDATE users SET city_id=1 WHERE id=$1`, userID); err != nil {
		t.Fatalf("failed to update user city: %v", err)
	}

	// List addresses (none)
	if lst, err := users.NewRepository(testDB).GetUserAddresses(context.Background(), userID); err != nil || len(lst) != 0 {
		t.Fatalf("expected 0 addresses initially")
	}

	// Create address (email verified already)
	addrIn := &users.AddressInput{CityID: 1, Label: "Home", Line1: "Street 1", Line2: nil, IsDefault: func() *bool { v := true; return &v }()}
	addrOut, err := users.NewRepository(testDB).CreateAddress(context.Background(), userID, addrIn)
	if err != nil {
		t.Fatalf("CreateAddress failed: %v", err)
	}
	if addrOut.ID == 0 || addrOut.UserID != userID {
		t.Fatalf("unexpected address output: %+v", addrOut)
	}

	// Update address: set archived
	now := time.Now()
	upd, err := users.NewRepository(testDB).UpdateAddress(context.Background(), addrOut.ID, &users.UpdateAddressRequest{ArchivedAt: &now})
	if err != nil {
		t.Fatalf("UpdateAddress failed: %v", err)
	}
	if upd.ArchivedAt == nil {
		t.Fatalf("expected archived_at to be set")
	}
}
