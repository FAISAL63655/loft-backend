package performance

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"encore.app/pkg/authn"
	auctionssvc "encore.app/svc/auctions"
	authsvc "encore.app/svc/auth"
	"encore.dev/beta/auth"
	"encore.dev/storage/sqldb"
)

// TestBiddingPerformance اختبار أداء المزايدات - PRD: ≥100 مزايدة في 10 ثوان
func TestBiddingPerformance(t *testing.T) {
	ctx := context.Background()
	db := testDB

	// تنظيف البيانات القديمة
	cleanupPerformanceTestData(t, db)
	defer cleanupPerformanceTestData(t, db)

	_ = auctionssvc.NewService(testDB)
	bidService := auctionssvc.NewBidService(testDB)

	// إنشاء مزاد نشط للاختبار
	auctionID := createActiveAuction(t, db)

	// إنشاء 20 مستخدم للمزايدة المتزامنة
	numUsers := 20
	users := make([]testUser, numUsers)
	for i := 0; i < numUsers; i++ {
		email := fmt.Sprintf("perf_user_%d@example.com", i)
		userID := createVerifiedUser(t, db, email)
		users[i] = testUser{
			ID:    userID,
			Email: email,
		}
	}

	// متغيرات لتتبع الأداء
	var successfulBids int32
	var failedBids int32
	var totalDuration time.Duration

	// قناة لتنسيق البداية المتزامنة
	startSignal := make(chan struct{})
	var wg sync.WaitGroup

	// عدد المزايدات لكل مستخدم
	bidsPerUser := 5
	totalBids := numUsers * bidsPerUser

	t.Logf("Starting performance test with %d users, %d bids each, total %d bids",
		numUsers, bidsPerUser, totalBids)

	// بدء وقت الاختبار
	startTime := time.Now()

	// إطلاق goroutines للمستخدمين
	for userIdx, user := range users {
		wg.Add(1)
		go func(idx int, u testUser) {
			defer wg.Done()

			// انتظار إشارة البداية
			<-startSignal

			// سياق المستخدم
			userCtx := auth.WithContext(ctx, auth.UID(strconv.FormatInt(u.ID, 10)), &authsvc.AuthData{
				UserID: u.ID,
				Role:   "verified",
				Email:  u.Email,
			})

			// تنفيذ المزايدات
			baseAmount := 1100.00
			for bidNum := 0; bidNum < bidsPerUser; bidNum++ {
				// حساب مبلغ المزايدة (يزيد تدريجياً)
				bidAmount := baseAmount + float64(idx*100) + float64(bidNum*50)

				bidStart := time.Now()
				_, err := bidService.PlaceBid(userCtx, auctionID, u.ID, bidAmount)
				bidDuration := time.Since(bidStart)

				if err == nil {
					atomic.AddInt32(&successfulBids, 1)
				} else {
					atomic.AddInt32(&failedBids, 1)
				}

				// تتبع المدة الإجمالية
				atomic.AddInt64((*int64)(&totalDuration), int64(bidDuration))

				// تأخير صغير لتوزيع الحمل
				time.Sleep(10 * time.Millisecond)
			}
		}(userIdx, user)
	}

	// إطلاق جميع المستخدمين في نفس الوقت
	close(startSignal)

	// انتظار انتهاء جميع المزايدات
	wg.Wait()

	// حساب الوقت الإجمالي
	totalTime := time.Since(startTime)

	// عرض النتائج
	t.Logf("Performance Test Results:")
	t.Logf("========================")
	t.Logf("Total bids attempted: %d", totalBids)
	t.Logf("Successful bids: %d", successfulBids)
	t.Logf("Failed bids: %d", failedBids)
	t.Logf("Total time: %v", totalTime)
	t.Logf("Average time per bid: %v", time.Duration(totalDuration/time.Duration(totalBids)))
	t.Logf("Throughput: %.2f bids/second", float64(successfulBids)/totalTime.Seconds())

	// التحقق من متطلبات PRD: ≥100 مزايدة في 10 ثوان
	if totalTime > 10*time.Second {
		t.Errorf("Performance requirement not met: took %v to process %d bids (should be ≤10s for 100 bids)",
			totalTime, totalBids)
	}

	if float64(successfulBids)/totalTime.Seconds() < 10 {
		t.Errorf("Throughput requirement not met: %.2f bids/second (should be ≥10 bids/second)",
			float64(successfulBids)/totalTime.Seconds())
	}
}

// TestConcurrentAuctionCreation اختبار إنشاء مزادات متزامنة
func TestConcurrentAuctionCreation(t *testing.T) {
	ctx := context.Background()
	db := testDB

	// تنظيف البيانات القديمة
	cleanupPerformanceTestData(t, db)
	defer cleanupPerformanceTestData(t, db)

	aSvc := auctionssvc.NewService(testDB)
	_ = aSvc

	// إنشاء عدة مستخدمين admin
	numAdmins := 5
	admins := make([]int64, numAdmins)
	for i := 0; i < numAdmins; i++ {
		admins[i] = createAdminUser(t, db, fmt.Sprintf("admin_%d@example.com", i))
	}

	// متغيرات لتتبع الأداء
	var successfulCreations int32
	var failedCreations int32

	var wg sync.WaitGroup
	startSignal := make(chan struct{})

	// عدد المزادات لكل admin
	auctionsPerAdmin := 10
	totalAuctions := numAdmins * auctionsPerAdmin

	t.Logf("Starting concurrent auction creation test with %d admins, %d auctions each",
		numAdmins, auctionsPerAdmin)

	startTime := time.Now()

	// إطلاق goroutines للمدراء
	for adminIdx, adminID := range admins {
		wg.Add(1)
		go func(idx int, aID int64) {
			defer wg.Done()

			<-startSignal

			// سياق المدير
			adminCtx := auth.WithContext(ctx, auth.UID(strconv.FormatInt(aID, 10)), &authsvc.AuthData{UserID: aID, Role: "admin", Email: fmt.Sprintf("admin_%d@example.com", idx)})

			// إنشاء المزادات
			for auctionNum := 0; auctionNum < auctionsPerAdmin; auctionNum++ {
				// create unique product each time to avoid active auction constraint
				prodID := mustCreateProductPigeon(t, db)
				req := &auctionssvc.CreateAuctionRequest{
					ProductID:  prodID,
					StartPrice: 1000.00 + float64(auctionNum*100),
					BidStep:    50,
					StartAt:    time.Now().Add(1 * time.Hour),
					EndAt:      time.Now().Add(25 * time.Hour),
				}
				_, err := aSvc.CreateAuction(adminCtx, req)
				if err == nil {
					atomic.AddInt32(&successfulCreations, 1)
				} else {
					atomic.AddInt32(&failedCreations, 1)
				}
				// تأخير صغير
				time.Sleep(5 * time.Millisecond)
			}
		}(adminIdx, adminID)
	}

	// إطلاق جميع المدراء
	close(startSignal)
	wg.Wait()

	totalTime := time.Since(startTime)

	// عرض النتائج
	t.Logf("Concurrent Auction Creation Results:")
	t.Logf("====================================")
	t.Logf("Total auctions attempted: %d", totalAuctions)
	t.Logf("Successful creations: %d", successfulCreations)
	t.Logf("Failed creations: %d", failedCreations)
	t.Logf("Total time: %v", totalTime)
	t.Logf("Throughput: %.2f auctions/second", float64(successfulCreations)/totalTime.Seconds())

	// التحقق من النجاح
	if failedCreations > 0 {
		t.Errorf("Some auction creations failed: %d out of %d", failedCreations, totalAuctions)
	}
}

// TestDatabaseConnectionPool اختبار تجمع اتصالات قاعدة البيانات
func TestDatabaseConnectionPool(t *testing.T) {
	ctx := context.Background()
	db := testDB

	// عدد الاتصالات المتزامنة
	numConnections := 50
	queriesPerConnection := 20

	var wg sync.WaitGroup
	startSignal := make(chan struct{})

	var successfulQueries int32
	var failedQueries int32
	var totalQueryTime int64

	t.Logf("Testing database connection pool with %d concurrent connections", numConnections)

	startTime := time.Now()

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(connID int) {
			defer wg.Done()

			<-startSignal

			for q := 0; q < queriesPerConnection; q++ {
				queryStart := time.Now()
				// استعلام بسيط للاختبار
				var count int
				err := db.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
				queryDuration := time.Since(queryStart)
				atomic.AddInt64(&totalQueryTime, int64(queryDuration))
				if err == nil {
					atomic.AddInt32(&successfulQueries, 1)
				} else {
					atomic.AddInt32(&failedQueries, 1)
				}
				// تأخير صغير
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	close(startSignal)
	wg.Wait()

	totalTime := time.Since(startTime)
	totalQueries := numConnections * queriesPerConnection

	// عرض النتائج
	t.Logf("Database Connection Pool Results:")
	t.Logf("=================================")
	t.Logf("Total queries: %d", totalQueries)
	t.Logf("Successful queries: %d", successfulQueries)
	t.Logf("Failed queries: %d", failedQueries)
	t.Logf("Total time: %v", totalTime)
	t.Logf("Average query time: %v", time.Duration(totalQueryTime/int64(totalQueries)))
	t.Logf("Throughput: %.2f queries/second", float64(successfulQueries)/totalTime.Seconds())

	// التحقق من النجاح
	if failedQueries > 0 {
		t.Errorf("Some queries failed: %d out of %d", failedQueries, totalQueries)
	}
}

// TestAPIResponseTime اختبار وقت استجابة API (مبسّط عبر استعلامات DB لعدم وجود واجهات مباشرة)
func TestAPIResponseTime(t *testing.T) {
	ctx := context.Background()
	db := testDB

	// تنظيف البيانات
	cleanupPerformanceTestData(t, db)
	defer cleanupPerformanceTestData(t, db)

	setupTestData(t, db)

	testCases := []struct {
		name        string
		operation   func() error
		maxDuration time.Duration
	}{
		{
			name: "List Auctions (DB)",
			operation: func() error {
				// محاكاة قراءة قائمة المزادات من DB مباشرة
				var n int
				return db.QueryRow(ctx, `SELECT COUNT(*) FROM auctions`).Scan(&n)
			},
			maxDuration: 100 * time.Millisecond,
		},
		{
			name: "Get Auction Details (DB)",
			operation: func() error {
				// احصل على آخر مزاد ثم اجلب تفاصيله عبر join مع products
				var pid int64
				if err := db.QueryRow(ctx, `SELECT product_id FROM auctions ORDER BY id DESC LIMIT 1`).Scan(&pid); err != nil {
					return err
				}
				var title string
				return db.QueryRow(ctx, `SELECT title FROM products WHERE id=$1`, pid).Scan(&title)
			},
			maxDuration: 50 * time.Millisecond,
		},
		{
			name: "Search Auctions (DB)",
			operation: func() error {
				var n int
				return db.QueryRow(ctx, `SELECT COUNT(*) FROM products p WHERE p.type='pigeon' AND p.title ILIKE '%Test%' AND p.id IN (SELECT product_id FROM auctions)`).Scan(&n)
			},
			maxDuration: 150 * time.Millisecond,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			numRuns := 10
			var totalDuration time.Duration
			var minDuration = time.Hour
			var maxDuration time.Duration
			for i := 0; i < numRuns; i++ {
				start := time.Now()
				err := tc.operation()
				duration := time.Since(start)
				if err != nil {
					t.Errorf("Operation failed: %v", err)
					continue
				}
				totalDuration += duration
				if duration < minDuration {
					minDuration = duration
				}
				if duration > maxDuration {
					maxDuration = duration
				}
				time.Sleep(10 * time.Millisecond)
			}
			avg := totalDuration / time.Duration(numRuns)
			if avg > tc.maxDuration {
				t.Errorf("%s: Average response time %v exceeds maximum %v", tc.name, avg, tc.maxDuration)
			}
		})
	}
}

// Helper types and functions

type testUser struct {
    ID    int64
    Email string
}

var perfPhoneCounter int64
var perfSlugCounter int64

func uniquePerfPhone() string {
	// Combine time, atomic counter and randomness to avoid collisions under parallel load
	rand.Seed(time.Now().UnixNano())
	n := time.Now().UnixNano()
	c := atomic.AddInt64(&perfPhoneCounter, 1)
	r := int64(rand.Intn(1_000_000))
	v := (n + c + r) % 100_000_000
	if v < 0 {
		v = -v
	}
	return fmt.Sprintf("+9665%08d", v)
}

func cleanupPerformanceTestData(t *testing.T, db *sqldb.Database) {
    ctx := context.Background()
    queries := []string{
        "DELETE FROM bids WHERE auction_id IN (SELECT id FROM auctions)",
        "DELETE FROM auctions",
        "DELETE FROM pigeons WHERE product_id IN (SELECT id FROM products WHERE title LIKE 'Performance Test%')",
        "DELETE FROM products WHERE title LIKE 'Performance Test%'",
        "DELETE FROM users WHERE email LIKE 'perf_%@example.com' OR email LIKE 'admin_%@example.com'",
    }
    for _, q := range queries {
        if _, err := db.Exec(ctx, q); err != nil {
            t.Logf("cleanup warning: %v", err)
        }
    }
}

func mustCreateProductPigeon(t *testing.T, db *sqldb.Database) int64 {
    ctx := context.Background()
    // Strongly unique slug for high-concurrency runs
    rand.Seed(time.Now().UnixNano())
    c := atomic.AddInt64(&perfSlugCounter, 1)
    slug := fmt.Sprintf("perf-pigeon-%d-%d-%06d", time.Now().UnixNano(), c, rand.Intn(1_000_000))
    var productID int64
    if err := db.QueryRow(ctx, `INSERT INTO products (type, title, slug, price_net, status) VALUES ('pigeon', 'Performance Test Pigeon', $1, 1000.00, 'available') RETURNING id`, slug).Scan(&productID); err != nil {
        t.Fatalf("Failed to create product: %v", err)
    }
    ring := fmt.Sprintf("PR-%d", time.Now().UnixNano())
	if _, err := db.Exec(ctx, `INSERT INTO pigeons (product_id, ring_number, sex) VALUES ($1, $2, 'male')`, productID, ring); err != nil {
		t.Fatalf("Failed to create pigeon: %v", err)
	}
	return productID
}

func createActiveAuction(t *testing.T, db *sqldb.Database) int64 {
	ctx := context.Background()

	productID := mustCreateProductPigeon(t, db)

	var auctionID int64
	err := db.QueryRow(ctx, `
		INSERT INTO auctions (
			product_id, start_price, bid_step, reserve_price, start_at, end_at, status, created_at, updated_at
		) VALUES (
			$1, 1000.00, 50, NULL, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '23 hours', 'live', NOW(), NOW()
		) RETURNING id
	`, productID).Scan(&auctionID)

	if err != nil {
		t.Fatalf("Failed to create active auction: %v", err)
	}

	return auctionID
}

func createVerifiedUser(t *testing.T, db *sqldb.Database, email string) int64 {
	ctx := context.Background()

	hashedPassword, err := authn.HashPassword("TestPass123!")
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	var userID int64
	err = db.QueryRow(ctx, `
		INSERT INTO users (
			name, email, password_hash, phone, city_id, role, state, 
			email_verified_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, 'verified', 'active', NOW(), NOW(), NOW()
		) RETURNING id
	`, "Perf User", email, hashedPassword, uniquePerfPhone(), 1).Scan(&userID)

	if err != nil {
		t.Fatalf("Failed to create verified user: %v", err)
	}

	return userID
}

func createAdminUser(t *testing.T, db *sqldb.Database, email string) int64 {
	ctx := context.Background()

	hashedPassword, err := authn.HashPassword("AdminPass123!")
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	var userID int64
	err = db.QueryRow(ctx, `
		INSERT INTO users (
			name, email, password_hash, phone, city_id, role, state, 
			email_verified_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, 'admin', 'active', NOW(), NOW(), NOW()
		) RETURNING id
	`, "Admin User", email, hashedPassword, uniquePerfPhone(), 1).Scan(&userID)

	if err != nil {
		t.Fatalf("Failed to create admin user: %v", err)
	}

	return userID
}

func setupTestData(t *testing.T, db *sqldb.Database) {
	ctx := context.Background()

	// إنشاء بعض المزادات للاختبار
	for i := 0; i < 20; i++ {
		prodID := mustCreateProductPigeon(t, db)
		_, err := db.Exec(ctx, `
			INSERT INTO auctions (
				product_id, start_price, bid_step, reserve_price, start_at, end_at, status, created_at, updated_at
			) VALUES (
				$1, $2, 50, NULL, $3, $4, 'scheduled', NOW(), NOW()
			)
		`, prodID, 1000.00+float64(i*100), time.Now().Add(time.Duration(i)*time.Hour), time.Now().Add(time.Duration(24+i)*time.Hour))

		if err != nil {
			t.Logf("Warning: failed to create test auction: %v", err)
		}
	}
}
