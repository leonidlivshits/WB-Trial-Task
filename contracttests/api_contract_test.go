package contracttests

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func mustLoadOpenAPIDoc(t *testing.T) map[string]any {
	t.Helper()

	path := filepath.Join(repoRoot(t), openAPIContractPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read openapi file %q: %v", path, err)
		return nil
	}

	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Errorf("parse openapi yaml %q: %v", path, err)
		return nil
	}
	return doc
}

func mustOperation(t *testing.T, doc map[string]any, path, method string) map[string]any {
	t.Helper()

	paths := mustMapValue(t, doc, "paths")
	pathItem := mustMapValue(t, paths, path)
	return mustMapValue(t, pathItem, method)
}

func TestOpenAPIContract_ParseAndVersion(t *testing.T) {
	t.Parallel()

	doc := mustLoadOpenAPIDoc(t)
	if doc == nil {
		return
	}
	version := mustStringValue(t, doc, "openapi")
	if version != "3.1.0" {
		t.Errorf("unexpected openapi version: got %q want %q", version, "3.1.0")
	}
}

func TestOpenAPIContract_RequiredPathsAndMethods(t *testing.T) {
	t.Parallel()

	doc := mustLoadOpenAPIDoc(t)
	if doc == nil {
		return
	}

	tests := []struct {
		name   string
		path   string
		method string
	}{
		{name: "top read endpoint", path: "/api/v1/top", method: "get"},
		{name: "stop words list endpoint", path: "/api/v1/stop-words", method: "get"},
		{name: "stop words add endpoint", path: "/api/v1/stop-words", method: "post"},
		{name: "stop words delete endpoint", path: "/api/v1/stop-words/{word}", method: "delete"},
		{name: "health endpoint", path: "/api/v1/health", method: "get"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_ = mustOperation(t, doc, tt.path, tt.method)
		})
	}
}

func TestOpenAPIContract_AdminOperationsRequireSecurity(t *testing.T) {
	t.Parallel()

	doc := mustLoadOpenAPIDoc(t)
	if doc == nil {
		return
	}
	tests := []struct {
		name   string
		path   string
		method string
	}{
		{name: "get stop words secured", path: "/api/v1/stop-words", method: "get"},
		{name: "post stop words secured", path: "/api/v1/stop-words", method: "post"},
		{name: "delete stop words secured", path: "/api/v1/stop-words/{word}", method: "delete"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			op := mustOperation(t, doc, tt.path, tt.method)
			securityItems := mustSliceValue(t, op, "security")

			var hasAdminToken bool
			for _, rawItem := range securityItems {
				item, ok := rawItem.(map[string]any)
				if !ok {
					continue
				}
				if _, ok := item["adminToken"]; ok {
					hasAdminToken = true
					break
				}
			}

			if !hasAdminToken {
				t.Errorf("security for %s %s does not contain adminToken", tt.method, tt.path)
			}
		})
	}
}

func TestOpenAPIContract_CoreComponents(t *testing.T) {
	t.Parallel()

	doc := mustLoadOpenAPIDoc(t)
	if doc == nil {
		return
	}
	components := mustMapValue(t, doc, "components")

	securitySchemes := mustMapValue(t, components, "securitySchemes")
	if _, ok := securitySchemes["adminToken"]; !ok {
		t.Errorf("missing security scheme %q", "adminToken")
	}

	schemas := mustMapValue(t, components, "schemas")
	coreSchemas := []string{
		"TopResponse",
		"TopItem",
		"ErrorResponse",
		"StopWordsResponse",
	}
	for _, schemaName := range coreSchemas {
		schemaName := schemaName
		t.Run(schemaName, func(t *testing.T) {
			t.Parallel()
			if _, ok := schemas[schemaName]; !ok {
				t.Errorf("missing schema %q", schemaName)
			}
		})
	}
}

func TestOpenAPIContract_TopNParameterBounds(t *testing.T) {
	t.Parallel()

	doc := mustLoadOpenAPIDoc(t)
	if doc == nil {
		return
	}
	topGetOperation := mustOperation(t, doc, "/api/v1/top", "get")
	params := mustSliceValue(t, topGetOperation, "parameters")

	var nParam map[string]any
	for _, rawParam := range params {
		param, ok := rawParam.(map[string]any)
		if !ok {
			continue
		}
		name, _ := param["name"].(string)
		in, _ := param["in"].(string)
		if name == "n" && in == "query" {
			nParam = param
			break
		}
	}

	if nParam == nil {
		t.Errorf("query parameter %q not found for GET /api/v1/top", "n")
		return
	}

	schema := mustMapValue(t, nParam, "schema")
	if got := mustIntValue(t, schema, "minimum"); got != 1 {
		t.Errorf("unexpected minimum for n: got %d want %d", got, 1)
	}
	if got := mustIntValue(t, schema, "maximum"); got != 100 {
		t.Errorf("unexpected maximum for n: got %d want %d", got, 100)
	}
	if got := mustIntValue(t, schema, "default"); got != 10 {
		t.Errorf("unexpected default for n: got %d want %d", got, 10)
	}
}
