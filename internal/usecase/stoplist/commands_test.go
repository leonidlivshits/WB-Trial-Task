package stoplist

import (
	"context"
	"sync"
	"testing"

	"wbtrialtask/internal/domain"
	"wbtrialtask/internal/engine/normalizer"
)

type memoryStopListStore struct {
	mu      sync.Mutex
	words   map[string]struct{}
	version int64
}

func newMemoryStopListStore() *memoryStopListStore {
	return &memoryStopListStore{
		words: make(map[string]struct{}),
	}
}

func (s *memoryStopListStore) Add(_ context.Context, word string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.words[word]; exists {
		return 0, domain.ErrAlreadyExists
	}
	s.words[word] = struct{}{}
	s.version++
	return s.version, nil
}

func (s *memoryStopListStore) Remove(_ context.Context, word string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.words[word]; !exists {
		return 0, domain.ErrNotFound
	}
	delete(s.words, word)
	s.version++
	return s.version, nil
}

func (s *memoryStopListStore) List(_ context.Context) ([]string, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	words := make([]string, 0, len(s.words))
	for word := range s.words {
		words = append(words, word)
	}
	return words, s.version, nil
}

func TestService_AddAndRemoveSyncsNormalizer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMemoryStopListStore()
	norm := normalizer.NewService(nil)
	svc := NewService(repo, norm)

	if err := svc.Sync(ctx); err != nil {
		t.Errorf("sync on startup returned error: %v", err)
		return
	}

	if _, err := svc.Add(ctx, "купить"); err != nil {
		t.Errorf("add stop-word returned error: %v", err)
		return
	}

	normalized, err := norm.Normalize("купить айфон")
	if err != nil {
		t.Errorf("normalize after add returned error: %v", err)
		return
	}
	if normalized != "айфон" {
		t.Errorf("unexpected normalization after add: got %q want %q", normalized, "айфон")
	}

	if _, err := svc.Remove(ctx, "купить"); err != nil {
		t.Errorf("remove stop-word returned error: %v", err)
		return
	}

	normalized, err = norm.Normalize("купить айфон")
	if err != nil {
		t.Errorf("normalize after remove returned error: %v", err)
		return
	}
	if normalized != "купить айфон" {
		t.Errorf("unexpected normalization after remove: got %q want %q", normalized, "купить айфон")
	}
}
