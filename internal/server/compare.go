package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/TheSnakeFang/rlviz/internal/alignment"
	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

const (
	MaxComparisonEvents             = 20_000
	MaxComparisonAlignmentWork      = 25_000_000
	MaxComparisonAlignmentWorkspace = 64 << 20
	MaxComparisonRawBytes           = 64 << 20
)

type comparisonSide struct {
	Context         rolloutindex.TrajectoryContext `json:"context"`
	Run             *model.Run                     `json:"run"`
	Case            *model.Case                    `json:"case"`
	Group           *model.Group                   `json:"group"`
	Trajectory      *model.Trajectory              `json:"trajectory"`
	Events          []*model.Event                 `json:"events"`
	EventProvenance []indexedProvenance            `json:"event_provenance"`
	Signals         []*model.Signal                `json:"signals"`
	Artifacts       []*model.Artifact              `json:"artifacts"`
}

type valueDifference struct {
	Left    any  `json:"left,omitempty"`
	Right   any  `json:"right,omitempty"`
	Changed bool `json:"changed"`
}

type countDifference struct {
	Left  int `json:"left"`
	Right int `json:"right"`
	Delta int `json:"delta"`
}

type optionalIntegerDifference struct {
	Left    *int64 `json:"left,omitempty"`
	Right   *int64 `json:"right,omitempty"`
	Delta   *int64 `json:"delta,omitempty"`
	Changed bool   `json:"changed"`
}

// verifierResult retains the canonical grader output and points back to the
// source event that produced it. RLViz deliberately does not normalize the
// output into a pass/fail verdict because grader payloads are domain-defined.
type verifierResult struct {
	EventID      string `json:"event_id"`
	Sequence     int64  `json:"sequence"`
	AlignmentKey string `json:"alignment_key,omitempty"`
	Output       any    `json:"output,omitempty"`
}

type comparisonDifferences struct {
	EventCount        countDifference           `json:"event_count"`
	Status            valueDifference           `json:"status"`
	Termination       valueDifference           `json:"termination"`
	Reward            valueDifference           `json:"reward"`
	Success           valueDifference           `json:"success"`
	TokenCount        optionalIntegerDifference `json:"token_count"`
	ContextEventCount countDifference           `json:"context_event_count"`
	CompactionCount   countDifference           `json:"compaction_count"`
	VerifierResults   valueDifference           `json:"verifier_results"`
}

func (api *indexedAPI) compare(response http.ResponseWriter, request *http.Request) {
	values := request.URL.Query()
	if err := validateComparisonQuery(values); err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	sourceID, _ := requiredSingle(values, "trajectory")
	leftID, _ := requiredSingle(values, "left")
	rightID, _ := requiredSingle(values, "right")
	if leftID == rightID {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", errors.New("left and right must identify different trajectories"))
		return
	}

	source, err := api.reader.Source(request.Context(), sourceID)
	if err != nil {
		api.writeReadError(response, "source_not_found", err)
		return
	}
	left, err := api.comparisonSide(request.Context(), sourceID, leftID)
	if err != nil {
		api.writeComparisonError(response, "left", err)
		return
	}
	right, err := api.comparisonSide(request.Context(), sourceID, rightID)
	if err != nil {
		api.writeComparisonError(response, "right", err)
		return
	}
	leftEvents := eventValues(left.Events)
	rightEvents := eventValues(right.Events)
	result, complexity, err := alignment.AlignBounded(leftEvents, rightEvents, MaxComparisonAlignmentWork, MaxComparisonAlignmentWorkspace)
	if err != nil {
		if errors.Is(err, alignment.ErrTooLarge) {
			writeJSONError(response, http.StatusRequestEntityTooLarge, "comparison_too_large", fmt.Errorf(
				"comparison divergent middle %dx%d requires %d alignment cells and %d workspace bytes; maximums are %d and %d",
				complexity.MiddleLeft, complexity.MiddleRight, complexity.WorkCells, complexity.WorkspaceBytes, MaxComparisonAlignmentWork, MaxComparisonAlignmentWorkspace,
			))
			return
		}
		writeJSONError(response, http.StatusInternalServerError, "alignment_failed", err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"source":      source,
		"left":        left,
		"right":       right,
		"alignment":   result,
		"differences": compareDifferences(left, right),
	})
}

func validateComparisonQuery(values url.Values) error {
	if err := validateQuery(values, map[string]bool{"trajectory": true, "left": true, "right": true}); err != nil {
		return err
	}
	for _, name := range []string{"trajectory", "left", "right"} {
		value, err := requiredSingle(values, name)
		if err != nil {
			return err
		}
		if len(value) > 256 {
			return fmt.Errorf("%s must be at most 256 characters", name)
		}
	}
	return nil
}

func (api *indexedAPI) comparisonSide(ctx context.Context, sourceID, trajectoryID string) (comparisonSide, error) {
	trajectoryContext, err := api.reader.TrajectoryContext(ctx, sourceID, trajectoryID)
	if err != nil {
		return comparisonSide{}, err
	}
	events, provenance, rawBytes, err := api.allComparisonEvents(ctx, sourceID, trajectoryID)
	if err != nil {
		return comparisonSide{}, err
	}
	signals, err := api.reader.SignalsPage(ctx, sourceID, trajectoryID, 0, MaxCompleteChildRecords)
	if err != nil {
		return comparisonSide{}, err
	}
	if signals.Total > int64(len(signals.Items)) {
		return comparisonSide{}, fmt.Errorf("%w: trajectory %q has %d signals; maximum is %d", errComparisonTooLarge, trajectoryID, signals.Total, MaxCompleteChildRecords)
	}
	artifacts, err := api.reader.ArtifactsPage(ctx, sourceID, trajectoryID, 0, MaxCompleteChildRecords)
	if err != nil {
		return comparisonSide{}, err
	}
	if artifacts.Total > int64(len(artifacts.Items)) {
		return comparisonSide{}, fmt.Errorf("%w: trajectory %q has %d artifacts; maximum is %d", errComparisonTooLarge, trajectoryID, artifacts.Total, MaxCompleteChildRecords)
	}
	rawBytes += signals.RawBytes + artifacts.RawBytes
	if rawBytes > MaxComparisonRawBytes {
		return comparisonSide{}, fmt.Errorf("%w: trajectory %q comparison input is %d raw bytes; maximum is %d", errComparisonTooLarge, trajectoryID, rawBytes, MaxComparisonRawBytes)
	}
	return comparisonSide{
		Context: trajectoryContext, Run: trajectoryContext.Run.Value, Case: trajectoryContext.Case.Value,
		Group: trajectoryContext.Group.Value, Trajectory: trajectoryContext.Trajectory.Value,
		Events: events, EventProvenance: provenance,
		Signals: canonicalSignals(signals.Items), Artifacts: canonicalArtifacts(artifacts.Items),
	}, nil
}

var errComparisonTooLarge = errors.New("comparison exceeds event limit")

func (api *indexedAPI) allComparisonEvents(ctx context.Context, sourceID, trajectoryID string) ([]*model.Event, []indexedProvenance, int64, error) {
	result := make([]*model.Event, 0)
	provenance := make([]indexedProvenance, 0)
	var rawBytes int64
	var after *int64
	for {
		page, err := api.reader.Events(ctx, rolloutindex.EventQuery{
			SourceID: sourceID, TrajectoryID: trajectoryID, AfterSequence: after, Limit: MaxIndexedPageLimit,
		})
		if err != nil {
			return nil, nil, 0, err
		}
		if page.Total > MaxComparisonEvents || len(result)+len(page.Events) > MaxComparisonEvents {
			return nil, nil, 0, fmt.Errorf("%w: trajectory %q has %d events; maximum is %d", errComparisonTooLarge, trajectoryID, page.Total, MaxComparisonEvents)
		}
		rawBytes += page.RawBytes
		if rawBytes > MaxComparisonRawBytes {
			return nil, nil, 0, fmt.Errorf("%w: trajectory %q events are %d raw bytes; maximum is %d", errComparisonTooLarge, trajectoryID, rawBytes, MaxComparisonRawBytes)
		}
		result = append(result, canonicalEvents(page.Events)...)
		provenance = append(provenance, eventProvenance(page.Events)...)
		if page.NextSequence == nil {
			break
		}
		if len(page.Events) == 0 || (after != nil && *page.NextSequence <= *after) {
			return nil, nil, 0, errors.New("index returned a non-advancing event page")
		}
		next := *page.NextSequence
		after = &next
	}
	return result, provenance, rawBytes, nil
}

func (api *indexedAPI) writeComparisonError(response http.ResponseWriter, side string, err error) {
	if errors.Is(err, errComparisonTooLarge) {
		writeJSONError(response, http.StatusRequestEntityTooLarge, "comparison_too_large", err)
		return
	}
	if errors.Is(err, rolloutindex.ErrNotFound) {
		writeJSONError(response, http.StatusNotFound, side+"_trajectory_not_found", err)
		return
	}
	writeJSONError(response, http.StatusInternalServerError, "index_query_failed", err)
}

func eventValues(events []*model.Event) []model.Event {
	result := make([]model.Event, 0, len(events))
	for _, event := range events {
		if event != nil {
			result = append(result, *event)
		}
	}
	return result
}

func compareDifferences(left, right comparisonSide) comparisonDifferences {
	leftStatus, leftTermination := "", ""
	rightStatus, rightTermination := "", ""
	if left.Trajectory != nil {
		leftStatus, leftTermination = left.Trajectory.Status, left.Trajectory.Termination
	}
	if right.Trajectory != nil {
		rightStatus, rightTermination = right.Trajectory.Status, right.Trajectory.Termination
	}
	leftReward, leftRewardOK := rewardValue(left.Signals)
	rightReward, rightRewardOK := rewardValue(right.Signals)
	leftSuccess, leftSuccessOK := booleanSignalValue(left.Signals, "pass", "success")
	rightSuccess, rightSuccessOK := booleanSignalValue(right.Signals, "pass", "success")
	leftTokens, leftTokensOK := integerSignalValue(left.Signals, "token_count", "total_tokens", "tokens")
	rightTokens, rightTokensOK := integerSignalValue(right.Signals, "token_count", "total_tokens", "tokens")
	leftContextEvents, leftCompactions := contextEventCounts(left.Events)
	rightContextEvents, rightCompactions := contextEventCounts(right.Events)
	leftVerifiers := verifierResults(left.Events)
	rightVerifiers := verifierResults(right.Events)
	return comparisonDifferences{
		EventCount:        countDifference{Left: len(left.Events), Right: len(right.Events), Delta: len(right.Events) - len(left.Events)},
		Status:            valueDifference{Left: leftStatus, Right: rightStatus, Changed: leftStatus != rightStatus},
		Termination:       valueDifference{Left: leftTermination, Right: rightTermination, Changed: leftTermination != rightTermination},
		Reward:            valueDifference{Left: optionalValue(leftReward, leftRewardOK), Right: optionalValue(rightReward, rightRewardOK), Changed: !valuesEqual(leftReward, leftRewardOK, rightReward, rightRewardOK)},
		Success:           valueDifference{Left: optionalValue(leftSuccess, leftSuccessOK), Right: optionalValue(rightSuccess, rightSuccessOK), Changed: !valuesEqual(leftSuccess, leftSuccessOK, rightSuccess, rightSuccessOK)},
		TokenCount:        optionalIntegerDifferenceValue(leftTokens, leftTokensOK, rightTokens, rightTokensOK),
		ContextEventCount: countDifference{Left: leftContextEvents, Right: rightContextEvents, Delta: rightContextEvents - leftContextEvents},
		CompactionCount:   countDifference{Left: leftCompactions, Right: rightCompactions, Delta: rightCompactions - leftCompactions},
		VerifierResults:   valueDifference{Left: leftVerifiers, Right: rightVerifiers, Changed: !verifierResultsEqual(leftVerifiers, rightVerifiers)},
	}
}

func rewardValue(signals []*model.Signal) (any, bool) {
	for _, signal := range signals {
		if signal != nil && strings.EqualFold(strings.TrimSpace(signal.Name), "reward") {
			return signal.Value, true
		}
	}
	return nil, false
}

func booleanSignalValue(signals []*model.Signal, names ...string) (bool, bool) {
	for _, name := range names {
		for _, signal := range signals {
			if signal == nil || !strings.EqualFold(strings.TrimSpace(signal.Name), name) {
				continue
			}
			value, ok := signal.Value.(bool)
			if ok {
				return value, true
			}
		}
	}
	return false, false
}

func integerSignalValue(signals []*model.Signal, names ...string) (int64, bool) {
	for _, name := range names {
		for _, signal := range signals {
			if signal == nil || !strings.EqualFold(strings.TrimSpace(signal.Name), name) {
				continue
			}
			number, ok := signal.Value.(json.Number)
			if !ok {
				continue
			}
			value, err := strconv.ParseInt(number.String(), 10, 64)
			if err == nil && value >= 0 {
				return value, true
			}
		}
	}
	return 0, false
}

func optionalIntegerDifferenceValue(left int64, leftOK bool, right int64, rightOK bool) optionalIntegerDifference {
	result := optionalIntegerDifference{Changed: !valuesEqual(left, leftOK, right, rightOK)}
	if leftOK {
		result.Left = &left
	}
	if rightOK {
		result.Right = &right
	}
	if leftOK && rightOK {
		delta := right - left
		result.Delta = &delta
	}
	return result
}

func contextEventCounts(events []*model.Event) (contextEvents, compactions int) {
	for _, event := range events {
		if event == nil {
			continue
		}
		if event.Context != nil {
			contextEvents++
			if event.Context.Operation == "compaction" {
				compactions++
			}
			continue
		}
		if strings.HasPrefix(event.AlignmentKey, "context:") {
			contextEvents++
		}
		if event.AlignmentKey == "context:compaction" {
			compactions++
		}
	}
	return contextEvents, compactions
}

func verifierResults(events []*model.Event) []verifierResult {
	results := make([]verifierResult, 0)
	for _, event := range events {
		if event == nil || event.Kind != "grader" {
			continue
		}
		results = append(results, verifierResult{
			EventID: event.ID, Sequence: event.Sequence, AlignmentKey: event.AlignmentKey, Output: event.Output,
		})
	}
	return results
}

func verifierResultsEqual(left, right []verifierResult) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].AlignmentKey != right[index].AlignmentKey || !reflect.DeepEqual(left[index].Output, right[index].Output) {
			return false
		}
	}
	return true
}

func optionalValue(value any, ok bool) any {
	if !ok {
		return nil
	}
	return value
}

func valuesEqual(left any, leftOK bool, right any, rightOK bool) bool {
	if leftOK != rightOK {
		return false
	}
	if !leftOK {
		return true
	}
	return reflect.DeepEqual(left, right)
}
