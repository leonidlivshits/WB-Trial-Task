package ingest

import (
	"context"
	"errors"
	"log"
	"time"

	"wbtrialtask/internal/domain"
)

type ContractValidator interface {
	ValidateAndMap(payload []byte) (domain.SearchEvent, error)
}

type RejectReporter interface {
	ReportReject(reason domain.RejectReason, err error)
}

type RuntimeHandler struct {
	validator      ContractValidator
	handler        TypedHandler
	rejectReporter RejectReporter
	metrics        IngestMetrics
	now            func() time.Time
}

type TypedHandler interface {
	Handle(ctx context.Context, evt domain.SearchEvent) error
}

type IngestMetrics interface {
	IncIngest()
	ObserveIngestLagMS(ms int64)
}

func NewRuntimeHandler(validator ContractValidator, handler TypedHandler, rejectReporter RejectReporter) *RuntimeHandler {
	return NewRuntimeHandlerWithMetrics(validator, handler, rejectReporter, nil)
}

func NewRuntimeHandlerWithMetrics(
	validator ContractValidator,
	handler TypedHandler,
	rejectReporter RejectReporter,
	metrics IngestMetrics,
) *RuntimeHandler {
	return &RuntimeHandler{
		validator:      validator,
		handler:        handler,
		rejectReporter: rejectReporter,
		metrics:        metrics,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (h *RuntimeHandler) HandleRaw(ctx context.Context, payload []byte) error {
	if h.validator == nil || h.handler == nil {
		return errors.New("runtime handler is not configured")
	}

	evt, err := h.validator.ValidateAndMap(payload)
	if err != nil {
		h.reportReject(err)
		return nil
	}

	err = h.handler.Handle(ctx, evt)
	if err != nil {
		if isRejectError(err) {
			h.reportReject(err)
			return nil
		}
		return err
	}

	if h.metrics != nil {
		h.metrics.IncIngest()
		if evt.IngestedAtMS > 0 {
			lagMS := h.now().UnixMilli() - evt.IngestedAtMS
			if lagMS < 0 {
				lagMS = 0
			}
			h.metrics.ObserveIngestLagMS(lagMS)
		}
	}

	return nil
}

func (h *RuntimeHandler) reportReject(err error) {
	reason := rejectReason(err)

	if h.rejectReporter != nil {
		h.rejectReporter.ReportReject(reason, err)
		return
	}
	log.Printf("event rejected reason=%s err=%v", reason, err)
}

func isRejectError(err error) bool {
	var rejectErr *domain.RejectError
	return errors.As(err, &rejectErr) && rejectErr != nil
}

func rejectReason(err error) domain.RejectReason {
	var rejectErr *domain.RejectError
	if errors.As(err, &rejectErr) && rejectErr != nil {
		return rejectErr.Reason
	}
	return domain.RejectReasonUnknown
}
