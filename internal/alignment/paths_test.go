package alignment

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

func pathEvent(kind, key string) model.Event {
	return model.Event{Kind: kind, AlignmentKey: key}
}

func TestBuildPathTreeSharedPrefixAndDivergence(t *testing.T) {
	tree := BuildPathTree([]PathTrajectory{
		{ID: "second", Events: []model.Event{pathEvent("message", ""), pathEvent("tool", "search"), pathEvent("error", "timeout")}},
		{ID: "first", Events: []model.Event{pathEvent("tool", "search"), pathEvent("observation", "result")}},
		{ID: "third", Events: []model.Event{pathEvent("tool", "search")}},
	})
	if tree.TrajectoryCount != 3 || tree.TerminalCount != 0 || tree.BehavioralEventCount != 5 || tree.NarrativeEventCount != 1 {
		t.Fatalf("tree counts = %#v", tree)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("roots = %#v", tree.Children)
	}
	search := tree.Children[0]
	if search.Count != 3 || search.TerminalCount != 1 || search.Depth != 0 || !reflect.DeepEqual(search.TrajectoryIDs, []string{"first", "second", "third"}) {
		t.Fatalf("shared prefix = %#v", search)
	}
	if len(search.Children) != 2 || search.Children[0].Depth != 1 || search.Children[1].Depth != 1 {
		t.Fatalf("divergence = %#v", search.Children)
	}
}

func TestBuildPathTreeNarrativeOnlyAndEmpty(t *testing.T) {
	empty := BuildPathTree(nil)
	if empty.TrajectoryCount != 0 || empty.TerminalCount != 0 || len(empty.Children) != 0 {
		t.Fatalf("empty tree = %#v", empty)
	}
	tree := BuildPathTree([]PathTrajectory{{ID: "n", Events: []model.Event{pathEvent("message", ""), pathEvent("generation", "")}}})
	if tree.TerminalCount != 1 || tree.NarrativeOnlyCount != 1 || tree.NarrativeEventCount != 2 || tree.RootNarrativeEvents != 2 || len(tree.Children) != 0 {
		t.Fatalf("narrative tree = %#v", tree)
	}
}

func TestBuildPathTreeDeterministicAndBoundedSamples(t *testing.T) {
	trajectories := make([]PathTrajectory, MaxSampleTrajectoryIDs+3)
	for i := range trajectories {
		trajectories[i] = PathTrajectory{ID: string(rune('z' - i)), Events: []model.Event{pathEvent("tool", "same")}}
	}
	forward := BuildPathTree(trajectories)
	for left, right := 0, len(trajectories)-1; left < right; left, right = left+1, right-1 {
		trajectories[left], trajectories[right] = trajectories[right], trajectories[left]
	}
	reverse := BuildPathTree(trajectories)
	one, _ := json.Marshal(forward)
	two, _ := json.Marshal(reverse)
	if string(one) != string(two) {
		t.Fatalf("nondeterministic trees:\n%s\n%s", one, two)
	}
	root := forward.Children[0]
	if len(root.TrajectoryIDs) != MaxSampleTrajectoryIDs || !root.TrajectoryIDsTruncated {
		t.Fatalf("trajectory sample = %#v", root)
	}
}

func TestBuildPathTreeUsesFingerprintEquivalencePrecedence(t *testing.T) {
	tree := BuildPathTree([]PathTrajectory{
		{ID: "b", Events: []model.Event{pathEvent("tool", "shared")}},
		{ID: "a", Events: []model.Event{pathEvent("error", "shared")}},
	})
	if len(tree.Children) != 1 || tree.Children[0].Count != 2 {
		t.Fatalf("alignment-key equivalent nodes = %#v", tree.Children)
	}
	if tree.Children[0].Fingerprint.Kind != "error" {
		t.Fatalf("canonical fingerprint = %#v", tree.Children[0].Fingerprint)
	}
}
