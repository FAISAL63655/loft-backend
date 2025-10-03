package cart

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strconv"
	"time"

	"encore.dev/beta/auth"
	"encore.dev/storage/sqldb"

	"encore.app/pkg/config"
	"encore.app/pkg/errs"
)

var db = sqldb.Named("coredb")

//encore:service
type Service struct{}

var svc *Service

func initService() (*Service, error) {
	// Initialize dynamic config with hot-reload (5 minutes)
	if config.GetGlobalManager() == nil {
		config.Initialize(db, 5*time.Minute)
	}
	// Ensure cart schema exists (dev convenience when migrations were already applied)
	ensureCartSchema()
	svc = &Service{}
	return svc, nil
}

// ensureCartSchema creates cart_items and its indexes if they don't exist.
// This is safe to run repeatedly and helps local dev when modifying existing migrations.
func ensureCartSchema() {
	std := db.Stdlib()
	// Create table
	_, _ = std.Exec(`
        CREATE TABLE IF NOT EXISTS cart_items (
            id BIGSERIAL PRIMARY KEY,
            user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
            qty INTEGER NOT NULL DEFAULT 1 CHECK (qty > 0),
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            UNIQUE (user_id, product_id)
        );
    `)
	// Indexes
	_, _ = std.Exec(`CREATE INDEX IF NOT EXISTS idx_cart_items_user_id ON cart_items(user_id);`)
	_, _ = std.Exec(`CREATE INDEX IF NOT EXISTS idx_cart_items_created_at ON cart_items(created_at DESC);`)
}

func getUserID(ctx context.Context) (int64, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return 0, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	uid, err := strconv.ParseInt(string(uidStr), 10, 64)
	if err != nil {
		return 0, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف مستخدم غير صالح"}
	}
	return uid, nil
}

func roundHalfUp(value float64, decimals int) float64 {
	mult := math.Pow(10, float64(decimals))
	if value >= 0 {
		return math.Floor(value*mult+0.5) / mult
	}
	return -math.Floor(-value*mult+0.5) / mult
}

func computeGross(net, vatRate float64) float64 {
	return roundHalfUp(net*(1+vatRate), 2)
}

//encore:api auth method=GET path=/cart
func GetCart(ctx context.Context) (*CartResponse, error) {
	if svc == nil {
		if _, err := initService(); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل التهيئة"}
		}
	}
	uid, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}
	repo := NewRepository(db.Stdlib())

	verified, err := repo.GetVerifiedState(ctx, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل التحقق من حالة التفعيل"}
	}
	if !verified {
		return &CartResponse{Pigeons: []PigeonCartItem{}, Supplies: []SupplyCartItem{}}, nil
	}

	p, s, err := repo.GetCart(ctx, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل جلب السلة"}
	}
	// Compute gross using VAT settings
	settings := config.GetSettings()
	vatRate := 0.0
	if settings != nil && settings.VATEnabled {
		vatRate = settings.VATRate
	}
	for i := range p {
		p[i].PriceGross = computeGross(p[i].PriceNet, vatRate)
	}
	for i := range s {
		s[i].PriceGross = computeGross(s[i].PriceNet, vatRate)
	}
	return &CartResponse{Pigeons: p, Supplies: s}, nil
}

//encore:api auth method=POST path=/cart
func AddToCart(ctx context.Context, req *AddToCartRequest) (*CartResponse, error) {
	if svc == nil {
		if _, err := initService(); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل التهيئة"}
		}
	}
	if req == nil || req.ProductID == 0 {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "بيانات غير مكتملة"}
	}

	uid, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}
	repo := NewRepository(db.Stdlib())

	verified, err := repo.GetVerifiedState(ctx, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل التحقق من حالة التفعيل"}
	}
	if !verified {
		return nil, &errs.Error{Code: errs.Forbidden, Message: "تتطلب العملية تفعيل البريد الإلكتروني"}
	}

	ptype, status, err := repo.GetProductTypeStatus(ctx, req.ProductID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &errs.Error{Code: errs.NotFound, Message: "المنتج غير موجود"}
		}
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل جلب نوع/حالة المنتج"}
	}

	switch ptype {
	case "pigeon":
		if status == "in_auction" || status == "auction_hold" || status == "sold" {
			return nil, &errs.Error{Code: errs.Conflict, Message: "لا يمكن إضافة الحمامة بحالتها الحالية"}
		}
		// في نموذج بدون حجز: نضيف إلى عناصر السلة مباشرة بكمية 1
		if err := repo.UpsertCartItem(ctx, uid, req.ProductID, 1); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل إضافة العنصر للسلة"}
		}

	case "supply":
		// رفض المستلزمات غير المتاحة: يجب أن تكون available حصراً
		if status != "available" {
			return nil, &errs.Error{Code: errs.Conflict, Message: "لا يمكن إضافة مستلزم غير متاح"}
		}
		qty := req.Qty
		if qty <= 0 {
			qty = 1
		}
		if err := repo.UpsertCartItem(ctx, uid, req.ProductID, qty); err != nil {
			return nil, &errs.Error{Code: errs.Conflict, Message: "فشل إضافة المستلزم للسلة"}
		}

	default:
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "نوع منتج غير مدعوم"}
	}

	p, s, err := repo.GetCart(ctx, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل تحديث السلة"}
	}
	// Compute gross
	settings := config.GetSettings()
	vatRate := 0.0
	if settings != nil && settings.VATEnabled {
		vatRate = settings.VATRate
	}
	for i := range p {
		p[i].PriceGross = computeGross(p[i].PriceNet, vatRate)
	}
	for i := range s {
		s[i].PriceGross = computeGross(s[i].PriceNet, vatRate)
	}
	return &CartResponse{Pigeons: p, Supplies: s}, nil
}

//encore:api auth method=PATCH path=/cart/items/:id
func UpdateCartItem(ctx context.Context, id string, req *UpdateCartItemRequest) (*CartResponse, error) {
	if svc == nil {
		if _, err := initService(); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل التهيئة"}
		}
	}
	uid, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}
	repo := NewRepository(db.Stdlib())

	verified, err := repo.GetVerifiedState(ctx, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل التحقق من حالة التفعيل"}
	}
	if !verified {
		return nil, &errs.Error{Code: errs.Forbidden, Message: "تتطلب العملية تفعيل البريد الإلكتروني"}
	}

	if req == nil || req.Qty <= 0 {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "كمية غير صالحة"}
	}

	resID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف غير صالح"}
	}

	if err := repo.UpdateCartItemQty(ctx, resID, uid, req.Qty); err != nil {
		return nil, &errs.Error{Code: errs.Conflict, Message: "فشل تحديث الكمية"}
	}

	p, s, err := repo.GetCart(ctx, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل تحديث السلة"}
	}
	settings := config.GetSettings()
	vatRate := 0.0
	if settings != nil && settings.VATEnabled {
		vatRate = settings.VATRate
	}
	for i := range p {
		p[i].PriceGross = computeGross(p[i].PriceNet, vatRate)
	}
	for i := range s {
		s[i].PriceGross = computeGross(s[i].PriceNet, vatRate)
	}
	return &CartResponse{Pigeons: p, Supplies: s}, nil
}

//encore:api auth method=DELETE path=/cart/items/:id
func DeleteCartItem(ctx context.Context, id string) (*CartResponse, error) {
	if svc == nil {
		if _, err := initService(); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل التهيئة"}
		}
	}
	uid, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}
	repo := NewRepository(db.Stdlib())

	verified, err := repo.GetVerifiedState(ctx, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل التحقق من حالة التفعيل"}
	}
	if !verified {
		return nil, &errs.Error{Code: errs.Forbidden, Message: "تتطلب العملية تفعيل البريد الإلكتروني"}
	}

	resID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف غير صالح"}
	}

	// احذف عنصر السلة بالمعرف مباشرة؛ وإن لم يوجد، جرّب اعتباره product_id (توافقًا للخلف)
	if err := repo.DeleteCartItemByID(ctx, resID, uid); err != nil {
		// fallback: delete by (user_id, product_id)
		_, relErr := repo.ReleasePigeonReservation(ctx, uid, resID)
		if relErr != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل حذف عنصر السلة"}
		}
	}

	p, s, err := repo.GetCart(ctx, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل تحديث السلة"}
	}
	settings := config.GetSettings()
	vatRate := 0.0
	if settings != nil && settings.VATEnabled {
		vatRate = settings.VATRate
	}
	for i := range p {
		p[i].PriceGross = computeGross(p[i].PriceNet, vatRate)
	}
	for i := range s {
		s[i].PriceGross = computeGross(s[i].PriceNet, vatRate)
	}
	return &CartResponse{Pigeons: p, Supplies: s}, nil
}

//encore:api auth method=POST path=/cart/merge
func MergeCart(ctx context.Context, req *MergeCartRequest) (*MergeCartResponse, error) {
	if svc == nil {
		if _, err := initService(); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل التهيئة"}
		}
	}

	uid, err := getUserID(ctx)
	if err != nil {
		return nil, err
	}
	repo := NewRepository(db.Stdlib())

	verified, err := repo.GetVerifiedState(ctx, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل التحقق من حالة التفعيل"}
	}
	if !verified {
		return nil, &errs.Error{Code: errs.Forbidden, Message: "تتطلب العملية تفعيل البريد الإلكتروني"}
	}

	// تحضير نتائج الدمج
	response := &MergeCartResponse{}

	// معالجة الحمام المحلي
	for _, localPigeon := range req.LocalPigeons {
		ptype, status, err := repo.GetProductTypeStatus(ctx, localPigeon.ProductID)
		if err != nil {
			response.MergeResults.FailedPigeons = append(response.MergeResults.FailedPigeons, struct {
				ProductID int64  `json:"product_id"`
				Reason    string `json:"reason"`
			}{ProductID: localPigeon.ProductID, Reason: "المنتج غير موجود"})
			continue
		}

		if ptype != "pigeon" {
			response.MergeResults.FailedPigeons = append(response.MergeResults.FailedPigeons, struct {
				ProductID int64  `json:"product_id"`
				Reason    string `json:"reason"`
			}{ProductID: localPigeon.ProductID, Reason: "ليس منتج حمامة"})
			continue
		}

		if status == "in_auction" || status == "auction_hold" || status == "sold" {
			response.MergeResults.FailedPigeons = append(response.MergeResults.FailedPigeons, struct {
				ProductID int64  `json:"product_id"`
				Reason    string `json:"reason"`
			}{ProductID: localPigeon.ProductID, Reason: "الحمامة غير متاحة"})
			continue
		}

		if err := repo.UpsertCartItem(ctx, uid, localPigeon.ProductID, 1); err != nil {
			response.MergeResults.FailedPigeons = append(response.MergeResults.FailedPigeons, struct {
				ProductID int64  `json:"product_id"`
				Reason    string `json:"reason"`
			}{ProductID: localPigeon.ProductID, Reason: "فشل في إضافة الحمامة"})
			continue
		}

		response.MergeResults.SuccessfulPigeons = append(response.MergeResults.SuccessfulPigeons, localPigeon.ProductID)
	}

	// معالجة المستلزمات المحلية
	for _, localSupply := range req.LocalSupplies {
		ptype, status, err := repo.GetProductTypeStatus(ctx, localSupply.ProductID)
		if err != nil {
			response.MergeResults.FailedSupplies = append(response.MergeResults.FailedSupplies, struct {
				ProductID int64  `json:"product_id"`
				Reason    string `json:"reason"`
			}{ProductID: localSupply.ProductID, Reason: "المنتج غير موجود"})
			continue
		}

		if ptype != "supply" {
			response.MergeResults.FailedSupplies = append(response.MergeResults.FailedSupplies, struct {
				ProductID int64  `json:"product_id"`
				Reason    string `json:"reason"`
			}{ProductID: localSupply.ProductID, Reason: "ليس منتج مستلزم"})
			continue
		}

		if status != "available" {
			response.MergeResults.FailedSupplies = append(response.MergeResults.FailedSupplies, struct {
				ProductID int64  `json:"product_id"`
				Reason    string `json:"reason"`
			}{ProductID: localSupply.ProductID, Reason: "المستلزم غير متاح"})
			continue
		}

		qty := localSupply.Qty
		if qty <= 0 {
			qty = 1
		}

		if err := repo.UpsertCartItem(ctx, uid, localSupply.ProductID, qty); err != nil {
			response.MergeResults.FailedSupplies = append(response.MergeResults.FailedSupplies, struct {
				ProductID int64  `json:"product_id"`
				Reason    string `json:"reason"`
			}{ProductID: localSupply.ProductID, Reason: "فشل في إضافة المستلزم"})
			continue
		}

		response.MergeResults.SuccessfulSupplies = append(response.MergeResults.SuccessfulSupplies, localSupply.ProductID)
	}

	// جلب السلة المحدثة
	p, s, err := repo.GetCart(ctx, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل جلب السلة المحدثة"}
	}

	// حساب الأسعار مع الضريبة
	settings := config.GetSettings()
	vatRate := 0.0
	if settings != nil && settings.VATEnabled {
		vatRate = settings.VATRate
	}
	for i := range p {
		p[i].PriceGross = computeGross(p[i].PriceNet, vatRate)
	}
	for i := range s {
		s[i].PriceGross = computeGross(s[i].PriceNet, vatRate)
	}

	response.CartResponse = CartResponse{Pigeons: p, Supplies: s}
	return response, nil
}
