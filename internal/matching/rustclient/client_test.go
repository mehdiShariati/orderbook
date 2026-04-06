package rustclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"e-orderbook/internal/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestClientSubmit_Success(t *testing.T) {
	orderID := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/submit" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		rem := "0.5"
		st := string(models.StatusPartiallyFill)
		resp := submitResp{
			Trades: []wireTrade{
				{
					ID:          uuid.New().String(),
					Symbol:      "T-USD",
					Price:       "10",
					Quantity:    "0.5",
					BuyOrderID:  orderID.String(),
					SellOrderID: uuid.New().String(),
					ExecutedAt:  time.Now().UTC().Format(time.RFC3339Nano),
				},
			},
			AffectedOrders: []wireOrder{
				{
					ID:        orderID.String(),
					UserID:    "u1",
					Symbol:    "T-USD",
					Side:      "buy",
					Type:      "limit",
					Price:     strPtr("10"),
					Quantity:  "1",
					Remaining: rem,
					Status:    st,
					CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
					UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
				},
			},
			Sequence: 3,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	price := decimal.RequireFromString("10")
	o := &models.Order{
		ID:        orderID,
		UserID:    "u1",
		Symbol:    "T-USD",
		Side:      models.SideBuy,
		Type:      models.OrderTypeLimit,
		Price:     &price,
		Quantity:  decimal.RequireFromString("1"),
		Remaining: decimal.RequireFromString("1"),
		Status:    models.StatusOpen,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	trades, affected, err := c.Submit(o, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) != 1 {
		t.Fatalf("trades: %d", len(trades))
	}
	if len(affected) != 1 {
		t.Fatalf("affected: %d", len(affected))
	}
	if o.Remaining.String() != "0.5" {
		t.Fatalf("order not updated in place: remaining %s", o.Remaining.String())
	}
}

func TestClientSubmit_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	price := decimal.RequireFromString("1")
	o := &models.Order{
		ID:        uuid.New(),
		UserID:    "u",
		Symbol:    "X",
		Side:      models.SideBuy,
		Type:      models.OrderTypeLimit,
		Price:     &price,
		Quantity:  decimal.RequireFromString("1"),
		Remaining: decimal.RequireFromString("1"),
		Status:    models.StatusOpen,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_, _, err = c.Submit(o, time.Now().UTC())
	if err == nil {
		t.Fatal("expected error")
	}
}

func strPtr(s string) *string { return &s }

func TestClientSnapshot_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"symbol":"Z-USD","bids":[{"price":"99","quantity":"1"}],"asks":[],"best_bid":"99","sequence":7}`)
	}))
	defer srv.Close()
	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	snap := c.Snapshot("Z-USD", 10)
	if snap.Symbol != "Z-USD" {
		t.Fatalf("symbol %q", snap.Symbol)
	}
	if snap.Sequence != 7 {
		t.Fatalf("seq %d", snap.Sequence)
	}
	if snap.BestBid == nil || *snap.BestBid != "99" {
		t.Fatal("best bid")
	}
}
