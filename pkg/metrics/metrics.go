package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	BidsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bids_total",
			Help: "Total number of bids placed",
		},
		[]string{"auction_id"},
	)

	BidLatencySeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bid_latency_seconds",
			Help:    "Latency of bid placement operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"auction_id"},
	)

	PaymentInitTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payment_init_total",
			Help: "Total number of payment init calls",
		},
		[]string{"method"},
	)

	WebhookOutcomesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webhook_outcomes_total",
			Help: "Outcomes of payment webhooks",
		},
		[]string{"status"},
	)

	WSConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ws_connections",
			Help: "Current number of active WebSocket connections",
		},
	)
)

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDurationSeconds,
		BidsTotal,
		BidLatencySeconds,
		PaymentInitTotal,
		WebhookOutcomesTotal,
		WSConnections,
	)
}

// ObserveHTTPRequest records metrics for an HTTP request
func ObserveHTTPRequest(method, path, status string, startedAt time.Time) {
	HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	HTTPRequestDurationSeconds.WithLabelValues(method, path, status).Observe(time.Since(startedAt).Seconds())
}
