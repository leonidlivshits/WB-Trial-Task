package domain

type StopListSnapshot struct {
	Version     int64
	Words       []string
	UpdatedAtMS int64
}
