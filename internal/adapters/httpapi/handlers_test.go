package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"wbtrialtask/internal/domain"
)

type stubTopQuery struct {
	snapshot domain.TopSnapshot
	err      error
	lastN    int
}

func (s *stubTopQuery) GetTop(_ context.Context, n int) (domain.TopSnapshot, error) {
	s.lastN = n
	return s.snapshot, s.err
}

type stubStopListCommands struct {
	words   []string
	version int64
	listErr error

	addVersion int64
	addErr     error

	removeVersion int64
	removeErr     error
}

func (s *stubStopListCommands) List(_ context.Context) ([]string, int64, error) {
	return s.words, s.version, s.listErr
}

func (s *stubStopListCommands) Add(_ context.Context, _ string) (int64, error) {
	return s.addVersion, s.addErr
}

func (s *stubStopListCommands) Remove(_ context.Context, _ string) (int64, error) {
	return s.removeVersion, s.removeErr
}

type stubAPIMetrics struct {
	rendered string

	snapshotAgeSetCalls int
	snapshotAgeMS       int64

	topReadCalls        int
	topReadLatencyCalls int
	topReadLatencyMS    int64
}

func (s *stubAPIMetrics) RenderPrometheus() string {
	return s.rendered
}

func (s *stubAPIMetrics) SetSnapshotAgeMS(ms int64) {
	s.snapshotAgeSetCalls++
	s.snapshotAgeMS = ms
}

func (s *stubAPIMetrics) IncTopRead() {
	s.topReadCalls++
}

func (s *stubAPIMetrics) ObserveTopReadLatencyMS(ms int64) {
	s.topReadLatencyCalls++
	s.topReadLatencyMS = ms
}

func TestGetTop_SuccessResponse(t *testing.T) {
	t.Parallel()

	top := &stubTopQuery{
		snapshot: domain.TopSnapshot{
			GeneratedAtMS: 1_779_656_406_123,
			WindowSeconds: 300,
			Items: []domain.TopItem{
				{Rank: 1, Query: "купить айфон", Score: 125, Count: 125, UniqueSources: 102},
			},
		},
	}
	apiMetrics := &stubAPIMetrics{}
	api := &API{
		top:     top,
		metrics: apiMetrics,
		now: func() time.Time {
			return time.UnixMilli(1_779_656_406_523).UTC()
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/top?n=1", nil)
	rec := httptest.NewRecorder()
	api.GetTop(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
		return
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Errorf("decode response: %v", err)
		return
	}

	if got := int(body["n"].(float64)); got != 1 {
		t.Errorf("unexpected n: got %d want %d", got, 1)
	}
	if got := int64(body["freshness_lag_ms"].(float64)); got != 400 {
		t.Errorf("unexpected freshness lag: got %d want %d", got, 400)
	}
	if top.lastN != 1 {
		t.Errorf("top service called with unexpected n: got %d want %d", top.lastN, 1)
	}
	if apiMetrics.snapshotAgeSetCalls != 1 {
		t.Errorf("snapshot age should be updated once, got %d", apiMetrics.snapshotAgeSetCalls)
	}
	if apiMetrics.snapshotAgeMS != 400 {
		t.Errorf("unexpected snapshot age metric: got %d want %d", apiMetrics.snapshotAgeMS, 400)
	}
	if apiMetrics.topReadCalls != 1 {
		t.Errorf("top read counter should be incremented once, got %d", apiMetrics.topReadCalls)
	}
}

func TestGetTop_InvalidN(t *testing.T) {
	t.Parallel()

	api := &API{top: &stubTopQuery{}, now: func() time.Time { return time.Now().UTC() }}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/top?n=101", nil)
	rec := httptest.NewRecorder()
	api.GetTop(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("unexpected status: got %d want %d", rec.Code, http.StatusBadRequest)
		return
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Errorf("decode response: %v", err)
		return
	}
	if body["code"] != "INVALID_ARGUMENT" {
		t.Errorf("unexpected error code: got %q want %q", body["code"], "INVALID_ARGUMENT")
	}
	if body["message"] != msgInvalidTopN {
		t.Errorf("unexpected error message: got %q", body["message"])
	}
}

func TestAdminEndpoints_RequireToken(t *testing.T) {
	t.Parallel()

	api := &API{
		stop:       &stubStopListCommands{},
		adminToken: "secret-token",
		now: func() time.Time {
			return time.Now().UTC()
		},
	}

	tests := []struct {
		name    string
		method  string
		path    string
		handler func(http.ResponseWriter, *http.Request)
		body    []byte
	}{
		{name: "list", method: http.MethodGet, path: "/api/v1/stop-words", handler: api.ListStopWords},
		{name: "add", method: http.MethodPost, path: "/api/v1/stop-words", handler: api.AddStopWord, body: []byte(`{"word":"a"}`)},
		{name: "delete", method: http.MethodDelete, path: "/api/v1/stop-words/x", handler: api.DeleteStopWord},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tt.method, tt.path, bytes.NewReader(tt.body))
			if tt.name == "delete" {
				req.SetPathValue("word", "x")
			}
			rec := httptest.NewRecorder()
			tt.handler(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("unexpected status: got %d want %d", rec.Code, http.StatusUnauthorized)
				return
			}

			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Errorf("decode response: %v", err)
				return
			}
			if body["code"] != "UNAUTHORIZED" {
				t.Errorf("unexpected code: got %q want %q", body["code"], "UNAUTHORIZED")
			}
			if body["message"] != msgInvalidAdminAuth {
				t.Errorf("unexpected message: got %q want %q", body["message"], msgInvalidAdminAuth)
			}
		})
	}
}

func TestListStopWords_SuccessResponse(t *testing.T) {
	t.Parallel()

	api := &API{
		stop: &stubStopListCommands{
			words:   []string{"бесплатно", "скачать"},
			version: 7,
		},
		adminToken: "secret-token",
		now: func() time.Time {
			return time.UnixMilli(1_779_656_406_123).UTC()
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stop-words", nil)
	req.Header.Set("X-Admin-Token", "secret-token")
	rec := httptest.NewRecorder()
	api.ListStopWords(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
		return
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Errorf("decode response: %v", err)
		return
	}
	if got := int64(body["version"].(float64)); got != 7 {
		t.Errorf("unexpected version: got %d want %d", got, 7)
	}
	if got := int64(body["updated_at_ms"].(float64)); got != 1_779_656_406_123 {
		t.Errorf("unexpected updated_at_ms: got %d want %d", got, 1_779_656_406_123)
	}
}

func TestAddStopWord_MapsDomainErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		addErr     error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid argument",
			addErr:     domain.ErrInvalidArgument,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ARGUMENT",
		},
		{
			name:       "already exists",
			addErr:     domain.ErrAlreadyExists,
			wantStatus: http.StatusConflict,
			wantCode:   "CONFLICT",
		},
		{
			name:       "unknown service error",
			addErr:     errors.New("db unavailable"),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "UNAVAILABLE",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := &API{
				stop: &stubStopListCommands{
					addErr: tt.addErr,
				},
				adminToken: "secret-token",
				now: func() time.Time {
					return time.Now().UTC()
				},
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/stop-words", bytes.NewReader([]byte(`{"word":"бесплатно"}`)))
			req.Header.Set("X-Admin-Token", "secret-token")
			rec := httptest.NewRecorder()
			api.AddStopWord(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("unexpected status: got %d want %d", rec.Code, tt.wantStatus)
				return
			}

			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Errorf("decode response: %v", err)
				return
			}
			if body["code"] != tt.wantCode {
				t.Errorf("unexpected code: got %q want %q", body["code"], tt.wantCode)
			}
		})
	}
}

func TestDeleteStopWord_MapsDomainErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		word       string
		removeErr  error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid argument",
			word:       " ",
			removeErr:  domain.ErrInvalidArgument,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_ARGUMENT",
		},
		{
			name:       "not found",
			word:       "xyz",
			removeErr:  domain.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "unknown service error",
			word:       "xyz",
			removeErr:  errors.New("db unavailable"),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "UNAVAILABLE",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := &API{
				stop: &stubStopListCommands{
					removeErr: tt.removeErr,
				},
				adminToken: "secret-token",
				now: func() time.Time {
					return time.Now().UTC()
				},
			}

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/stop-words/x", nil)
			req.Header.Set("X-Admin-Token", "secret-token")
			req.SetPathValue("word", tt.word)
			rec := httptest.NewRecorder()
			api.DeleteStopWord(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("unexpected status: got %d want %d", rec.Code, tt.wantStatus)
				return
			}

			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Errorf("decode response: %v", err)
				return
			}
			if body["code"] != tt.wantCode {
				t.Errorf("unexpected code: got %q want %q", body["code"], tt.wantCode)
			}
		})
	}
}

func TestHealth_UsesConfiguredVersionAndFlags(t *testing.T) {
	t.Parallel()

	api := &API{
		top: &stubTopQuery{
			snapshot: domain.TopSnapshot{},
		},
		serviceVersion: "1.0.0",
		kafkaConnected: true,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	api.Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
		return
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Errorf("decode response: %v", err)
		return
	}
	if got := body["version"].(string); got != "1.0.0" {
		t.Errorf("unexpected version: got %q want %q", got, "1.0.0")
	}
	if got := body["status"].(string); got != "ok" {
		t.Errorf("unexpected status field: got %q want %q", got, "ok")
	}
}

func TestMetrics_ReturnsPrometheusPayload(t *testing.T) {
	t.Parallel()

	api := &API{
		metrics: &stubAPIMetrics{
			rendered: "trends_ingest_events_total 3\n",
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	api.Metrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
		return
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "text/plain; version=0.0.4; charset=utf-8" {
		t.Errorf("unexpected content type: got %q", contentType)
	}
	if got := rec.Body.String(); got != "trends_ingest_events_total 3\n" {
		t.Errorf("unexpected metrics payload: got %q", got)
	}
}

func TestOpenAPISpec_ReturnsYAML(t *testing.T) {
	t.Parallel()

	tmpFile, err := os.CreateTemp(t.TempDir(), "openapi-*.yaml")
	if err != nil {
		t.Errorf("create temp file: %v", err)
		return
	}
	const payload = "openapi: 3.1.0\ninfo:\n  title: test\n"
	if _, err := tmpFile.WriteString(payload); err != nil {
		t.Errorf("write temp file: %v", err)
		return
	}
	if err := tmpFile.Close(); err != nil {
		t.Errorf("close temp file: %v", err)
		return
	}

	api := &API{openAPISpecPath: tmpFile.Name()}
	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	api.OpenAPISpec(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
		return
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/yaml; charset=utf-8" {
		t.Errorf("unexpected content type: got %q", ct)
	}
	if got := rec.Body.String(); got != payload {
		t.Errorf("unexpected body: got %q want %q", got, payload)
	}
}

func TestOpenAPISpec_Unavailable(t *testing.T) {
	t.Parallel()

	api := &API{openAPISpecPath: "missing/spec.yaml"}
	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	api.OpenAPISpec(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("unexpected status: got %d want %d", rec.Code, http.StatusServiceUnavailable)
		return
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Errorf("decode response: %v", err)
		return
	}
	if body["code"] != "UNAVAILABLE" {
		t.Errorf("unexpected code: got %q want %q", body["code"], "UNAVAILABLE")
	}
	if body["message"] != msgOpenAPIUnavailable {
		t.Errorf("unexpected message: got %q want %q", body["message"], msgOpenAPIUnavailable)
	}
}

func TestSwaggerUI_ReturnsHTML(t *testing.T) {
	t.Parallel()

	api := &API{}
	req := httptest.NewRequest(http.MethodGet, "/swagger/", nil)
	rec := httptest.NewRecorder()
	api.SwaggerUI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
		return
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("unexpected content type: got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "SwaggerUIBundle") {
		t.Errorf("swagger html must include SwaggerUIBundle init")
	}
	if !strings.Contains(body, "/openapi.yaml") {
		t.Errorf("swagger html must point to /openapi.yaml")
	}
}

func TestSwaggerRedirect(t *testing.T) {
	t.Parallel()

	api := &API{}
	req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	rec := httptest.NewRecorder()
	api.SwaggerRedirect(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("unexpected status: got %d want %d", rec.Code, http.StatusTemporaryRedirect)
		return
	}
	if loc := rec.Header().Get("Location"); loc != "/swagger/" {
		t.Errorf("unexpected redirect location: got %q want %q", loc, "/swagger/")
	}
}
