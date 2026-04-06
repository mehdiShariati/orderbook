package ws

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Hub fans out JSON messages to WebSocket clients subscribed by symbol.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[*websocket.Conn]struct{})}
}

func (h *Hub) add(symbol string, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	m := h.subs[symbol]
	if m == nil {
		m = make(map[*websocket.Conn]struct{})
		h.subs[symbol] = m
	}
	m[c] = struct{}{}
}

func (h *Hub) remove(symbol string, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m := h.subs[symbol]; m != nil {
		delete(m, c)
		if len(m) == 0 {
			delete(h.subs, symbol)
		}
	}
}

// Broadcast sends a typed envelope to all connections for the symbol.
func (h *Hub) Broadcast(symbol, typ string, data any) {
	raw, err := json.Marshal(map[string]any{"type": typ, "symbol": symbol, "data": data})
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.subs[symbol] {
		_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_ = c.WriteMessage(websocket.TextMessage, raw)
	}
}

// ServeMarket upgrades GET /ws/market?symbol=FOO and keeps the connection subscribed.
func ServeMarket(h *Hub, w http.ResponseWriter, r *http.Request) {
	sym := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if sym == "" {
		http.Error(w, "symbol query parameter required", http.StatusBadRequest)
		return
	}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.add(sym, c)
	defer h.remove(sym, c)
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			break
		}
	}
}
