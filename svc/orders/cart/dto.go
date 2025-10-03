package cart

// طلب إضافة للسلة
type AddToCartRequest struct {
	ProductID int64 `json:"product_id"`
	Qty       int   `json:"qty"` // للمستلزمات فقط؛ للحمام دائماً 1
}

// طلب تحديث عنصر سلة (للمستلزمات فقط)
type UpdateCartItemRequest struct {
	Qty int `json:"qty"`
}

// عنصر حمام في السلة
type PigeonCartItem struct {
	ProductID         int64   `json:"product_id"`
	Title             string  `json:"title"`
	PriceNet          float64 `json:"price_net"`
	PriceGross        float64 `json:"price_gross"`
	ReservedExpiresAt string  `json:"reserved_expires_at"` // ISO8601
}

// عنصر مستلزم في السلة
type SupplyCartItem struct {
	ReservationID int64   `json:"reservation_id"`
	ProductID     int64   `json:"product_id"`
	Title         string  `json:"title"`
	PriceNet      float64 `json:"price_net"`
	PriceGross    float64 `json:"price_gross"`
	Qty           int     `json:"qty"`
	ExpiresAt     string  `json:"expires_at"` // ISO8601
}

// عنصر سلة محلي للحمام (من العميل)
type LocalPigeonItem struct {
	ProductID int64 `json:"product_id"`
}

// عنصر سلة محلي للمستلزمات (من العميل)
type LocalSupplyItem struct {
	ProductID int64 `json:"product_id"`
	Qty       int   `json:"qty"`
}

// طلب دمج السلة المحلية مع سلة السيرفر
type MergeCartRequest struct {
	LocalPigeons  []LocalPigeonItem `json:"local_pigeons"`
	LocalSupplies []LocalSupplyItem `json:"local_supplies"`
}

// استجابة دمج السلة
type MergeCartResponse struct {
	CartResponse
	MergeResults struct {
		SuccessfulPigeons  []int64 `json:"successful_pigeons"`
		SuccessfulSupplies []int64 `json:"successful_supplies"`
		FailedPigeons      []struct {
			ProductID int64  `json:"product_id"`
			Reason    string `json:"reason"`
		} `json:"failed_pigeons"`
		FailedSupplies []struct {
			ProductID int64  `json:"product_id"`
			Reason    string `json:"reason"`
		} `json:"failed_supplies"`
	} `json:"merge_results"`
}

// الاستجابة العامة للسلة
type CartResponse struct {
	Pigeons  []PigeonCartItem `json:"pigeons"`
	Supplies []SupplyCartItem `json:"supplies"`
}
