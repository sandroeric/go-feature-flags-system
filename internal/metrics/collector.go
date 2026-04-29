package metrics

import (
	"sync/atomic"
	"time"
)

var latencyBuckets = []time.Duration{
	50 * time.Microsecond,
	100 * time.Microsecond,
	250 * time.Microsecond,
	500 * time.Microsecond,
	time.Millisecond,
	2 * time.Millisecond,
	5 * time.Millisecond,
	10 * time.Millisecond,
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
	2 * time.Second,
	5 * time.Second,
}

type Collector struct {
	evaluationCount   atomic.Uint64
	unknownFlagCount  atomic.Uint64
	syncSuccessCount  atomic.Uint64
	syncFailureCount  atomic.Uint64
	storeGeneration   atomic.Uint64
	storeVersion      atomic.Int64
	refreshDurationNS atomic.Int64
	latencyCount      atomic.Uint64
	latencySumNS      atomic.Uint64
	latencyBuckets    []atomic.Uint64
}

type Snapshot struct {
	EvaluationCount  uint64          `json:"evaluation_count"`
	UnknownFlagCount uint64          `json:"unknown_flag_count"`
	SyncSuccessCount uint64          `json:"sync_success_count"`
	SyncFailureCount uint64          `json:"sync_failure_count"`
	StoreGeneration  uint64          `json:"store_generation"`
	StoreVersion     int64           `json:"store_version"`
	RefreshDuration  string          `json:"refresh_duration"`
	EvaluateLatency  LatencySnapshot `json:"evaluate_latency"`
}

type LatencySnapshot struct {
	Count   uint64 `json:"count"`
	Average string `json:"average"`
	P50     string `json:"p50"`
	P95     string `json:"p95"`
	P99     string `json:"p99"`
}

func NewCollector() *Collector {
	return &Collector{
		latencyBuckets: make([]atomic.Uint64, len(latencyBuckets)+1),
	}
}

func (c *Collector) ObserveEvaluation(duration time.Duration, found bool, generation uint64, version int) {
	if c == nil {
		return
	}

	c.evaluationCount.Add(1)
	if !found {
		c.unknownFlagCount.Add(1)
	}
	c.storeGeneration.Store(generation)
	c.storeVersion.Store(int64(version))
	c.observeLatency(duration)
}

func (c *Collector) ObserveSync(success bool, duration time.Duration, generation uint64, version int) {
	if c == nil {
		return
	}

	if success {
		c.syncSuccessCount.Add(1)
		c.storeGeneration.Store(generation)
		c.storeVersion.Store(int64(version))
		c.refreshDurationNS.Store(duration.Nanoseconds())
		return
	}

	c.syncFailureCount.Add(1)
}

func (c *Collector) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{}
	}

	latencyCount := c.latencyCount.Load()
	latencySumNS := c.latencySumNS.Load()
	avg := time.Duration(0)
	if latencyCount > 0 {
		avg = time.Duration(latencySumNS / latencyCount)
	}

	return Snapshot{
		EvaluationCount:  c.evaluationCount.Load(),
		UnknownFlagCount: c.unknownFlagCount.Load(),
		SyncSuccessCount: c.syncSuccessCount.Load(),
		SyncFailureCount: c.syncFailureCount.Load(),
		StoreGeneration:  c.storeGeneration.Load(),
		StoreVersion:     c.storeVersion.Load(),
		RefreshDuration:  time.Duration(c.refreshDurationNS.Load()).String(),
		EvaluateLatency: LatencySnapshot{
			Count:   latencyCount,
			Average: avg.String(),
			P50:     c.quantileString(0.50),
			P95:     c.quantileString(0.95),
			P99:     c.quantileString(0.99),
		},
	}
}

func (c *Collector) observeLatency(duration time.Duration) {
	c.latencyCount.Add(1)
	c.latencySumNS.Add(uint64(duration.Nanoseconds()))

	for i, upperBound := range latencyBuckets {
		if duration <= upperBound {
			c.latencyBuckets[i].Add(1)
			return
		}
	}

	c.latencyBuckets[len(c.latencyBuckets)-1].Add(1)
}

func (c *Collector) quantileString(q float64) string {
	count := c.latencyCount.Load()
	if count == 0 {
		return "0s"
	}

	target := uint64(float64(count) * q)
	if target == 0 {
		target = 1
	}

	var seen uint64
	for i := range c.latencyBuckets {
		seen += c.latencyBuckets[i].Load()
		if seen >= target {
			if i >= len(latencyBuckets) {
				return ">" + latencyBuckets[len(latencyBuckets)-1].String()
			}
			return latencyBuckets[i].String()
		}
	}

	return ">" + latencyBuckets[len(latencyBuckets)-1].String()
}
