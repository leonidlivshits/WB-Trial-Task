package window

import "testing"

func TestEngine_SnapshotTopN(t *testing.T) {
	t.Parallel()

	engine := NewEngine(300)
	base := int64(1_700_000_000_000)

	engine.Add("nike", 1, base)
	engine.Add("iphone", 3, base+1000)
	engine.Add("dress", 2, base+2000)

	snap := engine.Snapshot(2)
	if len(snap.Items) != 2 {
		t.Errorf("unexpected top length: got %d want %d", len(snap.Items), 2)
		return
	}
	if snap.Items[0].Query != "iphone" {
		t.Errorf("unexpected rank1 query: got %q want %q", snap.Items[0].Query, "iphone")
	}
	if snap.Items[1].Query != "dress" {
		t.Errorf("unexpected rank2 query: got %q want %q", snap.Items[1].Query, "dress")
	}
	if snap.WindowSeconds != 300 {
		t.Errorf("unexpected window seconds: got %d want %d", snap.WindowSeconds, 300)
	}
}

func TestEngine_EvictsExpiredBuckets(t *testing.T) {
	t.Parallel()

	engine := NewEngine(5)
	base := int64(1_700_000_000_000)

	// В 0 сек
	engine.Add("old-query", 1, base)
	// В 6 сек запрос из 0 сек уже должен выйти из окна 5 секунд
	engine.Add("new-query", 1, base+6000)

	snap := engine.Snapshot(10)
	if len(snap.Items) != 1 {
		t.Errorf("unexpected items length: got %d want %d", len(snap.Items), 1)
		return
	}
	if snap.Items[0].Query != "new-query" {
		t.Errorf("unexpected remaining query: got %q want %q", snap.Items[0].Query, "new-query")
	}
}

func TestEngine_ReusesBucketAndSubtractsOldContribution(t *testing.T) {
	t.Parallel()

	engine := NewEngine(3)
	base := int64(1_700_000_000_000)

	// Секунда 0 попадает в бакет по модулю 3
	engine.Add("q1", 1, base)
	// Секунда 3 использует тот же индекс бакета, вклад q1 должен вычесться
	engine.Add("q2", 1, base+3000)

	snap := engine.Snapshot(10)
	if len(snap.Items) != 1 {
		t.Errorf("unexpected items length: got %d want %d", len(snap.Items), 1)
		return
	}
	if snap.Items[0].Query != "q2" {
		t.Errorf("unexpected remaining query: got %q want %q", snap.Items[0].Query, "q2")
	}
}
