package httpapi

import (
	"context"
	"net/http"
	"time"

	"wbtrialtask/internal/domain"
)

type TopQuery interface {
	GetTop(ctx context.Context, n int) (domain.TopSnapshot, error)
}

type StopListCommands interface {
	List(ctx context.Context) ([]string, int64, error)
	Add(ctx context.Context, word string) (int64, error)
	Remove(ctx context.Context, word string) (int64, error)
}

type API struct {
	top             TopQuery
	stop            StopListCommands
	adminToken      string
	serviceVersion  string
	openAPISpecPath string
	now             func() time.Time
	kafkaConnected  bool
	metrics         APIMetrics
}

type Config struct {
	AdminToken      string
	ServiceVersion  string
	KafkaConnected  bool
	Metrics         APIMetrics
	OpenAPISpecPath string
}

type APIMetrics interface {
	RenderPrometheus() string
	SetSnapshotAgeMS(ms int64)
	IncTopRead()
	ObserveTopReadLatencyMS(ms int64)
}

func NewRouter(top TopQuery, stop StopListCommands, cfg Config) http.Handler {
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = "1.0.0"
	}
	if cfg.OpenAPISpecPath == "" {
		cfg.OpenAPISpecPath = "docs/contracts/api.v1.openapi.yaml"
	}

	api := &API{
		top:             top,
		stop:            stop,
		adminToken:      cfg.AdminToken,
		serviceVersion:  cfg.ServiceVersion,
		openAPISpecPath: cfg.OpenAPISpecPath,
		now: func() time.Time {
			return time.Now().UTC()
		},
		kafkaConnected: cfg.KafkaConnected,
		metrics:        cfg.Metrics,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/top", api.GetTop)
	mux.HandleFunc("GET /api/v1/stop-words", api.ListStopWords)
	mux.HandleFunc("POST /api/v1/stop-words", api.AddStopWord)
	mux.HandleFunc("DELETE /api/v1/stop-words/{word}", api.DeleteStopWord)
	mux.HandleFunc("GET /api/v1/health", api.Health)
	mux.HandleFunc("GET /metrics", api.Metrics)
	mux.HandleFunc("GET /openapi.yaml", api.OpenAPISpec)
	mux.HandleFunc("GET /swagger", api.SwaggerRedirect)
	mux.HandleFunc("GET /swagger/", api.SwaggerUI)
	return mux
}
