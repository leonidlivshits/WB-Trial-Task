package domain

type SearchEvent struct {
	SchemaVersion     int
	EventType         string
	EventID           string
	IngestedAtMS      int64
	EventTimeMS       int64
	QueryRaw          string
	Source            string
	Platform          string
	AppVersion        string
	IPHash            string
	SessionIDHash     string
	AuthState         string
	UserIDHash        *string
	IsTestTraffic     bool
	IsReplay          bool
	ProducerRegion    string
	NormalizerVersion string
}

type TopItem struct {
	Rank          int
	Query         string
	Score         float64
	Count         int64
	UniqueSources int64
}

type TopSnapshot struct {
	GeneratedAtMS int64
	WindowSeconds int
	Items         []TopItem
}
