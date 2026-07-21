// Package alignment deterministically compares behavioral event sequences.
package alignment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

var ErrTooLarge = errors.New("alignment exceeds configured bounds")

// Complexity describes the divergent middle after exact suffix-first trimming.
// WorkCells counts DP cells including the zero row/column; WorkspaceBytes
// covers one direction byte per middle pair plus two rolling score rows.
type Complexity struct {
	CommonSuffix   int
	MiddleLeft     int
	MiddleRight    int
	WorkCells      int64
	WorkspaceBytes int64
}

// Operation describes how a pairwise alignment step consumes its inputs.
type Operation string

const (
	Match   Operation = "match"
	Replace Operation = "replace"
	Delete  Operation = "delete"
	Insert  Operation = "insert"
)

// Fingerprint is a stable, presentation-safe identity for an event. Digest is
// derived only from behavioral payloads; reasoning text, IDs, timestamps, raw
// source bytes, and source locations are deliberately excluded.
type Fingerprint struct {
	Kind         string `json:"kind"`
	Class        string `json:"class"`
	AlignmentKey string `json:"alignment_key,omitempty"`
	StateHash    string `json:"state_hash,omitempty"`
	Digest       string `json:"digest,omitempty"`
	Behavioral   bool   `json:"behavioral"`
}

// Step is one edit in an alignment. A nil index means that side has a gap.
type Step struct {
	Operation  Operation    `json:"operation"`
	LeftIndex  *int         `json:"left_index,omitempty"`
	RightIndex *int         `json:"right_index,omitempty"`
	Left       *Fingerprint `json:"left,omitempty"`
	Right      *Fingerprint `json:"right,omitempty"`
	Meaningful bool         `json:"meaningful"`
}

// Result contains the edit path and useful behavioral landmarks. Landmark
// values are indexes into Steps. Nil means the landmark does not exist.
type Result struct {
	Steps                     []Step `json:"steps"`
	CommonBehavioralPrefix    int    `json:"common_behavioral_prefix"`
	FirstMeaningfulDivergence *int   `json:"first_meaningful_divergence,omitempty"`
	LaterRealignment          *int   `json:"later_realignment,omitempty"`
}

// FingerprintEvent produces a deterministic semantic fingerprint. Explicit
// adapter alignment keys have highest precedence, followed by environment
// state hashes, then normalized behavioral payloads.
func FingerprintEvent(event model.Event) Fingerprint {
	kind := normalizeKind(event.Kind)
	class, behavioral := classify(kind)
	fp := Fingerprint{
		Kind:         kind,
		Class:        class,
		AlignmentKey: event.AlignmentKey,
		StateHash:    event.StateHash,
		Behavioral:   behavioral,
	}
	if behavioral && event.AlignmentKey == "" && event.StateHash == "" {
		payload := struct {
			Class  string `json:"class"`
			Input  any    `json:"input,omitempty"`
			Output any    `json:"output,omitempty"`
			Data   any    `json:"data,omitempty"`
		}{class, event.Input, event.Output, event.Data}
		encoded, err := json.Marshal(payload)
		if err != nil {
			// Canonical model payloads should be JSON-compatible. Keep malformed
			// in-memory values deterministic without making alignment fallible.
			encoded = []byte(class + ":unencodable")
		}
		sum := sha256.Sum256(encoded)
		fp.Digest = hex.EncodeToString(sum[:])
	}
	return fp
}

// Align computes a deterministic minimum-edit pairwise sequence alignment.
func Align(left, right []model.Event) Result {
	lf := fingerprintAll(left)
	rf := fingerprintAll(right)
	result, _, _ := alignFingerprintsBounded(lf, rf, 0, 0)
	return result
}

// AlignBounded computes the same alignment as Align but rejects the divergent
// middle before allocation when either bound is exceeded. Non-positive bounds
// disable that specific check.
func AlignBounded(left, right []model.Event, maxCells, maxWorkspaceBytes int64) (Result, Complexity, error) {
	lf := fingerprintAll(left)
	rf := fingerprintAll(right)
	return alignFingerprintsBounded(lf, rf, maxCells, maxWorkspaceBytes)
}

func complexityOf(lf, rf []Fingerprint) Complexity {
	// Backtracking prefers a diagonal at the end, so consume the exact suffix
	// first. This preserves duplicate-anchor tie behavior such as A vs A,A.
	suffix := 0
	for suffix < len(lf) && suffix < len(rf) && equivalent(lf[len(lf)-1-suffix], rf[len(rf)-1-suffix]) {
		suffix++
	}
	leftEnd, rightEnd := len(lf)-suffix, len(rf)-suffix
	return Complexity{CommonSuffix: suffix, MiddleLeft: leftEnd, MiddleRight: rightEnd}
}

func alignFingerprintsBounded(lf, rf []Fingerprint, maxCells, maxWorkspaceBytes int64) (Result, Complexity, error) {
	complexity := complexityOf(lf, rf)
	prefix, suffix := 0, complexity.CommonSuffix
	leftMiddle := lf[prefix : len(lf)-suffix]
	rightMiddle := rf[prefix : len(rf)-suffix]
	steps := make([]Step, 0, max(len(lf), len(rf)))
	for index := 0; index < prefix; index++ {
		steps = append(steps, pairedStep(Match, index, index, lf[index], rf[index]))
	}
	middle, work, workspace, err := alignMiddle(leftMiddle, rightMiddle, prefix, prefix, maxCells, maxWorkspaceBytes)
	complexity.WorkCells, complexity.WorkspaceBytes = work, workspace
	if err != nil {
		return Result{}, complexity, fmt.Errorf("%w: middle=%dx%d cells=%d workspace_bytes=%d", err, complexity.MiddleLeft, complexity.MiddleRight, complexity.WorkCells, complexity.WorkspaceBytes)
	}
	steps = append(steps, middle...)
	for offset := 0; offset < suffix; offset++ {
		li, ri := len(lf)-suffix+offset, len(rf)-suffix+offset
		steps = append(steps, pairedStep(Match, li, ri, lf[li], rf[ri]))
	}
	return summarize(steps), complexity, nil
}

const (
	directionDiagonal byte = iota
	directionDelete
	directionInsert
)

func alignMiddle(lf, rf []Fingerprint, leftOffset, rightOffset int, maxCells, maxWorkspaceBytes int64) ([]Step, int64, int64, error) {
	if len(lf) == 0 {
		steps := make([]Step, 0, len(rf))
		for j := range rf {
			ri, rcopy := rightOffset+j, rf[j]
			steps = append(steps, Step{Operation: Insert, RightIndex: &ri, Right: &rcopy, Meaningful: rcopy.Behavioral})
		}
		return steps, int64(len(rf) + 1), 0, nil
	}
	if len(rf) == 0 {
		steps := make([]Step, 0, len(lf))
		for i := range lf {
			li, lcopy := leftOffset+i, lf[i]
			steps = append(steps, Step{Operation: Delete, LeftIndex: &li, Left: &lcopy, Meaningful: lcopy.Behavioral})
		}
		return steps, int64(len(lf) + 1), 0, nil
	}

	// Ukkonen-style widening keeps near-identical long traces linear. Once a
	// result has distance <= k, every optimal path is contained in this band;
	// strict comparisons below preserve diagonal/delete/insert tie precedence.
	totalWork, peakWorkspace := int64(0), int64(0)
	band := abs(len(lf) - len(rf))
	for {
		cells := bandCells(len(lf), len(rf), band)
		workspace := cells + 2*int64(len(rf)+1)*8 + int64(len(lf)+1)*16
		if workspace > peakWorkspace {
			peakWorkspace = workspace
		}
		if (maxCells > 0 && totalWork+cells > maxCells) || (maxWorkspaceBytes > 0 && workspace > maxWorkspaceBytes) {
			return nil, totalWork + cells, peakWorkspace, ErrTooLarge
		}
		totalWork += cells
		steps, distance := alignBand(lf, rf, leftOffset, rightOffset, band)
		if distance <= band {
			return steps, totalWork, peakWorkspace, nil
		}
		if band >= max(len(lf), len(rf)) {
			return steps, totalWork, peakWorkspace, nil
		}
		if band == 0 {
			band = 1
		} else {
			band = min(max(len(lf), len(rf)), band*2)
		}
	}
}

func alignBand(lf, rf []Fingerprint, leftOffset, rightOffset, band int) ([]Step, int) {
	n, m := len(lf), len(rf)
	starts := make([]int, n+1)
	ends := make([]int, n+1)
	offsets := make([]int, n+1)
	directionCount := 0
	for i := 0; i <= n; i++ {
		starts[i], ends[i] = maxInt(0, i-band), min(m, i+band)
		offsets[i] = directionCount
		first := maxInt(1, starts[i])
		if i > 0 && ends[i] >= first {
			directionCount += ends[i] - first + 1
		}
	}
	directions := make([]byte, directionCount)
	previous, current := make([]int, m+1), make([]int, m+1)
	for j := starts[0]; j <= ends[0]; j++ {
		previous[j] = j
	}
	prevStart, prevEnd := starts[0], ends[0]
	const infinity = int(^uint(0)>>1) / 4
	for i := 1; i <= n; i++ {
		start, end := starts[i], ends[i]
		for j := start; j <= end; j++ {
			if j == 0 {
				current[j] = i
				continue
			}
			best, direction := infinity, directionDiagonal
			if j-1 >= prevStart && j-1 <= prevEnd {
				best = previous[j-1]
				if !equivalent(lf[i-1], rf[j-1]) {
					best++
				}
			}
			if j >= prevStart && j <= prevEnd {
				if deletion := previous[j] + 1; deletion < best {
					best, direction = deletion, directionDelete
				}
			}
			if j-1 >= start {
				if insertion := current[j-1] + 1; insertion < best {
					best, direction = insertion, directionInsert
				}
			}
			current[j] = best
			first := maxInt(1, start)
			directions[offsets[i]+j-first] = direction
		}
		previous, current = current, previous
		prevStart, prevEnd = start, end
	}
	if m < prevStart || m > prevEnd || previous[m] > band {
		if m >= prevStart && m <= prevEnd {
			return nil, previous[m]
		}
		return nil, infinity
	}
	distance := previous[m]
	steps := make([]Step, 0, max(n, m))
	for i, j := n, m; i > 0 || j > 0; {
		if i > 0 && j > 0 {
			first := maxInt(1, starts[i])
			direction := directions[offsets[i]+j-first]
			if direction == directionDiagonal {
				operation := Replace
				if equivalent(lf[i-1], rf[j-1]) {
					operation = Match
				}
				steps = append(steps, pairedStep(operation, leftOffset+i-1, rightOffset+j-1, lf[i-1], rf[j-1]))
				i, j = i-1, j-1
				continue
			}
			if direction == directionInsert {
				ri, rcopy := rightOffset+j-1, rf[j-1]
				steps = append(steps, Step{Operation: Insert, RightIndex: &ri, Right: &rcopy, Meaningful: rcopy.Behavioral})
				j--
				continue
			}
		}
		if i > 0 {
			li, lcopy := leftOffset+i-1, lf[i-1]
			steps = append(steps, Step{Operation: Delete, LeftIndex: &li, Left: &lcopy, Meaningful: lcopy.Behavioral})
			i--
			continue
		}
		ri, rcopy := rightOffset+j-1, rf[j-1]
		steps = append(steps, Step{Operation: Insert, RightIndex: &ri, Right: &rcopy, Meaningful: rcopy.Behavioral})
		j--
	}
	reverse(steps)
	return steps, distance
}

func bandCells(n, m, band int) int64 {
	var cells int64
	for i := 0; i <= n; i++ {
		start, end := maxInt(0, i-band), min(m, i+band)
		if end >= start {
			cells += int64(end - start + 1)
		}
	}
	return cells
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func pairedStep(operation Operation, li, ri int, left, right Fingerprint) Step {
	lcopy, rcopy := left, right
	return Step{operation, &li, &ri, &lcopy, &rcopy, operation != Match && (lcopy.Behavioral || rcopy.Behavioral)}
}

func equivalent(left, right Fingerprint) bool {
	if left.AlignmentKey != "" || right.AlignmentKey != "" {
		return left.AlignmentKey != "" && left.AlignmentKey == right.AlignmentKey
	}
	if left.StateHash != "" || right.StateHash != "" {
		return left.StateHash != "" && left.StateHash == right.StateHash
	}
	if left.Behavioral || right.Behavioral {
		return left.Behavioral && right.Behavioral && left.Class == right.Class && left.Digest == right.Digest
	}
	// Deliberately align reasoning/message events by normalized kind, without
	// claiming their free-form text is a behavioral divergence.
	return left.Kind == right.Kind
}

func summarize(steps []Step) Result {
	result := Result{Steps: steps}
	for i := range steps {
		step := steps[i]
		if step.Operation == Match && step.Left != nil && step.Left.Behavioral {
			result.CommonBehavioralPrefix++
			continue
		}
		if !step.Meaningful {
			continue
		}
		index := i
		result.FirstMeaningfulDivergence = &index
		break
	}
	if result.FirstMeaningfulDivergence != nil {
		for i := *result.FirstMeaningfulDivergence + 1; i < len(steps); i++ {
			if steps[i].Operation == Match && steps[i].Left != nil && steps[i].Left.Behavioral {
				index := i
				result.LaterRealignment = &index
				break
			}
		}
	}
	return result
}

func fingerprintAll(events []model.Event) []Fingerprint {
	result := make([]Fingerprint, len(events))
	for i := range events {
		result[i] = FingerprintEvent(events[i])
	}
	return result
}

func normalizeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func classify(kind string) (string, bool) {
	classes := []struct {
		name  string
		terms []string
	}{
		{"error", []string{"error", "exception", "failure"}},
		{"termination", []string{"termination", "terminal", "done", "finish"}},
		{"reward", []string{"reward", "grader", "grade", "score"}},
		{"observation", []string{"observation", "observe", "result"}},
		{"action", []string{"tool", "action", "environment", "env_", "env.", "transition"}},
		{"state", []string{"state"}},
	}
	for _, class := range classes {
		for _, term := range class.terms {
			if strings.Contains(kind, term) {
				return class.name, true
			}
		}
	}
	return "narrative", false
}

func reverse(values []Step) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func min(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
