package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"wbtrialtask/internal/adapters/contracts"
	"wbtrialtask/internal/adapters/httpapi"
	"wbtrialtask/internal/adapters/kafka"
	"wbtrialtask/internal/adapters/metrics"
	"wbtrialtask/internal/adapters/storage/postgres"
	"wbtrialtask/internal/app"
	"wbtrialtask/internal/config"
	"wbtrialtask/internal/engine/antifraud"
	"wbtrialtask/internal/engine/dedup"
	"wbtrialtask/internal/engine/normalizer"
	"wbtrialtask/internal/engine/window"
	"wbtrialtask/internal/ports"
	"wbtrialtask/internal/usecase/ingest"
	"wbtrialtask/internal/usecase/stoplist"
	"wbtrialtask/internal/usecase/top"
)

func main() {
	cfg := config.FromEnv()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := postgres.NewStopListRepository(rootCtx, cfg.Storage.StopListPostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("stop-list postgres close error: %v", err)
		}
	}()
	topEngine := window.NewEngine(cfg.Top.WindowSeconds)
	initialStopWords, _, err := store.List(rootCtx)
	if err != nil {
		log.Fatal(err)
	}
	norm := normalizer.NewService(initialStopWords)
	fraud := antifraud.NewFailClosedEvaluator(antifraud.Config{
		MaxRequestsPerMinutePerIP:  cfg.AntiFraud.MaxRequestsPerMinutePerIP,
		IPBlockDuration:            cfg.AntiFraud.IPBlockDuration,
		ContributionWindow:         cfg.AntiFraud.ContributionWindow,
		MaxUniqueQueriesPerSession: cfg.AntiFraud.MaxUniqueQueriesPerSession,
		SessionEntropyWindow:       cfg.AntiFraud.SessionEntropyWindow,
		SessionBlockDuration:       cfg.AntiFraud.SessionBlockDuration,
	})
	dedupStore := dedup.NewStore(cfg.Ingest.DedupTTL)

	typedIngestHandler := ingest.NewHandler(ingest.Dependencies{
		Normalizer:   norm,
		AntiFraud:    fraud,
		Deduplicator: dedupStore,
		Aggregator:   topEngine,
		Clock:        ports.RealClock{},
	})
	validator, err := contracts.NewSearchEventValidator(cfg.Contract.SearchEventSchemaPath)
	if err != nil {
		log.Fatal(err)
	}
	metricsCollector := metrics.NewPrometheusCollector()
	rejectReporter := metrics.NewRejectReporter(metricsCollector)
	runtimeIngestHandler := ingest.NewRuntimeHandlerWithMetrics(validator, typedIngestHandler, rejectReporter, metricsCollector)

	topQuery := top.NewService(topEngine)
	stopCommands := stoplist.NewService(store, norm)
	if err := stopCommands.Sync(rootCtx); err != nil {
		log.Fatal(err)
	}

	router := httpapi.NewRouter(topQuery, stopCommands, httpapi.Config{
		AdminToken:     cfg.HTTP.AdminToken,
		ServiceVersion: cfg.HTTP.ServiceVersion,
		KafkaConnected: cfg.Kafka.Enabled,
		Metrics:        metricsCollector,
	})

	svc := app.NewService(app.Dependencies{
		HTTPServer: &http.Server{
			Addr:    cfg.HTTP.Address,
			Handler: router,
		},
		Consumer: kafka.NewConsumer(kafka.Config{
			Enabled:              cfg.Kafka.Enabled,
			Brokers:              cfg.Kafka.Brokers,
			Topic:                cfg.Kafka.Topic,
			GroupID:              cfg.Kafka.GroupID,
			MinBytes:             cfg.Kafka.MinBytes,
			MaxBytes:             cfg.Kafka.MaxBytes,
			MaxWait:              cfg.Kafka.MaxWait,
			StartOffset:          cfg.Kafka.StartOffset,
			MaxProcessingRetries: cfg.Kafka.MaxProcessingRetries,
			RetryBackoff:         cfg.Kafka.RetryBackoff,
			RetryBackoffMax:      cfg.Kafka.RetryBackoffMax,
			CommitRetries:        cfg.Kafka.CommitRetries,
			CommitRetryBackoff:   cfg.Kafka.CommitRetryBackoff,
		}),
		EventHandle: runtimeIngestHandler.HandleRaw,
	})

	if err := svc.Run(rootCtx); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}
