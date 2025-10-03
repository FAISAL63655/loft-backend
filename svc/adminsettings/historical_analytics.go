package adminsettings

import (
	"context"
	"time"
)

// HistoricalSalesData represents sales data for a specific period
type HistoricalSalesData struct {
	Period    string `json:"period"`    // "2024-01", "2024-02", etc.
	Sales     int    `json:"sales"`     // Number of completed orders
	Auctions  int    `json:"auctions"`  // Number of ended auctions
	Revenue   int64  `json:"revenue"`   // Revenue in halalas
	CreatedAt string `json:"created_at"`
}

// HistoricalSalesResponse represents the response for historical sales data
type HistoricalSalesResponse struct {
	Data   []HistoricalSalesData `json:"data"`
	Period string                `json:"period"`
	Total  struct {
		Sales    int   `json:"sales"`
		Auctions int   `json:"auctions"`
		Revenue  int64 `json:"revenue"`
	} `json:"total"`
}

// ProductCategoryStats represents product performance by category
type ProductCategoryStats struct {
	Category    string `json:"category"`
	Count       int    `json:"count"`
	Percentage  float64 `json:"percentage"`
	Revenue     int64   `json:"revenue"`
	LastUpdated string  `json:"last_updated"`
}

// ProductPerformanceResponse represents product performance analytics
type ProductPerformanceResponse struct {
	Data        []ProductCategoryStats `json:"data"`
	TotalItems  int                    `json:"total_items"`
	Period      string                 `json:"period"`
	LastUpdated string                 `json:"last_updated"`
}

// RevenueAnalyticsData represents revenue data over time
type RevenueAnalyticsData struct {
	Period         string `json:"period"`
	TotalRevenue   int64  `json:"total_revenue"`
	OrdersRevenue  int64  `json:"orders_revenue"`
	AuctionRevenue int64  `json:"auction_revenue"`
	RefundAmount   int64  `json:"refund_amount"`
	NetRevenue     int64  `json:"net_revenue"`
	CreatedAt      string `json:"created_at"`
}

// RevenueAnalyticsResponse represents revenue analytics response
type RevenueAnalyticsResponse struct {
	Data   []RevenueAnalyticsData `json:"data"`
	Period string                 `json:"period"`
	Growth struct {
		Percentage float64 `json:"percentage"`
		Amount     int64   `json:"amount"`
	} `json:"growth"`
	Summary struct {
		TotalRevenue int64 `json:"total_revenue"`
		NetRevenue   int64 `json:"net_revenue"`
		RefundRate   float64 `json:"refund_rate"`
	} `json:"summary"`
}

// GetHistoricalSales returns historical sales and auction data
//encore:api auth method=GET path=/admin/analytics/sales
func (s *Service) GetHistoricalSales(ctx context.Context) (*HistoricalSalesResponse, error) {
	// Get data for the last 6 months
	now := time.Now()
	var data []HistoricalSalesData
	var totalSales, totalAuctions int
	var totalRevenue int64

	for i := 5; i >= 0; i-- {
		monthStart := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, now.Location())
		monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Second)

		// Get sales count for this month
		var salesCount int64
		err := db.QueryRow(ctx, `
			SELECT COUNT(*) 
			FROM orders 
			WHERE status = 'paid' 
			AND created_at >= $1 AND created_at <= $2
		`, monthStart, monthEnd).Scan(&salesCount)
		if err != nil {
			salesCount = 0
		}

		// Get auctions count for this month
		var auctionsCount int64
		err = db.QueryRow(ctx, `
			SELECT COUNT(*) 
			FROM auctions 
			WHERE status = 'ended' 
			AND ended_at >= $1 AND ended_at <= $2
		`, monthStart, monthEnd).Scan(&auctionsCount)
		if err != nil {
			auctionsCount = 0
		}

		// Calculate revenue for this month
		var monthRevenue int64
		err = db.QueryRow(ctx, `
			SELECT COALESCE(SUM(total_amount_net), 0) 
			FROM orders 
			WHERE status = 'paid' 
			AND created_at >= $1 AND created_at <= $2
		`, monthStart, monthEnd).Scan(&monthRevenue)
		if err != nil {
			monthRevenue = 0
		}

		data = append(data, HistoricalSalesData{
			Period:    getArabicMonth(monthStart.Month()),
			Sales:     int(salesCount),
			Auctions:  int(auctionsCount),
			Revenue:   monthRevenue,
			CreatedAt: monthStart.Format(time.RFC3339),
		})

		totalSales += int(salesCount)
		totalAuctions += int(auctionsCount)
		totalRevenue += monthRevenue
	}

	return &HistoricalSalesResponse{
		Data:   data,
		Period: "آخر 6 أشهر",
		Total: struct {
			Sales    int   `json:"sales"`
			Auctions int   `json:"auctions"`
			Revenue  int64 `json:"revenue"`
		}{
			Sales:    totalSales,
			Auctions: totalAuctions,
			Revenue:  totalRevenue,
		},
	}, nil
}

// GetProductPerformance returns product performance analytics by category
//encore:api auth method=GET path=/admin/analytics/products
func (s *Service) GetProductPerformance(ctx context.Context) (*ProductPerformanceResponse, error) {
	var data []ProductCategoryStats
	var totalItems int

	// Get pigeons count and revenue
	var pigeonsCount int
	var pigeonsRevenue int64
	err := db.QueryRow(ctx, `
		SELECT COUNT(p.id), COALESCE(SUM(oi.price_net * oi.quantity), 0)
		FROM products p
		LEFT JOIN order_items oi ON p.id = oi.product_id
		LEFT JOIN orders o ON oi.order_id = o.id AND o.status = 'paid'
		WHERE p.type = 'pigeon'
	`).Scan(&pigeonsCount, &pigeonsRevenue)
	if err != nil {
		pigeonsCount = 0
		pigeonsRevenue = 0
	}

	// Get supplies count and revenue
	var suppliesCount int
	var suppliesRevenue int64
	err = db.QueryRow(ctx, `
		SELECT COUNT(p.id), COALESCE(SUM(oi.price_net * oi.quantity), 0)
		FROM products p
		LEFT JOIN order_items oi ON p.id = oi.product_id
		LEFT JOIN orders o ON oi.order_id = o.id AND o.status = 'paid'
		WHERE p.type = 'supply'
	`).Scan(&suppliesCount, &suppliesRevenue)
	if err != nil {
		suppliesCount = 0
		suppliesRevenue = 0
	}

	totalItems = pigeonsCount + suppliesCount

	if totalItems > 0 {
		data = append(data, ProductCategoryStats{
			Category:    "حمام",
			Count:       pigeonsCount,
			Percentage:  float64(pigeonsCount) / float64(totalItems) * 100,
			Revenue:     pigeonsRevenue,
			LastUpdated: time.Now().Format(time.RFC3339),
		})

		data = append(data, ProductCategoryStats{
			Category:    "مستلزمات",
			Count:       suppliesCount,
			Percentage:  float64(suppliesCount) / float64(totalItems) * 100,
			Revenue:     suppliesRevenue,
			LastUpdated: time.Now().Format(time.RFC3339),
		})
	}

	return &ProductPerformanceResponse{
		Data:        data,
		TotalItems:  totalItems,
		Period:      "إجمالي المنتجات",
		LastUpdated: time.Now().Format(time.RFC3339),
	}, nil
}

// GetRevenueAnalytics returns detailed revenue analytics over time
//encore:api auth method=GET path=/admin/analytics/revenue
func (s *Service) GetRevenueAnalytics(ctx context.Context) (*RevenueAnalyticsResponse, error) {
	now := time.Now()
	var data []RevenueAnalyticsData
	var totalRevenue, totalRefunds int64

	for i := 5; i >= 0; i-- {
		monthStart := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, now.Location())
		monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Second)
		
		_ = monthStart.Format("2006-01") // period for potential future use

		// Get orders revenue for this month
		var ordersRevenue int64
		err := db.QueryRow(ctx, `
			SELECT COALESCE(SUM(total_amount_net), 0) 
			FROM orders 
			WHERE status = 'paid' 
			AND created_at >= $1 AND created_at <= $2
		`, monthStart, monthEnd).Scan(&ordersRevenue)
		if err != nil {
			ordersRevenue = 0
		}

		// Get auction revenue for this month (winner payments)
		var auctionRevenue int64
		err = db.QueryRow(ctx, `
			SELECT COALESCE(SUM(o.total_amount_net), 0)
			FROM orders o
			JOIN auctions a ON o.auction_id = a.id
			WHERE o.status = 'paid' 
			AND a.end_time >= $1 AND a.end_time <= $2
		`, monthStart, monthEnd).Scan(&auctionRevenue)
		if err != nil {
			auctionRevenue = 0
		}

		// Get refunds for this month
		var refundAmount int64
		err = db.QueryRow(ctx, `
			SELECT COALESCE(SUM(total_amount_net), 0) 
			FROM orders 
			WHERE status = 'refunded' 
			AND updated_at >= $1 AND updated_at <= $2
		`, monthStart, monthEnd).Scan(&refundAmount)
		if err != nil {
			refundAmount = 0
		}

		monthTotalRevenue := ordersRevenue + auctionRevenue
		monthNetRevenue := monthTotalRevenue - refundAmount

		data = append(data, RevenueAnalyticsData{
			Period:         getArabicMonth(monthStart.Month()),
			TotalRevenue:   monthTotalRevenue,
			OrdersRevenue:  ordersRevenue,
			AuctionRevenue: auctionRevenue,
			RefundAmount:   refundAmount,
			NetRevenue:     monthNetRevenue,
			CreatedAt:      monthStart.Format(time.RFC3339),
		})

		totalRevenue += monthTotalRevenue
		totalRefunds += refundAmount
	}

	// Calculate growth
	var growthPercentage float64
	var growthAmount int64
	if len(data) >= 2 {
		firstMonth := data[0].NetRevenue
		lastMonth := data[len(data)-1].NetRevenue
		if firstMonth > 0 {
			growthPercentage = float64(lastMonth-firstMonth) / float64(firstMonth) * 100
			growthAmount = lastMonth - firstMonth
		}
	}

	// Calculate refund rate
	refundRate := float64(0)
	if totalRevenue > 0 {
		refundRate = float64(totalRefunds) / float64(totalRevenue) * 100
	}

	return &RevenueAnalyticsResponse{
		Data:   data,
		Period: "آخر 6 أشهر",
		Growth: struct {
			Percentage float64 `json:"percentage"`
			Amount     int64   `json:"amount"`
		}{
			Percentage: growthPercentage,
			Amount:     growthAmount,
		},
		Summary: struct {
			TotalRevenue int64   `json:"total_revenue"`
			NetRevenue   int64   `json:"net_revenue"`
			RefundRate   float64 `json:"refund_rate"`
		}{
			TotalRevenue: totalRevenue,
			NetRevenue:   totalRevenue - totalRefunds,
			RefundRate:   refundRate,
		},
	}, nil
}

// Helper function to get Arabic month names
func getArabicMonth(month time.Month) string {
	months := map[time.Month]string{
		time.January:   "يناير",
		time.February:  "فبراير",
		time.March:     "مارس",
		time.April:     "أبريل",
		time.May:       "مايو",
		time.June:      "يونيو",
		time.July:      "يوليو",
		time.August:    "أغسطس",
		time.September: "سبتمبر",
		time.October:   "أكتوبر",
		time.November:  "نوفمبر",
		time.December:  "ديسمبر",
	}
	return months[month]
}
