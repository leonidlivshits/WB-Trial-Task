package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTP struct {
		Address        string
		AdminToken     string
		ServiceVersion string
	}
	Storage struct {
		StopListPostgresDSN string
	}
	Kafka struct {
		Enabled bool

		Brokers []string
		Topic   string
		GroupID string

		MinBytes int
		MaxBytes int
		MaxWait  time.Duration

		StartOffset int64

		MaxProcessingRetries int
		RetryBackoff         time.Duration
		RetryBackoffMax      time.Duration

		CommitRetries      int
		CommitRetryBackoff time.Duration
	}
	Contract struct {
		SearchEventSchemaPath string
	}
	Top struct {
		WindowSeconds int
	}
	Ingest struct {
		DedupTTL time.Duration
	}
	AntiFraud struct {
		MaxRequestsPerMinutePerIP  int
		IPBlockDuration            time.Duration
		ContributionWindow         time.Duration
		MaxUniqueQueriesPerSession int
		SessionEntropyWindow       time.Duration
		SessionBlockDuration       time.Duration
	}
}

func FromEnv() Config {
	var cfg Config

	cfg.HTTP.Address = getEnvString("HTTP_ADDRESS", ":8080")
	cfg.HTTP.AdminToken = getEnvString("HTTP_ADMIN_TOKEN", "")
	cfg.HTTP.ServiceVersion = getEnvString("HTTP_SERVICE_VERSION", "1.0.0")

	cfg.Storage.StopListPostgresDSN = getEnvString(
		"STORAGE_STOPLIST_POSTGRES_DSN",
		"",
	)

	cfg.Kafka.Enabled = getEnvBool("KAFKA_ENABLED", true)
	cfg.Kafka.Brokers = getEnvCSV("KAFKA_BROKERS", []string{"localhost:9092"})
	cfg.Kafka.Topic = getEnvString("KAFKA_TOPIC", "search.events.v1")
	cfg.Kafka.GroupID = getEnvString("KAFKA_GROUP_ID", "trends-service-v1")
	cfg.Kafka.MinBytes = getEnvInt("KAFKA_MIN_BYTES", 1<<10)
	cfg.Kafka.MaxBytes = getEnvInt("KAFKA_MAX_BYTES", 10<<20)
	cfg.Kafka.MaxWait = getEnvDuration("KAFKA_MAX_WAIT", 500*time.Millisecond)
	cfg.Kafka.StartOffset = getEnvInt64("KAFKA_START_OFFSET", -2)
	cfg.Kafka.MaxProcessingRetries = getEnvInt("KAFKA_MAX_PROCESSING_RETRIES", 3)
	cfg.Kafka.RetryBackoff = getEnvDuration("KAFKA_RETRY_BACKOFF", 100*time.Millisecond)
	cfg.Kafka.RetryBackoffMax = getEnvDuration("KAFKA_RETRY_BACKOFF_MAX", 3*time.Second)
	cfg.Kafka.CommitRetries = getEnvInt("KAFKA_COMMIT_RETRIES", 3)
	cfg.Kafka.CommitRetryBackoff = getEnvDuration("KAFKA_COMMIT_RETRY_BACKOFF", 100*time.Millisecond)

	cfg.Contract.SearchEventSchemaPath = getEnvString("CONTRACT_SEARCH_EVENT_SCHEMA_PATH", "docs/schemas/search_event.v1.schema.json")
	cfg.Top.WindowSeconds = getEnvInt("TOP_WINDOW_SECONDS", 300)
	cfg.Ingest.DedupTTL = getEnvDuration("INGEST_DEDUP_TTL", 10*time.Minute)

	cfg.AntiFraud.MaxRequestsPerMinutePerIP = getEnvInt("ANTI_FRAUD_MAX_REQUESTS_PER_MINUTE_PER_IP", 1000)
	cfg.AntiFraud.IPBlockDuration = getEnvDuration("ANTI_FRAUD_IP_BLOCK_DURATION", time.Hour)
	cfg.AntiFraud.ContributionWindow = getEnvDuration("ANTI_FRAUD_CONTRIBUTION_WINDOW", time.Hour)
	cfg.AntiFraud.MaxUniqueQueriesPerSession = getEnvInt("ANTI_FRAUD_MAX_UNIQUE_QUERIES_PER_SESSION", 100)
	cfg.AntiFraud.SessionEntropyWindow = getEnvDuration("ANTI_FRAUD_SESSION_ENTROPY_WINDOW", 5*time.Minute)
	cfg.AntiFraud.SessionBlockDuration = getEnvDuration("ANTI_FRAUD_SESSION_BLOCK_DURATION", time.Hour)

	return cfg
}

func (cfg Config) Validate() error {
	missing := make([]string, 0, 2)

	if strings.TrimSpace(cfg.HTTP.AdminToken) == "" {
		missing = append(missing, "HTTP_ADMIN_TOKEN")
	}
	if strings.TrimSpace(cfg.Storage.StopListPostgresDSN) == "" {
		missing = append(missing, "STORAGE_STOPLIST_POSTGRES_DSN")
	}

	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("missing required env variables: %s", strings.Join(missing, ", "))
}

func getEnvString(key, fallback string) string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	return raw
}

func getEnvBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt64(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvCSV(key string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
