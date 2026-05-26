package metrics

import (
	"strings"
	"testing"
)

func TestPrometheusCollector_RenderContainsCoreMetrics(t *testing.T) {
	t.Parallel()

	collector := NewPrometheusCollector()
	collector.IncIngest()
	collector.IncIngest()
	collector.IncDropped("INVALID_QUERY")
	collector.ObserveIngestLagMS(120)
	collector.SetSnapshotAgeMS(350)
	collector.IncTopRead()
	collector.ObserveTopReadLatencyMS(7)

	text := collector.RenderPrometheus()

	expectedSnippets := []string{
		"trends_ingest_events_total 2",
		`trends_dropped_events_total{reason="INVALID_QUERY"} 1`,
		"trends_ingest_lag_ms_last 120",
		"trends_snapshot_age_ms 350",
		"trends_top_read_requests_total 1",
		"trends_top_read_latency_ms_last 7",
	}

	for _, snippet := range expectedSnippets {
		snippet := snippet
		if !strings.Contains(text, snippet) {
			t.Errorf("metrics output does not contain %q", snippet)
		}
	}
}

func TestPrometheusCollector_ClampsNegativeValues(t *testing.T) {
	t.Parallel()

	collector := NewPrometheusCollector()
	collector.ObserveIngestLagMS(-50)
	collector.SetSnapshotAgeMS(-20)
	collector.ObserveTopReadLatencyMS(-3)

	text := collector.RenderPrometheus()

	if !strings.Contains(text, "trends_ingest_lag_ms_last 0") {
		t.Errorf("ingest lag should be clamped to zero")
	}
	if !strings.Contains(text, "trends_snapshot_age_ms 0") {
		t.Errorf("snapshot age should be clamped to zero")
	}
	if !strings.Contains(text, "trends_top_read_latency_ms_last 0") {
		t.Errorf("top read latency should be clamped to zero")
	}
}
