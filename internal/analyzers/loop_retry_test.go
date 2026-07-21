package analyzers

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

func TestLoopRetryDetectsRetryIgnoringResultsAndObservations(t *testing.T) {
	t.Parallel()
	a := event("a1", 1, "tool", map[string]any{"name": "shell", "args": map[string]any{"cmd": "go test"}})
	b := event("observation", 2, "observation", nil)
	c := event("a2", 3, "tool", a.Input)
	d := event("a3", 4, "tool", a.Input)
	a.Output, c.Output, d.Output = "failed", "failed differently", "passed"
	output, err := (LoopRetry{}).Analyze(context.Background(), testInput(a, b, c, d))
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Findings) != 1 || output.Findings[0].Kind != "retry" {
		t.Fatalf("findings = %#v", output.Findings)
	}
	if !reflect.DeepEqual(output.Findings[0].EventIDs, []string{"a1", "a2", "a3"}) {
		t.Fatalf("event ids = %v", output.Findings[0].EventIDs)
	}
	if len(output.Signals) != 1 || output.Signals[0].EventID != "a3" {
		t.Fatalf("signals = %#v", output.Signals)
	}
}

func TestLoopRetryDetectsMultiActionLoop(t *testing.T) {
	t.Parallel()
	var events []model.Event
	for i, name := range []string{"left", "right", "left", "right", "left", "right"} {
		events = append(events, event(fmt.Sprintf("e%d", i), int64(i), "environment_action", map[string]any{"action": name}))
	}
	output, err := (LoopRetry{}).Analyze(context.Background(), testInput(events...))
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Findings) != 1 || output.Findings[0].Kind != "loop" {
		t.Fatalf("findings = %#v", output.Findings)
	}
	if got := output.Findings[0].Metadata["period"]; got != float64(2) {
		t.Fatalf("period = %v", got)
	}
}

func TestLoopRetryIsDeterministicAndAvoidsFalsePositives(t *testing.T) {
	t.Parallel()
	input := testInput(event("a", 0, "tool", "one"), event("b", 1, "tool", "two"), event("c", 2, "tool", "three"))
	first, err := (LoopRetry{}).Analyze(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := (LoopRetry{}).Analyze(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("analysis was not deterministic")
	}
	if len(first.Findings) != 0 || len(first.Signals) != 0 {
		t.Fatalf("unexpected output = %#v", first)
	}
}

func TestLoopRetryUsesUniqueIDsForSeparateOccurrences(t *testing.T) {
	t.Parallel()
	var events []model.Event
	for i := 0; i < 6; i++ {
		events = append(events, event(fmt.Sprintf("e%d", i), int64(i), "tool", "same"))
	}
	output, err := (LoopRetry{}).Analyze(context.Background(), testInput(events...))
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Findings) != 2 || output.Findings[0].ID == output.Findings[1].ID {
		t.Fatalf("finding IDs = %#v", output.Findings)
	}
}

func TestLoopRetryHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (LoopRetry{}).Analyze(ctx, testInput(event("a", 0, "tool", "one")))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
