package matching

import (
	"sync"
	"time"

	"e-orderbook/internal/models"

	"github.com/google/uuid"
)

type Manager struct {
	mu    sync.Mutex
	books map[string]*OrderBook
}

func NewManager() *Manager {
	return &Manager{
		books: make(map[string]*OrderBook),
	}
}

func (m *Manager) getOrCreate(symbol string) *OrderBook {
	m.mu.Lock()
	defer m.mu.Unlock()
	ob := m.books[symbol]
	if ob == nil {
		ob = NewOrderBook(symbol)
		m.books[symbol] = ob
	}
	return ob
}

func (m *Manager) Submit(order *models.Order, now time.Time) ([]models.Trade, []*models.Order, error) {
	return m.getOrCreate(order.Symbol).Submit(order, now)
}

// Cancel removes a resting order from the in-memory book. It does not create an empty book for unknown symbols.
func (m *Manager) Cancel(orderID uuid.UUID, symbol string, now time.Time) (*models.Order, bool) {
	m.mu.Lock()
	ob := m.books[symbol]
	m.mu.Unlock()
	if ob == nil {
		return nil, false
	}
	return ob.Cancel(orderID, now)
}

func (m *Manager) Snapshot(symbol string, depth int) BookSnapshot {
	return m.getOrCreate(symbol).Snapshot(depth)
}

var _ Engine = (*Manager)(nil)
