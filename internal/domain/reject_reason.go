package domain

type RejectReason string

const (
	RejectReasonUnknown          RejectReason = "UNKNOWN_REJECT_REASON"
	RejectReasonInvalidJSON      RejectReason = "INVALID_JSON"
	RejectReasonSchemaValidation RejectReason = "SCHEMA_VALIDATION_FAILED"
	RejectReasonMappingFailed    RejectReason = "MAPPING_FAILED"
	RejectReasonInvalidQuery     RejectReason = "INVALID_QUERY"
	RejectReasonDuplicateEventID RejectReason = "DUPLICATE_EVENT_ID"

	RejectReasonAntiFraudMissingIdentifiers RejectReason = "ANTI_FRAUD_MISSING_IDENTIFIERS"
	RejectReasonAntiFraudIPRateLimit        RejectReason = "ANTI_FRAUD_IP_RATE_LIMIT"
	RejectReasonAntiFraudIPBlocked          RejectReason = "ANTI_FRAUD_IP_BLOCKED"
	RejectReasonAntiFraudContributionCap    RejectReason = "ANTI_FRAUD_CONTRIBUTION_CAP"
	RejectReasonAntiFraudSessionEntropy     RejectReason = "ANTI_FRAUD_SESSION_ENTROPY"
	RejectReasonAntiFraudDegraded           RejectReason = "ANTI_FRAUD_DEGRADED"
)

type RejectError struct {
	Reason RejectReason
	Err    error
}

func (e *RejectError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return string(e.Reason)
	}
	return string(e.Reason) + ": " + e.Err.Error()
}

func (e *RejectError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
