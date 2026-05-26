package antifraud

import (
	"fmt"
	"sync"
	"time"

	"wbtrialtask/internal/domain"
)

type Config struct {
	MaxRequestsPerMinutePerIP  int
	IPBlockDuration            time.Duration
	ContributionWindow         time.Duration
	MaxUniqueQueriesPerSession int
	SessionEntropyWindow       time.Duration
	SessionBlockDuration       time.Duration
}

type Evaluator struct {
	cfg Config

	mu            sync.Mutex
	counters      map[string]int
	bucketMin     int64
	lastCleanupAt time.Time

	ipBlockedUntil      map[string]time.Time
	sessionBlockedUntil map[string]time.Time
	contributionSeen    map[string]time.Time
	sessionEntropy      map[string]*sessionEntropyState
}

type sessionEvent struct {
	at    time.Time
	query string
}

type sessionEntropyState struct {
	events   []sessionEvent
	counts   map[string]int
	lastSeen time.Time
}

const (
	defaultMaxRequestsPerMinutePerIP  = 1000
	defaultIPBlockDuration            = time.Hour
	defaultContributionWindow         = time.Hour
	defaultMaxUniqueQueriesPerSession = 100
	defaultSessionEntropyWindow       = 5 * time.Minute
	defaultSessionBlockDuration       = time.Hour
	cleanupInterval                   = time.Minute
)

func NewFailClosedEvaluator(cfg Config) *Evaluator {
	if cfg.MaxRequestsPerMinutePerIP <= 0 {
		cfg.MaxRequestsPerMinutePerIP = defaultMaxRequestsPerMinutePerIP
	}
	if cfg.IPBlockDuration <= 0 {
		cfg.IPBlockDuration = defaultIPBlockDuration
	}
	if cfg.ContributionWindow <= 0 {
		cfg.ContributionWindow = defaultContributionWindow
	}
	if cfg.MaxUniqueQueriesPerSession <= 0 {
		cfg.MaxUniqueQueriesPerSession = defaultMaxUniqueQueriesPerSession
	}
	if cfg.SessionEntropyWindow <= 0 {
		cfg.SessionEntropyWindow = defaultSessionEntropyWindow
	}
	if cfg.SessionBlockDuration <= 0 {
		cfg.SessionBlockDuration = defaultSessionBlockDuration
	}

	return &Evaluator{
		cfg:                 cfg,
		counters:            make(map[string]int),
		ipBlockedUntil:      make(map[string]time.Time),
		sessionBlockedUntil: make(map[string]time.Time),
		contributionSeen:    make(map[string]time.Time),
		sessionEntropy:      make(map[string]*sessionEntropyState),
	}
}

func (e *Evaluator) Evaluate(evt domain.SearchEvent, normalizedQuery string, now time.Time) *domain.RejectError {
	if e == nil {
		return &domain.RejectError{
			Reason: domain.RejectReasonAntiFraudDegraded,
			Err:    fmt.Errorf("anti-fraud evaluator is nil"),
		}
	}
	if evt.IPHash == "" || evt.SessionIDHash == "" {
		return &domain.RejectError{
			Reason: domain.RejectReasonAntiFraudMissingIdentifiers,
			Err:    fmt.Errorf("missing required anti-fraud identifiers"),
		}
	}
	if normalizedQuery == "" {
		return &domain.RejectError{
			Reason: domain.RejectReasonAntiFraudDegraded,
			Err:    fmt.Errorf("normalized query is empty"),
		}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.validateConfigLocked(); err != nil {
		return &domain.RejectError{
			Reason: domain.RejectReasonAntiFraudDegraded,
			Err:    err,
		}
	}

	e.cleanupLocked(now)

	if until, ok := e.ipBlockedUntil[evt.IPHash]; ok && until.After(now) {
		return &domain.RejectError{
			Reason: domain.RejectReasonAntiFraudIPBlocked,
			Err:    fmt.Errorf("ip is blocked until %s", until.Format(time.RFC3339)),
		}
	}
	if until, ok := e.sessionBlockedUntil[evt.SessionIDHash]; ok && until.After(now) {
		return &domain.RejectError{
			Reason: domain.RejectReasonAntiFraudSessionEntropy,
			Err:    fmt.Errorf("session is blocked until %s", until.Format(time.RFC3339)),
		}
	}

	nowMin := now.Unix() / 60
	if e.bucketMin != nowMin {
		e.bucketMin = nowMin
		e.counters = make(map[string]int)
	}

	e.counters[evt.IPHash]++
	if e.counters[evt.IPHash] > e.cfg.MaxRequestsPerMinutePerIP {
		e.ipBlockedUntil[evt.IPHash] = now.Add(e.cfg.IPBlockDuration)
		return &domain.RejectError{
			Reason: domain.RejectReasonAntiFraudIPRateLimit,
			Err:    fmt.Errorf("ip rate limit exceeded"),
		}
	}

	keys := identityKeys(evt)
	for _, key := range keys {
		capKey := contributionKey(key, normalizedQuery)
		if expAt, ok := e.contributionSeen[capKey]; ok && expAt.After(now) {
			return &domain.RejectError{
				Reason: domain.RejectReasonAntiFraudContributionCap,
				Err:    fmt.Errorf("contribution cap exceeded for %s", key),
			}
		}
	}
	for _, key := range keys {
		e.contributionSeen[contributionKey(key, normalizedQuery)] = now.Add(e.cfg.ContributionWindow)
	}

	state := e.sessionEntropy[evt.SessionIDHash]
	if state == nil {
		state = &sessionEntropyState{
			counts: make(map[string]int),
		}
		e.sessionEntropy[evt.SessionIDHash] = state
	}
	state.lastSeen = now
	pruneSessionState(state, now.Add(-e.cfg.SessionEntropyWindow))

	state.events = append(state.events, sessionEvent{
		at:    now,
		query: normalizedQuery,
	})
	state.counts[normalizedQuery]++
	if len(state.counts) > e.cfg.MaxUniqueQueriesPerSession {
		e.sessionBlockedUntil[evt.SessionIDHash] = now.Add(e.cfg.SessionBlockDuration)
		delete(e.sessionEntropy, evt.SessionIDHash)
		return &domain.RejectError{
			Reason: domain.RejectReasonAntiFraudSessionEntropy,
			Err:    fmt.Errorf("session unique query entropy exceeded"),
		}
	}

	return nil
}

func (e *Evaluator) validateConfigLocked() error {
	switch {
	case e.cfg.MaxRequestsPerMinutePerIP <= 0:
		return fmt.Errorf("invalid anti-fraud config: MaxRequestsPerMinutePerIP must be > 0")
	case e.cfg.IPBlockDuration <= 0:
		return fmt.Errorf("invalid anti-fraud config: IPBlockDuration must be > 0")
	case e.cfg.ContributionWindow <= 0:
		return fmt.Errorf("invalid anti-fraud config: ContributionWindow must be > 0")
	case e.cfg.MaxUniqueQueriesPerSession <= 0:
		return fmt.Errorf("invalid anti-fraud config: MaxUniqueQueriesPerSession must be > 0")
	case e.cfg.SessionEntropyWindow <= 0:
		return fmt.Errorf("invalid anti-fraud config: SessionEntropyWindow must be > 0")
	case e.cfg.SessionBlockDuration <= 0:
		return fmt.Errorf("invalid anti-fraud config: SessionBlockDuration must be > 0")
	default:
		return nil
	}
}

func (e *Evaluator) cleanupLocked(now time.Time) {
	if !e.lastCleanupAt.IsZero() && now.Sub(e.lastCleanupAt) < cleanupInterval {
		return
	}
	e.lastCleanupAt = now

	for key, until := range e.ipBlockedUntil {
		if !until.After(now) {
			delete(e.ipBlockedUntil, key)
		}
	}
	for key, until := range e.sessionBlockedUntil {
		if !until.After(now) {
			delete(e.sessionBlockedUntil, key)
		}
	}
	for key, expAt := range e.contributionSeen {
		if !expAt.After(now) {
			delete(e.contributionSeen, key)
		}
	}
	for key, state := range e.sessionEntropy {
		pruneSessionState(state, now.Add(-e.cfg.SessionEntropyWindow))
		if len(state.counts) == 0 && now.Sub(state.lastSeen) > e.cfg.SessionEntropyWindow {
			delete(e.sessionEntropy, key)
		}
	}
}

func identityKeys(evt domain.SearchEvent) []string {
	keys := []string{
		"ip:" + evt.IPHash,
		"session:" + evt.SessionIDHash,
	}
	if evt.UserIDHash != nil && *evt.UserIDHash != "" {
		keys = append(keys, "user:"+*evt.UserIDHash)
	}
	return keys
}

func contributionKey(identityKey, query string) string {
	return identityKey + "|" + query
}

func pruneSessionState(state *sessionEntropyState, cutoff time.Time) {
	if state == nil || len(state.events) == 0 {
		return
	}

	cutIndex := 0
	for cutIndex < len(state.events) {
		evt := state.events[cutIndex]
		if !evt.at.Before(cutoff) {
			break
		}

		if state.counts[evt.query] <= 1 {
			delete(state.counts, evt.query)
		} else {
			state.counts[evt.query]--
		}
		cutIndex++
	}

	if cutIndex > 0 {
		copy(state.events, state.events[cutIndex:])
		state.events = state.events[:len(state.events)-cutIndex]
	}
}
