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
	svc = &Service{}
	return svc, nil
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

	// قراءة إعدادات المهلة من system settings
	settings := config.GetSettings()
	pigeonHold := settings.StockCheckoutHoldMinutes
	supplyHold := settings.StockSuppliesHoldMinutes
	maxHolds := settings.StockMaxActiveHoldsPerUser
	if pigeonHold <= 0 {
		pigeonHold = 10
	}
	if supplyHold <= 0 {
		supplyHold = 15
	}
	if maxHolds <= 0 {
		maxHolds = 5
	}

	active, err := repo.CountActiveHolds(ctx, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل التحقق من حدود الحجز"}
	}
	if active >= maxHolds {
		return nil, &errs.Error{Code: errs.Conflict, Message: "تم بلوغ الحد الأقصى للحجوزات النشطة"}
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
		if status == "in_auction" || status == "auction_hold" || status == "sold" || status == "reserved" || status == "payment_in_progress" {
			return nil, &errs.Error{Code: errs.Conflict, Message: "لا يمكن إضافة الحمامة بحالتها الحالية"}
		}
		ok, err := repo.ReservePigeon(ctx, uid, req.ProductID, pigeonHold)
		if err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل حجز الحمامة"}
		}
		if !ok {
			return nil, &errs.Error{Code: errs.Conflict, Message: "تعذر حجز الحمامة (قد لا تكون متاحة)"}
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
		if err := repo.AddSupplyReservation(ctx, uid, req.ProductID, qty, supplyHold); err != nil {
			return nil, &errs.Error{Code: errs.Conflict, Message: "فشل حجز المستلزم"}
		}

	default:
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "نوع منتج غير مدعوم"}
	}

	p, s, err := repo.GetCart(ctx, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل تحديث السلة"}
	}
	// Compute gross
	settings = config.GetSettings()
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

	resID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف غير صالح"}
	}
	if req == nil || req.Qty <= 0 {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "كمية غير صالحة"}
	}

	// منع تعديل حجوزات مرتبطة بفاتورة
	locked, err := repo.IsReservationInvoiced(ctx, resID, uid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل التحقق من حالة الحجز"}
	}
	if locked {
		return nil, &errs.Error{Code: errs.Conflict, Message: "لا يمكن تعديل حجز مرتبط بفاتورة"}
	}

	if err := repo.UpdateSupplyReservationQty(ctx, resID, uid, req.Qty); err != nil {
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

	// حاول حذف مستلزم أولاً (مع منع المحجوز بفاتورة)
	locked, err := repo.IsReservationInvoiced(ctx, resID, uid)
	if err == nil && locked {
		return nil, &errs.Error{Code: errs.Conflict, Message: "لا يمكن حذف حجز مرتبط بفاتورة"}
	}
	if err == nil {
		if err := repo.DeleteSupplyReservation(ctx, resID, uid); err == nil {
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
	}

	// إن لم تكن مستلزم صالح، افترض أنه معرف منتج حمامة وحرر الحجز
	ok, relErr := repo.ReleasePigeonReservation(ctx, uid, resID)
	if relErr != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل تحرير حجز الحمامة"}
	}
	if !ok {
		return nil, &errs.Error{Code: errs.NotFound, Message: "العنصر غير موجود"}
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

// نتيجة تنظيف الحجوزات
type CleanupResponse struct {
	Deleted int `json:"deleted"`
}

//encore:api private
func CleanupExpiredReservations(ctx context.Context) (*CleanupResponse, error) {
	var n int
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT cleanup_expired_reservations()`).Scan(&n); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل تنظيف الحجوزات"}
	}
	// Release expired pigeon reservations
	res, err := db.Stdlib().ExecContext(ctx, `
		UPDATE products
		SET status='available', reserved_by=NULL, reserved_expires_at=NULL, updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		WHERE type='pigeon' AND status='reserved' AND reserved_expires_at <= (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
	`)
	if err == nil {
		if rows, e := res.RowsAffected(); e == nil {
			n += int(rows)
		}
	}
	return &CleanupResponse{Deleted: n}, nil
}
