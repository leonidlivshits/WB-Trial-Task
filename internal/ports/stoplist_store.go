package ports

import "context"

type StopListStore interface {
	Add(ctx context.Context, word string) (version int64, err error)
	Remove(ctx context.Context, word string) (version int64, err error)
	List(ctx context.Context) (words []string, version int64, err error)
}
