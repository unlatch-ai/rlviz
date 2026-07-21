package alignment

import (
	"errors"
	"reflect"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

func event(kind string, data any) model.Event {
	return model.Event{Kind: kind, Data: data}
}

func keyed(kind, key string, data any) model.Event {
	return model.Event{Kind: kind, AlignmentKey: key, Data: data}
}

func operations(result Result) []Operation {
	resultOps := make([]Operation, len(result.Steps))
	for i := range result.Steps {
		resultOps[i] = result.Steps[i].Operation
	}
	return resultOps
}

func TestEquivalentReasoningLeadingToSameAction(t *testing.T) {
	left := []model.Event{event("reasoning", "check weather first"), event("tool_call", map[string]any{"name": "weather", "city": "SF"})}
	right := []model.Event{event("reasoning", "I should inspect conditions"), event("tool_call", map[string]any{"city": "SF", "name": "weather"})}

	result := Align(left, right)
	if got, want := operations(result), []Operation{Match, Match}; !reflect.DeepEqual(got, want) {
		t.Fatalf("operations = %v, want %v", got, want)
	}
	if result.CommonBehavioralPrefix != 1 || result.FirstMeaningfulDivergence != nil {
		t.Fatalf("unexpected summary: %+v", result)
	}
}

func TestRetryInsertionAndLaterRealignment(t *testing.T) {
	left := []model.Event{
		keyed("tool_call", "search", map[string]any{"q": "x"}),
		keyed("observation", "answer", "ok"),
	}
	right := []model.Event{
		keyed("tool_call", "search", map[string]any{"q": "x"}),
		keyed("tool_call", "retry", map[string]any{"q": "x"}),
		keyed("observation", "answer", "ok"),
	}
	result := Align(left, right)
	if got, want := operations(result), []Operation{Match, Insert, Match}; !reflect.DeepEqual(got, want) {
		t.Fatalf("operations = %v, want %v", got, want)
	}
	if result.CommonBehavioralPrefix != 1 || result.FirstMeaningfulDivergence == nil || *result.FirstMeaningfulDivergence != 1 {
		t.Fatalf("unexpected divergence: %+v", result)
	}
	if result.LaterRealignment == nil || *result.LaterRealignment != 2 {
		t.Fatalf("unexpected realignment: %+v", result)
	}
}

func TestDeletionAndLaterRealignment(t *testing.T) {
	left := []model.Event{keyed("action", "a", nil), keyed("action", "removed", nil), keyed("reward", "r", 1)}
	right := []model.Event{keyed("action", "a", nil), keyed("reward", "r", 9)}
	result := Align(left, right)
	if got, want := operations(result), []Operation{Match, Delete, Match}; !reflect.DeepEqual(got, want) {
		t.Fatalf("operations = %v, want %v", got, want)
	}
}

func TestBehavioralDifferencesReplace(t *testing.T) {
	tests := []struct {
		name  string
		left  model.Event
		right model.Event
	}{
		{"state", model.Event{Kind: "state", StateHash: "old"}, model.Event{Kind: "state", StateHash: "new"}},
		{"reward", event("reward", 1), event("reward", 0)},
		{"error", event("error", map[string]any{"code": "timeout"}), event("error", map[string]any{"code": "denied"})},
		{"termination", event("termination", "success"), event("termination", "max_steps")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Align([]model.Event{test.left}, []model.Event{test.right})
			if got := operations(result); !reflect.DeepEqual(got, []Operation{Replace}) {
				t.Fatalf("operations = %v", got)
			}
			if result.FirstMeaningfulDivergence == nil || *result.FirstMeaningfulDivergence != 0 {
				t.Fatalf("unexpected summary: %+v", result)
			}
		})
	}
}

func TestAlignmentKeyHasPrecedence(t *testing.T) {
	left := model.Event{Kind: "tool_call", AlignmentKey: "turn-3", StateHash: "a", Data: "left"}
	right := model.Event{Kind: "error", AlignmentKey: "turn-3", StateHash: "b", Data: "right"}
	result := Align([]model.Event{left}, []model.Event{right})
	if got := operations(result); !reflect.DeepEqual(got, []Operation{Match}) {
		t.Fatalf("operations = %v", got)
	}
}

func TestEmptyInputs(t *testing.T) {
	if result := Align(nil, nil); len(result.Steps) != 0 || result.FirstMeaningfulDivergence != nil {
		t.Fatalf("empty alignment = %+v", result)
	}
	right := Align(nil, []model.Event{event("action", "x")})
	if got := operations(right); !reflect.DeepEqual(got, []Operation{Insert}) {
		t.Fatalf("operations = %v", got)
	}
	left := Align([]model.Event{event("action", "x")}, nil)
	if got := operations(left); !reflect.DeepEqual(got, []Operation{Delete}) {
		t.Fatalf("operations = %v", got)
	}
}

func TestFingerprintIgnoresTransportAndReasoningText(t *testing.T) {
	one := model.Event{ID: "one", Sequence: 1, Timestamp: "now", Kind: "reasoning", Data: "alpha"}
	two := model.Event{ID: "two", Sequence: 99, Timestamp: "later", Kind: "reasoning", Data: "beta"}
	if got, want := FingerprintEvent(one), FingerprintEvent(two); got != want {
		t.Fatalf("reasoning fingerprints differ:\n%+v\n%+v", got, want)
	}

	a := event("tool_call", map[string]any{"b": 2, "a": 1})
	b := event("tool_call", map[string]any{"a": 1, "b": 2})
	if got, want := FingerprintEvent(a), FingerprintEvent(b); got != want {
		t.Fatalf("map order changed fingerprint:\n%+v\n%+v", got, want)
	}
}

func TestDeterministicAcrossRuns(t *testing.T) {
	left := []model.Event{event("reasoning", "x"), event("action", map[string]any{"x": 1}), event("reward", 1)}
	right := []model.Event{event("reasoning", "y"), event("action", map[string]any{"x": 2}), event("reward", 1)}
	want := Align(left, right)
	for i := 0; i < 100; i++ {
		if got := Align(left, right); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d was nondeterministic:\n%+v\n%+v", i, got, want)
		}
	}
}

func TestNarrativeInsertionIsNotMeaningfulDivergence(t *testing.T) {
	left := []model.Event{event("reasoning", "a"), keyed("action", "act", nil)}
	right := []model.Event{event("message", "extra"), event("reasoning", "b"), keyed("action", "act", nil)}
	result := Align(left, right)
	if result.FirstMeaningfulDivergence != nil {
		t.Fatalf("narrative-only insertion marked meaningful: %+v", result)
	}
}

func TestLongIdenticalAndNearIdenticalAlignments(t *testing.T) {
	const count = 10_000
	left := make([]model.Event, count)
	right := make([]model.Event, count)
	for index := range left {
		left[index] = keyed("tool", "step", nil)
		right[index] = keyed("tool", "step", nil)
	}
	result, complexity, err := AlignBounded(left, right, 100, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Steps) != count || complexity.CommonSuffix != count || complexity.WorkCells != 1 {
		t.Fatalf("identical complexity=%+v steps=%d", complexity, len(result.Steps))
	}
	for index, step := range result.Steps {
		if step.Operation != Match || *step.LeftIndex != index || *step.RightIndex != index {
			t.Fatalf("step %d = %+v", index, step)
		}
	}

	right[count/2] = keyed("tool", "changed", nil)
	result, complexity, err = AlignBounded(left, right, 100_000, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if complexity.CommonSuffix != count-count/2-1 || complexity.MiddleLeft != count/2+1 || complexity.MiddleRight != count/2+1 || complexity.WorkCells >= 100_000 {
		t.Fatalf("near-identical complexity=%+v", complexity)
	}
	if result.Steps[count/2].Operation != Replace || result.FirstMeaningfulDivergence == nil || *result.FirstMeaningfulDivergence != count/2 {
		t.Fatalf("middle step=%+v summary=%+v", result.Steps[count/2], result)
	}
}

func TestBoundedAlignmentRejectsDivergentMiddleBeforeAllocation(t *testing.T) {
	left := make([]model.Event, 5_000)
	right := make([]model.Event, 5_000)
	for index := range left {
		left[index] = keyed("tool", "left", nil)
		right[index] = keyed("tool", "right", nil)
	}
	_, complexity, err := AlignBounded(left, right, 25_000_000, 20_000_000)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("error=%v complexity=%+v", err, complexity)
	}
	if complexity.MiddleLeft != 5_000 || complexity.MiddleRight != 5_000 || (complexity.WorkCells <= 25_000_000 && complexity.WorkspaceBytes <= 20_000_000) {
		t.Fatalf("complexity=%+v", complexity)
	}
}

func TestTrimmedAlignmentGoldenTieBreaking(t *testing.T) {
	tests := []struct {
		name       string
		left       []model.Event
		right      []model.Event
		operations []Operation
		leftIndex  []int
		rightIndex []int
	}{
		{"suffix wins duplicate", []model.Event{keyed("tool", "a", nil)}, []model.Event{keyed("tool", "a", nil), keyed("tool", "a", nil)}, []Operation{Insert, Match}, []int{-1, 0}, []int{0, 1}},
		{"prefix middle suffix", []model.Event{keyed("tool", "a", nil), keyed("tool", "b", nil), keyed("tool", "a", nil)}, []model.Event{keyed("tool", "a", nil), keyed("tool", "a", nil)}, []Operation{Match, Delete, Match}, []int{0, 1, 2}, []int{0, -1, 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Align(test.left, test.right)
			if !reflect.DeepEqual(operations(result), test.operations) {
				t.Fatalf("operations=%v", operations(result))
			}
			for index, step := range result.Steps {
				li, ri := -1, -1
				if step.LeftIndex != nil {
					li = *step.LeftIndex
				}
				if step.RightIndex != nil {
					ri = *step.RightIndex
				}
				if li != test.leftIndex[index] || ri != test.rightIndex[index] {
					t.Fatalf("step %d indexes=(%d,%d)", index, li, ri)
				}
			}
		})
	}
}

func TestTrimmedAlignmentMatchesLegacyTieBreakingExhaustively(t *testing.T) {
	sequences := keySequences(4)
	for _, left := range sequences {
		for _, right := range sequences {
			got, want := Align(left, right), legacyAlign(left, right)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("left=%v right=%v\ngot=%+v\nwant=%+v", eventKeys(left), eventKeys(right), got, want)
			}
		}
	}
}

func keySequences(maxLength int) [][]model.Event {
	result := [][]model.Event{nil}
	for length := 1; length <= maxLength; length++ {
		count := 1 << length
		for mask := 0; mask < count; mask++ {
			sequence := make([]model.Event, length)
			for index := range sequence {
				key := "a"
				if mask&(1<<index) != 0 {
					key = "b"
				}
				sequence[index] = keyed("tool", key, nil)
			}
			result = append(result, sequence)
		}
	}
	return result
}

func eventKeys(events []model.Event) []string {
	keys := make([]string, len(events))
	for index := range events {
		keys[index] = events[index].AlignmentKey
	}
	return keys
}

// legacyAlign is the original full-matrix implementation kept as a golden
// oracle for short exhaustive tie-breaking regressions.
func legacyAlign(left, right []model.Event) Result {
	lf, rf := fingerprintAll(left), fingerprintAll(right)
	dp := make([][]int, len(lf)+1)
	for i := range dp {
		dp[i] = make([]int, len(rf)+1)
		dp[i][0] = i
	}
	for j := range dp[0] {
		dp[0][j] = j
	}
	for i := 1; i <= len(lf); i++ {
		for j := 1; j <= len(rf); j++ {
			diagonal := dp[i-1][j-1]
			if !equivalent(lf[i-1], rf[j-1]) {
				diagonal++
			}
			dp[i][j] = min(diagonal, dp[i-1][j]+1, dp[i][j-1]+1)
		}
	}
	steps := make([]Step, 0, max(len(lf), len(rf)))
	for i, j := len(lf), len(rf); i > 0 || j > 0; {
		if i > 0 && j > 0 {
			cost, operation := 1, Replace
			if equivalent(lf[i-1], rf[j-1]) {
				cost, operation = 0, Match
			}
			if dp[i][j] == dp[i-1][j-1]+cost {
				steps = append(steps, pairedStep(operation, i-1, j-1, lf[i-1], rf[j-1]))
				i, j = i-1, j-1
				continue
			}
		}
		if i > 0 && dp[i][j] == dp[i-1][j]+1 {
			li, lcopy := i-1, lf[i-1]
			steps = append(steps, Step{Operation: Delete, LeftIndex: &li, Left: &lcopy, Meaningful: lcopy.Behavioral})
			i--
			continue
		}
		ri, rcopy := j-1, rf[j-1]
		steps = append(steps, Step{Operation: Insert, RightIndex: &ri, Right: &rcopy, Meaningful: rcopy.Behavioral})
		j--
	}
	reverse(steps)
	return summarize(steps)
}
