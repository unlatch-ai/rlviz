package analyzers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

func TestInputDigestIsCanonicalAndDoesNotMutateInput(t *testing.T) {
	t.Parallel()
	input := testInput(
		event("e2", 2, "tool", map[string]any{"name": "write"}),
		event("e1", 1, "tool", map[string]any{"name": "read"}),
	)
	input.Signals = []model.Signal{
		{RecordType: model.RecordSignal, ID: "s2", TrajectoryID: "trajectory-1", Name: "reward", Value: 2.0},
		{RecordType: model.RecordSignal, ID: "s1", TrajectoryID: "trajectory-1", Name: "reward", Value: 1.0},
	}
	digest, err := InputDigest(input)
	if err != nil {
		t.Fatal(err)
	}
	normalized := NormalizeInput(input)
	want, err := InputDigest(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if digest != want {
		t.Fatalf("digest = %q, want %q", digest, want)
	}
	if input.Events[0].ID != "e2" || input.Signals[0].ID != "s2" {
		t.Fatal("InputDigest mutated caller input")
	}
}

func TestValidateInputRejectsInvalidCanonicalReferences(t *testing.T) {
	t.Parallel()
	tests := map[string]Input{
		"wrong record type":      testInput(model.Event{ID: "e1", TrajectoryID: "trajectory-1", Sequence: 1, Kind: "tool"}),
		"unsupported event kind": testInput(event("e1", 1, "made_up", nil)),
		"duplicate sequence":     testInput(event("e1", 1, "tool", nil), event("e2", 1, "tool", nil)),
		"other trajectory":       testInput(model.Event{RecordType: model.RecordEvent, ID: "e1", TrajectoryID: "other", Sequence: 1, Kind: "tool"}),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateInput(input); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	input := testInput(event("e1", 1, "tool", nil))
	input.Signals = []model.Signal{{RecordType: model.RecordSignal, ID: "s1", TrajectoryID: input.TrajectoryID, EventID: "missing", Name: "x", Value: true}}
	if err := ValidateInput(input); err == nil || !strings.Contains(err.Error(), "unknown event") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateOutputEnforcesProvenanceReferencesAndBounds(t *testing.T) {
	t.Parallel()
	input := testInput(event("e1", 1, "tool", nil))
	digest, err := InputDigest(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := Provenance{Name: "example", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64)}
	valid := Output{APIVersion: APIVersion, Provenance: expected, Findings: []Finding{{ID: "f1", TrajectoryID: input.TrajectoryID, EventIDs: []string{"e1"}, Kind: "test", Severity: "info", Title: "Example"}}}
	valid.Provenance.InputDigest = digest
	if err := ValidateOutput(valid, input, expected); err != nil {
		t.Fatal(err)
	}

	tests := map[string]Output{
		"wrong analyzer":         cloneOutput(t, valid, func(o *Output) { o.Provenance.Name = "other" }),
		"wrong input digest":     cloneOutput(t, valid, func(o *Output) { o.Provenance.InputDigest = "sha256:" + strings.Repeat("b", 64) }),
		"unknown event":          cloneOutput(t, valid, func(o *Output) { o.Findings[0].EventIDs = []string{"missing"} }),
		"oversized text":         cloneOutput(t, valid, func(o *Output) { o.Findings[0].Title = strings.Repeat("x", MaxTextBytes+1) }),
		"invalid fingerprint":    cloneOutput(t, valid, func(o *Output) { o.Findings[0].Fingerprint = "not-a-digest" }),
		"id collides with input": cloneOutput(t, valid, func(o *Output) { o.Findings[0].ID = "e1" }),
		"non-scalar signal": cloneOutput(t, valid, func(o *Output) {
			o.Signals = []model.Signal{{RecordType: model.RecordSignal, ID: "derived", TrajectoryID: input.TrajectoryID, Name: "bad", Value: []any{1.0}}}
		}),
	}
	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateOutput(output, input, expected); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func cloneOutput(t *testing.T, input Output, change func(*Output)) Output {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output Output
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	change(&output)
	return output
}

func testInput(events ...model.Event) Input {
	return Input{APIVersion: APIVersion, Operation: OperationAnalyze, TrajectoryID: "trajectory-1", Events: events}
}

func event(id string, sequence int64, kind string, input any) model.Event {
	return model.Event{RecordType: model.RecordEvent, ID: id, TrajectoryID: "trajectory-1", Sequence: sequence, Kind: kind, Input: input}
}
