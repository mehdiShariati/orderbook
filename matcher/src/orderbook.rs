use chrono::{DateTime, Utc};
use rust_decimal::Decimal;
use std::cmp::min;
use std::collections::{BTreeMap, HashMap, VecDeque};
use uuid::Uuid;

#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct Order {
    pub id: Uuid,
    pub user_id: String,
    pub symbol: String,
    pub side: String,
    #[serde(rename = "type")]
    pub type_: String,
    pub price: Option<String>,
    pub quantity: String,
    pub remaining: String,
    pub status: String,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
    #[serde(default)]
    pub time_priority_seq: u64,
}

#[derive(Clone, Debug, serde::Serialize, serde::Deserialize)]
pub struct Trade {
    pub id: Uuid,
    pub symbol: String,
    pub price: String,
    pub quantity: String,
    pub buy_order_id: Uuid,
    pub sell_order_id: Uuid,
    pub executed_at: DateTime<Utc>,
}

struct PriceLevel {
    queue: VecDeque<Uuid>,
    head: usize,
}

impl PriceLevel {
    fn add(&mut self, id: Uuid) {
        self.queue.push_back(id);
    }

    fn top_valid(&mut self, orders: &HashMap<Uuid, Order>) -> Option<Uuid> {
        while self.head < self.queue.len() {
            let id = self.queue[self.head];
            let o = match orders.get(&id) {
                Some(x) => x,
                None => {
                    self.head += 1;
                    continue;
                }
            };
            let rem = parse_dec(&o.remaining).unwrap_or(Decimal::ZERO);
            if rem <= Decimal::ZERO || o.status == "cancelled" || o.status == "filled" {
                self.head += 1;
                continue;
            }
            return Some(id);
        }
        if self.head > 0 {
            self.queue.clear();
            self.head = 0;
        }
        None
    }

    fn total_remaining(&self, orders: &HashMap<Uuid, Order>) -> Decimal {
        let mut total = Decimal::ZERO;
        for i in self.head..self.queue.len() {
            let id = self.queue[i];
            let Some(o) = orders.get(&id) else { continue };
            if o.status != "open" && o.status != "partial_filled" {
                continue;
            }
            if let Ok(r) = parse_dec(&o.remaining) {
                if r > Decimal::ZERO {
                    total += r;
                }
            }
        }
        total
    }
}

pub struct OrderBook {
    symbol: String,
    bid_levels: BTreeMap<Decimal, PriceLevel>,
    ask_levels: BTreeMap<Decimal, PriceLevel>,
    orders: HashMap<Uuid, Order>,
    seq: u64,
    pub sequence: u64,
}

fn parse_dec(s: &str) -> Result<Decimal, rust_decimal::Error> {
    s.parse()
}

fn dec_str(d: Decimal) -> String {
    d.normalize().to_string()
}

impl OrderBook {
    pub fn new(symbol: String) -> Self {
        Self {
            symbol,
            bid_levels: BTreeMap::new(),
            ask_levels: BTreeMap::new(),
            orders: HashMap::new(),
            seq: 0,
            sequence: 0,
        }
    }

    fn next_seq(&mut self) -> u64 {
        self.seq += 1;
        self.seq
    }

    fn bump_sequence(&mut self) {
        self.sequence += 1;
    }

    fn best_ask_price(&mut self) -> Option<Decimal> {
        loop {
            let p = *self.ask_levels.keys().next()?;
            let pl = self.ask_levels.get_mut(&p)?;
            if pl.total_remaining(&self.orders) <= Decimal::ZERO
                || pl.top_valid(&self.orders).is_none()
            {
                self.ask_levels.remove(&p);
                continue;
            }
            return Some(p);
        }
    }

    fn best_bid_price(&mut self) -> Option<Decimal> {
        loop {
            let p = *self.bid_levels.keys().next_back()?;
            let pl = self.bid_levels.get_mut(&p)?;
            if pl.total_remaining(&self.orders) <= Decimal::ZERO
                || pl.top_valid(&self.orders).is_none()
            {
                self.bid_levels.remove(&p);
                continue;
            }
            return Some(p);
        }
    }

    fn add_resting(&mut self, order: &mut Order) {
        order.time_priority_seq = self.next_seq();
        order.status = "open".to_string();
        let price = parse_dec(order.price.as_ref().expect("limit has price")).unwrap();
        let id = order.id;
        self.orders.insert(id, order.clone());
        if order.side == "buy" {
            let pl = self.bid_levels.entry(price).or_insert_with(|| PriceLevel {
                queue: VecDeque::new(),
                head: 0,
            });
            pl.add(id);
        } else {
            let pl = self.ask_levels.entry(price).or_insert_with(|| PriceLevel {
                queue: VecDeque::new(),
                head: 0,
            });
            pl.add(id);
        }
    }

    pub fn submit(&mut self, order: &mut Order, now: DateTime<Utc>) -> (Vec<Trade>, Vec<Order>) {
        let mut affected: HashMap<Uuid, Order> = HashMap::new();
        affected.insert(order.id, order.clone());

        if order.symbol != self.symbol {
            return (vec![], affected.into_values().collect());
        }

        let qty0 = parse_dec(&order.remaining).unwrap_or(Decimal::ZERO);
        if qty0 <= Decimal::ZERO {
            order.status = "rejected".to_string();
            order.updated_at = now;
            return (vec![], vec![order.clone()]);
        }

        let mut trades: Vec<Trade> = Vec::new();
        let mut executed_something = false;

        match order.side.as_str() {
            "buy" => {
                if order.type_ == "limit" && order.price.is_none() {
                    order.status = "rejected".to_string();
                    order.updated_at = now;
                    return (vec![], vec![order.clone()]);
                }
                while parse_dec(&order.remaining).unwrap_or(Decimal::ZERO) > Decimal::ZERO {
                    let best_ask = match self.best_ask_price() {
                        Some(p) => p,
                        None => break,
                    };
                    if order.type_ == "limit" {
                        let lim = parse_dec(order.price.as_ref().unwrap()).unwrap();
                        if best_ask > lim {
                            break;
                        }
                    }
                    let maker_id = {
                        let pl = self.ask_levels.get_mut(&best_ask).unwrap();
                        pl.top_valid(&self.orders)
                    };
                    let maker_id = match maker_id {
                        Some(id) => id,
                        None => break,
                    };
                    let maker_px = {
                        let m = self.orders.get(&maker_id).unwrap();
                        parse_dec(m.price.as_ref().unwrap()).unwrap()
                    };

                    let maker = self.orders.get_mut(&maker_id).unwrap();

                    let taker_rem = parse_dec(&order.remaining).unwrap();
                    let maker_rem = parse_dec(&maker.remaining).unwrap();
                    let exec_qty = min(taker_rem, maker_rem);
                    if exec_qty <= Decimal::ZERO {
                        break;
                    }
                    order.remaining = dec_str(taker_rem - exec_qty);
                    maker.remaining = dec_str(maker_rem - exec_qty);
                    order.updated_at = now;
                    maker.updated_at = now;
                    if parse_dec(&maker.remaining).unwrap_or(Decimal::ZERO) <= Decimal::ZERO {
                        maker.remaining = dec_str(Decimal::ZERO);
                        maker.status = "filled".to_string();
                        self.orders.remove(&maker_id);
                    } else {
                        maker.status = "partial_filled".to_string();
                    }
                    affected.insert(
                        maker_id,
                        self.orders
                            .get(&maker_id)
                            .cloned()
                            .unwrap_or_else(|| Order {
                                id: maker_id,
                                user_id: String::new(),
                                symbol: self.symbol.clone(),
                                side: "sell".to_string(),
                                type_: "limit".to_string(),
                                price: Some(dec_str(maker_px)),
                                quantity: String::new(),
                                remaining: dec_str(Decimal::ZERO),
                                status: "filled".to_string(),
                                created_at: now,
                                updated_at: now,
                                time_priority_seq: 0,
                            }),
                    );
                    executed_something = true;

                    trades.push(Trade {
                        id: Uuid::new_v4(),
                        symbol: self.symbol.clone(),
                        price: dec_str(maker_px),
                        quantity: dec_str(exec_qty),
                        buy_order_id: order.id,
                        sell_order_id: maker_id,
                        executed_at: now,
                    });
                }
                if parse_dec(&order.remaining).unwrap_or(Decimal::ZERO) <= Decimal::ZERO {
                    order.remaining = dec_str(Decimal::ZERO);
                    order.status = "filled".to_string();
                } else if order.type_ == "market" {
                    order.status = "partial_filled".to_string();
                } else {
                    if executed_something {
                        order.status = "partial_filled".to_string();
                    } else {
                        order.status = "open".to_string();
                    }
                    self.add_resting(order);
                    affected.insert(order.id, self.orders.get(&order.id).cloned().unwrap_or_else(|| order.clone()));
                }
            }
            "sell" => {
                if order.type_ == "limit" && order.price.is_none() {
                    order.status = "rejected".to_string();
                    order.updated_at = now;
                    return (vec![], vec![order.clone()]);
                }
                while parse_dec(&order.remaining).unwrap_or(Decimal::ZERO) > Decimal::ZERO {
                    let best_bid = match self.best_bid_price() {
                        Some(p) => p,
                        None => break,
                    };
                    if order.type_ == "limit" {
                        let lim = parse_dec(order.price.as_ref().unwrap()).unwrap();
                        if best_bid < lim {
                            break;
                        }
                    }
                    let maker_id = {
                        let pl = self.bid_levels.get_mut(&best_bid).unwrap();
                        pl.top_valid(&self.orders)
                    };
                    let maker_id = match maker_id {
                        Some(id) => id,
                        None => break,
                    };
                    let maker = self.orders.get_mut(&maker_id).unwrap();

                    let taker_rem = parse_dec(&order.remaining).unwrap();
                    let maker_rem = parse_dec(&maker.remaining).unwrap();
                    let exec_qty = min(taker_rem, maker_rem);
                    if exec_qty <= Decimal::ZERO {
                        break;
                    }
                    order.remaining = dec_str(taker_rem - exec_qty);
                    maker.remaining = dec_str(maker_rem - exec_qty);
                    order.updated_at = now;
                    maker.updated_at = now;

                    let maker_price_str = maker.price.clone();

                    if parse_dec(&maker.remaining).unwrap_or(Decimal::ZERO) <= Decimal::ZERO {
                        maker.remaining = dec_str(Decimal::ZERO);
                        maker.status = "filled".to_string();
                        self.orders.remove(&maker_id);
                    } else {
                        maker.status = "partial_filled".to_string();
                    }
                    affected.insert(
                        maker_id,
                        self.orders
                            .get(&maker_id)
                            .cloned()
                            .unwrap_or_else(|| Order {
                                id: maker_id,
                                user_id: String::new(),
                                symbol: self.symbol.clone(),
                                side: "buy".to_string(),
                                type_: "limit".to_string(),
                                price: maker_price_str.clone(),
                                quantity: String::new(),
                                remaining: dec_str(Decimal::ZERO),
                                status: "filled".to_string(),
                                created_at: now,
                                updated_at: now,
                                time_priority_seq: 0,
                            }),
                    );
                    executed_something = true;

                    let maker_px = parse_dec(maker_price_str.as_ref().unwrap()).unwrap();

                    trades.push(Trade {
                        id: Uuid::new_v4(),
                        symbol: self.symbol.clone(),
                        price: dec_str(maker_px),
                        quantity: dec_str(exec_qty),
                        buy_order_id: maker_id,
                        sell_order_id: order.id,
                        executed_at: now,
                    });
                }
                if parse_dec(&order.remaining).unwrap_or(Decimal::ZERO) <= Decimal::ZERO {
                    order.remaining = dec_str(Decimal::ZERO);
                    order.status = "filled".to_string();
                } else if order.type_ == "market" {
                    order.status = "partial_filled".to_string();
                } else {
                    if executed_something {
                        order.status = "partial_filled".to_string();
                    } else {
                        order.status = "open".to_string();
                    }
                    self.add_resting(order);
                    affected.insert(order.id, self.orders.get(&order.id).cloned().unwrap_or_else(|| order.clone()));
                }
            }
            _ => {
                order.status = "rejected".to_string();
                order.updated_at = now;
                return (vec![], vec![order.clone()]);
            }
        }

        self.bump_sequence();
        affected.insert(order.id, order.clone());
        let mut out: Vec<Order> = affected.into_values().collect();
        out.sort_by_key(|o| o.id);
        (trades, out)
    }

    pub fn cancel(&mut self, id: Uuid, now: DateTime<Utc>) -> Option<Order> {
        let o = self.orders.get(&id)?.clone();
        if o.status != "open" && o.status != "partial_filled" {
            return None;
        }
        let mut o = o;
        o.status = "cancelled".to_string();
        o.updated_at = now;
        self.orders.remove(&id);
        self.bump_sequence();
        Some(o)
    }

    pub fn snapshot(&mut self, depth: usize) -> BookSnapshot {
        let depth = if depth == 0 { 50 } else { depth };

        let mut bid_prices: Vec<Decimal> = self
            .bid_levels
            .iter()
            .filter(|(_, pl)| pl.total_remaining(&self.orders) > Decimal::ZERO)
            .map(|(p, _)| *p)
            .collect();
        bid_prices.sort_by(|a, b| b.cmp(a));

        let mut ask_prices: Vec<Decimal> = self
            .ask_levels
            .iter()
            .filter(|(_, pl)| pl.total_remaining(&self.orders) > Decimal::ZERO)
            .map(|(p, _)| *p)
            .collect();
        ask_prices.sort_by(|a, b| a.cmp(b));

        let best_bid = bid_prices.first().map(|p| dec_str(*p));
        let best_ask = ask_prices.first().map(|p| dec_str(*p));

        let mut bids = Vec::new();
        for p in bid_prices.into_iter().take(depth) {
            if let Some(pl) = self.bid_levels.get(&p) {
                let t = pl.total_remaining(&self.orders);
                if t > Decimal::ZERO {
                    bids.push(BookLevel {
                        price: dec_str(p),
                        quantity: dec_str(t),
                    });
                }
            }
        }
        let mut asks = Vec::new();
        for p in ask_prices.into_iter().take(depth) {
            if let Some(pl) = self.ask_levels.get(&p) {
                let t = pl.total_remaining(&self.orders);
                if t > Decimal::ZERO {
                    asks.push(BookLevel {
                        price: dec_str(p),
                        quantity: dec_str(t),
                    });
                }
            }
        }

        BookSnapshot {
            symbol: self.symbol.clone(),
            bids,
            asks,
            best_bid,
            best_ask,
            sequence: self.sequence,
        }
    }
}

#[derive(serde::Serialize)]
pub struct BookLevel {
    pub price: String,
    pub quantity: String,
}

#[derive(serde::Serialize)]
pub struct BookSnapshot {
    pub symbol: String,
    pub bids: Vec<BookLevel>,
    pub asks: Vec<BookLevel>,
    pub best_bid: Option<String>,
    pub best_ask: Option<String>,
    pub sequence: u64,
}
