package kafka

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type Config struct {
	Enabled bool

	Brokers []string
	Topic   string
	GroupID string

	MinBytes int
	MaxBytes int
	MaxWait  time.Duration

	StartOffset int64

	MaxProcessingRetries int
	RetryBackoff         time.Duration
	RetryBackoffMax      time.Duration

	CommitRetries      int
	CommitRetryBackoff time.Duration
}

type Consumer struct {
	cfg Config

	reader reader
}

type reader interface {
	FetchMessage(ctx context.Context) (kafkago.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafkago.Message) error
	Close() error
}

func NewConsumer(cfg Config) *Consumer {
	normalized := normalizeConfig(cfg)

	consumer := &Consumer{
		cfg: normalized,
	}
	if !normalized.Enabled {
		return consumer
	}

	consumer.reader = kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        normalized.Brokers,
		Topic:          normalized.Topic,
		GroupID:        normalized.GroupID,
		MinBytes:       normalized.MinBytes,
		MaxBytes:       normalized.MaxBytes,
		MaxWait:        normalized.MaxWait,
		CommitInterval: 0,
		StartOffset:    normalized.StartOffset,
	})
	return consumer
}

func newConsumerForTests(cfg Config, rd reader) *Consumer {
	normalized := normalizeConfig(cfg)
	return &Consumer{
		cfg:    normalized,
		reader: rd,
	}
}

func (c *Consumer) Run(ctx context.Context, handler func(context.Context, []byte) error) error {
	if handler == nil {
		return errors.New("kafka handler is required")
	}
	if !c.cfg.Enabled {
		<-ctx.Done()
		return ctx.Err()
	}
	if c.reader == nil {
		return errors.New("kafka reader is required")
	}
	defer func() {
		if err := c.reader.Close(); err != nil {
			log.Printf("kafka consumer close error: %v", err)
		}
	}()

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, kafkago.ErrGenerationEnded) || errors.Is(err, kafkago.ErrGroupClosed) {
				continue
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return fmt.Errorf("kafka fetch failed: %w", err)
		}

		if err := c.handleWithRetry(ctx, handler, msg); err != nil {
			return err
		}
		if err := c.commitWithRetry(ctx, msg); err != nil {
			return err
		}
	}
}

func (c *Consumer) handleWithRetry(ctx context.Context, handler func(context.Context, []byte) error, msg kafkago.Message) error {
	for attempt := 0; ; attempt++ {
		err := handler(ctx, msg.Value)
		if err == nil {
			return nil
		}

		if attempt >= c.cfg.MaxProcessingRetries {
			return fmt.Errorf(
				"kafka handler failed topic=%s partition=%d offset=%d attempts=%d: %w",
				msg.Topic, msg.Partition, msg.Offset, attempt+1, err,
			)
		}

		c.waitBeforeRetry(ctx, backoff(attempt, c.cfg.RetryBackoff, c.cfg.RetryBackoffMax))
	}
}

func (c *Consumer) commitWithRetry(ctx context.Context, msg kafkago.Message) error {
	for attempt := 0; ; attempt++ {
		err := c.reader.CommitMessages(ctx, msg)
		if err == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if attempt >= c.cfg.CommitRetries {
			return fmt.Errorf(
				"kafka commit failed topic=%s partition=%d offset=%d attempts=%d: %w",
				msg.Topic, msg.Partition, msg.Offset, attempt+1, err,
			)
		}

		c.waitBeforeRetry(ctx, c.cfg.CommitRetryBackoff)
	}
}

func (c *Consumer) waitBeforeRetry(ctx context.Context, delay time.Duration) {
	if delay <= 0 {
		return
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
}

func backoff(attempt int, base, max time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	if attempt <= 0 {
		if max > 0 && base > max {
			return max
		}
		return base
	}

	delay := base
	for i := 0; i < attempt; i++ {
		delay *= 2
		if max > 0 && delay >= max {
			return max
		}
	}
	return delay
}

func normalizeConfig(cfg Config) Config {
	if len(cfg.Brokers) == 0 {
		cfg.Brokers = []string{"localhost:9092"}
	}
	if cfg.Topic == "" {
		cfg.Topic = "search.events.v1"
	}
	if cfg.GroupID == "" {
		cfg.GroupID = "trends-service-v1"
	}
	if cfg.MinBytes <= 0 {
		cfg.MinBytes = 1 << 10
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 10 << 20
	}
	if cfg.MaxBytes < cfg.MinBytes {
		cfg.MaxBytes = cfg.MinBytes
	}
	if cfg.MaxWait <= 0 {
		cfg.MaxWait = 500 * time.Millisecond
	}
	if cfg.StartOffset == 0 {
		cfg.StartOffset = kafkago.FirstOffset
	}

	if cfg.MaxProcessingRetries < 0 {
		cfg.MaxProcessingRetries = 0
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 100 * time.Millisecond
	}
	if cfg.RetryBackoffMax <= 0 {
		cfg.RetryBackoffMax = 3 * time.Second
	}
	if cfg.RetryBackoff > cfg.RetryBackoffMax {
		cfg.RetryBackoff = cfg.RetryBackoffMax
	}
	if cfg.CommitRetries < 0 {
		cfg.CommitRetries = 0
	}
	if cfg.CommitRetryBackoff <= 0 {
		cfg.CommitRetryBackoff = 100 * time.Millisecond
	}
	return cfg
}
