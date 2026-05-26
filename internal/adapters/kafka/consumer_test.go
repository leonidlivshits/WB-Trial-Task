package kafka

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type fetchResult struct {
	msg kafkago.Message
	err error
}

type stubReader struct {
	fetchResults []fetchResult
	fetchIndex   int

	commitErrs  []error
	commitIndex int

	commitCalls int
	committed   []kafkago.Message
	closeCalls  int
}

func (s *stubReader) FetchMessage(ctx context.Context) (kafkago.Message, error) {
	if s.fetchIndex >= len(s.fetchResults) {
		<-ctx.Done()
		return kafkago.Message{}, ctx.Err()
	}
	result := s.fetchResults[s.fetchIndex]
	s.fetchIndex++
	if result.err != nil {
		return kafkago.Message{}, result.err
	}
	return result.msg, nil
}

func (s *stubReader) CommitMessages(_ context.Context, msgs ...kafkago.Message) error {
	s.commitCalls++
	s.committed = append(s.committed, msgs...)
	if s.commitIndex >= len(s.commitErrs) {
		return nil
	}
	err := s.commitErrs[s.commitIndex]
	s.commitIndex++
	return err
}

func (s *stubReader) Close() error {
	s.closeCalls++
	return nil
}

func TestConsumer_RunCommitsAfterSuccess(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msg := kafkago.Message{Topic: "search.events.v1", Partition: 1, Offset: 42, Value: []byte(`{"ok":true}`)}
	rd := &stubReader{
		fetchResults: []fetchResult{
			{msg: msg},
			{err: context.Canceled},
		},
	}
	consumer := newConsumerForTests(Config{
		Enabled:              true,
		MaxProcessingRetries: 0,
		CommitRetries:        0,
		RetryBackoff:         0,
		CommitRetryBackoff:   0,
	}, rd)

	handlerCalls := 0
	err := consumer.Run(ctx, func(_ context.Context, payload []byte) error {
		handlerCalls++
		if string(payload) != `{"ok":true}` {
			t.Errorf("unexpected payload: %s", string(payload))
		}
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context canceled stop, got %v", err)
	}
	if handlerCalls != 1 {
		t.Errorf("unexpected handler calls: got %d want %d", handlerCalls, 1)
	}
	if rd.commitCalls != 1 {
		t.Errorf("unexpected commit calls: got %d want %d", rd.commitCalls, 1)
	}
	if len(rd.committed) != 1 || rd.committed[0].Offset != 42 {
		t.Errorf("unexpected committed messages: %+v", rd.committed)
	}
	if rd.closeCalls != 1 {
		t.Errorf("reader should be closed once, got %d", rd.closeCalls)
	}
}

func TestConsumer_RunContinuesAfterRebalanceError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msg := kafkago.Message{Topic: "search.events.v1", Partition: 1, Offset: 43, Value: []byte(`{"ok":true}`)}
	rd := &stubReader{
		fetchResults: []fetchResult{
			{err: kafkago.ErrGenerationEnded},
			{msg: msg},
			{err: context.Canceled},
		},
	}
	consumer := newConsumerForTests(Config{
		Enabled:              true,
		MaxProcessingRetries: 0,
		CommitRetries:        0,
		RetryBackoff:         0,
		CommitRetryBackoff:   0,
	}, rd)

	handlerCalls := 0
	err := consumer.Run(ctx, func(_ context.Context, _ []byte) error {
		handlerCalls++
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context canceled stop, got %v", err)
	}
	if handlerCalls != 1 {
		t.Errorf("unexpected handler calls: got %d want %d", handlerCalls, 1)
	}
	if rd.commitCalls != 1 {
		t.Errorf("unexpected commit calls: got %d want %d", rd.commitCalls, 1)
	}
}

func TestConsumer_RunRetriesHandlerBeforeCommit(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msg := kafkago.Message{Topic: "search.events.v1", Partition: 1, Offset: 100, Value: []byte(`{}`)}
	rd := &stubReader{
		fetchResults: []fetchResult{
			{msg: msg},
			{err: context.Canceled},
		},
	}
	consumer := newConsumerForTests(Config{
		Enabled:              true,
		MaxProcessingRetries: 2,
		RetryBackoff:         0,
		CommitRetries:        0,
		CommitRetryBackoff:   0,
	}, rd)

	handlerCalls := 0
	err := consumer.Run(ctx, func(_ context.Context, _ []byte) error {
		handlerCalls++
		if handlerCalls < 3 {
			return errors.New("transient handler error")
		}
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context canceled stop, got %v", err)
	}
	if handlerCalls != 3 {
		t.Errorf("unexpected handler calls: got %d want %d", handlerCalls, 3)
	}
	if rd.commitCalls != 1 {
		t.Errorf("commit should happen once after successful retry, got %d", rd.commitCalls)
	}
}

func TestConsumer_RunStopsWithoutCommitWhenHandlerRetriesExhausted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	msg := kafkago.Message{Topic: "search.events.v1", Partition: 2, Offset: 7, Value: []byte(`{}`)}
	rd := &stubReader{
		fetchResults: []fetchResult{
			{msg: msg},
		},
	}
	consumer := newConsumerForTests(Config{
		Enabled:              true,
		MaxProcessingRetries: 1,
		RetryBackoff:         0,
		CommitRetries:        0,
		CommitRetryBackoff:   0,
	}, rd)

	err := consumer.Run(ctx, func(_ context.Context, _ []byte) error {
		return errors.New("permanent handler error")
	})

	if err == nil {
		t.Errorf("expected handler error, got nil")
		return
	}
	if !strings.Contains(err.Error(), "kafka handler failed") {
		t.Errorf("unexpected error text: %v", err)
	}
	if rd.commitCalls != 0 {
		t.Errorf("commit must not happen after failed processing, got %d calls", rd.commitCalls)
	}
}

func TestConsumer_RunRetriesCommit(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msg := kafkago.Message{Topic: "search.events.v1", Partition: 0, Offset: 55, Value: []byte(`{}`)}
	rd := &stubReader{
		fetchResults: []fetchResult{
			{msg: msg},
			{err: context.Canceled},
		},
		commitErrs: []error{
			errors.New("temporary commit 1"),
			errors.New("temporary commit 2"),
			nil,
		},
	}
	consumer := newConsumerForTests(Config{
		Enabled:              true,
		MaxProcessingRetries: 0,
		RetryBackoff:         0,
		CommitRetries:        2,
		CommitRetryBackoff:   0,
	}, rd)

	err := consumer.Run(ctx, func(_ context.Context, _ []byte) error {
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context canceled stop, got %v", err)
	}
	if rd.commitCalls != 3 {
		t.Errorf("unexpected commit attempts: got %d want %d", rd.commitCalls, 3)
	}
}

func TestConsumer_RunFailsWhenCommitRetriesExhausted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	msg := kafkago.Message{Topic: "search.events.v1", Partition: 3, Offset: 11, Value: []byte(`{}`)}
	rd := &stubReader{
		fetchResults: []fetchResult{
			{msg: msg},
		},
		commitErrs: []error{
			errors.New("temporary commit 1"),
			errors.New("temporary commit 2"),
		},
	}
	consumer := newConsumerForTests(Config{
		Enabled:              true,
		MaxProcessingRetries: 0,
		RetryBackoff:         0,
		CommitRetries:        1,
		CommitRetryBackoff:   0,
	}, rd)

	err := consumer.Run(ctx, func(_ context.Context, _ []byte) error {
		return nil
	})

	if err == nil {
		t.Errorf("expected commit error, got nil")
		return
	}
	if !strings.Contains(err.Error(), "kafka commit failed") {
		t.Errorf("unexpected error text: %v", err)
	}
	if rd.commitCalls != 2 {
		t.Errorf("unexpected commit attempts: got %d want %d", rd.commitCalls, 2)
	}
}

func TestNormalizeConfig_Defaults(t *testing.T) {
	t.Parallel()

	cfg := normalizeConfig(Config{})

	if len(cfg.Brokers) == 0 {
		t.Errorf("brokers must have default value")
	}
	if cfg.Topic == "" {
		t.Errorf("topic must have default value")
	}
	if cfg.GroupID == "" {
		t.Errorf("group id must have default value")
	}
	if cfg.MinBytes <= 0 || cfg.MaxBytes <= 0 {
		t.Errorf("min/max bytes must be positive, got min=%d max=%d", cfg.MinBytes, cfg.MaxBytes)
	}
	if cfg.MaxWait <= 0 {
		t.Errorf("max wait must be positive")
	}
	if cfg.StartOffset != kafkago.FirstOffset {
		t.Errorf("unexpected start offset: got %d want %d", cfg.StartOffset, kafkago.FirstOffset)
	}
	if cfg.RetryBackoff <= 0 || cfg.RetryBackoffMax <= 0 {
		t.Errorf("retry backoffs must be positive")
	}
	if cfg.CommitRetryBackoff <= 0 {
		t.Errorf("commit retry backoff must be positive")
	}
}

func TestBackoff_ExponentialWithCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		attempt int
		base    time.Duration
		max     time.Duration
		want    time.Duration
	}{
		{
			name:    "first attempt uses base",
			attempt: 0,
			base:    100 * time.Millisecond,
			max:     2 * time.Second,
			want:    100 * time.Millisecond,
		},
		{
			name:    "second attempt doubles",
			attempt: 1,
			base:    100 * time.Millisecond,
			max:     2 * time.Second,
			want:    200 * time.Millisecond,
		},
		{
			name:    "cap reached",
			attempt: 10,
			base:    100 * time.Millisecond,
			max:     500 * time.Millisecond,
			want:    500 * time.Millisecond,
		},
		{
			name:    "zero base gives zero",
			attempt: 5,
			base:    0,
			max:     time.Second,
			want:    0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := backoff(tt.attempt, tt.base, tt.max)
			if got != tt.want {
				t.Errorf("backoff(%d, %s, %s) = %s, want %s", tt.attempt, tt.base, tt.max, got, tt.want)
			}
		})
	}
}
