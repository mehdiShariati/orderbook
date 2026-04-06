package middleware

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisRateLimit returns middleware that limits requests per minute per client IP (X-Forwarded-For aware).
func RedisRateLimit(rdb *redis.Client, perMinute int) func(http.Handler) http.Handler {
	if rdb == nil || perMinute <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			key := "rl:orderbook:" + ip
			ctx := r.Context()
			n, err := rdb.Incr(ctx, key).Result()
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			if n == 1 {
				_ = rdb.Expire(ctx, key, time.Minute)
			}
			if n > int64(perMinute) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	if x := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); x != "" {
		if i := strings.IndexByte(x, ','); i >= 0 {
			return strings.TrimSpace(x[:i])
		}
		return x
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
