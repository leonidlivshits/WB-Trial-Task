package normalizer

import "testing"

func TestService_Normalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		stopWords []string
		input     string
		want      string
		wantErr   bool
	}{
		{
			name:    "lowercase and trim spaces",
			input:   "  Nike Air  ",
			want:    "nike air",
			wantErr: false,
		},
		{
			name:    "collapse spaces and remove symbols",
			input:   "Nike!!!    Air***Max",
			want:    "nike air max",
			wantErr: false,
		},
		{
			name:      "remove configured stop words",
			stopWords: []string{"купить", "в"},
			input:     "Купить   айфон в москве!!!",
			want:      "айфон москве",
			wantErr:   false,
		},
		{
			name:      "all words removed by stop list returns error",
			stopWords: []string{"купить", "в"},
			input:     "купить в",
			wantErr:   true,
		},
		{
			name:    "single rune query is invalid",
			input:   "a",
			wantErr: true,
		},
		{
			name:    "letters and digits are preserved",
			input:   "iPhone 15 Pro!!",
			want:    "iphone 15 pro",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := NewService(tt.stopWords)
			got, err := svc.Normalize(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
				return
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("unexpected normalized value: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestService_ReplaceStopWords(t *testing.T) {
	t.Parallel()

	svc := NewService(nil)

	before, err := svc.Normalize("купить айфон")
	if err != nil {
		t.Errorf("unexpected error before replace: %v", err)
		return
	}
	if before != "купить айфон" {
		t.Errorf("unexpected value before replace: got %q want %q", before, "купить айфон")
	}

	svc.ReplaceStopWords([]string{"купить"})

	after, err := svc.Normalize("купить айфон")
	if err != nil {
		t.Errorf("unexpected error after replace: %v", err)
		return
	}
	if after != "айфон" {
		t.Errorf("unexpected value after replace: got %q want %q", after, "айфон")
	}
}
