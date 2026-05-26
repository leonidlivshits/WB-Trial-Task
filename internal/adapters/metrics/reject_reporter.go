package metrics

import (
	"log"

	"wbtrialtask/internal/domain"
)

type RejectReporter struct {
	collector Collector
}

func NewRejectReporter(collector Collector) *RejectReporter {
	return &RejectReporter{collector: collector}
}

func (r *RejectReporter) ReportReject(reason domain.RejectReason, err error) {
	if r.collector != nil {
		r.collector.IncDropped(string(reason))
	}
	log.Printf("payload rejected reason=%s err=%v", reason, err)
}
