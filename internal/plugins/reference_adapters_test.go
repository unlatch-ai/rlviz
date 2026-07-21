package plugins

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

func TestReferenceAdaptersValidate(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is unavailable")
	}
	tests := []struct {
		name    string
		adapter string
		source  string
		format  string
		inspect func(*testing.T, []*model.Record)
	}{
		{
			name:    "inspect ai eval log json",
			adapter: filepath.Join("..", "..", "examples", "adapters", "inspect-ai"),
			source:  filepath.Join("..", "..", "examples", "traces", "inspect-ai-eval.json"),
			format:  "inspect-ai-eval-log-json-v2",
			inspect: inspectInspectAIRecords,
		},
		{
			name:    "verifiers generate outputs",
			adapter: filepath.Join("..", "..", "examples", "adapters", "verifiers"),
			source:  filepath.Join("..", "..", "examples", "traces", "verifiers-generate.json"),
			format:  "prime-verifiers-generate-outputs",
			inspect: inspectVerifiersRecords,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plugin, err := Load(test.adapter)
			if err != nil {
				t.Fatal(err)
			}
			store := &TrustStore{Path: filepath.Join(t.TempDir(), "trust.json")}
			if err := store.Trust(plugin); err != nil {
				t.Fatal(err)
			}
			host := NewHost(store)
			report, err := host.ValidateAdapter(context.Background(), plugin, test.source, "")
			if err != nil {
				t.Fatal(err)
			}
			if report.Format != test.format || !report.Deterministic || report.Records < 6 {
				t.Fatalf("report=%#v", report)
			}
			request, err := NewRequest("stream", test.source, "")
			if err != nil {
				t.Fatal(err)
			}
			var records []*model.Record
			if _, err := host.Stream(context.Background(), plugin, request, func(record *model.Record) error {
				records = append(records, record)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			test.inspect(t, records)
		})
	}
}

func inspectInspectAIRecords(t *testing.T, records []*model.Record) {
	t.Helper()
	foundCompaction := false
	foundTruncation := false
	foundGrader := false
	for _, record := range records {
		event, ok := record.Value.(*model.Event)
		if !ok {
			continue
		}
		if event.AlignmentKey == "context:compaction" {
			data, ok := event.Data.(map[string]any)
			if !ok || data["before_tokens"] != json.Number("8120") || data["after_tokens"] != json.Number("2140") || data["type"] != "summary" {
				t.Fatalf("compaction data=%#v", event.Data)
			}
			assertContext(t, event.Context, "compaction", "source_native", "", 2140, 8120)
			if event.Metadata["provenance"] != "source_native" {
				t.Fatalf("compaction metadata=%#v", event.Metadata)
			}
			foundCompaction = true
		}
		if event.AlignmentKey == "context:truncation" {
			data, ok := event.Data.(map[string]any)
			if !ok || data["operation"] != "truncation" || data["type"] != "trim" {
				t.Fatalf("truncation data=%#v", event.Data)
			}
			assertContext(t, event.Context, "truncation", "source_native", "", 4096, 7900)
			foundTruncation = true
		}
		if event.Kind == "grader" && event.AlignmentKey == "grader:policy_correctness" {
			foundGrader = true
		}
	}
	if !foundCompaction || !foundTruncation || !foundGrader {
		t.Fatalf("mapped semantics: compaction=%t truncation=%t grader=%t", foundCompaction, foundTruncation, foundGrader)
	}
}

func inspectVerifiersRecords(t *testing.T, records []*model.Record) {
	t.Helper()
	wantInputTokens := map[int64]bool{3: false, 6: false}
	for _, record := range records {
		event, ok := record.Value.(*model.Event)
		if !ok || event.Kind != "generation" {
			continue
		}
		data, ok := event.Data.(map[string]any)
		if !ok {
			t.Fatalf("generation data=%#v", event.Data)
		}
		inputTokens, ok := data["prompt_tokens_from_mask"].(json.Number)
		if !ok {
			t.Fatalf("prompt token count=%#v", data["prompt_tokens_from_mask"])
		}
		parsedInputTokens, err := inputTokens.Int64()
		if err != nil {
			t.Fatalf("parse prompt token count: %v", err)
		}
		if _, ok := wantInputTokens[parsedInputTokens]; !ok {
			t.Fatalf("unexpected prompt token count %d", parsedInputTokens)
		}
		assertContext(
			t,
			event.Context,
			"",
			"adapter_derived",
			"count of non-zero entries in TrajectoryStep.tokens.prompt_mask",
			parsedInputTokens,
			-1,
		)
		if event.Metadata["context_provenance"] != "adapter_derived_from_prompt_mask" {
			t.Fatalf("generation metadata=%#v", event.Metadata)
		}
		wantInputTokens[parsedInputTokens] = true
	}
	for inputTokens, found := range wantInputTokens {
		if !found {
			t.Fatalf("no mapped Verifiers generation event with %d input tokens", inputTokens)
		}
	}
}

func assertContext(t *testing.T, context *model.Context, operation, provenance, derivation string, inputTokens, inputTokensBefore int64) {
	t.Helper()
	if context == nil {
		t.Fatal("context is nil")
	}
	if context.Operation != operation || context.Provenance != provenance || context.Derivation != derivation {
		t.Fatalf("context=%#v", context)
	}
	if context.InputTokens == nil || *context.InputTokens != inputTokens {
		t.Fatalf("context input_tokens=%v, want %d", context.InputTokens, inputTokens)
	}
	if inputTokensBefore < 0 {
		if context.InputTokensBefore != nil {
			t.Fatalf("context input_tokens_before=%d, want absent", *context.InputTokensBefore)
		}
		return
	}
	if context.InputTokensBefore == nil || *context.InputTokensBefore != inputTokensBefore {
		t.Fatalf("context input_tokens_before=%v, want %d", context.InputTokensBefore, inputTokensBefore)
	}
}
