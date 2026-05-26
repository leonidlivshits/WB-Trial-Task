package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wbtrialtask/internal/domain"
)

type benchTopQuery struct {
	snapshot domain.TopSnapshot
}

func (bq benchTopQuery) GetTop(_ context.Context, _ int) (domain.TopSnapshot, error) {
	return bq.snapshot, nil
}

func BenchmarkGetTopReadHeavy(b *testing.B) {
	items := make([]domain.TopItem, 0, 50)
	for i := 1; i <= 50; i++ {
		items = append(items, domain.TopItem{
			Rank:          i,
			Query:         fmt.Sprintf("query-%d", i),
			Score:         float64(1000 - i),
			Count:         int64(1000 - i),
			UniqueSources: int64(500 - i),
		})
	}

	top := benchTopQuery{
		snapshot: domain.TopSnapshot{
			GeneratedAtMS: time.Now().UTC().Add(-50 * time.Millisecond).UnixMilli(),
			WindowSeconds: 300,
			Items:         items,
		},
	}
	api := &API{
		top: top,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}

	parallelismValues := []int{1, 10, 50}
	for _, parallelism := range parallelismValues {
		parallelism := parallelism
		b.Run(fmt.Sprintf("parallel_%d", parallelism), func(b *testing.B) {
			b.ReportAllocs()
			b.SetParallelism(parallelism)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					req := httptest.NewRequest(http.MethodGet, "/api/v1/top?n=10", nil)
					rec := httptest.NewRecorder()
					api.GetTop(rec, req)
					if rec.Code != http.StatusOK {
						b.Errorf("unexpected status: got %d want %d", rec.Code, http.StatusOK)
						return
					}
				}
			})
		})
	}
}
