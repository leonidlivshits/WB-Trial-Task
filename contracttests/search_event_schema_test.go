package contracttests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func mustCompileSearchEventSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	schemaPath := filepath.Join(repoRoot(t), searchEventSchemaPath)
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()

	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		t.Errorf("compile schema %q: %v", schemaPath, err)
		return nil
	}
	return schema
}

func mustLoadInstance(t *testing.T, path string) any {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Errorf("open instance %q: %v", path, err)
		return nil
	}
	defer f.Close()

	instance, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Errorf("unmarshal instance %q: %v", path, err)
		return nil
	}
	return instance
}

func TestSearchEventSchema_FixtureSuites(t *testing.T) {
	t.Parallel()

	schema := mustCompileSearchEventSchema(t)
	if schema == nil {
		return
	}
	root := repoRoot(t)

	tests := []struct {
		name       string
		fixtureDir string
		wantValid  bool
	}{
		{
			name:       "valid fixtures",
			fixtureDir: filepath.Join(root, "docs", "contracts", "examples", "valid"),
			wantValid:  true,
		},
		{
			name:       "invalid fixtures",
			fixtureDir: filepath.Join(root, "docs", "contracts", "examples", "invalid"),
			wantValid:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, path := range mustListJSONFiles(t, tt.fixtureDir) {
				path := path
				t.Run(filepath.Base(path), func(t *testing.T) {
					t.Parallel()

					instance := mustLoadInstance(t, path)
					if instance == nil {
						return
					}
					err := schema.Validate(instance)

					if tt.wantValid && err != nil {
						t.Errorf("expected valid fixture, got error: %v", err)
					}
					if !tt.wantValid && err == nil {
						t.Errorf("expected invalid fixture to fail validation")
					}
				})
			}
		})
	}
}

func TestSearchEventSchema_AuthStateConditionalRules(t *testing.T) {
	t.Parallel()

	schema := mustCompileSearchEventSchema(t)
	if schema == nil {
		return
	}

	tests := []struct {
		name      string
		instance  map[string]any
		wantValid bool
	}{
		{
			name: "authenticated requires non-null user_id_hash",
			instance: map[string]any{
				"schema_version":  1,
				"event_type":      "search.query.submitted",
				"event_id":        "0196b4d0-5f0f-7c2f-a5b4-8c4c9b9f3a10",
				"ingested_at_ms":  int64(1779656405123),
				"query_raw":       "buy iphone",
				"source":          "search_bar",
				"ip_hash":         "ip_7c9f4dA1b2",
				"session_id_hash": "s_92ab10QwEr",
				"auth_state":      "authenticated",
				"user_id_hash":    nil,
			},
			wantValid: false,
		},
		{
			name: "anonymous requires null user_id_hash",
			instance: map[string]any{
				"schema_version":  1,
				"event_type":      "search.query.submitted",
				"event_id":        "0196b4d0-6a20-7f2d-b18d-6f0ab4e2c111",
				"ingested_at_ms":  int64(1779656406123),
				"query_raw":       "nike sneakers",
				"source":          "catalog",
				"ip_hash":         "ip_A1b2C3d4E5",
				"session_id_hash": "s_Z9y8X7w6V5",
				"auth_state":      "anonymous",
				"user_id_hash":    "u_should_not_exist",
			},
			wantValid: false,
		},
		{
			name: "authenticated with user_id_hash is valid",
			instance: map[string]any{
				"schema_version":  1,
				"event_type":      "search.query.submitted",
				"event_id":        "0196b4d0-7b31-7d9f-a0f4-9137be44d222",
				"ingested_at_ms":  int64(1779656407123),
				"query_raw":       "summer dress",
				"source":          "suggest",
				"ip_hash":         "ip_Xx77Yy88Zz",
				"session_id_hash": "s_Kk11Ll22Mm",
				"auth_state":      "authenticated",
				"user_id_hash":    "u_Pp33Qq44Rr",
			},
			wantValid: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := schema.Validate(tt.instance)
			if tt.wantValid && err != nil {
				t.Errorf("expected valid instance, got error: %v", err)
			}
			if !tt.wantValid && err == nil {
				t.Errorf("expected invalid instance to fail validation")
			}
		})
	}
}
