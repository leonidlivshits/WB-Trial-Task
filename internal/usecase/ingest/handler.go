package ingest

import (
	"context"
	"errors"
	"time"

	"wbtrialtask/internal/domain"
	"wbtrialtask/internal/ports"
)

type Normalizer interface {
	Normalize(raw string) (string, error)
}

type AntiFraud interface {
	Evaluate(evt domain.SearchEvent, normalizedQuery string, now time.Time) *domain.RejectError
}

type Deduplicator interface {
	SeenOrAdd(key string, now time.Time) bool
}

type Aggregator interface {
	Add(query string, score float64, ingestedAtMS int64)
}

type Dependencies struct {
	Normalizer   Normalizer
	AntiFraud    AntiFraud
	Deduplicator Deduplicator
	Aggregator   Aggregator
	Clock        ports.Clock
}

type Handler struct {
	deps Dependencies
}

var errHandlerNotConfigured = errors.New("ingest handler is not configured")

func NewHandler(deps Dependencies) *Handler {
	return &Handler{deps: deps}
}

func (h *Handler) Handle(_ context.Context, evt domain.SearchEvent) error {
	if !h.isConfigured() {
		return errHandlerNotConfigured
	}

	now := h.deps.Clock.Now().UTC()

	if h.deps.Deduplicator.SeenOrAdd(evt.EventID, now) {
		return reject(domain.RejectReasonDuplicateEventID, errors.New("duplicate event id"))
	}

	query, err := h.deps.Normalizer.Normalize(evt.QueryRaw)
	if err != nil {
		return reject(domain.RejectReasonInvalidQuery, err)
	}

	if rejectErr := h.deps.AntiFraud.Evaluate(evt, query, now); rejectErr != nil {
		return rejectErr
	}

	ingestedAtMS := evt.IngestedAtMS
	if ingestedAtMS <= 0 {
		ingestedAtMS = now.UnixMilli()
	}

	h.deps.Aggregator.Add(query, 1, ingestedAtMS)
	return nil
}

func (h *Handler) isConfigured() bool {
	return h.deps.Clock != nil &&
		h.deps.Normalizer != nil &&
		h.deps.AntiFraud != nil &&
		h.deps.Deduplicator != nil &&
		h.deps.Aggregator != nil
}

func reject(reason domain.RejectReason, err error) *domain.RejectError {
	return &domain.RejectError{
		Reason: reason,
		Err:    err,
	}
}
