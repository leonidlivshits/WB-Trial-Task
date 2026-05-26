package ports

import "wbtrialtask/internal/domain"

type TopSnapshotReader interface {
	Snapshot(n int) domain.TopSnapshot
}
