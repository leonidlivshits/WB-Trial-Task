package window

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"wbtrialtask/internal/domain"
)

type bucket struct {
	epochSecond int64
	counts      map[string]float64
}

type snapshotState struct {
	generatedAtMS int64
	items         []domain.TopItem
}

type Engine struct {
	mu              sync.RWMutex
	windowSeconds   int
	buckets         []bucket
	totals          map[string]float64
	cached          atomic.Value
	dirty           bool
	lastRebuildAt   time.Time
	rebuildInterval time.Duration
}

func NewEngine(windowSeconds int) *Engine {
	if windowSeconds <= 0 {
		windowSeconds = 300
	}

	buckets := make([]bucket, windowSeconds)
	for i := range buckets {
		buckets[i].counts = make(map[string]float64)
	}

	engine := &Engine{
		windowSeconds:   windowSeconds,
		buckets:         buckets,
		totals:          make(map[string]float64),
		dirty:           true,
		rebuildInterval: 250 * time.Millisecond,
	}
	engine.cached.Store(snapshotState{})
	return engine
}

func (e *Engine) Add(query string, score float64, ingestedAtMS int64) {
	if query == "" || score <= 0 {
		return
	}

	sec := ingestedAtMS / 1000
	if sec <= 0 {
		sec = time.Now().UTC().Unix()
	}

	now := time.Now().UTC()

	e.mu.Lock()
	e.evictExpiredBucketsLocked(sec)

	idx := e.bucketIndex(sec)
	b := &e.buckets[idx]
	if b.epochSecond != sec {
		e.removeBucketContributionLocked(b)
		b.epochSecond = sec
		b.counts = make(map[string]float64)
	}

	b.counts[query] += score
	e.totals[query] += score
	e.dirty = true

	if e.shouldRebuildLocked(now) {
		e.cached.Store(e.buildSnapshotStateLocked(now))
		e.lastRebuildAt = now
		e.dirty = false
	}
	e.mu.Unlock()
}

func (e *Engine) Snapshot(n int) domain.TopSnapshot {
	if n <= 0 {
		n = 10
	}

	now := time.Now().UTC()

	e.mu.Lock()
	if e.dirty {
		e.cached.Store(e.buildSnapshotStateLocked(now))
		e.lastRebuildAt = now
		e.dirty = false
	}
	state := e.cached.Load().(snapshotState)
	e.mu.Unlock()

	if len(state.items) == 0 {
		return domain.TopSnapshot{
			GeneratedAtMS: state.generatedAtMS,
			WindowSeconds: e.windowSeconds,
			Items:         []domain.TopItem{},
		}
	}

	if n > len(state.items) {
		n = len(state.items)
	}
	out := make([]domain.TopItem, n)
	copy(out, state.items[:n])

	return domain.TopSnapshot{
		GeneratedAtMS: state.generatedAtMS,
		WindowSeconds: e.windowSeconds,
		Items:         out,
	}
}

func (e *Engine) shouldRebuildLocked(now time.Time) bool {
	if !e.dirty {
		return false
	}
	if e.lastRebuildAt.IsZero() {
		return true
	}
	return now.Sub(e.lastRebuildAt) >= e.rebuildInterval
}

func (e *Engine) buildSnapshotStateLocked(now time.Time) snapshotState {
	items := make([]domain.TopItem, 0, len(e.totals))
	for query, score := range e.totals {
		if score <= 0 {
			continue
		}
		items = append(items, domain.TopItem{
			Query: query,
			Score: score,
			Count: int64(score),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].Query < items[j].Query
		}
		return items[i].Score > items[j].Score
	})

	for i := range items {
		items[i].Rank = i + 1
	}

	return snapshotState{
		generatedAtMS: now.UnixMilli(),
		items:         items,
	}
}

func (e *Engine) bucketIndex(second int64) int {
	return int(second % int64(e.windowSeconds))
}

func (e *Engine) evictExpiredBucketsLocked(currentSecond int64) {
	maxAge := int64(e.windowSeconds)
	for i := range e.buckets {
		b := &e.buckets[i]
		if b.epochSecond == 0 {
			continue
		}
		if currentSecond-b.epochSecond >= maxAge {
			e.removeBucketContributionLocked(b)
			b.epochSecond = 0
			b.counts = make(map[string]float64)
		}
	}
}

func (e *Engine) removeBucketContributionLocked(b *bucket) {
	for query, cnt := range b.counts {
		total := e.totals[query] - cnt
		if total <= 0 {
			delete(e.totals, query)
			continue
		}
		e.totals[query] = total
	}
}
