package antifraud

import (
	"testing"
	"time"

	"wbtrialtask/internal/domain"
)

func TestEvaluator_EvaluateRejectsMissingIdentifiers(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	evaluator := NewFailClosedEvaluator(Config{})

	tests := []struct {
		name string
		evt  domain.SearchEvent
	}{
		{
			name: "missing ip",
			evt: domain.SearchEvent{
				SessionIDHash: "s_session_123",
			},
		},
		{
			name: "missing session",
			evt: domain.SearchEvent{
				IPHash: "ip_abc12345",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rejectErr := evaluator.Evaluate(tt.evt, "nike", now)
			if rejectErr == nil {
				t.Errorf("expected reject error, got nil")
				return
			}
			if rejectErr.Reason != domain.RejectReasonAntiFraudMissingIdentifiers {
				t.Errorf("unexpected reason: got %q want %q", rejectErr.Reason, domain.RejectReasonAntiFraudMissingIdentifiers)
			}
		})
	}
}

func TestEvaluator_EvaluateRateLimitAndBlock(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	evaluator := NewFailClosedEvaluator(Config{
		MaxRequestsPerMinutePerIP:  2,
		IPBlockDuration:            time.Hour,
		ContributionWindow:         time.Hour,
		MaxUniqueQueriesPerSession: 100,
		SessionEntropyWindow:       5 * time.Minute,
		SessionBlockDuration:       time.Hour,
	})

	evt := domain.SearchEvent{
		IPHash:        "ip_abc12345",
		SessionIDHash: "s_session_123",
	}

	if rejectErr := evaluator.Evaluate(evt, "q1", now); rejectErr != nil {
		t.Errorf("first request should pass, got %v", rejectErr)
	}
	if rejectErr := evaluator.Evaluate(evt, "q2", now.Add(10*time.Second)); rejectErr != nil {
		t.Errorf("second request should pass, got %v", rejectErr)
	}

	rejectErr := evaluator.Evaluate(evt, "q3", now.Add(20*time.Second))
	if rejectErr == nil {
		t.Errorf("expected rate limit reject, got nil")
		return
	}
	if rejectErr.Reason != domain.RejectReasonAntiFraudIPRateLimit {
		t.Errorf("unexpected reason: got %q want %q", rejectErr.Reason, domain.RejectReasonAntiFraudIPRateLimit)
	}

	blockErr := evaluator.Evaluate(evt, "q4", now.Add(30*time.Second))
	if blockErr == nil {
		t.Errorf("expected blocked ip reject, got nil")
		return
	}
	if blockErr.Reason != domain.RejectReasonAntiFraudIPBlocked {
		t.Errorf("unexpected reason: got %q want %q", blockErr.Reason, domain.RejectReasonAntiFraudIPBlocked)
	}

	afterBlockErr := evaluator.Evaluate(evt, "q5", now.Add(time.Hour+time.Minute))
	if afterBlockErr != nil {
		t.Errorf("request after block should pass, got %v", afterBlockErr)
	}
}

func TestEvaluator_EvaluateContributionCapByIdentity(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	userID := "u_user_12345"

	tests := []struct {
		name   string
		first  domain.SearchEvent
		second domain.SearchEvent
	}{
		{
			name: "cap by ip",
			first: domain.SearchEvent{
				IPHash:        "ip_abc12345",
				SessionIDHash: "s_session_1",
				UserIDHash:    &userID,
			},
			second: domain.SearchEvent{
				IPHash:        "ip_abc12345",
				SessionIDHash: "s_session_2",
			},
		},
		{
			name: "cap by session",
			first: domain.SearchEvent{
				IPHash:        "ip_abc12345",
				SessionIDHash: "s_session_1",
				UserIDHash:    &userID,
			},
			second: domain.SearchEvent{
				IPHash:        "ip_other_999",
				SessionIDHash: "s_session_1",
			},
		},
		{
			name: "cap by user",
			first: domain.SearchEvent{
				IPHash:        "ip_abc12345",
				SessionIDHash: "s_session_1",
				UserIDHash:    &userID,
			},
			second: domain.SearchEvent{
				IPHash:        "ip_other_999",
				SessionIDHash: "s_session_2",
				UserIDHash:    &userID,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			evaluator := NewFailClosedEvaluator(Config{
				MaxRequestsPerMinutePerIP:  100,
				IPBlockDuration:            time.Hour,
				ContributionWindow:         time.Hour,
				MaxUniqueQueriesPerSession: 100,
				SessionEntropyWindow:       5 * time.Minute,
				SessionBlockDuration:       time.Hour,
			})

			if rejectErr := evaluator.Evaluate(tt.first, "nike", now); rejectErr != nil {
				t.Errorf("first request should pass, got %v", rejectErr)
			}

			rejectErr := evaluator.Evaluate(tt.second, "nike", now.Add(time.Minute))
			if rejectErr == nil {
				t.Errorf("expected contribution cap reject, got nil")
				return
			}
			if rejectErr.Reason != domain.RejectReasonAntiFraudContributionCap {
				t.Errorf("unexpected reason: got %q want %q", rejectErr.Reason, domain.RejectReasonAntiFraudContributionCap)
			}
		})
	}
}

func TestEvaluator_EvaluateContributionCapExpires(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	evaluator := NewFailClosedEvaluator(Config{
		MaxRequestsPerMinutePerIP:  100,
		IPBlockDuration:            time.Hour,
		ContributionWindow:         time.Hour,
		MaxUniqueQueriesPerSession: 100,
		SessionEntropyWindow:       5 * time.Minute,
		SessionBlockDuration:       time.Hour,
	})

	evt := domain.SearchEvent{
		IPHash:        "ip_abc12345",
		SessionIDHash: "s_session_1",
	}

	if rejectErr := evaluator.Evaluate(evt, "nike", now); rejectErr != nil {
		t.Errorf("first request should pass, got %v", rejectErr)
	}
	if rejectErr := evaluator.Evaluate(evt, "nike", now.Add(time.Hour+time.Second)); rejectErr != nil {
		t.Errorf("request after contribution window should pass, got %v", rejectErr)
	}
}

func TestEvaluator_EvaluateSessionEntropyBlock(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	evaluator := NewFailClosedEvaluator(Config{
		MaxRequestsPerMinutePerIP:  100,
		IPBlockDuration:            time.Hour,
		ContributionWindow:         time.Hour,
		MaxUniqueQueriesPerSession: 3,
		SessionEntropyWindow:       5 * time.Minute,
		SessionBlockDuration:       30 * time.Minute,
	})

	evt := domain.SearchEvent{
		IPHash:        "ip_abc12345",
		SessionIDHash: "s_entropy_1",
	}

	queries := []string{"q1", "q2", "q3"}
	for i, query := range queries {
		if rejectErr := evaluator.Evaluate(evt, query, now.Add(time.Duration(i)*time.Minute)); rejectErr != nil {
			t.Errorf("request %d should pass, got %v", i+1, rejectErr)
		}
	}

	rejectErr := evaluator.Evaluate(evt, "q4", now.Add(3*time.Minute))
	if rejectErr == nil {
		t.Errorf("expected entropy reject, got nil")
		return
	}
	if rejectErr.Reason != domain.RejectReasonAntiFraudSessionEntropy {
		t.Errorf("unexpected reason: got %q want %q", rejectErr.Reason, domain.RejectReasonAntiFraudSessionEntropy)
	}

	blockErr := evaluator.Evaluate(evt, "q5", now.Add(4*time.Minute))
	if blockErr == nil {
		t.Errorf("expected session block reject, got nil")
		return
	}
	if blockErr.Reason != domain.RejectReasonAntiFraudSessionEntropy {
		t.Errorf("unexpected reason: got %q want %q", blockErr.Reason, domain.RejectReasonAntiFraudSessionEntropy)
	}
}

func TestEvaluator_EvaluateRejectsEmptyNormalizedQuery(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	evaluator := NewFailClosedEvaluator(Config{})

	rejectErr := evaluator.Evaluate(domain.SearchEvent{
		IPHash:        "ip_abc12345",
		SessionIDHash: "s_session_1",
	}, "", now)
	if rejectErr == nil {
		t.Errorf("expected reject, got nil")
		return
	}
	if rejectErr.Reason != domain.RejectReasonAntiFraudDegraded {
		t.Errorf("unexpected reason: got %q want %q", rejectErr.Reason, domain.RejectReasonAntiFraudDegraded)
	}
}
