package cart

import (
	"context"
	"database/sql"
	"errors"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) GetVerifiedState(ctx context.Context, userID int64) (bool, error) {
	var verified bool
	err := r.db.QueryRowContext(ctx, `SELECT email_verified_at IS NOT NULL FROM users WHERE id=$1 AND state='active'`, userID).Scan(&verified)
	if err != nil {
		return false, err
	}
	return verified, nil
}

func (r *Repository) GetCart(ctx context.Context, userID int64) (pigeons []PigeonCartItem, supplies []SupplyCartItem, err error) {
    // Pigeons in cart (no reservations)
    rows, err := r.db.QueryContext(ctx, `
        SELECT p.id, p.title, p.price_net
        FROM cart_items ci
        JOIN products p ON p.id = ci.product_id
        WHERE ci.user_id = $1 AND p.type='pigeon'
        ORDER BY p.id DESC
    `, userID)
    if err != nil {
        return nil, nil, err
    }
    defer rows.Close()
    for rows.Next() {
        var it PigeonCartItem
        if err := rows.Scan(&it.ProductID, &it.Title, &it.PriceNet); err != nil {
            return nil, nil, err
        }
        // No reservation now; leave ReservedExpiresAt empty
        pigeons = append(pigeons, it)
    }

    // Supplies in cart
    rows2, err := r.db.QueryContext(ctx, `
        SELECT ci.id, p.id, p.title, p.price_net, ci.qty
        FROM cart_items ci
        JOIN products p ON p.id = ci.product_id
        WHERE ci.user_id=$1 AND p.type='supply'
        ORDER BY ci.id DESC
    `, userID)
    if err != nil {
        return nil, nil, err
    }
    defer rows2.Close()
    for rows2.Next() {
        var it SupplyCartItem
        if err := rows2.Scan(&it.ReservationID, &it.ProductID, &it.Title, &it.PriceNet, &it.Qty); err != nil {
            return nil, nil, err
        }
        // ExpiresAt is not used in no-reservation model; leave empty
        supplies = append(supplies, it)
    }

    return pigeons, supplies, nil
}

func (r *Repository) GetProductTypeStatus(ctx context.Context, productID int64) (ptype, status string, err error) {
    err = r.db.QueryRowContext(ctx, `SELECT type::text, status::text FROM products WHERE id=$1`, productID).Scan(&ptype, &status)
    return
}

// Upsert or insert a cart item (no-reservation model)
func (r *Repository) UpsertCartItem(ctx context.Context, userID, productID int64, qty int) error {
    if qty <= 0 { qty = 1 }
    _, err := r.db.ExecContext(ctx, `
        INSERT INTO cart_items (user_id, product_id, qty)
        VALUES ($1,$2,$3)
        ON CONFLICT (user_id, product_id)
        DO UPDATE SET qty = EXCLUDED.qty
    `, userID, productID, qty)
    return err
}

// Update cart item quantity by cart item id
func (r *Repository) UpdateCartItemQty(ctx context.Context, itemID, userID int64, qty int) error {
    res, err := r.db.ExecContext(ctx, `UPDATE cart_items SET qty=$1 WHERE id=$2 AND user_id=$3`, qty, itemID, userID)
    if err != nil { return err }
    n, _ := res.RowsAffected()
    if n == 0 { return errors.New("cart_item_not_found") }
    return nil
}

// Delete cart item by id
func (r *Repository) DeleteCartItemByID(ctx context.Context, itemID, userID int64) error {
    res, err := r.db.ExecContext(ctx, `DELETE FROM cart_items WHERE id=$1 AND user_id=$2`, itemID, userID)
    if err != nil { return err }
    n, _ := res.RowsAffected()
    if n == 0 { return errors.New("cart_item_not_found") }
    return nil
}

// Get cart item product type for validation
func (r *Repository) GetCartItemProductType(ctx context.Context, itemID, userID int64) (ptype string, err error) {
    err = r.db.QueryRowContext(ctx, `
        SELECT p.type::text
        FROM cart_items ci
        JOIN products p ON p.id = ci.product_id
        WHERE ci.id=$1 AND ci.user_id=$2
    `, itemID, userID).Scan(&ptype)
    return
}

// IsReservationInvoiced removed in no-reservation model

// ReleasePigeonReservation releases pigeon reservation (optional future enhancement)
// Deprecated: reservation model removed. Use DeleteCartItemByID with cart item id.
func (r *Repository) ReleasePigeonReservation(ctx context.Context, userID, productID int64) (bool, error) {
    res, err := r.db.ExecContext(ctx, `DELETE FROM cart_items WHERE user_id=$1 AND product_id=$2`, userID, productID)
    if err != nil { return false, err }
    n, _ := res.RowsAffected()
    return n == 1, nil
}
