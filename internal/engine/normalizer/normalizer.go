package normalizer

import (
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"wbtrialtask/internal/domain"
)

var (
	nonAlnumSpace = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
	multiSpace    = regexp.MustCompile(`\s+`)
)

type Service struct {
	mu        sync.RWMutex
	stopWords map[string]struct{}
}

func NewService(stopWords []string) *Service {
	return &Service{stopWords: normalizeStopWords(stopWords)}
}

func (s *Service) Normalize(raw string) (string, error) {
	q := strings.ToLower(raw)
	q = strings.TrimSpace(q)
	q = nonAlnumSpace.ReplaceAllString(q, " ")
	q = multiSpace.ReplaceAllString(q, " ")
	if q == "" {
		return "", domain.ErrInvalidQuery
	}

	parts := strings.Fields(q)
	filtered := make([]string, 0, len(parts))

	s.mu.RLock()
	for _, p := range parts {
		if _, blocked := s.stopWords[p]; blocked {
			continue
		}
		filtered = append(filtered, p)
	}
	s.mu.RUnlock()

	q = strings.Join(filtered, " ")
	if utf8.RuneCountInString(q) < 2 {
		return "", domain.ErrInvalidQuery
	}
	return q, nil
}

func (s *Service) ReplaceStopWords(words []string) {
	set := normalizeStopWords(words)

	s.mu.Lock()
	s.stopWords = set
	s.mu.Unlock()
}

func normalizeStopWords(words []string) map[string]struct{} {
	set := make(map[string]struct{}, len(words))
	for _, word := range words {
		word = strings.TrimSpace(strings.ToLower(word))
		if word == "" {
			continue
		}
		set[word] = struct{}{}
	}
	return set
}
