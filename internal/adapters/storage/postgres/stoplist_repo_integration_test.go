//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"wbtrialtask/internal/domain"
)

func TestStopListRepository_IntegrationCRUD(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	repo, err := NewStopListRepository(ctx, dsn)
	if err != nil {
		t.Errorf("create repository: %v", err)
		return
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			t.Errorf("close repository: %v", closeErr)
		}
	}()

	word := "integration-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	defer func() {
		_, _ = repo.Remove(context.Background(), word)
	}()

	addVersion, err := repo.Add(ctx, word)
	if err != nil {
		t.Errorf("add word: %v", err)
		return
	}
	if addVersion <= 0 {
		t.Errorf("add version must be positive, got %d", addVersion)
	}

	_, err = repo.Add(ctx, word)
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Errorf("duplicate add: got %v want %v", err, domain.ErrAlreadyExists)
	}

	words, listVersion, err := repo.List(ctx)
	if err != nil {
		t.Errorf("list words: %v", err)
		return
	}
	if !contains(words, word) {
		t.Errorf("list does not contain inserted word %q", word)
	}
	if listVersion < addVersion {
		t.Errorf("list version must be >= add version, got list=%d add=%d", listVersion, addVersion)
	}

	removeVersion, err := repo.Remove(ctx, word)
	if err != nil {
		t.Errorf("remove word: %v", err)
		return
	}
	if removeVersion <= addVersion {
		t.Errorf("remove version must grow, got remove=%d add=%d", removeVersion, addVersion)
	}

	_, err = repo.Remove(ctx, word)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("remove missing: got %v want %v", err, domain.ErrNotFound)
	}
}

func contains(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}
