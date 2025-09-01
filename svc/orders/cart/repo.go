package cart

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
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
	// Pigeons reserved by this user
	rows, err := r.db.QueryContext(ctx, `
        SELECT p.id, p.title, p.price_net, p.reserved_expires_at
        FROM products p
        WHERE p.type='pigeon' AND p.status='reserved' AND p.reserved_by=$1 AND p.reserved_expires_at > (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
        ORDER BY p.id DESC
    `, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it PigeonCartItem
		var expiresAt sql.NullTime
		if err := rows.Scan(&it.ProductID, &it.Title, &it.PriceNet, &expiresAt); err != nil {
			return nil, nil, err
		}
		if expiresAt.Valid {
			it.ReservedExpiresAt = expiresAt.Time.UTC().Format(time.RFC3339)
		}
		pigeons = append(pigeons, it)
	}

	// Supplies reservations
	rows2, err := r.db.QueryContext(ctx, `
        SELECT sr.id, p.id, p.title, p.price_net, sr.qty, sr.expires_at
        FROM stock_reservations sr
        JOIN products p ON p.id=sr.product_id
        WHERE sr.user_id=$1 AND (sr.expires_at > (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') OR sr.invoice_id IS NOT NULL)
        ORDER BY sr.id DESC
    `, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var it SupplyCartItem
		var expiresAt time.Time
		if err := rows2.Scan(&it.ReservationID, &it.ProductID, &it.Title, &it.PriceNet, &it.Qty, &expiresAt); err != nil {
			return nil, nil, err
		}
		it.ExpiresAt = expiresAt.UTC().Format(time.RFC3339)
		supplies = append(supplies, it)
	}

	return pigeons, supplies, nil
}

func (r *Repository) CountActiveHolds(ctx context.Context, userID int64) (int, error) {
	var c int
	err := r.db.QueryRowContext(ctx, `
        SELECT (
            SELECT COUNT(*) FROM products WHERE type='pigeon' AND status='reserved' AND reserved_by=$1 AND reserved_expires_at>(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
        ) + (
            SELECT COUNT(*) FROM stock_reservations WHERE user_id=$1 AND (expires_at>(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') OR invoice_id IS NOT NULL)
        ) as total
    `, userID).Scan(&c)
	if err != nil {
		return 0, err
	}
	return c, nil
}

func (r *Repository) GetProductTypeStatus(ctx context.Context, productID int64) (ptype, status string, err error) {
	err = r.db.QueryRowContext(ctx, `SELECT type::text, status::text FROM products WHERE id=$1`, productID).Scan(&ptype, &status)
	return
}

func (r *Repository) ReservePigeon(ctx context.Context, userID, productID int64, holdMinutes int) (bool, error) {
	holdMinutesStr := strconv.Itoa(holdMinutes)
	res, err := r.db.ExecContext(ctx, `
        UPDATE products SET status='reserved', reserved_by=$1, reserved_expires_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') + ($3 || ' minutes')::interval
        WHERE id=$2 AND type='pigeon' AND status='available'
    `, userID, productID, holdMinutesStr)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (r *Repository) AddSupplyReservation(ctx context.Context, userID, productID int64, qty int, holdMinutes int) error {
	// Use a single transaction for advisory lock + insert
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashint8($1))`, productID); err != nil {
		return err
	}
	holdMinutesStr := strconv.Itoa(holdMinutes)
	if _, err = tx.ExecContext(ctx, `
        INSERT INTO stock_reservations (product_id, user_id, qty, expires_at)
        VALUES ($1,$2,$3, (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') + ($4 || ' minutes')::interval)
    `, productID, userID, qty, holdMinutesStr); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) UpdateSupplyReservationQty(ctx context.Context, resID, userID int64, qty int) error {
	// Ensure ownership and update but forbid invoiced reservations
	res, err := r.db.ExecContext(ctx, `
        UPDATE stock_reservations SET qty=$1 
        WHERE id=$2 AND user_id=$3 AND (expires_at>(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') AND invoice_id IS NULL)
    `, qty, resID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("reservation_not_found_or_locked")
	}
	return nil
}

func (r *Repository) DeleteSupplyReservation(ctx context.Context, resID, userID int64) error {
	// Forbid deleting invoiced reservations
	res, err := r.db.ExecContext(ctx, `DELETE FROM stock_reservations WHERE id=$1 AND user_id=$2 AND invoice_id IS NULL`, resID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("reservation_not_found_or_locked")
	}
	return nil
}

func (r *Repository) IsReservationInvoiced(ctx context.Context, resID, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM stock_reservations WHERE id=$1 AND user_id=$2 AND invoice_id IS NOT NULL)`, resID, userID).Scan(&exists)
	return exists, err
}

// ReleasePigeonReservation releases pigeon reservation (optional future enhancement)
func (r *Repository) ReleasePigeonReservation(ctx context.Context, userID, productID int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
        UPDATE products SET status='available', reserved_by=NULL, reserved_expires_at=NULL, updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
        WHERE id=$1 AND type='pigeon' AND status='reserved' AND reserved_by=$2
    `, productID, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}
