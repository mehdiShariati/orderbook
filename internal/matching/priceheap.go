package matching

import (
	"container/heap"

	"github.com/shopspring/decimal"
)

// priceHeap is a min-heap or max-heap (depending on less function) over decimal prices.
type priceHeap struct {
	prices []decimal.Decimal
	less   func(a, b decimal.Decimal) bool
}

func newMinPriceHeap() *priceHeap {
	return &priceHeap{
		less: func(a, b decimal.Decimal) bool { return a.LessThan(b) },
	}
}

func newMaxPriceHeap() *priceHeap {
	return &priceHeap{
		less: func(a, b decimal.Decimal) bool { return a.GreaterThan(b) },
	}
}

func (h priceHeap) Len() int { return len(h.prices) }

func (h priceHeap) Less(i, j int) bool {
	return h.less(h.prices[i], h.prices[j])
}

func (h priceHeap) Swap(i, j int) { h.prices[i], h.prices[j] = h.prices[j], h.prices[i] }

func (h *priceHeap) Push(x any) {
	h.prices = append(h.prices, x.(decimal.Decimal))
}

func (h *priceHeap) Pop() any {
	n := len(h.prices)
	x := h.prices[n-1]
	h.prices = h.prices[:n-1]
	return x
}

func (h *priceHeap) PushPrice(p decimal.Decimal) {
	heap.Push(h, p)
}

// Peek returns the best (min or max) price currently in the heap.
func (h *priceHeap) Peek() (decimal.Decimal, bool) {
	if len(h.prices) == 0 {
		return decimal.Decimal{}, false
	}
	return h.prices[0], true
}

func (h *priceHeap) PopPrice() (decimal.Decimal, bool) {
	if len(h.prices) == 0 {
		return decimal.Decimal{}, false
	}
	p := heap.Pop(h).(decimal.Decimal)
	return p, true
}

