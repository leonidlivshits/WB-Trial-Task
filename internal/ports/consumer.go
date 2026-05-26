package ports

import (
	"context"
)

type EventConsumer interface {
	Run(ctx context.Context, handler func(context.Context, []byte) error) error
}
