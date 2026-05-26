package contracts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"wbtrialtask/internal/domain"
)

func repoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Errorf("getwd: %v", err)
		return ""
	}

	dir := wd
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "docs", "schemas", "search_event.v1.schema.json")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}

	t.Errorf("repo root not found from %q", wd)
	return ""
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read file %q: %v", path, err)
		return nil
	}
	return b
}

func TestSearchEventValidator_ValidateAndMap(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	if root == "" {
		return
	}

	validator, err := NewSearchEventValidator(filepath.Join(root, "docs", "schemas", "search_event.v1.schema.json"))
	if err != nil {
		t.Errorf("create validator: %v", err)
		return
	}

	tests := []struct {
		name       string
		payload    []byte
		wantErr    bool
		wantReason domain.RejectReason
		check      func(t *testing.T, evt domain.SearchEvent)
	}{
		{
			name:    "valid full payload maps fields",
			payload: mustReadFile(t, filepath.Join(root, "docs", "contracts", "examples", "valid", "01_authenticated_full.json")),
			wantErr: false,
			check: func(t *testing.T, evt domain.SearchEvent) {
				t.Helper()
				if evt.EventID == "" {
					t.Errorf("event_id is empty")
				}
				if evt.AuthState != "authenticated" {
					t.Errorf("unexpected auth_state: %q", evt.AuthState)
				}
				if evt.UserIDHash == nil || *evt.UserIDHash == "" {
					t.Errorf("user_id_hash should be mapped for authenticated user")
				}
			},
		},
		{
			name:       "schema invalid payload rejected",
			payload:    mustReadFile(t, filepath.Join(root, "docs", "contracts", "examples", "invalid", "04_invalid_source_enum.json")),
			wantErr:    true,
			wantReason: domain.RejectReasonSchemaValidation,
		},
		{
			name:       "malformed json rejected",
			payload:    []byte("{bad json"),
			wantErr:    true,
			wantReason: domain.RejectReasonInvalidJSON,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			evt, err := validator.ValidateAndMap(tt.payload)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
				return
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
				return
			}
			if !tt.wantErr {
				if tt.check != nil {
					tt.check(t, evt)
				}
				return
			}

			var rejectErr *domain.RejectError
			if !errors.As(err, &rejectErr) || rejectErr == nil {
				t.Errorf("expected domain.RejectError, got %T", err)
				return
			}
			if rejectErr.Reason != tt.wantReason {
				t.Errorf("unexpected reject reason: got %q want %q", rejectErr.Reason, tt.wantReason)
			}
		})
	}
}
