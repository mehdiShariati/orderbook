module e-orderbook

go 1.18

require (
	github.com/go-chi/chi/v5 v5.0.10
	github.com/go-redis/redis/v8 v8.11.5
	github.com/google/uuid v1.3.0
	github.com/gorilla/websocket v1.5.0
	github.com/jackc/pgconn v1.14.3
	github.com/jackc/pgx/v4 v4.18.2
	github.com/nats-io/nats.go v1.49.0
	github.com/prometheus/client_golang v1.14.0
	github.com/shopspring/decimal v1.3.1
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.3 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgtype v1.14.0 // indirect
	github.com/jackc/puddle v1.3.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/nats-io/nats-server/v2 v2.12.6 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
)

replace golang.org/x/crypto => golang.org/x/crypto v0.17.0

replace github.com/nats-io/nats.go => github.com/nats-io/nats.go v1.16.0

replace golang.org/x/sys => golang.org/x/sys v0.15.0

replace github.com/nats-io/nkeys => github.com/nats-io/nkeys v0.3.0
