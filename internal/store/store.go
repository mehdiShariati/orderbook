package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"e-orderbook/internal/models"
	"e-orderbook/internal/observability"

	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/shopspring/decimal"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func (s *Store) Close() { s.pool.Close() }

func observeDB(op string, start time.Time) {
	observability.DBQueryLatency.WithLabelValues(op).Observe(time.Since(start).Seconds())
}

func (s *Store) Ping(ctx context.Context) error {
	t0 := time.Now()
	defer observeDB("ping", t0)
	return s.pool.Ping(ctx)
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func parseDecimal(s string) (decimal.Decimal, error) {
	return decimal.NewFromString(s)
}

func ptrDecimalString(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	v := d.String()
	return &v
}

func scanOrder(row pgx.Row) (*models.Order, error) {
	var (
		id               uuid.UUID
		userID           string
		symbol           string
		side             string
		typ              string
		priceStr         *string
		qtyStr           string
		remainingStr     string
		status           string
		idempotencyKey   *string
		createdAt        time.Time
		updatedAt        time.Time
	)
	if err := row.Scan(&id, &userID, &symbol, &side, &typ, &priceStr, &qtyStr, &remainingStr, &status, &idempotencyKey, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	qty, err := parseDecimal(qtyStr)
	if err != nil {
		return nil, fmt.Errorf("quantity: %w", err)
	}
	rem, err := parseDecimal(remainingStr)
	if err != nil {
		return nil, fmt.Errorf("remaining_quantity: %w", err)
	}
	var price *decimal.Decimal
	if priceStr != nil && *priceStr != "" {
		p, err := parseDecimal(*priceStr)
		if err != nil {
			return nil, fmt.Errorf("price: %w", err)
		}
		price = &p
	}
	return &models.Order{
		ID:               id,
		UserID:           userID,
		Symbol:           symbol,
		Side:             models.OrderSide(side),
		Type:             models.OrderType(typ),
		Price:            price,
		Quantity:         qty,
		Remaining:        rem,
		Status:           models.OrderStatus(status),
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		TimePrioritySeq:  0,
	}, nil
}

func (s *Store) GetOrder(ctx context.Context, id uuid.UUID) (*models.Order, error) {
	t0 := time.Now()
	defer observeDB("get_order", t0)
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, symbol, side, type, price, quantity, remaining_quantity, status, idempotency_key, created_at, updated_at
		FROM orders WHERE id = $1`, id)
	o, err := scanOrder(row)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (s *Store) GetOrderByIdempotencyKey(ctx context.Context, key string) (*models.Order, error) {
	t0 := time.Now()
	defer observeDB("get_order_idempotency", t0)
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, symbol, side, type, price, quantity, remaining_quantity, status, idempotency_key, created_at, updated_at
		FROM orders WHERE idempotency_key = $1`, key)
	o, err := scanOrder(row)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (s *Store) InsertOrderTx(ctx context.Context, tx pgx.Tx, o *models.Order, idempotencyKey *string) error {
	t0 := time.Now()
	defer observeDB("insert_order_tx", t0)
	price := ptrDecimalString(o.Price)
	_, err := tx.Exec(ctx, `
		INSERT INTO orders (id, user_id, symbol, side, type, price, quantity, remaining_quantity, status, idempotency_key, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		o.ID, o.UserID, o.Symbol, string(o.Side), string(o.Type), price,
		o.Quantity.String(), o.Remaining.String(), string(o.Status), idempotencyKey,
		o.CreatedAt, o.UpdatedAt,
	)
	return err
}

func (s *Store) UpdateOrderTx(ctx context.Context, tx pgx.Tx, o *models.Order) error {
	t0 := time.Now()
	defer observeDB("update_order_tx", t0)
	_, err := tx.Exec(ctx, `
		UPDATE orders SET remaining_quantity = $2, status = $3, updated_at = $4 WHERE id = $1`,
		o.ID, o.Remaining.String(), string(o.Status), o.UpdatedAt,
	)
	return err
}

func (s *Store) InsertTradesTx(ctx context.Context, tx pgx.Tx, trades []models.Trade) error {
	t0 := time.Now()
	defer observeDB("insert_trades_tx", t0)
	for _, t := range trades {
		_, err := tx.Exec(ctx, `
			INSERT INTO trades (id, symbol, price, quantity, buy_order_id, sell_order_id, executed_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			t.ID, t.Symbol, t.Price.String(), t.Quantity.String(), t.BuyOrderID, t.SellOrderID, t.ExecutedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) MarkCancelled(ctx context.Context, id uuid.UUID, now time.Time) error {
	t0 := time.Now()
	defer observeDB("mark_cancelled", t0)
	ct, err := s.pool.Exec(ctx, `
		UPDATE orders SET status = $2, updated_at = $3
		WHERE id = $1 AND status IN ('open', 'partial_filled')`,
		id, string(models.StatusCancelled), now,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateOrderFromModel(ctx context.Context, o *models.Order) error {
	t0 := time.Now()
	defer observeDB("update_order", t0)
	_, err := s.pool.Exec(ctx, `
		UPDATE orders SET remaining_quantity = $2, status = $3, updated_at = $4 WHERE id = $1`,
		o.ID, o.Remaining.String(), string(o.Status), o.UpdatedAt,
	)
	return err
}

type TradeRow struct {
	ID          uuid.UUID `json:"id"`
	Symbol      string    `json:"symbol"`
	Price       string    `json:"price"`
	Quantity    string    `json:"quantity"`
	BuyOrderID  uuid.UUID `json:"buy_order_id"`
	SellOrderID uuid.UUID `json:"sell_order_id"`
	ExecutedAt  time.Time `json:"executed_at"`
}

func (s *Store) ListTradesBySymbol(ctx context.Context, symbol string, limit int) ([]TradeRow, error) {
	t0 := time.Now()
	defer observeDB("list_trades", t0)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, symbol, price, quantity, buy_order_id, sell_order_id, executed_at
		FROM trades WHERE symbol = $1 ORDER BY executed_at DESC LIMIT $2`,
		symbol, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TradeRow
	for rows.Next() {
		var tr TradeRow
		if err := rows.Scan(&tr.ID, &tr.Symbol, &tr.Price, &tr.Quantity, &tr.BuyOrderID, &tr.SellOrderID, &tr.ExecutedAt); err != nil {
			return nil, err
		}
		out = append(out, tr)
	}
	return out, rows.Err()
}
