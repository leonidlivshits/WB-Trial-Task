package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

const (
	defaultTopN         = 10
	minTopN             = 1
	maxTopN             = 100
	defaultWindowSecond = 300
)

var errInvalidTopN = errors.New("invalid top n")

func (a *API) GetTop(w http.ResponseWriter, r *http.Request) {
	startedAt := a.now()

	if a.top == nil {
		writeError(w, http.StatusServiceUnavailable, codeUnavailable, msgTopUnavailable)
		return
	}

	n, err := parseTopN(r.URL.Query().Get("n"))
	if err != nil {
		writeError(w, http.StatusBadRequest, codeInvalidArgument, msgInvalidTopN)
		return
	}

	snap, err := a.top.GetTop(r.Context(), n)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, codeUnavailable, msgTopUnavailable)
		return
	}

	windowSeconds := snap.WindowSeconds
	if windowSeconds <= 0 {
		windowSeconds = defaultWindowSecond
	}
	freshnessLagMS := snapshotLagMS(a.now().UnixMilli(), snap.GeneratedAtMS)

	if a.metrics != nil {
		a.metrics.IncTopRead()
		a.metrics.SetSnapshotAgeMS(freshnessLagMS)
		a.metrics.ObserveTopReadLatencyMS(nonNegativeMS(a.now().Sub(startedAt).Milliseconds()))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at_ms":  snap.GeneratedAtMS,
		"window_seconds":   windowSeconds,
		"n":                n,
		"items":            snap.Items,
		"freshness_lag_ms": freshnessLagMS,
	})
}

func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	snapshotReady := a.top != nil
	var snapGeneratedAtMS int64
	if snapshotReady {
		snap, err := a.top.GetTop(r.Context(), 1)
		snapshotReady = err == nil
		if err == nil {
			snapGeneratedAtMS = snap.GeneratedAtMS
		}
	}

	status := "ok"
	if !snapshotReady {
		status = "degraded"
	}
	if a.metrics != nil {
		ageMS := snapshotLagMS(a.now().UnixMilli(), snapGeneratedAtMS)
		if snapshotReady && snapGeneratedAtMS > 0 {
			a.metrics.SetSnapshotAgeMS(ageMS)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":          status,
		"version":         a.serviceVersion,
		"kafka_connected": a.kafkaConnected,
		"snapshot_ready":  snapshotReady,
	})
}

func (a *API) Metrics(w http.ResponseWriter, _ *http.Request) {
	if a.metrics == nil {
		writeError(w, http.StatusServiceUnavailable, codeUnavailable, msgMetricsUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(a.metrics.RenderPrometheus()))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{
		"code":    code,
		"message": message,
	})
}

func parseTopN(raw string) (int, error) {
	if raw == "" {
		return defaultTopN, nil
	}

	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errInvalidTopN
	}
	if n < minTopN || n > maxTopN {
		return 0, errInvalidTopN
	}
	return n, nil
}

func snapshotLagMS(nowMS, generatedAtMS int64) int64 {
	if generatedAtMS <= 0 {
		return 0
	}
	return nonNegativeMS(nowMS - generatedAtMS)
}

func nonNegativeMS(ms int64) int64 {
	if ms < 0 {
		return 0
	}
	return ms
}
