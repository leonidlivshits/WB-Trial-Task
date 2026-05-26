package stoplist

import (
	"context"
	"strings"

	"wbtrialtask/internal/domain"
	"wbtrialtask/internal/ports"
)

type Service struct {
	store   ports.StopListStore
	updater StopWordsUpdater
}

type StopWordsUpdater interface {
	ReplaceStopWords(words []string)
}

func NewService(store ports.StopListStore, updater StopWordsUpdater) *Service {
	return &Service{
		store:   store,
		updater: updater,
	}
}

func (s *Service) Add(ctx context.Context, word string) (int64, error) {
	word = strings.TrimSpace(strings.ToLower(word))
	if word == "" {
		return 0, domain.ErrInvalidArgument
	}
	version, err := s.store.Add(ctx, word)
	if err != nil {
		return 0, err
	}
	if err := s.Sync(ctx); err != nil {
		return version, err
	}
	return version, nil
}

func (s *Service) Remove(ctx context.Context, word string) (int64, error) {
	word = strings.TrimSpace(strings.ToLower(word))
	if word == "" {
		return 0, domain.ErrInvalidArgument
	}
	version, err := s.store.Remove(ctx, word)
	if err != nil {
		return 0, err
	}
	if err := s.Sync(ctx); err != nil {
		return version, err
	}
	return version, nil
}

func (s *Service) List(ctx context.Context) ([]string, int64, error) {
	return s.store.List(ctx)
}

func (s *Service) Sync(ctx context.Context) error {
	if s.updater == nil {
		return nil
	}

	words, _, err := s.store.List(ctx)
	if err != nil {
		return err
	}
	s.updater.ReplaceStopWords(words)
	return nil
}
