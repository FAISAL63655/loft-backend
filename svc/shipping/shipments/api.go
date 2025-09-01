package shipments

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"encore.dev/beta/auth"
	"encore.dev/storage/sqldb"

	"encore.app/pkg/errs"
)

var db = sqldb.Named("coredb")

//encore:service
type Service struct{}

func initService() (*Service, error) { return &Service{}, nil }

// Shipment represents a shipment record.
type Shipment struct {
	ID             int64           `json:"id"`
	OrderID        int64           `json:"order_id"`
	CompanyID      *int64          `json:"company_id,omitempty"`
	DeliveryMethod string          `json:"delivery_method"`
	Status         string          `json:"status"`
	TrackingRef    *string         `json:"tracking_ref,omitempty"`
	Events         json.RawMessage `json:"events"`
}

// ListShipmentsQuery represents query params for listing shipments by order id.
type ListShipmentsQuery struct {
	OrderID int64 `query:"order_id"`
}

// ListShipmentsResponse represents a list of shipments.
type ListShipmentsResponse struct {
	Items []Shipment `json:"items"`
}

// GetShipment returns a shipment by id (owner or admin).
//
//encore:api auth method=GET path=/shipments/:id
func (s *Service) GetShipment(ctx context.Context, id string) (*Shipment, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	u64, err := strconv.ParseInt(string(uidStr), 10, 64)
	if err != nil {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف مستخدم غير صالح"}
	}
	uid := u64

	var sid int64
	if v, err := strconv.ParseInt(id, 10, 64); err == nil {
		sid = v
	} else {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف غير صالح"}
	}

	var ownerID int64
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT o.user_id FROM shipments s JOIN orders o ON o.id=s.order_id WHERE s.id=$1`, sid).Scan(&ownerID); err != nil {
		return nil, &errs.Error{Code: errs.ShpNotFound, Message: "الشحنة غير موجودة"}
	}
	if ownerID != uid {
		var role string
		if err := db.Stdlib().QueryRowContext(ctx, `SELECT role::text FROM users WHERE id=$1`, uid).Scan(&role); err != nil {
			return nil, &errs.Error{Code: errs.Unauthenticated, Message: "فشل التحقق من الدور"}
		}
		if strings.ToLower(role) != "admin" {
			return nil, &errs.Error{Code: errs.Forbidden, Message: "غير مصرح"}
		}
	}

	var shp Shipment
	if err := db.Stdlib().QueryRowContext(ctx, `
		SELECT id, order_id, company_id, delivery_method::text, status::text, tracking_ref, events
		FROM shipments WHERE id=$1
	`, sid).Scan(&shp.ID, &shp.OrderID, &shp.CompanyID, &shp.DeliveryMethod, &shp.Status, &shp.TrackingRef, &shp.Events); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل قراءة الشحنة"}
	}
	return &shp, nil
}

// ListShipmentsByOrder lists shipments for a given order (owner or admin).
//
//encore:api auth method=GET path=/shipments
func (s *Service) ListShipmentsByOrder(ctx context.Context, q *ListShipmentsQuery) (*ListShipmentsResponse, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	u64, err := strconv.ParseInt(string(uidStr), 10, 64)
	if err != nil {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف مستخدم غير صالح"}
	}
	uid := u64

	if q == nil || q.OrderID == 0 {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "order_id مطلوب"}
	}
	var ownerID int64
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT user_id FROM orders WHERE id=$1`, q.OrderID).Scan(&ownerID); err != nil {
		return nil, &errs.Error{Code: "ORD_NOT_FOUND", Message: "الطلب غير موجود"}
	}
	if ownerID != uid {
		var role string
		if err := db.Stdlib().QueryRowContext(ctx, `SELECT role::text FROM users WHERE id=$1`, uid).Scan(&role); err != nil {
			return nil, &errs.Error{Code: errs.Unauthenticated, Message: "فشل التحقق من الدور"}
		}
		if strings.ToLower(role) != "admin" {
			return nil, &errs.Error{Code: errs.Forbidden, Message: "غير مصرح"}
		}
	}

	rows, err := db.Stdlib().QueryContext(ctx, `
		SELECT id, order_id, company_id, delivery_method::text, status::text, tracking_ref, events
		FROM shipments WHERE order_id=$1 ORDER BY created_at DESC
	`, q.OrderID)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل الاستعلام عن الشحنات"}
	}
	defer rows.Close()
	var items []Shipment
	for rows.Next() {
		var s Shipment
		if err := rows.Scan(&s.ID, &s.OrderID, &s.CompanyID, &s.DeliveryMethod, &s.Status, &s.TrackingRef, &s.Events); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل قراءة الشحنة"}
		}
		items = append(items, s)
	}
	return &ListShipmentsResponse{Items: items}, nil
}

// UpdateShipmentRequest updates shipment status and optionally tracking reference, appending an event.
type UpdateShipmentRequest struct {
	Status      *string `json:"status,omitempty"`
	TrackingRef *string `json:"tracking_ref,omitempty"`
	EventNote   *string `json:"event,omitempty"`
}

// UpdateShipment updates shipment fields and appends an event (Admin only).
//
//encore:api auth method=PATCH path=/shipments/:id
func (s *Service) UpdateShipment(ctx context.Context, id string, req *UpdateShipmentRequest) (*Shipment, error) {
	// Admin guard
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	u64, err := strconv.ParseInt(string(uidStr), 10, 64)
	if err != nil {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف مستخدم غير صالح"}
	}
	uid := u64
	var role string
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT role::text FROM users WHERE id=$1`, uid).Scan(&role); err != nil {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "فشل التحقق من الدور"}
	}
	if strings.ToLower(role) != "admin" {
		return nil, &errs.Error{Code: errs.Forbidden, Message: "صلاحية مدير مطلوبة"}
	}

	var sid int64
	if v, err := strconv.ParseInt(id, 10, 64); err == nil {
		sid = v
	} else {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف غير صالح"}
	}

	// Load current status for potential change
	var currentStatus string
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT status::text FROM shipments WHERE id=$1`, sid).Scan(&currentStatus); err != nil {
		return nil, &errs.Error{Code: errs.ShpNotFound, Message: "الشحنة غير موجودة"}
	}
	// Ensure related order is paid (fast-fail; DB trigger also enforces)
	var ordStatus string
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT o.status::text FROM shipments s JOIN orders o ON o.id=s.order_id WHERE s.id=$1`, sid).Scan(&ordStatus); err != nil {
		return nil, &errs.Error{Code: "ORD_NOT_FOUND", Message: "الطلب غير موجود"}
	}
	if ordStatus != "paid" {
		return nil, &errs.Error{Code: errs.ShpOrderNotPaid, Message: "لا يمكن تعديل الشحنة إلا لطلب مدفوع"}
	}

	// If status change requested, validate and append event
	if req != nil && req.Status != nil {
		st := strings.ToLower(*req.Status)
		switch st {
		case "pending", "processing", "shipped", "delivered", "failed", "returned":
			// ok
		default:
			return nil, &errs.Error{Code: errs.InvalidArgument, Message: "قيمة حالة شحنة غير صالحة"}
		}
		if _, err := db.Stdlib().ExecContext(ctx, `SELECT add_shipment_event($1, $2::shipment_status, $3)`, sid, st, req.EventNote); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل تحديث حالة الشحنة"}
		}
	}
	// Update tracking reference if provided
	if req != nil && req.TrackingRef != nil {
		if _, err := db.Stdlib().ExecContext(ctx, `UPDATE shipments SET tracking_ref=$1 WHERE id=$2`, *req.TrackingRef, sid); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل تحديث رقم التتبع"}
		}
	}

	var shp Shipment
	if err := db.Stdlib().QueryRowContext(ctx, `
		SELECT id, order_id, company_id, delivery_method::text, status::text, tracking_ref, events
		FROM shipments WHERE id=$1
	`, sid).Scan(&shp.ID, &shp.OrderID, &shp.CompanyID, &shp.DeliveryMethod, &shp.Status, &shp.TrackingRef, &shp.Events); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل قراءة الشحنة"}
	}
	return &shp, nil
}
