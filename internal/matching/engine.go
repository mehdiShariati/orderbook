package matching

import (
	"time"

	"e-orderbook/internal/models"

	"github.com/google/uuid"
)

// Engine abstracts the in-process Go matcher or a remote Rust matcher.
type Engine interface {
	Submit(order *models.Order, now time.Time) ([]models.Trade, []*models.Order, error)
	Cancel(orderID uuid.UUID, symbol string, now time.Time) (*models.Order, bool)
	Snapshot(symbol string, depth int) BookSnapshot
}
