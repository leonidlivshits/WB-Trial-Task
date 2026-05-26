package window

import "sync/atomic"

type AtomicVersion struct {
	v atomic.Int64
}

func (a *AtomicVersion) Next() int64 {
	return a.v.Add(1)
}

func (a *AtomicVersion) Current() int64 {
	return a.v.Load()
}
