package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"e-orderbook/internal/api"
	"e-orderbook/internal/events"
	"e-orderbook/internal/matching"
	"e-orderbook/internal/matching/rustclient"
	apimw "e-orderbook/internal/middleware"
	"e-orderbook/internal/observability"
	"e-orderbook/internal/store"
	"e-orderbook/internal/ws"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"
	"github.com/go-redis/redis/v8"
)

func main() {
	log.SetFlags(0)

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/orderbook?sslmode=disable"
	}

	ctx := context.Background()
	st, err := store.New(ctx, dsn)
	if err != nil {
		log.Printf(`{"level":"error","msg":"db","err":%q}`, err.Error())
		os.Exit(1)
	}
	defer st.Close()

	if err := store.Migrate(ctx, st.Pool()); err != nil {
		log.Printf(`{"level":"error","msg":"migrate","err":%q}`, err.Error())
		os.Exit(1)
	}

	var eng matching.Engine
	if u := os.Getenv("MATCHER_URL"); u != "" {
		c, err := rustclient.New(u)
		if err != nil {
			log.Printf(`{"level":"error","msg":"rust matcher client","err":%q}`, err.Error())
			os.Exit(1)
		}
		eng = c
		log.Printf(`{"level":"info","msg":"using Rust matcher","url":%q}`, u)
	} else {
		eng = matching.NewManager()
		log.Printf(`{"level":"info","msg":"using in-process Go matcher"}`)
	}

	var nc *nats.Conn
	if nurl := os.Getenv("NATS_URL"); nurl != "" {
		nc, err = connectNATS(nurl)
		if err != nil {
			log.Printf(`{"level":"error","msg":"nats","err":%q}`, err.Error())
			os.Exit(1)
		}
		defer nc.Close()
		log.Printf(`{"level":"info","msg":"connected to NATS"}`)
	}

	var rdb *redis.Client
	if ru := os.Getenv("REDIS_URL"); ru != "" {
		opt, err := redis.ParseURL(ru)
		if err != nil {
			log.Printf(`{"level":"error","msg":"redis url","err":%q}`, err.Error())
			os.Exit(1)
		}
		rdb = redis.NewClient(opt)
		defer func() { _ = rdb.Close() }()
	}

	bus := events.New(nc, st)
	var hub *ws.Hub
	if os.Getenv("WEBSOCKET_ENABLED") != "false" {
		hub = ws.NewHub()
	}

	h := api.NewHandler(st, eng, bus, hub, rdb, nc)

	ratePerMin := 600
	if s := os.Getenv("RATE_LIMIT_PER_MIN"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			ratePerMin = n
		}
	}

	r := chi.NewRouter()
	r.Use(observability.HTTPMiddleware)
	r.Use(apimw.RedisRateLimit(rdb, ratePerMin))

	r.Handle("/metrics", observability.MetricsHandler())
	r.Get("/health/live", h.HealthLive)
	r.Get("/health/ready", h.HealthReady)
	r.Get("/ws/market", h.MarketWebSocket)

	r.Post("/orders", h.CreateOrder)
	r.Get("/orders/{id}", h.GetOrder)
	r.Delete("/orders/{id}", h.CancelOrder)
	r.Get("/book/{symbol}", h.GetBook)
	r.Get("/trades/{symbol}", h.GetTrades)

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf(`{"level":"info","msg":"listening","addr":%q}`, addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf(`{"level":"error","msg":"server","err":%q}`, err.Error())
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf(`{"level":"warn","msg":"shutdown","err":%q}`, err.Error())
	}
}

func connectNATS(url string) (*nats.Conn, error) {
	var last error
	for i := 0; i < 60; i++ {
		nc, err := nats.Connect(url)
		if err == nil {
			return nc, nil
		}
		last = err
		time.Sleep(time.Second)
	}
	return nil, last
}
