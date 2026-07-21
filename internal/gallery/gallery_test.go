package gallery

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

func TestGenerateIsDeterministicAndCanonical(t *testing.T) {
	first, second := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")
	if err := Generate(first); err != nil {
		t.Fatal(err)
	}
	if err := Generate(second); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"coding-agent-bugfix.ndjson", "web-research-agent.ndjson", "checkout-cohort.ndjson"} {
		left, err := os.ReadFile(filepath.Join(first, name))
		if err != nil {
			t.Fatal(err)
		}
		right, err := os.ReadFile(filepath.Join(second, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(left, right) {
			t.Fatalf("%s changed between deterministic generations", name)
		}
		committed, err := os.ReadFile(filepath.Join("..", "..", "examples", "gallery", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(left, committed) {
			t.Fatalf("%s is stale; run make gallery", name)
		}
		if err := model.Decode(bytes.NewReader(left), func(*model.Record) error { return nil }); err != nil {
			t.Fatalf("%s is not canonical: %v", name, err)
		}
	}
}

func TestGalleryHasRequestedShapes(t *testing.T) {
	directory := t.TempDir()
	if err := Generate(directory); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		events       int
		trajectories int
	}{
		{"coding-agent-bugfix.ndjson", 300, 1},
		{"web-research-agent.ndjson", 120, 1},
		{"checkout-cohort.ndjson", 0, 16},
	}
	for _, test := range tests {
		content, _ := os.ReadFile(filepath.Join(directory, test.name))
		events, trajectories := 0, 0
		if err := model.Decode(bytes.NewReader(content), func(record *model.Record) error {
			switch record.Value.(type) {
			case *model.Event:
				events++
			case *model.Trajectory:
				trajectories++
			case *model.Run:
				if record.Value.(*model.Run).Metadata["synthetic"] != true {
					t.Errorf("%s run is not synthetic", test.name)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if test.events > 0 && events != test.events {
			t.Errorf("%s events=%d want %d", test.name, events, test.events)
		}
		if trajectories != test.trajectories {
			t.Errorf("%s trajectories=%d want %d", test.name, trajectories, test.trajectories)
		}
	}
}

func TestGalleryEncodesFailureAndRetryShapeInvariants(t *testing.T) {
	directory := t.TempDir()
	if err := Generate(directory); err != nil {
		t.Fatal(err)
	}

	bugfix, err := os.ReadFile(filepath.Join(directory, "coding-agent-bugfix.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	compactions := 0
	if err := model.Decode(bytes.NewReader(bugfix), func(record *model.Record) error {
		if event, ok := record.Value.(*model.Event); ok {
			kinds = append(kinds, event.Kind)
			if event.Context != nil && event.Context.Operation == "compaction" {
				compactions++
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	longestAlternation := 0
	for start := range kinds {
		length := 0
		for index := start; index < len(kinds); index++ {
			want := "tool"
			if length%2 == 1 {
				want = "error"
			}
			if kinds[index] != want {
				break
			}
			length++
		}
		longestAlternation = max(longestAlternation, length)
	}
	if longestAlternation < 6 {
		t.Fatalf("retry comb has %d alternating tool/error events, want at least 6 (3 retries)", longestAlternation)
	}
	if compactions < 1 {
		t.Fatal("bugfix trace has no source-backed compaction")
	}

	cohort, err := os.ReadFile(filepath.Join(directory, "checkout-cohort.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	failureClasses := map[string]bool{}
	if err := model.Decode(bytes.NewReader(cohort), func(record *model.Record) error {
		if signal, ok := record.Value.(*model.Signal); ok && signal.Name == "failure_class" {
			encoded, marshalErr := json.Marshal(signal.Value)
			if marshalErr != nil {
				return marshalErr
			}
			var class string
			if unmarshalErr := json.Unmarshal(encoded, &class); unmarshalErr != nil {
				return unmarshalErr
			}
			failureClasses[class] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, class := range []string{"policy", "infrastructure"} {
		if !failureClasses[class] {
			t.Fatalf("checkout cohort has no %s failure: %#v", class, failureClasses)
		}
	}
}
