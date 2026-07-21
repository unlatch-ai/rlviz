package alignment

import (
	"sort"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

const MaxSampleTrajectoryIDs = 32

// PathTrajectory is one independently observed behavioral sequence. Source
// parent/branch annotations intentionally do not participate in aggregation:
// callers must expose that source-native topology separately.
type PathTrajectory struct {
	ID     string
	Events []model.Event
}

// PathTree is a compact prefix tree over behavioral events. Narrative events
// do not create nodes, but their aggregate counts remain available both before
// the first behavioral event and after each behavioral prefix.
type PathTree struct {
	TrajectoryCount      int        `json:"trajectory_count"`
	TerminalCount        int        `json:"terminal_count"`
	NarrativeOnlyCount   int        `json:"narrative_only_count"`
	BehavioralEventCount int        `json:"behavioral_event_count"`
	NarrativeEventCount  int        `json:"narrative_event_count"`
	RootNarrativeEvents  int        `json:"root_narrative_event_count"`
	Children             []PathNode `json:"children"`
}

// PathNode represents an equivalent behavioral prefix shared by Count
// trajectories. TrajectoryIDs is a deterministic bounded sample.
type PathNode struct {
	Fingerprint            Fingerprint `json:"fingerprint"`
	Count                  int         `json:"count"`
	TerminalCount          int         `json:"terminal_count"`
	TrajectoryIDs          []string    `json:"trajectory_ids"`
	TrajectoryIDsTruncated bool        `json:"trajectory_ids_truncated,omitempty"`
	NarrativeEventCount    int         `json:"narrative_event_count"`
	Depth                  int         `json:"depth"`
	Children               []PathNode  `json:"children"`
}

type mutablePathNode struct {
	fingerprint Fingerprint
	count       int
	terminal    int
	narrative   int
	ids         map[string]struct{}
	children    map[string]*mutablePathNode
}

// BuildPathTree aggregates equivalent behavioral prefixes deterministically.
// Input ordering does not affect the returned tree or trajectory ID samples.
func BuildPathTree(trajectories []PathTrajectory) PathTree {
	root := &mutablePathNode{children: make(map[string]*mutablePathNode)}
	result := PathTree{TrajectoryCount: len(trajectories)}
	for _, trajectory := range trajectories {
		current := root
		behavioral := 0
		for _, event := range trajectory.Events {
			fingerprint := FingerprintEvent(event)
			if !fingerprint.Behavioral {
				result.NarrativeEventCount++
				current.narrative++
				continue
			}
			result.BehavioralEventCount++
			behavioral++
			key := fingerprintKey(fingerprint)
			node := current.children[key]
			if node == nil {
				node = &mutablePathNode{fingerprint: fingerprint, ids: make(map[string]struct{}), children: make(map[string]*mutablePathNode)}
				current.children[key] = node
			} else if fingerprintPresentationKey(fingerprint) < fingerprintPresentationKey(node.fingerprint) {
				// Equivalent alignment/state keys can occur on differently named
				// event kinds. Retain a canonical representative so input order
				// cannot change the serialized tree.
				node.fingerprint = fingerprint
			}
			node.count++
			node.ids[trajectory.ID] = struct{}{}
			current = node
		}
		if behavioral == 0 {
			result.NarrativeOnlyCount++
			result.TerminalCount++
		} else {
			current.terminal++
		}
	}
	result.RootNarrativeEvents = root.narrative
	result.Children = freezePathChildren(root.children, 0)
	return result
}

func fingerprintKey(fingerprint Fingerprint) string {
	if fingerprint.AlignmentKey != "" {
		return "alignment\x00" + fingerprint.AlignmentKey
	}
	if fingerprint.StateHash != "" {
		return "state\x00" + fingerprint.StateHash
	}
	return "behavior\x00" + fingerprint.Class + "\x00" + fingerprint.Digest
}

func fingerprintPresentationKey(fingerprint Fingerprint) string {
	return fingerprint.Kind + "\x00" + fingerprint.Class + "\x00" + fingerprint.AlignmentKey + "\x00" + fingerprint.StateHash + "\x00" + fingerprint.Digest
}

func freezePathChildren(children map[string]*mutablePathNode, depth int) []PathNode {
	keys := make([]string, 0, len(children))
	for key := range children {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]PathNode, 0, len(keys))
	for _, key := range keys {
		mutable := children[key]
		ids := make([]string, 0, len(mutable.ids))
		for id := range mutable.ids {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		truncated := len(ids) > MaxSampleTrajectoryIDs
		if truncated {
			ids = ids[:MaxSampleTrajectoryIDs]
		}
		result = append(result, PathNode{
			Fingerprint: mutable.fingerprint, Count: mutable.count, TerminalCount: mutable.terminal,
			TrajectoryIDs: ids, TrajectoryIDsTruncated: truncated, NarrativeEventCount: mutable.narrative,
			Depth: depth, Children: freezePathChildren(mutable.children, depth+1),
		})
	}
	return result
}
