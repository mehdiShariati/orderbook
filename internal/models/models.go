package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type OrderSide string

const (
	SideBuy  OrderSide = "buy"
	SideSell OrderSide = "sell"
)

type OrderType string

const (
	OrderTypeLimit  OrderType = "limit"
	OrderTypeMarket OrderType = "market"
)

type OrderStatus string

const (
	StatusOpen          OrderStatus = "open"
	StatusPartiallyFill OrderStatus = "partial_filled"
	StatusFilled        OrderStatus = "filled"
	StatusCancelled     OrderStatus = "cancelled"
	StatusRejected      OrderStatus = "rejected"
)

type Order struct {
	ID        uuid.UUID
	UserID    string
	Symbol    string
	Side      OrderSide
	Type      OrderType

	// Price is only applicable for limit orders; for market orders it is nil.
	Price *decimal.Decimal

	Quantity         decimal.Decimal
	Remaining        decimal.Decimal
	Status           OrderStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
	TimePrioritySeq uint64
}

type Trade struct {
	ID uuid.UUID

	Symbol string
	Price  decimal.Decimal
	Quantity decimal.Decimal

	BuyOrderID  uuid.UUID
	SellOrderID uuid.UUID

	ExecutedAt time.Time
}

