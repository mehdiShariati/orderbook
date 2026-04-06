package rustclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"e-orderbook/internal/matching"
	"e-orderbook/internal/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Client calls the Rust matcher HTTP API.
type Client struct {
	base    string
	http    *http.Client
}

func New(baseURL string) (*Client, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil, fmt.Errorf("empty matcher URL")
	}
	return &Client{
		base: baseURL,
		http: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

type wireOrder struct {
	ID               string  `json:"id"`
	UserID           string  `json:"user_id"`
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"`
	Type             string  `json:"type"`
	Price            *string `json:"price,omitempty"`
	Quantity         string  `json:"quantity"`
	Remaining        string  `json:"remaining"`
	Status           string  `json:"status"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	TimePrioritySeq  uint64  `json:"time_priority_seq"`
}

func wireFromOrder(o *models.Order) wireOrder {
	w := wireOrder{
		ID:              o.ID.String(),
		UserID:          o.UserID,
		Symbol:          o.Symbol,
		Side:            string(o.Side),
		Type:            string(o.Type),
		Quantity:        o.Quantity.String(),
		Remaining:       o.Remaining.String(),
		Status:          string(o.Status),
		CreatedAt:       o.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:       o.UpdatedAt.UTC().Format(time.RFC3339Nano),
		TimePrioritySeq: o.TimePrioritySeq,
	}
	if o.Price != nil {
		p := o.Price.String()
		w.Price = &p
	}
	return w
}

func orderFromWire(w *wireOrder) (*models.Order, error) {
	id, err := uuid.Parse(w.ID)
	if err != nil {
		return nil, err
	}
	qty, err := decimal.NewFromString(w.Quantity)
	if err != nil {
		return nil, err
	}
	rem, err := decimal.NewFromString(w.Remaining)
	if err != nil {
		return nil, err
	}
	var price *decimal.Decimal
	if w.Price != nil && *w.Price != "" {
		p, err := decimal.NewFromString(*w.Price)
		if err != nil {
			return nil, err
		}
		price = &p
	}
	t1, err := time.Parse(time.RFC3339Nano, w.CreatedAt)
	if err != nil {
		t1, err = time.Parse(time.RFC3339, w.CreatedAt)
		if err != nil {
			return nil, err
		}
	}
	t2, err := time.Parse(time.RFC3339Nano, w.UpdatedAt)
	if err != nil {
		t2, err = time.Parse(time.RFC3339, w.UpdatedAt)
		if err != nil {
			return nil, err
		}
	}
	return &models.Order{
		ID:              id,
		UserID:          w.UserID,
		Symbol:          w.Symbol,
		Side:            models.OrderSide(w.Side),
		Type:            models.OrderType(w.Type),
		Price:           price,
		Quantity:        qty,
		Remaining:       rem,
		Status:          models.OrderStatus(w.Status),
		CreatedAt:       t1.UTC(),
		UpdatedAt:       t2.UTC(),
		TimePrioritySeq: w.TimePrioritySeq,
	}, nil
}

type submitBody struct {
	Order wireOrder `json:"order"`
}

type submitResp struct {
	Trades         []wireTrade `json:"trades"`
	AffectedOrders []wireOrder `json:"affected_orders"`
	Sequence       uint64      `json:"sequence"`
}

type wireTrade struct {
	ID          string `json:"id"`
	Symbol      string `json:"symbol"`
	Price       string `json:"price"`
	Quantity    string `json:"quantity"`
	BuyOrderID  string `json:"buy_order_id"`
	SellOrderID string `json:"sell_order_id"`
	ExecutedAt  string `json:"executed_at"`
}

func tradeFromWire(w *wireTrade) (models.Trade, error) {
	id, err := uuid.Parse(w.ID)
	if err != nil {
		return models.Trade{}, err
	}
	buy, err := uuid.Parse(w.BuyOrderID)
	if err != nil {
		return models.Trade{}, err
	}
	sell, err := uuid.Parse(w.SellOrderID)
	if err != nil {
		return models.Trade{}, err
	}
	px, err := decimal.NewFromString(w.Price)
	if err != nil {
		return models.Trade{}, err
	}
	qty, err := decimal.NewFromString(w.Quantity)
	if err != nil {
		return models.Trade{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, w.ExecutedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, w.ExecutedAt)
		if err != nil {
			return models.Trade{}, err
		}
	}
	return models.Trade{
		ID:          id,
		Symbol:      w.Symbol,
		Price:       px,
		Quantity:    qty,
		BuyOrderID:  buy,
		SellOrderID: sell,
		ExecutedAt:  t.UTC(),
	}, nil
}

func (c *Client) Submit(order *models.Order, now time.Time) ([]models.Trade, []*models.Order, error) {
	body := submitBody{Order: wireFromOrder(order)}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.base+"/v1/submit", bytes.NewReader(raw))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("matcher: %s: %s", resp.Status, string(b))
	}
	var out submitResp
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, nil, err
	}
	trades := make([]models.Trade, 0, len(out.Trades))
	for i := range out.Trades {
		t, err := tradeFromWire(&out.Trades[i])
		if err != nil {
			return nil, nil, err
		}
		trades = append(trades, t)
	}
	affected := make([]*models.Order, 0, len(out.AffectedOrders))
	for i := range out.AffectedOrders {
		o, err := orderFromWire(&out.AffectedOrders[i])
		if err != nil {
			return nil, nil, err
		}
		affected = append(affected, o)
	}
	for _, o := range affected {
		if o.ID == order.ID {
			*order = *o
			break
		}
	}
	return trades, affected, nil
}

type cancelBody struct {
	OrderID uuid.UUID `json:"order_id"`
	Symbol  string    `json:"symbol"`
}

type cancelResp struct {
	Order    *wireOrder `json:"order"`
	Sequence uint64     `json:"sequence"`
}

func (c *Client) Cancel(orderID uuid.UUID, symbol string, now time.Time) (*models.Order, bool) {
	body := cancelBody{OrderID: orderID, Symbol: strings.ToUpper(strings.TrimSpace(symbol))}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, false
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.base+"/v1/cancel", bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	var out cancelResp
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, false
	}
	if out.Order == nil {
		return nil, false
	}
	o, err := orderFromWire(out.Order)
	if err != nil {
		return nil, false
	}
	return o, true
}

type snapResp struct {
	Symbol   string `json:"symbol"`
	Bids     []struct {
		Price    string `json:"price"`
		Quantity string `json:"quantity"`
	} `json:"bids"`
	Asks     []struct {
		Price    string `json:"price"`
		Quantity string `json:"quantity"`
	} `json:"asks"`
	BestBid  *string `json:"best_bid"`
	BestAsk  *string `json:"best_ask"`
	Sequence uint64  `json:"sequence"`
}

func (c *Client) Snapshot(symbol string, depth int) matching.BookSnapshot {
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	u, err := url.Parse(c.base + "/v1/book/" + url.PathEscape(sym))
	if err != nil {
		return matching.BookSnapshot{Symbol: sym}
	}
	q := u.Query()
	q.Set("depth", strconv.Itoa(depth))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	if err != nil {
		return matching.BookSnapshot{Symbol: sym}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return matching.BookSnapshot{Symbol: sym}
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return matching.BookSnapshot{Symbol: sym}
	}
	if resp.StatusCode != http.StatusOK {
		return matching.BookSnapshot{Symbol: sym}
	}
	var out snapResp
	if err := json.Unmarshal(b, &out); err != nil {
		return matching.BookSnapshot{Symbol: sym}
	}
	snap := matching.BookSnapshot{
		Symbol:   out.Symbol,
		BestBid:  out.BestBid,
		BestAsk:  out.BestAsk,
		Sequence: out.Sequence,
	}
	for _, x := range out.Bids {
		snap.Bids = append(snap.Bids, matching.BookLevel{Price: x.Price, Quantity: x.Quantity})
	}
	for _, x := range out.Asks {
		snap.Asks = append(snap.Asks, matching.BookLevel{Price: x.Price, Quantity: x.Quantity})
	}
	return snap
}

