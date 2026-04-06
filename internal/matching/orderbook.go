package matching

import (
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"e-orderbook/internal/models"
)

type PriceLevel struct {
	price decimal.Decimal

	// queue holds resting orders in arrival order. We advance `head` when orders are filled/cancelled.
	queue []*models.Order
	head  int
}

func (pl *PriceLevel) add(order *models.Order) {
	pl.queue = append(pl.queue, order)
}

func (pl *PriceLevel) topValid() *models.Order {
	for pl.head < len(pl.queue) {
		o := pl.queue[pl.head]
		if o.Remaining.LessThanOrEqual(decimal.Zero) || o.Status == models.StatusCancelled || o.Status == models.StatusFilled {
			pl.head++
			continue
		}
		return o
	}

	// Optional compaction for long-running systems.
	if pl.head > 0 {
		pl.queue = nil
		pl.head = 0
	}
	return nil
}

func (pl *PriceLevel) totalRemaining() decimal.Decimal {
	total := decimal.Zero
	for i := pl.head; i < len(pl.queue); i++ {
		o := pl.queue[i]
		if o.Remaining.GreaterThan(decimal.Zero) && (o.Status == models.StatusOpen || o.Status == models.StatusPartiallyFill) {
			total = total.Add(o.Remaining)
		}
	}
	return total
}

type OrderBook struct {
	symbol string
	mu     sync.Mutex

	// For buys: best bid is highest price.
	bidHeap   *priceHeap
	bidLevels map[string]*PriceLevel // key: price.String()

	// For sells: best ask is lowest price.
	askHeap   *priceHeap
	askLevels map[string]*PriceLevel // key: price.String()

	// orders contains only currently resting orders (open/partially_filled), for cancel lookup.
	orders map[uuid.UUID]*models.Order

	seq uint64
}

func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		symbol:    symbol,
		bidHeap:   newMaxPriceHeap(),
		bidLevels: make(map[string]*PriceLevel),
		askHeap:   newMinPriceHeap(),
		askLevels: make(map[string]*PriceLevel),
		orders:    make(map[uuid.UUID]*models.Order),
	}
}

func priceKey(p decimal.Decimal) string { return p.String() }

func (ob *OrderBook) nextSeq() uint64 {
	ob.seq++
	return ob.seq
}

func (ob *OrderBook) bestBid() (decimal.Decimal, bool) {
	for {
		p, ok := ob.bidHeap.Peek()
		if !ok {
			return decimal.Decimal{}, false
		}
		k := priceKey(p)
		pl, exists := ob.bidLevels[k]
		if !exists || pl.totalRemaining().LessThanOrEqual(decimal.Zero) || pl.topValid() == nil {
			// Lazy deletion.
			ob.bidHeap.PopPrice()
			continue
		}
		return p, true
	}
}

func (ob *OrderBook) bestAsk() (decimal.Decimal, bool) {
	for {
		p, ok := ob.askHeap.Peek()
		if !ok {
			return decimal.Decimal{}, false
		}
		k := priceKey(p)
		pl, exists := ob.askLevels[k]
		if !exists || pl.totalRemaining().LessThanOrEqual(decimal.Zero) || pl.topValid() == nil {
			ob.askHeap.PopPrice()
			continue
		}
		return p, true
	}
}

func (ob *OrderBook) addResting(order *models.Order) {
	order.TimePrioritySeq = ob.nextSeq()
	order.Status = models.StatusOpen
	if order.Side == models.SideBuy {
		k := priceKey(*order.Price)
		pl := ob.bidLevels[k]
		if pl == nil {
			pl = &PriceLevel{price: *order.Price}
			ob.bidLevels[k] = pl
			ob.bidHeap.PushPrice(*order.Price)
		}
		pl.add(order)
	} else {
		k := priceKey(*order.Price)
		pl := ob.askLevels[k]
		if pl == nil {
			pl = &PriceLevel{price: *order.Price}
			ob.askLevels[k] = pl
			ob.askHeap.PushPrice(*order.Price)
		}
		pl.add(order)
	}
	ob.orders[order.ID] = order
}

func minDecimal(a, b decimal.Decimal) decimal.Decimal {
	if a.LessThan(b) {
		return a
	}
	return b
}

func touchAffected(affected map[uuid.UUID]*models.Order, o *models.Order) {
	if o != nil {
		affected[o.ID] = o
	}
}

func affectedSlice(affected map[uuid.UUID]*models.Order) []*models.Order {
	out := make([]*models.Order, 0, len(affected))
	for _, o := range affected {
		out = append(out, o)
	}
	return out
}

// Submit attempts to match the incoming order against the book and returns:
// trades, every order struct that was mutated (for persistence), and no error for domain outcomes.
// It assumes ob.symbol matches order.Symbol.
func (ob *OrderBook) Submit(order *models.Order, now time.Time) ([]models.Trade, []*models.Order, error) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	affected := make(map[uuid.UUID]*models.Order)
	touchAffected(affected, order)

	if order.Symbol != ob.symbol {
		return nil, affectedSlice(affected), nil
	}

	if order.Quantity.LessThanOrEqual(decimal.Zero) {
		order.Status = models.StatusRejected
		order.UpdatedAt = now
		return nil, affectedSlice(affected), nil
	}

	trades := make([]models.Trade, 0)
	executedSomething := false

	switch order.Side {
	case models.SideBuy:
		if order.Type == models.OrderTypeLimit && order.Price == nil {
			order.Status = models.StatusRejected
			order.UpdatedAt = now
			return nil, affectedSlice(affected), nil
		}
		for order.Remaining.GreaterThan(decimal.Zero) {
			bestAsk, ok := ob.bestAsk()
			if !ok {
				break
			}

			// For buy limit, stop if best ask is above our limit.
			if order.Type == models.OrderTypeLimit && bestAsk.GreaterThan(*order.Price) {
				break
			}

			pl := ob.askLevels[priceKey(bestAsk)]
			if pl == nil {
				break
			}
			maker := pl.topValid()
			if maker == nil {
				break
			}

			execQty := minDecimal(order.Remaining, maker.Remaining)
			if execQty.LessThanOrEqual(decimal.Zero) {
				break
			}

			order.Remaining = order.Remaining.Sub(execQty)
			maker.Remaining = maker.Remaining.Sub(execQty)
			order.UpdatedAt = now
			maker.UpdatedAt = now

			if maker.Remaining.LessThanOrEqual(decimal.Zero) {
				maker.Remaining = decimal.Zero
				maker.Status = models.StatusFilled
				delete(ob.orders, maker.ID)
			} else {
				maker.Status = models.StatusPartiallyFill
			}
			touchAffected(affected, maker)

			executedSomething = true

			trades = append(trades, models.Trade{
				ID: uuid.New(),
				// Execution uses maker (resting) price.
				Symbol:      ob.symbol,
				Price:       *maker.Price,
				Quantity:    execQty,
				BuyOrderID:  order.ID,
				SellOrderID: maker.ID,
				ExecutedAt:  now,
			})
		}

		if order.Remaining.LessThanOrEqual(decimal.Zero) {
			order.Remaining = decimal.Zero
			order.Status = models.StatusFilled
		} else {
			if order.Type == models.OrderTypeMarket {
				// Market order does not rest; any remainder is unfilled.
				order.Status = models.StatusPartiallyFill
				order.Remaining = order.Remaining // keep remainder for visibility
			} else {
				if executedSomething {
					order.Status = models.StatusPartiallyFill
				} else {
					order.Status = models.StatusOpen
				}
				ob.addResting(order)
			}
		}

		return trades, affectedSlice(affected), nil

	case models.SideSell:
		if order.Type == models.OrderTypeLimit && order.Price == nil {
			order.Status = models.StatusRejected
			order.UpdatedAt = now
			return nil, affectedSlice(affected), nil
		}
		for order.Remaining.GreaterThan(decimal.Zero) {
			bestBid, ok := ob.bestBid()
			if !ok {
				break
			}

			// For sell limit, stop if best bid is below our limit.
			if order.Type == models.OrderTypeLimit && bestBid.LessThan(*order.Price) {
				break
			}

			pl := ob.bidLevels[priceKey(bestBid)]
			if pl == nil {
				break
			}
			maker := pl.topValid()
			if maker == nil {
				break
			}

			execQty := minDecimal(order.Remaining, maker.Remaining)
			if execQty.LessThanOrEqual(decimal.Zero) {
				break
			}

			order.Remaining = order.Remaining.Sub(execQty)
			maker.Remaining = maker.Remaining.Sub(execQty)
			order.UpdatedAt = now
			maker.UpdatedAt = now

			if maker.Remaining.LessThanOrEqual(decimal.Zero) {
				maker.Remaining = decimal.Zero
				maker.Status = models.StatusFilled
				delete(ob.orders, maker.ID)
			} else {
				maker.Status = models.StatusPartiallyFill
			}
			touchAffected(affected, maker)

			executedSomething = true

			trades = append(trades, models.Trade{
				ID: uuid.New(),
				// Execution uses maker (resting) price.
				Symbol:      ob.symbol,
				Price:       *maker.Price,
				Quantity:    execQty,
				BuyOrderID:  maker.ID,
				SellOrderID: order.ID,
				ExecutedAt:  now,
			})
		}

		if order.Remaining.LessThanOrEqual(decimal.Zero) {
			order.Remaining = decimal.Zero
			order.Status = models.StatusFilled
		} else {
			if order.Type == models.OrderTypeMarket {
				// Market order does not rest; any remainder is unfilled.
				order.Status = models.StatusPartiallyFill
				order.Remaining = order.Remaining
			} else {
				if executedSomething {
					order.Status = models.StatusPartiallyFill
				} else {
					order.Status = models.StatusOpen
				}
				ob.addResting(order)
			}
		}

		return trades, affectedSlice(affected), nil
	default:
		order.Status = models.StatusRejected
		order.UpdatedAt = now
		return nil, affectedSlice(affected), nil
	}
}

func (ob *OrderBook) Cancel(orderID uuid.UUID, now time.Time) (*models.Order, bool) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	o := ob.orders[orderID]
	if o == nil {
		return nil, false
	}
	if o.Status != models.StatusOpen && o.Status != models.StatusPartiallyFill {
		return nil, false
	}

	o.Status = models.StatusCancelled
	o.Remaining = o.Remaining // keep remainder
	o.UpdatedAt = now
	delete(ob.orders, orderID)

	return o, true
}

type BookLevel struct {
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
}

type BookSnapshot struct {
	Symbol   string      `json:"symbol"`
	Bids     []BookLevel `json:"bids"`
	Asks     []BookLevel `json:"asks"`
	BestBid  *string     `json:"best_bid,omitempty"`
	BestAsk  *string     `json:"best_ask,omitempty"`
	Sequence uint64      `json:"sequence,omitempty"`
}

func orderLevelToBookLevel(price decimal.Decimal, total decimal.Decimal) BookLevel {
	return BookLevel{
		Price:    price.String(),
		Quantity: total.String(),
	}
}

func (ob *OrderBook) Snapshot(depth int) BookSnapshot {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	bidPrices := make([]decimal.Decimal, 0, len(ob.bidLevels))
	for _, pl := range ob.bidLevels {
		if pl.totalRemaining().GreaterThan(decimal.Zero) && pl.topValid() != nil {
			bidPrices = append(bidPrices, pl.price)
		}
	}
	sort.Slice(bidPrices, func(i, j int) bool { return bidPrices[i].GreaterThan(bidPrices[j]) })

	askPrices := make([]decimal.Decimal, 0, len(ob.askLevels))
	for _, pl := range ob.askLevels {
		if pl.totalRemaining().GreaterThan(decimal.Zero) && pl.topValid() != nil {
			askPrices = append(askPrices, pl.price)
		}
	}
	sort.Slice(askPrices, func(i, j int) bool { return askPrices[i].LessThan(askPrices[j]) })

	if depth <= 0 {
		depth = 50
	}

	snap := BookSnapshot{
		Symbol: ob.symbol,
		Bids:   make([]BookLevel, 0, depth),
		Asks:   make([]BookLevel, 0, depth),
	}

	for i := 0; i < len(bidPrices) && i < depth; i++ {
		p := bidPrices[i]
		pl := ob.bidLevels[priceKey(p)]
		if pl == nil {
			continue
		}
		total := pl.totalRemaining()
		if total.LessThanOrEqual(decimal.Zero) {
			continue
		}
		snap.Bids = append(snap.Bids, orderLevelToBookLevel(p, total))
	}

	for i := 0; i < len(askPrices) && i < depth; i++ {
		p := askPrices[i]
		pl := ob.askLevels[priceKey(p)]
		if pl == nil {
			continue
		}
		total := pl.totalRemaining()
		if total.LessThanOrEqual(decimal.Zero) {
			continue
		}
		snap.Asks = append(snap.Asks, orderLevelToBookLevel(p, total))
	}

	if len(snap.Bids) > 0 {
		s := snap.Bids[0].Price
		snap.BestBid = &s
	}
	if len(snap.Asks) > 0 {
		s := snap.Asks[0].Price
		snap.BestAsk = &s
	}

	return snap
}

