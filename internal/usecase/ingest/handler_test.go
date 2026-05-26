package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"wbtrialtask/internal/domain"
)

type stubNormalizerForHandler struct {
	query   string
	err     error
	calls   int
	lastRaw string
}

func (s *stubNormalizerForHandler) Normalize(raw string) (string, error) {
	s.calls++
	s.lastRaw = raw
	if s.err != nil {
		return "", s.err
	}
	return s.query, nil
}

type stubAntiFraudForHandler struct {
	reject        *domain.RejectError
	calls         int
	lastQuery     string
	lastEventID   string
	lastEvaluated time.Time
}

func (s *stubAntiFraudForHandler) Evaluate(evt domain.SearchEvent, normalizedQuery string, now time.Time) *domain.RejectError {
	s.calls++
	s.lastEventID = evt.EventID
	s.lastQuery = normalizedQuery
	s.lastEvaluated = now
	return s.reject
}

type stubDedupForHandler struct {
	duplicate bool
	calls     int
	lastKey   string
	lastNow   time.Time
}

func (s *stubDedupForHandler) SeenOrAdd(key string, now time.Time) bool {
	s.calls++
	s.lastKey = key
	s.lastNow = now
	return s.duplicate
}

type stubAggregatorForHandler struct {
	calls        int
	lastQuery    string
	lastScore    float64
	lastIngestMS int64
}

func (s *stubAggregatorForHandler) Add(query string, score float64, ingestedAtMS int64) {
	s.calls++
	s.lastQuery = query
	s.lastScore = score
	s.lastIngestMS = ingestedAtMS
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

func TestHandler_HandleRejectsDuplicateEvent(t *testing.T) {
	t.Parallel()

	norm := &stubNormalizerForHandler{query: "nike air"}
	fraud := &stubAntiFraudForHandler{}
	dedup := &stubDedupForHandler{duplicate: true}
	agg := &stubAggregatorForHandler{}

	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	handler := NewHandler(Dependencies{
		Normalizer:   norm,
		AntiFraud:    fraud,
		Deduplicator: dedup,
		Aggregator:   agg,
		Clock:        fixedClock{now: now},
	})

	err := handler.Handle(context.Background(), domain.SearchEvent{
		EventID:  "event-1",
		QueryRaw: "Nike Air",
	})

	var rejectErr *domain.RejectError
	if !errors.As(err, &rejectErr) {
		t.Errorf("expected reject error, got %v", err)
		return
	}
	if rejectErr.Reason != domain.RejectReasonDuplicateEventID {
		t.Errorf("unexpected reason: got %q want %q", rejectErr.Reason, domain.RejectReasonDuplicateEventID)
	}
	if norm.calls != 0 {
		t.Errorf("normalizer should not be called for duplicates, got %d calls", norm.calls)
	}
	if fraud.calls != 0 {
		t.Errorf("anti-fraud should not be called for duplicates, got %d calls", fraud.calls)
	}
	if agg.calls != 0 {
		t.Errorf("aggregator should not be called for duplicates, got %d calls", agg.calls)
	}
}

func TestHandler_HandleRejectsInvalidQuery(t *testing.T) {
	t.Parallel()

	norm := &stubNormalizerForHandler{err: domain.ErrInvalidQuery}
	fraud := &stubAntiFraudForHandler{}
	dedup := &stubDedupForHandler{}
	agg := &stubAggregatorForHandler{}

	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	handler := NewHandler(Dependencies{
		Normalizer:   norm,
		AntiFraud:    fraud,
		Deduplicator: dedup,
		Aggregator:   agg,
		Clock:        fixedClock{now: now},
	})

	err := handler.Handle(context.Background(), domain.SearchEvent{
		EventID:  "event-2",
		QueryRaw: "!",
	})

	var rejectErr *domain.RejectError
	if !errors.As(err, &rejectErr) {
		t.Errorf("expected reject error, got %v", err)
		return
	}
	if rejectErr.Reason != domain.RejectReasonInvalidQuery {
		t.Errorf("unexpected reason: got %q want %q", rejectErr.Reason, domain.RejectReasonInvalidQuery)
	}
	if fraud.calls != 0 {
		t.Errorf("anti-fraud should not run for invalid query, got %d calls", fraud.calls)
	}
	if agg.calls != 0 {
		t.Errorf("aggregator should not be called for invalid query, got %d calls", agg.calls)
	}
}

func TestHandler_HandleRejectsAntiFraudRule(t *testing.T) {
	t.Parallel()

	norm := &stubNormalizerForHandler{query: "nike air"}
	fraud := &stubAntiFraudForHandler{
		reject: &domain.RejectError{
			Reason: domain.RejectReasonAntiFraudContributionCap,
			Err:    errors.New("cap"),
		},
	}
	dedup := &stubDedupForHandler{}
	agg := &stubAggregatorForHandler{}

	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	handler := NewHandler(Dependencies{
		Normalizer:   norm,
		AntiFraud:    fraud,
		Deduplicator: dedup,
		Aggregator:   agg,
		Clock:        fixedClock{now: now},
	})

	err := handler.Handle(context.Background(), domain.SearchEvent{
		EventID:       "event-3",
		QueryRaw:      "Nike Air",
		IPHash:        "ip_abc12345",
		SessionIDHash: "s_qwe12345",
	})

	var rejectErr *domain.RejectError
	if !errors.As(err, &rejectErr) {
		t.Errorf("expected reject error, got %v", err)
		return
	}
	if rejectErr.Reason != domain.RejectReasonAntiFraudContributionCap {
		t.Errorf("unexpected reason: got %q want %q", rejectErr.Reason, domain.RejectReasonAntiFraudContributionCap)
	}
	if agg.calls != 0 {
		t.Errorf("aggregator should not be called for anti-fraud reject, got %d calls", agg.calls)
	}
}

func TestHandler_HandleAddsToAggregatorWithClockTimeWhenEventIngestTimeMissing(t *testing.T) {
	t.Parallel()

	norm := &stubNormalizerForHandler{query: "nike air"}
	fraud := &stubAntiFraudForHandler{}
	dedup := &stubDedupForHandler{}
	agg := &stubAggregatorForHandler{}

	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	handler := NewHandler(Dependencies{
		Normalizer:   norm,
		AntiFraud:    fraud,
		Deduplicator: dedup,
		Aggregator:   agg,
		Clock:        fixedClock{now: now},
	})

	err := handler.Handle(context.Background(), domain.SearchEvent{
		EventID:       "event-4",
		QueryRaw:      "Nike Air",
		IPHash:        "ip_abc12345",
		SessionIDHash: "s_qwe12345",
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if agg.calls != 1 {
		t.Errorf("unexpected aggregator calls: got %d want %d", agg.calls, 1)
	}
	if agg.lastQuery != "nike air" {
		t.Errorf("unexpected query: got %q want %q", agg.lastQuery, "nike air")
	}
	if agg.lastScore != 1 {
		t.Errorf("unexpected score: got %v want %v", agg.lastScore, 1.0)
	}
	if agg.lastIngestMS != now.UnixMilli() {
		t.Errorf("unexpected ingest time: got %d want %d", agg.lastIngestMS, now.UnixMilli())
	}
}
