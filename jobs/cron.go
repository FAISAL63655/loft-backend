package jobs

import (
	"context"

	"net/http"

	"encore.app/coredb"
	"encore.app/pkg/audit"
	"encore.app/svc/auctions"
	"encore.app/svc/payments/worker"
	"encore.dev/cron"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//encore:service
type Service struct{}

func initService() (*Service, error) { return &Service{}, nil }

//encore:api private
func RunAuctionTick(ctx context.Context) error { return auctions.TickAuctions(ctx) }

var _ = cron.NewJob("auction-tick", cron.JobConfig{
	Title:    "Tick auctions (start/close)",
	Every:    1 * cron.Minute,
	Endpoint: RunAuctionTick,
})

// Reservation cleaner removed (no-reservation model)

//encore:api private
func RunPaymentInProgressCleaner(ctx context.Context) (*worker.CleanupResponse, error) {
	return worker.CleanupExpiredPaymentSessions(ctx)
}

var _ = cron.NewJob("payment-in-progress-cleaner", cron.JobConfig{
	Title:    "Cleanup stale payment_in_progress sessions",
	Every:    10 * cron.Minute,
	Endpoint: RunPaymentInProgressCleaner,
})

//encore:api private
func RunDailyAdminDigest(ctx context.Context) error {
	// Disabled by default via system setting key 'admin.digest.enabled' (string 'true' to enable)
	var enabled bool
	_ = coredb.DB.QueryRow(ctx, `SELECT COALESCE(value,'false')='true' FROM system_settings WHERE key='admin.digest.enabled'`).Scan(&enabled)
	if !enabled {
		return nil
	}

	var pending48 int
	_ = coredb.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM orders
		WHERE status='pending_payment'
		  AND created_at <= (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') - INTERVAL '48 hours'`).Scan(&pending48)

	var auctionsEnded24 int
	_ = coredb.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM auctions
		WHERE status='ended'
		  AND end_at >= (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') - INTERVAL '24 hours'`).Scan(&auctionsEnded24)

	var newUsers24 int
	_ = coredb.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM users
		WHERE created_at >= (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') - INTERVAL '24 hours'`).Scan(&newUsers24)

	var failedInvoices24 int
	_ = coredb.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM invoices
		WHERE status='failed'
		  AND updated_at >= (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') - INTERVAL '24 hours'`).Scan(&failedInvoices24)

	meta := map[string]interface{}{
		"pending_payment_over_48h": pending48,
		"auctions_ended_24h":       auctionsEnded24,
		"new_users_24h":            newUsers24,
		"failed_invoices_24h":      failedInvoices24,
	}

	_, _ = audit.Log(ctx, coredb.DB, audit.Entry{
		Action:     "ADMIN.DIGEST",
		EntityType: "system",
		EntityID:   "daily_admin_digest",
		Meta:       meta,
	})

	return nil
}

var _ = cron.NewJob("daily-admin-digest", cron.JobConfig{
	Title:    "Daily admin digest (optional)",
	Every:    24 * cron.Hour,
	Endpoint: RunDailyAdminDigest,
})

//encore:api public raw method=GET path=/metrics
func Metrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

// ===== Admin endpoints (temporary) to manually trigger cron jobs locally =====

type RunAllCronResponse struct {
    AuctionTick         string                    `json:"auction_tick"`
    PaymentCleaner      string                    `json:"payment_cleaner"`
    DailyAdminDigest    string                    `json:"daily_admin_digest"`
    PaymentCleanerStats *worker.CleanupResponse   `json:"payment_cleaner_stats,omitempty"`
}

//encore:api public method=POST path=/admin/cron/run-all
func RunAllCronJobs(ctx context.Context) (*RunAllCronResponse, error) {
    out := &RunAllCronResponse{}

    if err := RunAuctionTick(ctx); err != nil {
        out.AuctionTick = err.Error()
    } else {
        out.AuctionTick = "ok"
    }

    if resp, err := RunPaymentInProgressCleaner(ctx); err != nil {
        out.PaymentCleaner = err.Error()
    } else {
        out.PaymentCleaner = "ok"
        out.PaymentCleanerStats = resp
    }

    if err := RunDailyAdminDigest(ctx); err != nil {
        out.DailyAdminDigest = err.Error()
    } else {
        out.DailyAdminDigest = "ok"
    }

    return out, nil
}

//encore:api public method=POST path=/admin/cron/auction-tick
func RunAuctionTickAdmin(ctx context.Context) error { return RunAuctionTick(ctx) }

//encore:api public method=POST path=/admin/cron/payment-cleaner
func RunPaymentCleanerAdmin(ctx context.Context) (*worker.CleanupResponse, error) { return RunPaymentInProgressCleaner(ctx) }

//encore:api public method=POST path=/admin/cron/daily-admin-digest
func RunDailyAdminDigestAdmin(ctx context.Context) error { return RunDailyAdminDigest(ctx) }
