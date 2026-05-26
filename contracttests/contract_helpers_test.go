package contracttests

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

const (
	searchEventSchemaPath = "docs/schemas/search_event.v1.schema.json"
	openAPIContractPath   = "docs/contracts/api.v1.openapi.yaml"
)

func repoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Errorf("getwd: %v", err)
	}

	dir := wd
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, searchEventSchemaPath)
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

func mustListFilesWithExt(t *testing.T, dir, ext string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Errorf("read dir %q: %v", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ext {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}

	sort.Strings(files)
	if len(files) == 0 {
		t.Errorf("no %q files found in %q", ext, dir)
	}
	return files
}

func mustListJSONFiles(t *testing.T, dir string) []string {
	t.Helper()
	return mustListFilesWithExt(t, dir, ".json")
}

func mustMapValue(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := parent[key]
	if !ok {
		t.Errorf("missing key %q", key)
		return nil
	}
	result, ok := value.(map[string]any)
	if !ok {
		t.Errorf("key %q has unexpected type %T", key, value)
		return nil
	}
	return result
}

func mustSliceValue(t *testing.T, parent map[string]any, key string) []any {
	t.Helper()

	value, ok := parent[key]
	if !ok {
		t.Errorf("missing key %q", key)
		return nil
	}
	result, ok := value.([]any)
	if !ok {
		t.Errorf("key %q has unexpected type %T", key, value)
		return nil
	}
	return result
}

func mustStringValue(t *testing.T, parent map[string]any, key string) string {
	t.Helper()

	value, ok := parent[key]
	if !ok {
		t.Errorf("missing key %q", key)
		return ""
	}
	result, ok := value.(string)
	if !ok {
		t.Errorf("key %q has unexpected type %T", key, value)
		return ""
	}
	return result
}

func mustIntValue(t *testing.T, parent map[string]any, key string) int {
	t.Helper()

	value, ok := parent[key]
	if !ok {
		t.Errorf("missing key %q", key)
		return 0
	}

	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		t.Errorf("key %q has unexpected numeric type %T", key, value)
		return 0
	}
}
