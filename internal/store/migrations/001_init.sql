CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY,
    user_id TEXT NOT NULL,
    symbol TEXT NOT NULL,
    side TEXT NOT NULL,
    type TEXT NOT NULL,
    price TEXT,
    quantity TEXT NOT NULL,
    remaining_quantity TEXT NOT NULL,
    status TEXT NOT NULL,
    idempotency_key TEXT UNIQUE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS trades (
    id UUID PRIMARY KEY,
    symbol TEXT NOT NULL,
    price TEXT NOT NULL,
    quantity TEXT NOT NULL,
    buy_order_id UUID NOT NULL,
    sell_order_id UUID NOT NULL,
    executed_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_orders_symbol ON orders (symbol);
CREATE INDEX IF NOT EXISTS idx_trades_symbol_executed ON trades (symbol, executed_at DESC);
