package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"e-orderbook/internal/events"
	"e-orderbook/internal/matching"
	"e-orderbook/internal/models"
	"e-orderbook/internal/observability"
	"e-orderbook/internal/store"
	"e-orderbook/internal/ws"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/nats-io/nats.go"
	"github.com/go-redis/redis/v8"
	"github.com/shopspring/decimal"
)

type Handler struct {
	store  *store.Store
	engine matching.Engine
	bus    *events.Bus
	hub    *ws.Hub
	redis  *redis.Client
	nats   *nats.Conn
}

func NewHandler(s *store.Store, e matching.Engine, bus *events.Bus, hub *ws.Hub, rdb *redis.Client, nc *nats.Conn) *Handler {
	return &Handler{store: s, engine: e, bus: bus, hub: hub, redis: rdb, nats: nc}
}

type createOrderRequest struct {
	UserID   string  `json:"user_id"`
	Symbol   string  `json:"symbol"`
	Side     string  `json:"side"`
	Type     string  `json:"type"`
	Price    *string `json:"price,omitempty"`
	Quantity string  `json:"quantity"`
}

type orderResponse struct {
	ID                 string  `json:"id"`
	UserID             string  `json:"user_id"`
	Symbol             string  `json:"symbol"`
	Side               string  `json:"side"`
	Type               string  `json:"type"`
	Price              *string `json:"price,omitempty"`
	Quantity           string  `json:"quantity"`
	RemainingQuantity  string  `json:"remaining_quantity"`
	Status             string  `json:"status"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

func orderToResp(o *models.Order) orderResponse {
	r := orderResponse{
		ID:                o.ID.String(),
		UserID:            o.UserID,
		Symbol:            o.Symbol,
		Side:              string(o.Side),
		Type:              string(o.Type),
		Quantity:          o.Quantity.String(),
		RemainingQuantity: o.Remaining.String(),
		Status:            string(o.Status),
		CreatedAt:         o.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:         o.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if o.Price != nil {
		p := o.Price.String()
		r.Price = &p
	}
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idemKey != "" {
		existing, err := h.store.GetOrderByIdempotencyKey(r.Context(), idemKey)
		if err == nil {
			writeJSON(w, http.StatusOK, orderToResp(existing))
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
	}

	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if symbol == "" || strings.TrimSpace(req.UserID) == "" {
		writeError(w, http.StatusBadRequest, "user_id and symbol are required")
		return
	}

	side := models.OrderSide(strings.ToLower(strings.TrimSpace(req.Side)))
	if side != models.SideBuy && side != models.SideSell {
		writeError(w, http.StatusBadRequest, "side must be buy or sell")
		return
	}

	typ := models.OrderType(strings.ToLower(strings.TrimSpace(req.Type)))
	if typ != models.OrderTypeLimit && typ != models.OrderTypeMarket {
		writeError(w, http.StatusBadRequest, "type must be limit or market")
		return
	}

	qty, err := decimal.NewFromString(strings.TrimSpace(req.Quantity))
	if err != nil || qty.LessThanOrEqual(decimal.Zero) {
		writeError(w, http.StatusBadRequest, "quantity must be a positive decimal")
		return
	}

	var price *decimal.Decimal
	if typ == models.OrderTypeLimit {
		if req.Price == nil || strings.TrimSpace(*req.Price) == "" {
			writeError(w, http.StatusBadRequest, "limit orders require price")
			return
		}
		p, err := decimal.NewFromString(strings.TrimSpace(*req.Price))
		if err != nil || p.LessThanOrEqual(decimal.Zero) {
			writeError(w, http.StatusBadRequest, "price must be a positive decimal")
			return
		}
		price = &p
	}

	now := time.Now().UTC()
	order := &models.Order{
		ID:        uuid.New(),
		UserID:    strings.TrimSpace(req.UserID),
		Symbol:    symbol,
		Side:      side,
		Type:      typ,
		Price:     price,
		Quantity:  qty,
		Remaining: qty,
		Status:    models.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}

	var idemPtr *string
	if idemKey != "" {
		idemPtr = &idemKey
	}

	tx, err := h.store.Pool().Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	if err := h.store.InsertOrderTx(r.Context(), tx, order, idemPtr); err != nil {
		if store.IsUniqueViolation(err) && idemKey != "" {
			existing, e2 := h.store.GetOrderByIdempotencyKey(r.Context(), idemKey)
			if e2 == nil {
				writeJSON(w, http.StatusOK, orderToResp(existing))
				return
			}
		}
		writeError(w, http.StatusInternalServerError, "failed to persist order")
		return
	}

	mStart := time.Now()
	trades, affected, err := h.engine.Submit(order, time.Now().UTC())
	matchResult := "ok"
	if err != nil {
		matchResult = "error"
	}
	observability.MatchLatency.WithLabelValues(matchResult).Observe(float64(time.Since(mStart).Milliseconds()))
	if err != nil {
		writeError(w, http.StatusBadGateway, "matching failed")
		return
	}
	if err := h.store.InsertTradesTx(r.Context(), tx, trades); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist trades")
		return
	}
	for _, o := range affected {
		if err := h.store.UpdateOrderTx(r.Context(), tx, o); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update order")
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit")
		return
	}

	observability.OrdersReceived.Inc()
	observability.TradesExecuted.Add(float64(len(trades)))

	h.bus.OrderCreated(r.Context(), order)
	for i := range trades {
		t := trades[i]
		h.bus.TradeExecuted(r.Context(), &t)
	}
	snap0 := h.engine.Snapshot(order.Symbol, 50)
	h.bus.BookUpdated(r.Context(), order.Symbol, snap0.Sequence)
	if h.hub != nil {
		snap1 := h.engine.Snapshot(order.Symbol, 50)
		h.hub.Broadcast(order.Symbol, "book", snap1)
		for i := range trades {
			h.hub.Broadcast(order.Symbol, "trade", trades[i])
		}
	}

	writeJSON(w, http.StatusCreated, orderToResp(order))
}

func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid order id")
		return
	}
	o, err := h.store.GetOrder(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, orderToResp(o))
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid order id")
		return
	}
	o, err := h.store.GetOrder(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if o.Status != models.StatusOpen && o.Status != models.StatusPartiallyFill {
		writeError(w, http.StatusConflict, "order cannot be cancelled")
		return
	}

	now := time.Now().UTC()
	co, ok := h.engine.Cancel(id, o.Symbol, now)
	if ok {
		if err := h.store.UpdateOrderFromModel(r.Context(), co); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update order")
			return
		}
		h.bus.OrderCanceled(r.Context(), co)
		snapc := h.engine.Snapshot(o.Symbol, 50)
		h.bus.BookUpdated(r.Context(), o.Symbol, snapc.Sequence)
		if h.hub != nil {
			snap := h.engine.Snapshot(o.Symbol, 50)
			h.hub.Broadcast(o.Symbol, "book", snap)
		}
		writeJSON(w, http.StatusOK, orderToResp(co))
		return
	}

	if err := h.store.MarkCancelled(r.Context(), id, now); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "order cannot be cancelled")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to cancel order")
		return
	}
	o2, err := h.store.GetOrder(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	h.bus.OrderCanceled(r.Context(), o2)
	snapm := h.engine.Snapshot(o2.Symbol, 50)
	h.bus.BookUpdated(r.Context(), o2.Symbol, snapm.Sequence)
	if h.hub != nil {
		snap := h.engine.Snapshot(o2.Symbol, 50)
		h.hub.Broadcast(o2.Symbol, "book", snap)
	}
	writeJSON(w, http.StatusOK, orderToResp(o2))
}

func (h *Handler) GetBook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "symbol")))
	if symbol == "" {
		writeError(w, http.StatusBadRequest, "symbol required")
		return
	}
	depth := 50
	if d := strings.TrimSpace(r.URL.Query().Get("depth")); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "depth must be between 1 and 500")
			return
		}
		depth = n
	}
	snap := h.engine.Snapshot(symbol, depth)
	writeJSON(w, http.StatusOK, snap)
}

func (h *Handler) GetTrades(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "symbol")))
	if symbol == "" {
		writeError(w, http.StatusBadRequest, "symbol required")
		return
	}
	limit := 100
	if l := strings.TrimSpace(r.URL.Query().Get("limit")); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		limit = n
	}
	rows, err := h.store.ListTradesBySymbol(r.Context(), symbol, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"symbol": symbol,
		"trades": rows,
	})
}

func (h *Handler) HealthLive(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HealthReady(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	if h.redis != nil {
		if err := h.redis.Ping(r.Context()).Err(); err != nil {
			writeError(w, http.StatusServiceUnavailable, "redis unavailable")
			return
		}
	}
	if h.nats != nil && !h.nats.IsConnected() {
		writeError(w, http.StatusServiceUnavailable, "nats unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// MarketWebSocket serves GET /ws/market?symbol=...
func (h *Handler) MarketWebSocket(w http.ResponseWriter, r *http.Request) {
	if h.hub == nil {
		writeError(w, http.StatusServiceUnavailable, "websocket hub disabled")
		return
	}
	ws.ServeMarket(h.hub, w, r)
}
