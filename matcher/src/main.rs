mod orderbook;

use axum::extract::{Path, Query, State};
use axum::routing::{get, post};
use axum::{Json, Router};
use chrono::Utc;
use orderbook::{BookSnapshot, Order, OrderBook, Trade};
use serde::Deserialize;
use std::collections::HashMap;
use std::sync::{Arc, RwLock};
use tower_http::trace::TraceLayer;
use tracing::info;
use uuid::Uuid;

#[derive(Clone)]
struct AppState {
    books: Arc<RwLock<HashMap<String, Arc<RwLock<OrderBook>>>>>,
}

impl AppState {
    fn get_or_create(&self, symbol: &str) -> Arc<RwLock<OrderBook>> {
        let mut m = self.books.write().unwrap();
        m.entry(symbol.to_string())
            .or_insert_with(|| Arc::new(RwLock::new(OrderBook::new(symbol.to_string()))))
            .clone()
    }
}

#[derive(Deserialize)]
struct SubmitBody {
    order: Order,
}

#[derive(serde::Serialize)]
struct SubmitResponse {
    trades: Vec<Trade>,
    affected_orders: Vec<Order>,
    sequence: u64,
}

async fn submit(
    State(st): State<AppState>,
    Json(body): Json<SubmitBody>,
) -> Json<SubmitResponse> {
    let mut o = body.order;
    let sym = o.symbol.clone();
    let book = st.get_or_create(&sym);
    let now = Utc::now();
    let mut ob = book.write().unwrap();
    let (trades, affected) = ob.submit(&mut o, now);
    let seq = ob.sequence;
    Json(SubmitResponse {
        trades,
        affected_orders: affected,
        sequence: seq,
    })
}

#[derive(Deserialize)]
struct CancelBody {
    order_id: Uuid,
    symbol: String,
}

#[derive(serde::Serialize)]
struct CancelResponse {
    order: Option<Order>,
    sequence: u64,
}

async fn cancel(State(st): State<AppState>, Json(body): Json<CancelBody>) -> Json<CancelResponse> {
    let sym = body.symbol.to_uppercase();
    let book = st.get_or_create(&sym);
    let now = Utc::now();
    let mut ob = book.write().unwrap();
    let o = ob.cancel(body.order_id, now);
    let seq = ob.sequence;
    Json(CancelResponse {
        order: o,
        sequence: seq,
    })
}

#[derive(Deserialize)]
struct SnapQuery {
    depth: Option<usize>,
}

async fn snapshot(
    State(st): State<AppState>,
    Path(symbol): Path<String>,
    Query(q): Query<SnapQuery>,
) -> Json<BookSnapshot> {
    let sym = symbol.to_uppercase();
    let book = st.get_or_create(&sym);
    let mut ob = book.write().unwrap();
    let depth = q.depth.unwrap_or(50);
    Json(ob.snapshot(depth))
}

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
        .init();

    let st = AppState {
        books: Arc::new(RwLock::new(HashMap::new())),
    };

    let app = Router::new()
        .route("/v1/submit", post(submit))
        .route("/v1/cancel", post(cancel))
        .route("/v1/book/{symbol}", get(snapshot))
        .with_state(st)
        .layer(TraceLayer::new_for_http());

    let addr = std::env::var("MATCHER_ADDR").unwrap_or_else(|_| "0.0.0.0:9090".to_string());
    let listener = tokio::net::TcpListener::bind(&addr).await.unwrap();
    info!("matcher listening on {}", addr);
    axum::serve(listener, app).await.unwrap();
}
