package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Collector interface {
	IncIngest()
	IncDropped(reason string)
	ObserveIngestLagMS(ms int64)
	SetSnapshotAgeMS(ms int64)
	IncTopRead()
	ObserveTopReadLatencyMS(ms int64)
	RenderPrometheus() string
}

type Noop struct{}

func (Noop) IncIngest()                      {}
func (Noop) IncDropped(_ string)             {}
func (Noop) ObserveIngestLagMS(_ int64)      {}
func (Noop) SetSnapshotAgeMS(_ int64)        {}
func (Noop) IncTopRead()                     {}
func (Noop) ObserveTopReadLatencyMS(_ int64) {}
func (Noop) RenderPrometheus() string        { return "" }

type PrometheusCollector struct {
	mu sync.RWMutex

	ingestTotal uint64
	dropped     map[string]uint64

	ingestLagLastMS int64
	ingestLagSumMS  int64
	ingestLagCount  uint64

	snapshotAgeMS int64

	topReadTotal         uint64
	topReadLatencyLastMS int64
	topReadLatencySumMS  int64
	topReadLatencyCount  uint64
}

func NewPrometheusCollector() *PrometheusCollector {
	return &PrometheusCollector{
		dropped: make(map[string]uint64),
	}
}

func (c *PrometheusCollector) IncIngest() {
	c.mu.Lock()
	c.ingestTotal++
	c.mu.Unlock()
}

func (c *PrometheusCollector) IncDropped(reason string) {
	c.mu.Lock()
	if reason == "" {
		reason = "UNKNOWN_REJECT_REASON"
	}
	c.dropped[reason]++
	c.mu.Unlock()
}

func (c *PrometheusCollector) ObserveIngestLagMS(ms int64) {
	ms = clampNonNegative(ms)
	c.mu.Lock()
	c.ingestLagLastMS = ms
	c.ingestLagSumMS += ms
	c.ingestLagCount++
	c.mu.Unlock()
}

func (c *PrometheusCollector) SetSnapshotAgeMS(ms int64) {
	ms = clampNonNegative(ms)
	c.mu.Lock()
	c.snapshotAgeMS = ms
	c.mu.Unlock()
}

func (c *PrometheusCollector) IncTopRead() {
	c.mu.Lock()
	c.topReadTotal++
	c.mu.Unlock()
}

func (c *PrometheusCollector) ObserveTopReadLatencyMS(ms int64) {
	ms = clampNonNegative(ms)
	c.mu.Lock()
	c.topReadLatencyLastMS = ms
	c.topReadLatencySumMS += ms
	c.topReadLatencyCount++
	c.mu.Unlock()
}

func (c *PrometheusCollector) RenderPrometheus() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var b strings.Builder

	b.WriteString("# HELP trends_ingest_events_total Total number of successfully ingested events.\n")
	b.WriteString("# TYPE trends_ingest_events_total counter\n")
	b.WriteString(fmt.Sprintf("trends_ingest_events_total %d\n", c.ingestTotal))

	b.WriteString("# HELP trends_dropped_events_total Total number of dropped events by reason.\n")
	b.WriteString("# TYPE trends_dropped_events_total counter\n")
	keys := make([]string, 0, len(c.dropped))
	for reason := range c.dropped {
		keys = append(keys, reason)
	}
	sort.Strings(keys)
	for _, reason := range keys {
		b.WriteString(fmt.Sprintf("trends_dropped_events_total{reason=%q} %d\n", escapeLabel(reason), c.dropped[reason]))
	}

	b.WriteString("# HELP trends_ingest_lag_ms_last Last observed ingest lag in milliseconds.\n")
	b.WriteString("# TYPE trends_ingest_lag_ms_last gauge\n")
	b.WriteString(fmt.Sprintf("trends_ingest_lag_ms_last %d\n", c.ingestLagLastMS))

	b.WriteString("# HELP trends_ingest_lag_ms_avg Average ingest lag in milliseconds.\n")
	b.WriteString("# TYPE trends_ingest_lag_ms_avg gauge\n")
	b.WriteString(fmt.Sprintf("trends_ingest_lag_ms_avg %.2f\n", avg(c.ingestLagSumMS, c.ingestLagCount)))

	b.WriteString("# HELP trends_snapshot_age_ms Current top snapshot age in milliseconds.\n")
	b.WriteString("# TYPE trends_snapshot_age_ms gauge\n")
	b.WriteString(fmt.Sprintf("trends_snapshot_age_ms %d\n", c.snapshotAgeMS))

	b.WriteString("# HELP trends_top_read_requests_total Total number of top read requests.\n")
	b.WriteString("# TYPE trends_top_read_requests_total counter\n")
	b.WriteString(fmt.Sprintf("trends_top_read_requests_total %d\n", c.topReadTotal))

	b.WriteString("# HELP trends_top_read_latency_ms_last Last observed top read latency in milliseconds.\n")
	b.WriteString("# TYPE trends_top_read_latency_ms_last gauge\n")
	b.WriteString(fmt.Sprintf("trends_top_read_latency_ms_last %d\n", c.topReadLatencyLastMS))

	b.WriteString("# HELP trends_top_read_latency_ms_avg Average top read latency in milliseconds.\n")
	b.WriteString("# TYPE trends_top_read_latency_ms_avg gauge\n")
	b.WriteString(fmt.Sprintf("trends_top_read_latency_ms_avg %.2f\n", avg(c.topReadLatencySumMS, c.topReadLatencyCount)))

	return b.String()
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func clampNonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func avg(sum int64, count uint64) float64 {
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}
