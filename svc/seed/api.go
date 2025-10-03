package seed

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"encore.dev/storage/sqldb"

	"encore.app/pkg/authn"
	"encore.app/pkg/config"
)

// Database handle
var db = sqldb.Named("coredb")

// Optional Encore secret for seed protection. If not set, falls back to OS env SEED_SECRET
var secrets struct {
    SeedSecret string //encore:secret
}

//encore:service
type Service struct{}

// SeedRequest allows customizing counts (all optional)
type SeedRequest struct {
	UsersRegistered int `json:"users_registered"`
	UsersVerified   int `json:"users_verified"`
	UsersAdmins     int `json:"users_admins"`

	Pigeons  int `json:"pigeons"`
	Supplies int `json:"supplies"`

	AuctionsLive         int `json:"auctions_live"`
	AuctionsScheduled    int `json:"auctions_scheduled"`
	AuctionsEnded        int `json:"auctions_ended"`
	AuctionsCancelled    int `json:"auctions_cancelled"`
	AuctionsWinnerUnpaid int `json:"auctions_winner_unpaid"`

	BidsPerLiveAuction int `json:"bids_per_live"`
	DirectOrders       int `json:"direct_orders"`
}

// SeedResponse summarizes what was created
type SeedResponse struct {
	Created struct {
		UsersRegistered int `json:"users_registered"`
		UsersVerified   int `json:"users_verified"`
		UsersAdmins     int `json:"users_admins"`

		Pigeons  int `json:"pigeons"`
		Supplies int `json:"supplies"`

		AuctionsLive         int `json:"auctions_live"`
		AuctionsScheduled    int `json:"auctions_scheduled"`
		AuctionsEnded        int `json:"auctions_ended"`
		AuctionsCancelled    int `json:"auctions_cancelled"`
		AuctionsWinnerUnpaid int `json:"auctions_winner_unpaid"`

		Bids int `json:"bids"`
		Orders int `json:"orders"`
	} `json:"created"`
	Notes []string `json:"notes"`
}
//encore:api public raw method=POST path=/dev/seed
func RunSeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
    // Verify secret
    expected := strings.TrimSpace(getExpectedSecret())
    provided := strings.TrimSpace(r.Header.Get("X-Seed-Secret"))
    if expected == "" || provided == "" || provided != expected {
        writeJSON(w, http.StatusForbidden, map[string]any{
            "ok":      false,
            "message": "forbidden: invalid or missing X-Seed-Secret",
        })
        return
    }

    // Ensure config manager initialized (avoid nil deref in config.GetSettings)
    if config.GetGlobalManager() == nil {
        _ = config.Initialize(db, 5*time.Minute)
    }

    // Parse request (optional)
    var req SeedRequest
    _ = json.NewDecoder(r.Body).Decode(&req)
    applyDefaults(&req)
    // Initialize response and add start note
    resp := &SeedResponse{}
    resp.Notes = append(resp.Notes, "Seeding started")

    // Ensure we have at least one city (migrations already insert many)
    cityIDs, err := getCityIDs(ctx)
    if err != nil || len(cityIDs) == 0 {
        writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "message": "no cities available", "error": errString(err)})
        return
    }

    rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 1) Users
	password := "Password123" // strong enough for dev seed
	var verifiedIDs, adminIDs []int64
	if _, n, err := seedUsers(ctx, rng, cityIDs, "registered", req.UsersRegistered, password, false); err == nil {
		resp.Created.UsersRegistered = n
	} else { resp.Notes = append(resp.Notes, "users_registered error: "+err.Error()) }
	if ids, n, err := seedUsers(ctx, rng, cityIDs, "verified", req.UsersVerified, password, true); err == nil {
		verifiedIDs = ids
		resp.Created.UsersVerified = n
	} else { resp.Notes = append(resp.Notes, "users_verified error: "+err.Error()) }
	if ids, n, err := seedUsers(ctx, rng, cityIDs, "admin", req.UsersAdmins, password, true); err == nil {
		adminIDs = ids
		resp.Created.UsersAdmins = n
	} else { resp.Notes = append(resp.Notes, "users_admins error: "+err.Error()) }

	// 2) Products (pigeons & supplies)
	var pigeonIDs, supplyIDs []int64
	if ids, n, err := seedPigeons(ctx, rng, req.Pigeons); err == nil {
		pigeonIDs = ids
		resp.Created.Pigeons = n
	} else { resp.Notes = append(resp.Notes, "pigeons error: "+err.Error()) }
	if ids, n, err := seedSupplies(ctx, rng, req.Supplies); err == nil {
		supplyIDs = ids
		resp.Created.Supplies = n
	} else { resp.Notes = append(resp.Notes, "supplies error: "+err.Error()) }

	// 3) Auctions for some pigeons
	var liveIDs []int64
	if ids, n, err := seedAuctions(ctx, rng, pigeonIDs, "live", req.AuctionsLive); err == nil { liveIDs = ids; resp.Created.AuctionsLive = n } else { resp.Notes = append(resp.Notes, "auctions_live error: "+err.Error()) }
	if _, n, err := seedAuctions(ctx, rng, pigeonIDs, "scheduled", req.AuctionsScheduled); err == nil { resp.Created.AuctionsScheduled = n } else { resp.Notes = append(resp.Notes, "auctions_scheduled error: "+err.Error()) }
	if _, n, err := seedAuctions(ctx, rng, pigeonIDs, "ended", req.AuctionsEnded); err == nil { resp.Created.AuctionsEnded = n } else { resp.Notes = append(resp.Notes, "auctions_ended error: "+err.Error()) }
	if _, n, err := seedAuctions(ctx, rng, pigeonIDs, "cancelled", req.AuctionsCancelled); err == nil { resp.Created.AuctionsCancelled = n } else { resp.Notes = append(resp.Notes, "auctions_cancelled error: "+err.Error()) }
	if _, n, err := seedAuctions(ctx, rng, pigeonIDs, "winner_unpaid", req.AuctionsWinnerUnpaid); err == nil { resp.Created.AuctionsWinnerUnpaid = n } else { resp.Notes = append(resp.Notes, "auctions_winner_unpaid error: "+err.Error()) }

	// 4) Bids for live auctions (only verified/admin allowed by DB trigger)
	if len(liveIDs) > 0 && (len(verifiedIDs) > 0 || len(adminIDs) > 0) {
		cnt, err := seedBids(ctx, rng, liveIDs, append(verifiedIDs, adminIDs...), req.BidsPerLiveAuction)
		if err == nil { resp.Created.Bids = cnt } else { resp.Notes = append(resp.Notes, "bids error: "+err.Error()) }
	}

	// 5) Direct orders (use verified users, buy supplies)
	if len(verifiedIDs) > 0 && len(supplyIDs) > 0 && req.DirectOrders > 0 {
		cnt, err := seedDirectOrders(ctx, rng, verifiedIDs, supplyIDs, req.DirectOrders)
		if err == nil { resp.Created.Orders = cnt } else { resp.Notes = append(resp.Notes, "orders error: "+err.Error()) }
	}

	resp.Notes = append(resp.Notes, "Seeding finished")
	writeJSON(w, http.StatusOK, resp)
}

func getExpectedSecret() string {
	if strings.TrimSpace(secrets.SeedSecret) != "" {
		return strings.TrimSpace(secrets.SeedSecret)
	}
	return strings.TrimSpace(os.Getenv("SEED_SECRET"))
}

func applyDefaults(r *SeedRequest) {
	if r.UsersRegistered == 0 { r.UsersRegistered = 10 }
	if r.UsersVerified == 0 { r.UsersVerified = 8 }
	if r.UsersAdmins == 0 { r.UsersAdmins = 2 }
	if r.Pigeons == 0 { r.Pigeons = 12 }
	if r.Supplies == 0 { r.Supplies = 12 }
	if r.AuctionsLive == 0 { r.AuctionsLive = 3 }
	if r.AuctionsScheduled == 0 { r.AuctionsScheduled = 2 }
	if r.AuctionsEnded == 0 { r.AuctionsEnded = 1 }
	if r.AuctionsCancelled == 0 { r.AuctionsCancelled = 1 }
	if r.AuctionsWinnerUnpaid == 0 { r.AuctionsWinnerUnpaid = 1 }
	if r.BidsPerLiveAuction == 0 { r.BidsPerLiveAuction = 6 }
	if r.DirectOrders == 0 { r.DirectOrders = 5 }
}

func errString(err error) string { if err == nil { return "" }; return err.Error() }

// --- Helpers ---

func getCityIDs(ctx context.Context) ([]int64, error) {
	rows, err := db.Stdlib().QueryContext(ctx, `SELECT id FROM cities WHERE enabled = true ORDER BY id`)
	if err != nil { return nil, err }
	defer rows.Close()
	var ids []int64
	for rows.Next() { var id int64; if err := rows.Scan(&id); err != nil { return nil, err }; ids = append(ids, id) }
	return ids, nil
}

func randomChoice[T any](r *rand.Rand, arr []T) T {
	return arr[r.Intn(len(arr))]
}

func seedUsers(ctx context.Context, r *rand.Rand, cityIDs []int64, role string, n int, password string, verify bool) ([]int64, int, error) {
	if n <= 0 { return nil, 0, nil }
	hash, _ := authn.HashPassword(password)
	created := 0
	ids := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("%s User %d", strings.Title(role), r.Intn(100000))
		email := strings.ToLower(fmt.Sprintf("seed.%s.%d@loft.local", role, r.Intn(1_000_000_000)))
		phone := fmt.Sprintf("05%08d", r.Intn(100000000))
		city := randomChoice(r, cityIDs)
		var id int64
		var verifiedAt sql.NullTime
		if verify { verifiedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true} }
		err := db.Stdlib().QueryRowContext(ctx, `
			INSERT INTO users (name, email, phone, password_hash, city_id, role, state, email_verified_at)
			VALUES ($1, LOWER($2), $3, $4, $5, $6, 'active', $7)
			ON CONFLICT (email) DO NOTHING
			RETURNING id
		`, name, email, phone, hash, city, role, verifiedAt).Scan(&id)
		if err != nil {
			if err == sql.ErrNoRows {
				// Already exists; fetch id
				if e := db.Stdlib().QueryRowContext(ctx, `SELECT id FROM users WHERE email=LOWER($1)`, email).Scan(&id); e != nil { continue }
			} else { continue }
		}
		if id > 0 { ids = append(ids, id); created++ }
	}
	return ids, created, nil
}

func seedPigeons(ctx context.Context, r *rand.Rand, n int) ([]int64, int, error) {
	if n <= 0 { return nil, 0, nil }
	ids := make([]int64, 0, n)
	created := 0
	for i := 0; i < n; i++ {
		title := fmt.Sprintf("حمامة مميزة %d", r.Intn(1000000))
		slug := fmt.Sprintf("seed-pigeon-%d-%d", time.Now().Unix(), r.Intn(1_000_000))
		price := float64(500 + r.Intn(4500)) // 500..5000 SAR net
		var pid int64
		err := db.Stdlib().QueryRowContext(ctx, `
			INSERT INTO products (type, title, slug, description, price_net, status)
			VALUES ('pigeon', $1, $2, $3, $4, 'available')
			RETURNING id
		`, title, slug, "Seed pigeon", price).Scan(&pid)
		if err != nil { continue }
		ring := fmt.Sprintf("RN-%06d", r.Intn(1_000_000))
		sex := []string{"male","female","unknown"}[r.Intn(3)]
		_, err = db.Stdlib().ExecContext(ctx, `
			INSERT INTO pigeons (product_id, ring_number, sex, lineage)
			VALUES ($1, $2, $3, $4)
		`, pid, ring, sex, "Seed lineage")
		if err != nil { continue }
		ids = append(ids, pid); created++
	}
	return ids, created, nil
}

func seedSupplies(ctx context.Context, r *rand.Rand, n int) ([]int64, int, error) {
	if n <= 0 { return nil, 0, nil }
	ids := make([]int64, 0, n)
	created := 0
	for i := 0; i < n; i++ {
		title := fmt.Sprintf("مستلزم حمام %d", r.Intn(1000000))
		slug := fmt.Sprintf("seed-supply-%d-%d", time.Now().Unix(), r.Intn(1_000_000))
		price := float64(20 + r.Intn(180)) // 20..200 SAR net
		var pid int64
		err := db.Stdlib().QueryRowContext(ctx, `
			INSERT INTO products (type, title, slug, description, price_net, status)
			VALUES ('supply', $1, $2, $3, $4, 'available')
			RETURNING id
		`, title, slug, "Seed supply", price).Scan(&pid)
		if err != nil { continue }
		sku := fmt.Sprintf("SKU-%06d", r.Intn(1_000_000))
		stock := 5 + r.Intn(50)
		low := 5
		_, err = db.Stdlib().ExecContext(ctx, `
			INSERT INTO supplies (product_id, sku, stock_qty, low_stock_threshold)
			VALUES ($1, $2, $3, $4)
		`, pid, sku, stock, low)
		if err != nil { continue }
		ids = append(ids, pid); created++
	}
	return ids, created, nil
}

func seedAuctions(ctx context.Context, r *rand.Rand, pigeonIDs []int64, status string, n int) ([]int64, int, error) {
	if n <= 0 || len(pigeonIDs) == 0 { return nil, 0, nil }
	ids := make([]int64, 0, n)
	created := 0
	used := map[int64]bool{}
	for i := 0; i < n; i++ {
		// pick unique product for active-like statuses to avoid uq_auction_active_product
		var productID int64
		for tries := 0; tries < 100; tries++ {
			p := pigeonIDs[r.Intn(len(pigeonIDs))]
			if !used[p] { productID = p; break }
		}
		if productID == 0 { productID = pigeonIDs[r.Intn(len(pigeonIDs))] }

		startPrice := float64(300 + r.Intn(900))
		bidStep := 10 + r.Intn(40)
		reserve := sql.NullFloat64{Valid: r.Intn(2) == 0, Float64: startPrice + float64(50+ r.Intn(200))}

		startAt := time.Now().Add(-30 * time.Minute)
		endAt := time.Now().Add(30 * time.Minute)
		s := status
		switch status {
		case "scheduled":
			startAt = time.Now().Add(1 * time.Hour)
			endAt = time.Now().Add(24 * time.Hour)
		case "ended":
			startAt = time.Now().Add(-48 * time.Hour)
			endAt = time.Now().Add(-24 * time.Hour)
		case "cancelled":
			startAt = time.Now().Add(-48 * time.Hour)
			endAt = time.Now().Add(24 * time.Hour)
		case "winner_unpaid":
			startAt = time.Now().Add(-24 * time.Hour)
			endAt = time.Now().Add(-1 * time.Hour)
		}

		var auctionID int64
		err := db.Stdlib().QueryRowContext(ctx, `
			INSERT INTO auctions (product_id, start_price, bid_step, reserve_price, start_at, end_at, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id
		`, productID, startPrice, bidStep, reserve, startAt, endAt, s).Scan(&auctionID)
		if err != nil { continue }

		// update product status for active auctions
		if s == "live" || s == "scheduled" {
			_, _ = db.Stdlib().ExecContext(ctx, `UPDATE products SET status='in_auction', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, productID)
		}
		ids = append(ids, auctionID); created++; used[productID] = true
	}
	return ids, created, nil
}

func seedBids(ctx context.Context, r *rand.Rand, liveAuctionIDs []int64, bidderUserIDs []int64, bidsPerAuction int) (int, error) {
	if bidsPerAuction <= 0 || len(liveAuctionIDs) == 0 || len(bidderUserIDs) == 0 { return 0, nil }
	inserted := 0
	for _, aucID := range liveAuctionIDs {
		var startPrice float64
		var bidStep int
		if err := db.Stdlib().QueryRowContext(ctx, `SELECT start_price, bid_step FROM auctions WHERE id=$1`, aucID).Scan(&startPrice, &bidStep); err != nil { continue }
		current := startPrice
		for i := 0; i < bidsPerAuction; i++ {
			current = current + float64(bidStep)
			bidder := bidderUserIDs[r.Intn(len(bidderUserIDs))]
			// INSERT; trigger validates role and updates snapshots and anti-sniping
			if _, err := db.Stdlib().ExecContext(ctx, `INSERT INTO bids (auction_id, user_id, amount, bidder_name_snapshot) VALUES ($1,$2,$3,'')`, aucID, bidder, round2(current)); err == nil { inserted++ } else { break }
		}
	}
	return inserted, nil
}

func seedDirectOrders(ctx context.Context, r *rand.Rand, userIDs []int64, supplyIDs []int64, n int) (int, error) {
	if n <= 0 { return 0, nil }
	settings := config.GetSettings()
	vat := 0.0
	if settings != nil && settings.VATEnabled { vat = settings.VATRate }
	created := 0
	for i := 0; i < n; i++ {
		uid := userIDs[r.Intn(len(userIDs))]
		pid := supplyIDs[r.Intn(len(supplyIDs))]
		// read price_net
		var priceNet float64
		if err := db.Stdlib().QueryRowContext(ctx, `SELECT price_net FROM products WHERE id=$1`, pid).Scan(&priceNet); err != nil { continue }
		unitGross := round2(priceNet * (1+vat))
		qty := 1 + r.Intn(3)

		tx, err := db.Stdlib().BeginTx(ctx, nil)
		if err != nil { continue }
		var orderID int64
		if err := tx.QueryRowContext(ctx, `INSERT INTO orders (user_id, source) VALUES ($1,'direct') RETURNING id`, uid).Scan(&orderID); err != nil { _ = tx.Rollback(); continue }
		lineGross := round2(unitGross * float64(qty))
		if _, err := tx.ExecContext(ctx, `INSERT INTO order_items (order_id, product_id, qty, unit_price_gross, line_total_gross) VALUES ($1,$2,$3,$4,$5)`, orderID, pid, qty, unitGross, lineGross); err != nil { _ = tx.Rollback(); continue }
		// Touch order to trigger totals function
		if _, err := tx.ExecContext(ctx, `UPDATE orders SET updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, orderID); err != nil { _ = tx.Rollback(); continue }
		if err := tx.Commit(); err != nil { _ = tx.Rollback(); continue }
		created++
	}
	return created, nil
}

func round2(v float64) float64 { return math.Floor(v*100+0.5)/100 }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
