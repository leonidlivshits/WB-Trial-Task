package dedup

import (
	"sync"
	"time"
)

type Store struct {
	mu   sync.Mutex
	ttl  time.Duration
	seen map[string]time.Time
}

func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Store{
		ttl:  ttl,
		seen: make(map[string]time.Time),
	}
}

func (s *Store) SeenOrAdd(key string, now time.Time) bool {
	if key == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanup(now)
	if exp, ok := s.seen[key]; ok && exp.After(now) {
		return true
	}

	s.seen[key] = now.Add(s.ttl)
	return false
}

func (s *Store) cleanup(now time.Time) {
	for key, exp := range s.seen {
		if exp.Before(now) || exp.Equal(now) {
			delete(s.seen, key)
		}
	}
}
