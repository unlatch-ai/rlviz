package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSchemaDocumentsAreValidJSON(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob(filepath.Join("..", "..", "schemas", "v1alpha1", "*.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 7 {
		t.Fatalf("found %d schema documents, want 7", len(paths))
	}
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
				t.Fatalf("unexpected $schema: %v", document["$schema"])
			}
			if document["$id"] == "" {
				t.Fatal("$id is required")
			}
		})
	}
}

func TestCanonicalSchemaCoversAllRecordTypes(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "schemas", "v1alpha1", "canonical-record.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for _, recordType := range []RecordType{RecordRun, RecordCase, RecordGroup, RecordTrajectory, RecordEvent, RecordSignal, RecordArtifact, RecordComplete} {
		if _, ok := document.Defs[string(recordType)]; !ok {
			t.Errorf("schema has no definition for %q", recordType)
		}
	}
}
