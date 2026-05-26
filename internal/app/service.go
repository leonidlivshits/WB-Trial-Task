package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"wbtrialtask/internal/ports"
)

type Dependencies struct {
	HTTPServer  *http.Server
	Consumer    ports.EventConsumer
	EventHandle func(context.Context, []byte) error
}

type Service struct {
	deps Dependencies
}

func NewService(deps Dependencies) *Service {
	return &Service{deps: deps}
}

func (s *Service) Run(ctx context.Context) error {
	if s.deps.HTTPServer == nil {
		return errors.New("http server is required")
	}

	errCh := make(chan error, 2)

	go func() {
		if err := s.deps.HTTPServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	if s.deps.Consumer != nil && s.deps.EventHandle != nil {
		go func() {
			if err := s.deps.Consumer.Run(ctx, s.deps.EventHandle); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- err
			}
		}()
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.deps.HTTPServer.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}
