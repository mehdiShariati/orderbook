// Stress tool: sustained POST /orders load against a running API.
// See docs/stress.md for methodology and how to interpret output.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	base := flag.String("url", "http://localhost:8080", "API base URL (no trailing slash)")
	workers := flag.Int("c", 32, "concurrent workers (used when -rate=0)")
	duration := flag.Duration("z", 30*time.Second, "how long to run")
	rate := flag.Int("rate", 0, "target requests/sec across all workers (0 = each worker runs as fast as possible)")
	symbol := flag.String("symbol", "STRESS-USD", "order symbol")
	timeout := flag.Duration("timeout", 10*time.Second, "per-request HTTP timeout")
	flag.Parse()

	baseURL := strings.TrimRight(*base, "/")
	if *duration <= 0 {
		fmt.Fprintln(os.Stderr, "-z must be positive")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	client := &http.Client{Timeout: *timeout}

	var (
		okCount   uint64
		errCount  uint64
		http429   uint64
		orderSeq  uint64
		latMu     sync.Mutex
		latencies []time.Duration
	)

	record := func(d time.Duration, statusCode int, err error) {
		if err != nil {
			atomic.AddUint64(&errCount, 1)
			return
		}
		if statusCode == 429 {
			atomic.AddUint64(&http429, 1)
		}
		if statusCode >= 200 && statusCode < 300 {
			atomic.AddUint64(&okCount, 1)
		} else {
			atomic.AddUint64(&errCount, 1)
		}
		latMu.Lock()
		latencies = append(latencies, d)
		latMu.Unlock()
	}

	start := time.Now()

	if *rate > 0 {
		// Steady QPS: one goroutine emits ticks, worker pool drains.
		interval := time.Duration(float64(time.Second) / float64(*rate))
		if interval < time.Microsecond {
			interval = time.Microsecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		jobs := make(chan struct{}, *rate*2)
		var wg sync.WaitGroup
		for i := 0; i < *workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					case _, ok := <-jobs:
						if !ok {
							return
						}
						doOne(client, baseURL, *symbol, &orderSeq, record)
					}
				}
			}()
		}
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					select {
					case jobs <- struct{}{}:
					default:
					}
				}
			}
		}()
		<-ctx.Done()
		close(jobs)
		wg.Wait()
	} else {
		var wg sync.WaitGroup
		for i := 0; i < *workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						doOne(client, baseURL, *symbol, &orderSeq, record)
					}
				}
			}()
		}
		wg.Wait()
	}

	elapsed := time.Since(start)
	printReport(elapsed, okCount, errCount, http429, latencies)
}

func doOne(client *http.Client, baseURL, symbol string, orderSeq *uint64, record func(time.Duration, int, error)) {
	n := atomic.AddUint64(orderSeq, 1)
	body := fmt.Sprintf(
		`{"user_id":"stress-%d","symbol":%q,"side":"buy","type":"limit","price":"100","quantity":"0.001"}`,
		n, symbol,
	)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/orders", bytes.NewReader([]byte(body)))
	if err != nil {
		record(0, 0, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	t0 := time.Now()
	resp, err := client.Do(req)
	d := time.Since(t0)
	if err != nil {
		record(d, 0, err)
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	record(d, resp.StatusCode, nil)
}

func printReport(elapsed time.Duration, okCount, errCount, http429 uint64, latencies []time.Duration) {
	fmt.Printf("duration: %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("2xx:      %d\n", okCount)
	fmt.Printf("errors:   %d (non-2xx or transport failure)\n", errCount)
	if http429 > 0 {
		fmt.Printf("429:      %d (rate limited — raise RATE_LIMIT_PER_MIN or see docs/stress.md)\n", http429)
	}
	total := okCount + errCount
	if total > 0 {
		fmt.Printf("rps:      %.1f (all responses / duration)\n", float64(total)/elapsed.Seconds())
	}
	if len(latencies) == 0 {
		return
	}
	ns := make([]int64, len(latencies))
	for i, d := range latencies {
		ns[i] = d.Nanoseconds()
	}
	sort.Slice(ns, func(i, j int) bool { return ns[i] < ns[j] })
	p := func(q float64) time.Duration {
		idx := int(float64(len(ns)-1) * q)
		if idx < 0 {
			idx = 0
		}
		return time.Duration(ns[idx])
	}
	fmt.Printf("latency (2xx + error HTTP, not transport failures): n=%d\n", len(ns))
	fmt.Printf("  min: %s\n", time.Duration(ns[0]).Round(time.Microsecond))
	fmt.Printf("  p50: %s\n", p(0.50).Round(time.Microsecond))
	fmt.Printf("  p95: %s\n", p(0.95).Round(time.Microsecond))
	fmt.Printf("  p99: %s\n", p(0.99).Round(time.Microsecond))
	fmt.Printf("  max: %s\n", time.Duration(ns[len(ns)-1]).Round(time.Microsecond))
}
