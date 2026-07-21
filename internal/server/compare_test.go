package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

func comparisonHandler(t *testing.T, eventsPerLeft int) http.Handler {
	right := 1
	if eventsPerLeft == 3 {
		right = 3
	}
	return comparisonHandlerCounts(t, eventsPerLeft, right, eventsPerLeft == 3)
}

func comparisonHandlerCounts(t *testing.T, eventsPerLeft, eventsPerRight int, divergent bool) http.Handler {
	t.Helper()
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "comparison.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	records := []any{
		&model.Run{RecordType: model.RecordRun, ID: "run"},
		&model.Case{RecordType: model.RecordCase, ID: "case", RunID: "run"},
		&model.Group{RecordType: model.RecordGroup, ID: "group", CaseID: "case"},
		&model.Trajectory{RecordType: model.RecordTrajectory, ID: "left", GroupID: "group", Status: "failed", Termination: "timeout"},
		&model.Trajectory{RecordType: model.RecordTrajectory, ID: "right", GroupID: "group", Status: "completed", Termination: "answer"},
	}
	if divergent && eventsPerLeft == 3 && eventsPerRight == 3 {
		records = append(records,
			&model.Event{RecordType: model.RecordEvent, ID: "left-a", TrajectoryID: "left", Sequence: 0, Kind: "tool", AlignmentKey: "a"},
			&model.Event{RecordType: model.RecordEvent, ID: "left-b", TrajectoryID: "left", Sequence: 1, Kind: "tool", AlignmentKey: "b"},
			&model.Event{RecordType: model.RecordEvent, ID: "left-c", TrajectoryID: "left", Sequence: 2, Kind: "tool", AlignmentKey: "c"},
			&model.Event{RecordType: model.RecordEvent, ID: "right-a", TrajectoryID: "right", Sequence: 0, Kind: "tool", AlignmentKey: "a"},
			&model.Event{RecordType: model.RecordEvent, ID: "right-x", TrajectoryID: "right", Sequence: 1, Kind: "error", AlignmentKey: "x"},
			&model.Event{RecordType: model.RecordEvent, ID: "right-c", TrajectoryID: "right", Sequence: 2, Kind: "tool", AlignmentKey: "c"},
		)
	} else if divergent {
		for sequence := 0; sequence < eventsPerLeft; sequence++ {
			records = append(records, &model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("left-%d", sequence), TrajectoryID: "left", Sequence: int64(sequence), Kind: "tool", AlignmentKey: "left"})
		}
		for sequence := 0; sequence < eventsPerRight; sequence++ {
			records = append(records, &model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("right-%d", sequence), TrajectoryID: "right", Sequence: int64(sequence), Kind: "tool", AlignmentKey: "right"})
		}
	} else {
		for sequence := 0; sequence < eventsPerLeft; sequence++ {
			records = append(records, &model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("left-%d", sequence), TrajectoryID: "left", Sequence: int64(sequence), Kind: "message"})
		}
		for sequence := 0; sequence < eventsPerRight; sequence++ {
			records = append(records, &model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("right-%d", sequence), TrajectoryID: "right", Sequence: int64(sequence), Kind: "message"})
		}
	}
	records = append(records,
		&model.Signal{RecordType: model.RecordSignal, ID: "left-reward", TrajectoryID: "left", Name: "reward", Value: 0.0},
		&model.Signal{RecordType: model.RecordSignal, ID: "right-reward", TrajectoryID: "right", Name: "reward", Value: 1.0},
		&model.Signal{RecordType: model.RecordSignal, ID: "left-pass", TrajectoryID: "left", Name: "pass", Value: false},
		&model.Signal{RecordType: model.RecordSignal, ID: "right-pass", TrajectoryID: "right", Name: "pass", Value: true},
		&model.Signal{RecordType: model.RecordSignal, ID: "left-tokens", TrajectoryID: "left", Name: "total_tokens", Value: 100},
		&model.Signal{RecordType: model.RecordSignal, ID: "right-tokens", TrajectoryID: "right", Name: "token_count", Value: 125},
		&model.Artifact{RecordType: model.RecordArtifact, ID: "right-log", TrajectoryID: "right", Name: "log", MediaType: "text/plain", Text: "done"},
	)
	records = append(records, &model.Complete{RecordType: model.RecordComplete, Records: int64(len(records))})
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	_, err = store.Replace(t.Context(), rolloutindex.Source{ID: "source", Path: "fixture.ndjson", Fingerprint: "fixture", Size: int64(input.Len()), ModTime: time.Unix(1, 0)}, bytes.NewReader(input.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return NewIndexedHandler(store, "secret")
}

func TestIndexedCompareLoadsEveryPaginatedEvent(t *testing.T) {
	handler := comparisonHandlerCounts(t, 401, 401, false)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/compare?trajectory=source&left=left&right=right", true)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	payload := decodeIndexedResponse(t, response)
	if got := len(payload["left"].(map[string]any)["events"].([]any)); got != 401 {
		t.Fatalf("left event count = %d", got)
	}
	if got := len(payload["right"].(map[string]any)["events"].([]any)); got != 401 {
		t.Fatalf("right event count = %d", got)
	}
	if got := len(payload["alignment"].(map[string]any)["steps"].([]any)); got != 401 {
		t.Fatalf("alignment step count = %d", got)
	}
}

type bytePagedComparisonReader struct{ IndexedReader }

func (bytePagedComparisonReader) Events(_ context.Context, query rolloutindex.EventQuery) (rolloutindex.EventPage, error) {
	sequence := int64(0)
	if query.AfterSequence != nil {
		sequence = *query.AfterSequence + 1
	}
	page := rolloutindex.EventPage{
		Events:   []rolloutindex.IndexedRecord[*model.Event]{{Value: &model.Event{ID: fmt.Sprintf("event-%d", sequence), TrajectoryID: query.TrajectoryID, Sequence: sequence, Kind: "tool"}}},
		Total:    3,
		RawBytes: MaxComparisonRawBytes / 2,
	}
	if sequence < 2 {
		page.NextSequence = &sequence
	} else {
		page.RawBytes = 1
	}
	return page, nil
}

func TestComparisonRejectsCumulativeRawByteMaxPlusOne(t *testing.T) {
	api := indexedAPI{reader: bytePagedComparisonReader{}}
	_, _, _, err := api.allComparisonEvents(t.Context(), "source", "trajectory")
	if !errors.Is(err, errComparisonTooLarge) {
		t.Fatalf("error = %v, want errComparisonTooLarge", err)
	}
}

func TestIndexedCompareReturnsDivergenceRealignmentAndDifferences(t *testing.T) {
	handler := comparisonHandler(t, 3)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/compare?trajectory=source&left=left&right=right", true)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	payload := decodeIndexedResponse(t, response)
	aligned := payload["alignment"].(map[string]any)
	if aligned["first_meaningful_divergence"] != float64(1) || aligned["later_realignment"] != float64(2) {
		t.Fatalf("alignment = %#v", aligned)
	}
	if len(aligned["steps"].([]any)) != 3 {
		t.Fatalf("steps = %#v", aligned["steps"])
	}
	left := payload["left"].(map[string]any)
	right := payload["right"].(map[string]any)
	if left["trajectory"].(map[string]any)["id"] != "left" || len(left["events"].([]any)) != 3 || len(right["artifacts"].([]any)) != 1 {
		t.Fatalf("sides = left:%#v right:%#v", left, right)
	}
	differences := payload["differences"].(map[string]any)
	if differences["status"].(map[string]any)["changed"] != true || differences["termination"].(map[string]any)["changed"] != true {
		t.Fatalf("differences = %#v", differences)
	}
	reward := differences["reward"].(map[string]any)
	if reward["left"] != float64(0) || reward["right"] != float64(1) || reward["changed"] != true {
		t.Fatalf("reward = %#v", reward)
	}
	success := differences["success"].(map[string]any)
	if success["left"] != false || success["right"] != true || success["changed"] != true {
		t.Fatalf("success = %#v", success)
	}
	tokens := differences["token_count"].(map[string]any)
	if tokens["left"] != float64(100) || tokens["right"] != float64(125) || tokens["delta"] != float64(25) || tokens["changed"] != true {
		t.Fatalf("token_count = %#v", tokens)
	}
}

func TestCompareDifferencesReportsExplicitContextAndVerifierData(t *testing.T) {
	left := comparisonSide{
		Events: []*model.Event{
			{ID: "left-compact", Sequence: 2, Kind: "state", AlignmentKey: "context:compaction", Context: &model.Context{Operation: "compaction", Provenance: "source_native"}},
			{ID: "left-restore", Sequence: 3, Kind: "state", AlignmentKey: "context:restore"},
			{ID: "left-grader", Sequence: 4, Kind: "grader", AlignmentKey: "grader:suite", Output: map[string]any{"verdict": "fail", "score": json.Number("0")}},
		},
		Signals: []*model.Signal{
			{Name: "pass", Value: false},
			{Name: "token_count", Value: json.Number("9007199254740993")},
		},
	}
	right := comparisonSide{
		Events: []*model.Event{
			{ID: "right-grader", Sequence: 2, Kind: "grader", AlignmentKey: "grader:suite", Output: map[string]any{"verdict": "pass", "score": json.Number("1")}},
		},
		Signals: []*model.Signal{
			{Name: "success", Value: true},
			{Name: "tokens", Value: json.Number("9007199254741000")},
		},
	}

	differences := compareDifferences(left, right)
	if differences.Success.Left != false || differences.Success.Right != true || !differences.Success.Changed {
		t.Fatalf("success = %#v", differences.Success)
	}
	if differences.TokenCount.Left == nil || *differences.TokenCount.Left != 9007199254740993 ||
		differences.TokenCount.Right == nil || *differences.TokenCount.Right != 9007199254741000 ||
		differences.TokenCount.Delta == nil || *differences.TokenCount.Delta != 7 {
		t.Fatalf("token_count = %#v", differences.TokenCount)
	}
	if differences.ContextEventCount != (countDifference{Left: 2, Right: 0, Delta: -2}) ||
		differences.CompactionCount != (countDifference{Left: 1, Right: 0, Delta: -1}) {
		t.Fatalf("context counts = %#v %#v", differences.ContextEventCount, differences.CompactionCount)
	}
	leftResults := differences.VerifierResults.Left.([]verifierResult)
	if len(leftResults) != 1 || leftResults[0].EventID != "left-grader" || leftResults[0].Sequence != 4 || leftResults[0].AlignmentKey != "grader:suite" {
		t.Fatalf("left verifier results = %#v", leftResults)
	}
	if !differences.VerifierResults.Changed {
		t.Fatal("verifier results should differ")
	}
}

func TestContextEventCountsPrefersStructuredContextAndFallsBackToLegacyKeys(t *testing.T) {
	events := []*model.Event{
		{ID: "structured-both", Context: &model.Context{Operation: "compaction", Provenance: "source_native"}, AlignmentKey: "context:compaction"},
		{ID: "structured-observation", Context: &model.Context{Provenance: "adapter_derived"}},
		{ID: "structured-truncation", Context: &model.Context{Operation: "truncation", Provenance: "source_native"}, AlignmentKey: "context:compaction"},
		{ID: "legacy-compaction", AlignmentKey: "context:compaction"},
		{ID: "legacy-restore", AlignmentKey: "context:restore"},
		{ID: "unrelated", Data: map[string]any{"operation": "compaction"}},
	}

	contextEvents, compactions := contextEventCounts(events)
	if contextEvents != 5 || compactions != 2 {
		t.Fatalf("context counts = %d, %d; want 5, 2", contextEvents, compactions)
	}
}

func TestCompareDifferencesDoesNotInferMissingOrMistypedMetrics(t *testing.T) {
	left := comparisonSide{
		Events: []*model.Event{
			{ID: "state", Kind: "state", Data: map[string]any{"operation": "compaction"}},
			{ID: "tool", Kind: "tool", AlignmentKey: "contextual:compaction"},
		},
		Signals: []*model.Signal{
			{Name: "pass", Value: json.Number("1")},
			{Name: "token_count", Value: json.Number("1.5")},
		},
	}
	differences := compareDifferences(left, comparisonSide{})
	encoded, err := json.Marshal(differences)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["success"]["left"]; ok {
		t.Fatalf("numeric pass was inferred: %s", encoded)
	}
	if _, ok := payload["token_count"]["left"]; ok {
		t.Fatalf("fractional token total was inferred: %s", encoded)
	}
	if payload["context_event_count"]["left"] != float64(0) || payload["compaction_count"]["left"] != float64(0) {
		t.Fatalf("implicit context operation was inferred: %s", encoded)
	}
}

func TestVerifierDifferenceIgnoresSourceIdentityButRetainsIt(t *testing.T) {
	output := map[string]any{"verdict": "pass", "score": json.Number("1")}
	left := comparisonSide{Events: []*model.Event{{ID: "left-grader", Sequence: 8, Kind: "grader", AlignmentKey: "grader:suite", Output: output}}}
	right := comparisonSide{Events: []*model.Event{{ID: "right-grader", Sequence: 21, Kind: "grader", AlignmentKey: "grader:suite", Output: output}}}

	difference := compareDifferences(left, right).VerifierResults
	if difference.Changed {
		t.Fatal("source event identity alone should not change a verifier result")
	}
	leftResult := difference.Left.([]verifierResult)[0]
	rightResult := difference.Right.([]verifierResult)[0]
	if leftResult.EventID != "left-grader" || rightResult.EventID != "right-grader" || leftResult.Sequence != 8 || rightResult.Sequence != 21 {
		t.Fatalf("verifier provenance was not retained: left=%#v right=%#v", leftResult, rightResult)
	}
}

func TestIndexedCompareRequiresAuthenticationAndStrictQuery(t *testing.T) {
	handler := comparisonHandler(t, 3)
	target := "/api/v1/indexed/compare?trajectory=source&left=left&right=right"
	unauthorized := indexedRequest(t, handler, http.MethodGet, target, false)
	if unauthorized.Code != http.StatusUnauthorized || decodeIndexedResponse(t, unauthorized)["code"] != "unauthorized" {
		t.Fatalf("unauthorized = %d %s", unauthorized.Code, unauthorized.Body.String())
	}
	for _, invalid := range []string{
		"/api/v1/indexed/compare?trajectory=source&left=left",
		"/api/v1/indexed/compare?trajectory=source&left=left&right=left",
		"/api/v1/indexed/compare?trajectory=source&left=left&right=right&extra=x",
		"/api/v1/indexed/compare?trajectory=source&left=left&left=other&right=right",
	} {
		response := indexedRequest(t, handler, http.MethodGet, invalid, true)
		if response.Code != http.StatusBadRequest || decodeIndexedResponse(t, response)["code"] != "invalid_query" {
			t.Errorf("%s = %d %s", invalid, response.Code, response.Body.String())
		}
	}
}

func TestIndexedCompareReportsMissingSide(t *testing.T) {
	handler := comparisonHandler(t, 3)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/compare?trajectory=source&left=missing&right=right", true)
	if response.Code != http.StatusNotFound || decodeIndexedResponse(t, response)["code"] != "left_trajectory_not_found" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestIndexedCompareEnforcesEventLimitFromRealIndex(t *testing.T) {
	handler := comparisonHandler(t, MaxComparisonEvents+1)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/compare?trajectory=source&left=left&right=right", true)
	if response.Code != http.StatusRequestEntityTooLarge || decodeIndexedResponse(t, response)["code"] != "comparison_too_large" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestIndexedCompareAllowsLongComparableTraces(t *testing.T) {
	handler := comparisonHandlerCounts(t, 5_000, 5_000, false)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/compare?trajectory=source&left=left&right=right", true)
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if got := len(decodeIndexedResponse(t, response)["alignment"].(map[string]any)["steps"].([]any)); got != 5_000 {
		t.Fatalf("alignment steps = %d", got)
	}
}

func TestIndexedCompareBoundsPathologicalDivergentWork(t *testing.T) {
	handler := comparisonHandlerCounts(t, 5_000, 5_000, true)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/compare?trajectory=source&left=left&right=right", true)
	if response.Code != http.StatusRequestEntityTooLarge || decodeIndexedResponse(t, response)["code"] != "comparison_too_large" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}
