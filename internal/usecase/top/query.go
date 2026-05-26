package top

import (
	"context"

	"wbtrialtask/internal/domain"
	"wbtrialtask/internal/ports"
)

type Service struct {
	reader ports.TopSnapshotReader
}

func NewService(reader ports.TopSnapshotReader) *Service {
	return &Service{reader: reader}
}

func (s *Service) GetTop(_ context.Context, n int) (domain.TopSnapshot, error) {
	if n <= 0 {
		n = 10
	}
	return s.reader.Snapshot(n), nil
}
