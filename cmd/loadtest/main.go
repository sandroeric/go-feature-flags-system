package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type evaluateRequest struct {
	FlagKey string          `json:"flag_key"`
	Context evaluateContext `json:"context"`
}

type evaluateContext struct {
	UserID  string `json:"user_id"`
	Country string `json:"country,omitempty"`
}

func main() {
	var (
		baseURL     = flag.String("base-url", "http://localhost:8080", "base URL of the API")
		flagKey     = flag.String("flag-key", "checkout_flow", "flag key to evaluate")
		requests    = flag.Int("requests", 10000, "number of evaluation requests")
		concurrency = flag.Int("concurrency", 50, "number of concurrent workers")
		userPrefix  = flag.String("user-prefix", "loadtest-user", "prefix used for generated user IDs")
		country     = flag.String("country", "BR", "country used in evaluation context")
		timeout     = flag.Duration("timeout", 5*time.Second, "per-request timeout")
	)
	flag.Parse()

	if *requests <= 0 {
		fmt.Fprintln(os.Stderr, "requests must be positive")
		os.Exit(1)
	}
	if *concurrency <= 0 {
		fmt.Fprintln(os.Stderr, "concurrency must be positive")
		os.Exit(1)
	}

	client := &http.Client{Timeout: *timeout}
	latencies := make([]time.Duration, *requests)

	jobs := make(chan int)
	var failures atomic.Int64
	var wg sync.WaitGroup

	start := time.Now()
	for worker := 0; worker < *concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				reqBody, err := json.Marshal(evaluateRequest{
					FlagKey: *flagKey,
					Context: evaluateContext{
						UserID:  fmt.Sprintf("%s-%d", *userPrefix, i),
						Country: *country,
					},
				})
				if err != nil {
					failures.Add(1)
					continue
				}

				req, err := http.NewRequest(http.MethodPost, *baseURL+"/evaluate", bytes.NewReader(reqBody))
				if err != nil {
					failures.Add(1)
					continue
				}
				req.Header.Set("Content-Type", "application/json")

				reqStart := time.Now()
				resp, err := client.Do(req)
				latencies[i] = time.Since(reqStart)
				if err != nil {
					failures.Add(1)
					continue
				}

				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					failures.Add(1)
				}
			}
		}()
	}

	for i := 0; i < *requests; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	total := time.Since(start)

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	successes := *requests - int(failures.Load())
	fmt.Printf("load test finished\n")
	fmt.Printf("requests=%d successes=%d failures=%d concurrency=%d total=%s\n", *requests, successes, failures.Load(), *concurrency, total)
	if successes == 0 {
		os.Exit(1)
	}

	fmt.Printf("throughput=%.2f req/s\n", float64(successes)/total.Seconds())
	fmt.Printf("latency p50=%s p95=%s p99=%s max=%s\n",
		quantile(latencies, 0.50),
		quantile(latencies, 0.95),
		quantile(latencies, 0.99),
		latencies[len(latencies)-1],
	)
}

func quantile(latencies []time.Duration, q float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	index := int(float64(len(latencies)-1) * q)
	if index < 0 {
		index = 0
	}
	if index >= len(latencies) {
		index = len(latencies) - 1
	}
	return latencies[index]
}
