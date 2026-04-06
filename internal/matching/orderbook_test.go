package matching

import (
	"testing"
	"time"

	"e-orderbook/internal/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestCrossLimitMatch(t *testing.T) {
	ob := NewOrderBook("BTC-USD")
	now := time.Unix(1700000000, 0).UTC()

	ask := &models.Order{
		ID:        uuid.New(),
		UserID:    "u1",
		Symbol:    "BTC-USD",
		Side:      models.SideSell,
		Type:      models.OrderTypeLimit,
		Price:     ptrDec("50000"),
		Quantity:  dec("1"),
		Remaining: dec("1"),
		Status:    models.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	tr1, aff1, err := ob.Submit(ask, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr1) != 0 {
		t.Fatalf("expected no trades on first resting order, got %d", len(tr1))
	}
	if len(aff1) != 1 {
		t.Fatalf("expected 1 affected order, got %d", len(aff1))
	}

	bid := &models.Order{
		ID:        uuid.New(),
		UserID:    "u2",
		Symbol:    "BTC-USD",
		Side:      models.SideBuy,
		Type:      models.OrderTypeLimit,
		Price:     ptrDec("50000"),
		Quantity:  dec("1"),
		Remaining: dec("1"),
		Status:    models.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	tr2, aff2, err := ob.Submit(bid, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr2) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(tr2))
	}
	if tr2[0].Price.String() != "50000" {
		t.Fatalf("unexpected trade price: %s", tr2[0].Price.String())
	}
	if bid.Status != models.StatusFilled {
		t.Fatalf("expected taker filled, got %s", bid.Status)
	}
	if len(aff2) < 1 {
		t.Fatalf("expected affected orders")
	}
}

func ptrDec(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func dec(s string) decimal.Decimal {
	return decimal.RequireFromString(s)
}

func TestPartialFillResting(t *testing.T) {
	ob := NewOrderBook("P-USD")
	now := time.Unix(1700000000, 0).UTC()

	sell := &models.Order{
		ID:        uuid.New(),
		UserID:    "maker",
		Symbol:    "P-USD",
		Side:      models.SideSell,
		Type:      models.OrderTypeLimit,
		Price:     ptrDec("100"),
		Quantity:  dec("2"),
		Remaining: dec("2"),
		Status:    models.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, _, err := ob.Submit(sell, now); err != nil {
		t.Fatal(err)
	}

	buy := &models.Order{
		ID:        uuid.New(),
		UserID:    "taker",
		Symbol:    "P-USD",
		Side:      models.SideBuy,
		Type:      models.OrderTypeLimit,
		Price:     ptrDec("100"),
		Quantity:  dec("1"),
		Remaining: dec("1"),
		Status:    models.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	trades, _, err := ob.Submit(buy, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if buy.Status != models.StatusFilled {
		t.Fatalf("buy status %s", buy.Status)
	}
	// Maker partially filled: 2 - 1 = 1 remaining
	if sell.Remaining.String() != "1" {
		t.Fatalf("maker remaining %s", sell.Remaining.String())
	}
	if sell.Status != models.StatusPartiallyFill {
		t.Fatalf("maker status %s", sell.Status)
	}
}

func TestMarketBuyConsumesAsk(t *testing.T) {
	ob := NewOrderBook("M-USD")
	now := time.Unix(1700000100, 0).UTC()

	ask := &models.Order{
		ID:        uuid.New(),
		UserID:    "a",
		Symbol:    "M-USD",
		Side:      models.SideSell,
		Type:      models.OrderTypeLimit,
		Price:     ptrDec("50"),
		Quantity:  dec("1"),
		Remaining: dec("1"),
		Status:    models.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, _, err := ob.Submit(ask, now); err != nil {
		t.Fatal(err)
	}

	mkt := &models.Order{
		ID:        uuid.New(),
		UserID:    "b",
		Symbol:    "M-USD",
		Side:      models.SideBuy,
		Type:      models.OrderTypeMarket,
		Price:     nil,
		Quantity:  dec("1"),
		Remaining: dec("1"),
		Status:    models.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	trades, _, err := ob.Submit(mkt, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) != 1 {
		t.Fatalf("trades %d", len(trades))
	}
	if mkt.Status != models.StatusFilled {
		t.Fatalf("market buy should be filled when liquidity exists, got %s", mkt.Status)
	}
}

func TestCancelRemovesResting(t *testing.T) {
	ob := NewOrderBook("C-USD")
	now := time.Unix(1700000200, 0).UTC()

	o := &models.Order{
		ID:        uuid.New(),
		UserID:    "u",
		Symbol:    "C-USD",
		Side:      models.SideBuy,
		Type:      models.OrderTypeLimit,
		Price:     ptrDec("1"),
		Quantity:  dec("5"),
		Remaining: dec("5"),
		Status:    models.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, _, err := ob.Submit(o, now); err != nil {
		t.Fatal(err)
	}
	co, ok := ob.Cancel(o.ID, now.Add(time.Second))
	if !ok || co == nil {
		t.Fatal("expected cancel ok")
	}
	if co.Status != models.StatusCancelled {
		t.Fatalf("status %s", co.Status)
	}
	snap := ob.Snapshot(10)
	if len(snap.Bids) != 0 {
		t.Fatalf("book should be empty, bids=%d", len(snap.Bids))
	}
}

func TestFIFOAtSamePrice(t *testing.T) {
	ob := NewOrderBook("F-USD")
	t0 := time.Unix(1700000300, 0).UTC()

	sell1 := &models.Order{
		ID:        uuid.New(),
		UserID:    "s1",
		Symbol:    "F-USD",
		Side:      models.SideSell,
		Type:      models.OrderTypeLimit,
		Price:     ptrDec("10"),
		Quantity:  dec("0.4"),
		Remaining: dec("0.4"),
		Status:    models.StatusOpen,
		CreatedAt: t0,
		UpdatedAt: t0,
	}
	if _, _, err := ob.Submit(sell1, t0); err != nil {
		t.Fatal(err)
	}
	sell2 := &models.Order{
		ID:        uuid.New(),
		UserID:    "s2",
		Symbol:    "F-USD",
		Side:      models.SideSell,
		Type:      models.OrderTypeLimit,
		Price:     ptrDec("10"),
		Quantity:  dec("0.6"),
		Remaining: dec("0.6"),
		Status:    models.StatusOpen,
		CreatedAt: t0.Add(time.Millisecond),
		UpdatedAt: t0.Add(time.Millisecond),
	}
	if _, _, err := ob.Submit(sell2, t0); err != nil {
		t.Fatal(err)
	}

	buy := &models.Order{
		ID:        uuid.New(),
		UserID:    "b",
		Symbol:    "F-USD",
		Side:      models.SideBuy,
		Type:      models.OrderTypeLimit,
		Price:     ptrDec("10"),
		Quantity:  dec("0.4"),
		Remaining: dec("0.4"),
		Status:    models.StatusOpen,
		CreatedAt: t0.Add(2 * time.Millisecond),
		UpdatedAt: t0.Add(2 * time.Millisecond),
	}
	trades, _, err := ob.Submit(buy, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	if trades[0].SellOrderID != sell1.ID {
		t.Fatal("first resting sell at same price should be consumed first (FIFO)")
	}
}

func TestSnapshotBestBidAsk(t *testing.T) {
	ob := NewOrderBook("S-USD")
	now := time.Unix(1700000400, 0).UTC()
	bid := &models.Order{
		ID: uuid.New(), UserID: "b", Symbol: "S-USD", Side: models.SideBuy, Type: models.OrderTypeLimit,
		Price: ptrDec("9"), Quantity: dec("1"), Remaining: dec("1"), Status: models.StatusOpen,
		CreatedAt: now, UpdatedAt: now,
	}
	ask := &models.Order{
		ID: uuid.New(), UserID: "a", Symbol: "S-USD", Side: models.SideSell, Type: models.OrderTypeLimit,
		Price: ptrDec("11"), Quantity: dec("1"), Remaining: dec("1"), Status: models.StatusOpen,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, _, err := ob.Submit(bid, now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ob.Submit(ask, now); err != nil {
		t.Fatal(err)
	}
	snap := ob.Snapshot(5)
	if snap.BestBid == nil || *snap.BestBid != "9" {
		t.Fatalf("best bid %v", snap.BestBid)
	}
	if snap.BestAsk == nil || *snap.BestAsk != "11" {
		t.Fatalf("best ask %v", snap.BestAsk)
	}
	if len(snap.Bids) < 1 || len(snap.Asks) < 1 {
		t.Fatalf("expected bids and asks in snapshot: bids=%d asks=%d", len(snap.Bids), len(snap.Asks))
	}
}
