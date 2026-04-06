package events

import (
	"context"
	"encoding/json"
	"time"

	"e-orderbook/internal/models"
	"e-orderbook/internal/store"

	"github.com/nats-io/nats.go"
)

const (
	SubjectOrderCreated   = "events.order.created"
	SubjectOrderCanceled  = "events.order.canceled"
	SubjectTradeExecuted  = "events.trade.executed"
	SubjectBookUpdated    = "events.book.updated"
	SubjectDLQ            = "events.dlq"
)

// Bus publishes domain events to NATS (optional) and persists audit rows (optional).
type Bus struct {
	nc    *nats.Conn
	store *store.Store
}

func New(nc *nats.Conn, st *store.Store) *Bus {
	return &Bus{nc: nc, store: st}
}

func (b *Bus) publishRetry(subject string, payload any) {
	if b.nc == nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	var last error
	for attempt := 0; attempt < 6; attempt++ {
		if err := b.nc.Publish(subject, data); err == nil {
			return
		}
		last = err
		time.Sleep(time.Duration(50*(1<<attempt)) * time.Millisecond)
	}
	if last != nil {
		dlq := map[string]any{"subject": subject, "error": last.Error(), "payload": json.RawMessage(data)}
		if raw, err := json.Marshal(dlq); err == nil {
			_ = b.nc.Publish(SubjectDLQ, raw)
		}
	}
}

func (b *Bus) persist(ctx context.Context, typ string, payload any) {
	if b.store == nil {
		return
	}
	_ = b.store.InsertEvent(ctx, typ, payload)
}

func (b *Bus) OrderCreated(ctx context.Context, o *models.Order) {
	p := map[string]any{
		"order_id": o.ID.String(),
		"symbol":   o.Symbol,
		"side":     string(o.Side),
		"type":     string(o.Type),
		"status":   string(o.Status),
	}
	b.persist(ctx, "OrderCreated", p)
	b.publishRetry(SubjectOrderCreated, p)
}

func (b *Bus) OrderCanceled(ctx context.Context, o *models.Order) {
	p := map[string]any{"order_id": o.ID.String(), "symbol": o.Symbol, "status": string(o.Status)}
	b.persist(ctx, "OrderCanceled", p)
	b.publishRetry(SubjectOrderCanceled, p)
}

func (b *Bus) TradeExecuted(ctx context.Context, t *models.Trade) {
	p := map[string]any{
		"trade_id":      t.ID.String(),
		"symbol":        t.Symbol,
		"price":         t.Price.String(),
		"quantity":      t.Quantity.String(),
		"buy_order_id":  t.BuyOrderID.String(),
		"sell_order_id": t.SellOrderID.String(),
	}
	b.persist(ctx, "TradeExecuted", p)
	b.publishRetry(SubjectTradeExecuted, p)
}

func (b *Bus) BookUpdated(ctx context.Context, symbol string, sequence uint64) {
	p := map[string]any{"symbol": symbol, "sequence": sequence}
	b.persist(ctx, "BookUpdated", p)
	b.publishRetry(SubjectBookUpdated, p)
}
