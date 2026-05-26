package contracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"wbtrialtask/internal/domain"
)

type SearchEventValidator struct {
	schema *jsonschema.Schema
}

var errEmptyPayload = errors.New("payload is empty")

type searchEventPayload struct {
	SchemaVersion     int     `json:"schema_version"`
	EventType         string  `json:"event_type"`
	EventID           string  `json:"event_id"`
	IngestedAtMS      int64   `json:"ingested_at_ms"`
	EventTimeMS       int64   `json:"event_time_ms"`
	QueryRaw          string  `json:"query_raw"`
	Source            string  `json:"source"`
	Platform          string  `json:"platform"`
	AppVersion        string  `json:"app_version"`
	IPHash            string  `json:"ip_hash"`
	SessionIDHash     string  `json:"session_id_hash"`
	AuthState         string  `json:"auth_state"`
	UserIDHash        *string `json:"user_id_hash"`
	IsTestTraffic     bool    `json:"is_test_traffic"`
	IsReplay          bool    `json:"is_replay"`
	ProducerRegion    string  `json:"producer_region"`
	NormalizerVersion string  `json:"normalizer_version"`
}

func NewSearchEventValidator(schemaPath string) (*SearchEventValidator, error) {
	absPath, err := filepath.Abs(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("resolve schema path: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	schema, err := compiler.Compile(absPath)
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}

	return &SearchEventValidator{schema: schema}, nil
}

func (v *SearchEventValidator) ValidateAndMap(payload []byte) (domain.SearchEvent, error) {
	if len(payload) == 0 {
		return domain.SearchEvent{}, reject(domain.RejectReasonInvalidJSON, errEmptyPayload)
	}

	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		return domain.SearchEvent{}, reject(domain.RejectReasonInvalidJSON, err)
	}

	if err := v.schema.Validate(instance); err != nil {
		return domain.SearchEvent{}, reject(domain.RejectReasonSchemaValidation, err)
	}

	var input searchEventPayload
	if err := json.Unmarshal(payload, &input); err != nil {
		return domain.SearchEvent{}, reject(domain.RejectReasonMappingFailed, err)
	}

	return domain.SearchEvent{
		SchemaVersion:     input.SchemaVersion,
		EventType:         input.EventType,
		EventID:           input.EventID,
		IngestedAtMS:      input.IngestedAtMS,
		EventTimeMS:       input.EventTimeMS,
		QueryRaw:          input.QueryRaw,
		Source:            input.Source,
		Platform:          input.Platform,
		AppVersion:        input.AppVersion,
		IPHash:            input.IPHash,
		SessionIDHash:     input.SessionIDHash,
		AuthState:         input.AuthState,
		UserIDHash:        input.UserIDHash,
		IsTestTraffic:     input.IsTestTraffic,
		IsReplay:          input.IsReplay,
		ProducerRegion:    input.ProducerRegion,
		NormalizerVersion: input.NormalizerVersion,
	}, nil
}

func reject(reason domain.RejectReason, err error) error {
	return &domain.RejectError{
		Reason: reason,
		Err:    err,
	}
}
