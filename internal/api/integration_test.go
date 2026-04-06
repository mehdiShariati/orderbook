package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"e-orderbook/internal/events"
	"e-orderbook/internal/matching"
	"e-orderbook/internal/models"
	"e-orderbook/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for integration tests, e.g. postgres://postgres:postgres@localhost:5432/orderbook?sslmode=disable")
	}
	return dsn
}

func setupStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	st, err := store.New(ctx, testDSN(t))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := store.Migrate(ctx, st.Pool()); err != nil {
		st.Close()
		t.Fatalf("migrate: %v", err)
	}
	truncateTables(t, st)
	t.Cleanup(func() { st.Close() })
	return st
}

func truncateTables(t *testing.T, st *store.Store) {
	t.Helper()
	_, err := st.Pool().Exec(context.Background(), `TRUNCATE orders, trades, events RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func testRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Post("/orders", h.CreateOrder)
	r.Get("/orders/{id}", h.GetOrder)
	r.Delete("/orders/{id}", h.CancelOrder)
	r.Get("/book/{symbol}", h.GetBook)
	r.Get("/trades/{symbol}", h.GetTrades)
	return r
}

func TestIntegration_CreateOrderAndGet(t *testing.T) {
	st := setupStore(t)
	eng := matching.NewManager()
	h := NewHandler(st, eng, events.New(nil, st), nil, nil, nil)
	srv := httptest.NewServer(testRouter(h))
	defer srv.Close()

	body := `{"user_id":"it-user","symbol":"INT-USD","side":"buy","type":"limit","price":"100","quantity":"2"}`
	res, err := http.Post(srv.URL+"/orders", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", res.StatusCode)
	}
	var out struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ID == "" {
		t.Fatal("expected id")
	}

	res2, err := http.Get(srv.URL + "/orders/" + out.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("get status %d", res2.StatusCode)
	}
}

func TestIntegration_SubmitErrorRollsBack(t *testing.T) {
	st := setupStore(t)
	h := NewHandler(st, submitFailEngine{}, events.New(nil, st), nil, nil, nil)
	srv := httptest.NewServer(testRouter(h))
	defer srv.Close()

	body := `{"user_id":"it-user","symbol":"RB-USD","side":"buy","type":"limit","price":"50","quantity":"1"}`
	res, err := http.Post(srv.URL+"/orders", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", res.StatusCode)
	}
	var n int
	if err := st.Pool().QueryRow(context.Background(), `SELECT COUNT(*) FROM orders`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 orders after rollback, got %d", n)
	}
}

func TestIntegration_IdempotencyKey(t *testing.T) {
	st := setupStore(t)
	eng := matching.NewManager()
	h := NewHandler(st, eng, events.New(nil, st), nil, nil, nil)
	srv := httptest.NewServer(testRouter(h))
	defer srv.Close()

	body := `{"user_id":"idem","symbol":"ID-USD","side":"buy","type":"limit","price":"10","quantity":"1"}`
	req1, _ := http.NewRequest(http.MethodPost, srv.URL+"/orders", bytes.NewReader([]byte(body)))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "idem-key-1")
	res1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	var first struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res1.Body).Decode(&first); err != nil {
		res1.Body.Close()
		t.Fatal(err)
	}
	res1.Body.Close()
	if res1.StatusCode != http.StatusCreated {
		t.Fatalf("first status %d", res1.StatusCode)
	}
	if first.ID == "" {
		t.Fatal("expected id on first response")
	}

	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/orders", bytes.NewReader([]byte(body)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "idem-key-1")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("second want 200, got %d", res2.StatusCode)
	}
	var second struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("idempotency: same id want %s got %s", first.ID, second.ID)
	}
	var n int
	if err := st.Pool().QueryRow(context.Background(), `SELECT COUNT(*) FROM orders`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want 1 order row, got %d", n)
	}
}

// submitFailEngine implements matching.Engine and always fails Submit (simulates remote matcher outage).
type submitFailEngine struct{}

func (submitFailEngine) Submit(*models.Order, time.Time) ([]models.Trade, []*models.Order, error) {
	return nil, nil, errors.New("injected matcher failure")
}

func (submitFailEngine) Cancel(uuid.UUID, string, time.Time) (*models.Order, bool) {
	return nil, false
}

func (submitFailEngine) Snapshot(string, int) matching.BookSnapshot {
	return matching.BookSnapshot{Symbol: "?"}
}

var _ matching.Engine = submitFailEngine{}
