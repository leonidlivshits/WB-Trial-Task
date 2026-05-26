package config

import (
	"strings"
	"testing"
)

func TestValidate_MissingRequiredValues(t *testing.T) {
	cfg := Config{}

	err := cfg.Validate()
	if err == nil {
		t.Errorf("expected validation error, got nil")
		return
	}
	msg := err.Error()
	if !strings.Contains(msg, "HTTP_ADMIN_TOKEN") {
		t.Errorf("error must mention HTTP_ADMIN_TOKEN, got %q", msg)
	}
	if !strings.Contains(msg, "STORAGE_STOPLIST_POSTGRES_DSN") {
		t.Errorf("error must mention STORAGE_STOPLIST_POSTGRES_DSN, got %q", msg)
	}
}

func TestValidate_OK(t *testing.T) {
	cfg := Config{}
	cfg.HTTP.AdminToken = "secret"
	cfg.Storage.StopListPostgresDSN = "postgres://user:pass@localhost:5432/db?sslmode=disable"

	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestFromEnv_SecretsAreEmptyByDefault(t *testing.T) {
	t.Setenv("HTTP_ADMIN_TOKEN", "")
	t.Setenv("STORAGE_STOPLIST_POSTGRES_DSN", "")

	cfg := FromEnv()

	if cfg.HTTP.AdminToken != "" {
		t.Errorf("expected empty admin token by default, got %q", cfg.HTTP.AdminToken)
	}
	if cfg.Storage.StopListPostgresDSN != "" {
		t.Errorf("expected empty postgres dsn by default, got %q", cfg.Storage.StopListPostgresDSN)
	}
}
