package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"wbtrialtask/internal/domain"
)

type stubValidator struct {
	evt domain.SearchEvent
	err error
}

func (s stubValidator) ValidateAndMap(_ []byte) (domain.SearchEvent, error) {
	return s.evt, s.err
}

type stubTypedHandler struct {
	called bool
	err    error
}

func (s *stubTypedHandler) Handle(_ context.Context, _ domain.SearchEvent) error {
	s.called = true
	return s.err
}

type stubRejectReporter struct {
	calls  int
	reason domain.RejectReason
}

func (s *stubRejectReporter) ReportReject(reason domain.RejectReason, _ error) {
	s.calls++
	s.reason = reason
}

type noopNormalizer struct{}

func (noopNormalizer) Normalize(raw string) (string, error) {
	return raw, nil
}

type allowAntiFraud struct{}

func (allowAntiFraud) Evaluate(_ domain.SearchEvent, _ string, _ time.Time) *domain.RejectError {
	return nil
}

type noopAggregator struct{}

func (noopAggregator) Add(_ string, _ float64, _ int64) {}

type noopClock struct{}

func (noopClock) Now() time.Time {
	return time.Now().UTC()
}

type noopDedup struct{}

func (noopDedup) SeenOrAdd(_ string, _ time.Time) bool {
	return false
}

type stubIngestMetrics struct {
	ingestCalls int
	lagCalls    int
	lagMS       int64
}

func (s *stubIngestMetrics) IncIngest() {
	s.ingestCalls++
}

func (s *stubIngestMetrics) ObserveIngestLagMS(ms int64) {
	s.lagCalls++
	s.lagMS = ms
}

func TestRuntimeHandler_DropsInvalidPayloadWithReason(t *testing.T) {
	t.Parallel()

	reporter := &stubRejectReporter{}
	typed := &stubTypedHandler{}
	handler := NewRuntimeHandler(
		stubValidator{
			err: &domain.RejectError{
				Reason: domain.RejectReasonSchemaValidation,
				Err:    errors.New("schema invalid"),
			},
		},
		typed,
		reporter,
	)

	err := handler.HandleRaw(context.Background(), []byte(`{}`))
	if err != nil {
		t.Errorf("expected nil error for dropped payload, got %v", err)
	}
	if typed.called {
		t.Errorf("typed handler should not be called for rejected payload")
	}
	if reporter.calls != 1 {
		t.Errorf("unexpected reporter calls: got %d want %d", reporter.calls, 1)
	}
	if reporter.reason != domain.RejectReasonSchemaValidation {
		t.Errorf("unexpected reason: got %q want %q", reporter.reason, domain.RejectReasonSchemaValidation)
	}
}

func TestRuntimeHandler_PassesValidMappedEvent(t *testing.T) {
	t.Parallel()

	evt := domain.SearchEvent{
		EventID:       "event-1",
		QueryRaw:      "nike",
		IPHash:        "ip_valid_123",
		SessionIDHash: "s_valid_123",
	}

	typedIngest := NewHandler(Dependencies{
		Normalizer:   noopNormalizer{},
		AntiFraud:    allowAntiFraud{},
		Deduplicator: noopDedup{},
		Aggregator:   noopAggregator{},
		Clock:        noopClock{},
	})

	handler := NewRuntimeHandler(
		stubValidator{evt: evt},
		typedIngest,
		&stubRejectReporter{},
	)

	err := handler.HandleRaw(context.Background(), []byte(`{}`))
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRuntimeHandler_DropsTypedRejectWithReason(t *testing.T) {
	t.Parallel()

	reporter := &stubRejectReporter{}
	typed := &stubTypedHandler{
		err: &domain.RejectError{
			Reason: domain.RejectReasonDuplicateEventID,
			Err:    errors.New("duplicate"),
		},
	}
	handler := NewRuntimeHandler(
		stubValidator{
			evt: domain.SearchEvent{
				EventID:       "event-1",
				QueryRaw:      "nike",
				IPHash:        "ip_valid_123",
				SessionIDHash: "s_valid_123",
			},
		},
		typed,
		reporter,
	)

	err := handler.HandleRaw(context.Background(), []byte(`{}`))
	if err != nil {
		t.Errorf("expected nil error for dropped payload, got %v", err)
	}
	if !typed.called {
		t.Errorf("typed handler should be called for mapped payload")
	}
	if reporter.calls != 1 {
		t.Errorf("unexpected reporter calls: got %d want %d", reporter.calls, 1)
	}
	if reporter.reason != domain.RejectReasonDuplicateEventID {
		t.Errorf("unexpected reason: got %q want %q", reporter.reason, domain.RejectReasonDuplicateEventID)
	}
}

func TestRuntimeHandler_ReturnsTypedNonRejectError(t *testing.T) {
	t.Parallel()

	typedErr := errors.New("db unavailable")
	handler := NewRuntimeHandler(
		stubValidator{
			evt: domain.SearchEvent{
				EventID:       "event-1",
				QueryRaw:      "nike",
				IPHash:        "ip_valid_123",
				SessionIDHash: "s_valid_123",
			},
		},
		&stubTypedHandler{err: typedErr},
		&stubRejectReporter{},
	)

	err := handler.HandleRaw(context.Background(), []byte(`{}`))
	if !errors.Is(err, typedErr) {
		t.Errorf("expected typed error to bubble up, got %v", err)
	}
}

func TestRuntimeHandler_ReportsIngestMetricsOnSuccess(t *testing.T) {
	t.Parallel()

	metrics := &stubIngestMetrics{}
	evt := domain.SearchEvent{
		EventID:       "event-1",
		QueryRaw:      "nike",
		IPHash:        "ip_valid_123",
		SessionIDHash: "s_valid_123",
		IngestedAtMS:  1_000,
	}

	handler := NewRuntimeHandlerWithMetrics(
		stubValidator{evt: evt},
		&stubTypedHandler{},
		&stubRejectReporter{},
		metrics,
	)
	handler.now = func() time.Time {
		return time.UnixMilli(1_500).UTC()
	}

	err := handler.HandleRaw(context.Background(), []byte(`{}`))
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if metrics.ingestCalls != 1 {
		t.Errorf("unexpected ingest calls: got %d want %d", metrics.ingestCalls, 1)
	}
	if metrics.lagCalls != 1 {
		t.Errorf("unexpected lag calls: got %d want %d", metrics.lagCalls, 1)
	}
	if metrics.lagMS != 500 {
		t.Errorf("unexpected lag value: got %d want %d", metrics.lagMS, 500)
	}
}
