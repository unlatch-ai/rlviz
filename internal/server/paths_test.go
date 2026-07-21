package server

import (
	"context"
	"errors"
	"net/http"
	"testing"

	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

func TestIndexedPathsSharedPrefixAndAuthentication(t *testing.T) {
	handler := testIndexedHandler(t)
	target := "/api/v1/indexed/paths?trajectory=source-group&group_id=group-search"
	unauthorized := indexedRequest(t, handler, http.MethodGet, target, false)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
	response := indexedRequest(t, handler, http.MethodGet, target, true)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	payload := decodeIndexedResponse(t, response)
	tree := payload["tree"].(map[string]any)
	if payload["count"] != float64(2) || payload["total_events"] != float64(4) || tree["trajectory_count"] != float64(2) {
		t.Fatalf("path response = %#v", payload)
	}
	children := tree["children"].([]any)
	if len(children) != 1 || children[0].(map[string]any)["count"] != float64(2) {
		t.Fatalf("shared prefix = %#v", children)
	}
	// These independently sampled trajectories still carry event parent links;
	// the API reports that source-native metadata separately from the derived
	// prefix tree instead of using it to alter aggregation.
	if payload["source_native_branches"] != true || payload["source_native_branch_count"] != float64(2) {
		t.Fatalf("native metadata = %#v", payload)
	}
}

type pathAPIReader struct {
	IndexedReader
	summaries []rolloutindex.TrajectorySummary
	events    map[string][]*model.Event
}

func (reader pathAPIReader) Source(context.Context, string) (rolloutindex.SourceInfo, error) {
	return rolloutindex.SourceInfo{Source: rolloutindex.Source{ID: "source"}}, nil
}

func (reader pathAPIReader) GroupSummaries(context.Context, string, string) ([]rolloutindex.TrajectorySummary, error) {
	return reader.summaries, nil
}

func (reader pathAPIReader) GroupSummariesPage(context.Context, string, string, int) (rolloutindex.SummaryPage, error) {
	return rolloutindex.SummaryPage{Items: reader.summaries, Total: int64(len(reader.summaries))}, nil
}

func (reader pathAPIReader) Events(_ context.Context, query rolloutindex.EventQuery) (rolloutindex.EventPage, error) {
	values := reader.events[query.TrajectoryID]
	items := make([]rolloutindex.IndexedRecord[*model.Event], 0, len(values))
	for _, event := range values {
		items = append(items, rolloutindex.IndexedRecord[*model.Event]{Value: event})
	}
	return rolloutindex.EventPage{Events: items, Total: int64(len(items))}, nil
}

func TestIndexedPathsSeparatesIndependentAndNativeTopology(t *testing.T) {
	trajectory := func(id, parent string) rolloutindex.TrajectorySummary {
		return rolloutindex.TrajectorySummary{Trajectory: rolloutindex.IndexedRecord[*model.Trajectory]{Value: &model.Trajectory{ID: id, GroupID: "group", ParentID: parent}}, EventCount: 1}
	}
	event := func(id string) *model.Event {
		return &model.Event{ID: id, TrajectoryID: id, Kind: "tool", AlignmentKey: "same"}
	}
	independent := pathAPIReader{summaries: []rolloutindex.TrajectorySummary{trajectory("a", ""), trajectory("b", "")}, events: map[string][]*model.Event{"a": {event("a")}, "b": {event("b")}}}
	response := indexedRequest(t, NewIndexedHandler(independent, "secret"), http.MethodGet, "/api/v1/indexed/paths?trajectory=source&group_id=group", true)
	payload := decodeIndexedResponse(t, response)
	if payload["source_native_branches"] != false || payload["source_native_branch_count"] != float64(0) {
		t.Fatalf("independent topology = %#v", payload)
	}

	native := pathAPIReader{summaries: []rolloutindex.TrajectorySummary{trajectory("a", ""), trajectory("b", "a")}, events: independent.events}
	response = indexedRequest(t, NewIndexedHandler(native, "secret"), http.MethodGet, "/api/v1/indexed/paths?trajectory=source&group_id=group", true)
	payload = decodeIndexedResponse(t, response)
	if payload["source_native_branches"] != true || payload["source_native_branch_count"] != float64(1) {
		t.Fatalf("native topology = %#v", payload)
	}
	children := payload["tree"].(map[string]any)["children"].([]any)
	if len(children) != 1 || children[0].(map[string]any)["count"] != float64(2) {
		t.Fatalf("native metadata changed derived tree = %#v", children)
	}
}

func TestIndexedPathsRejectsGroupsOverEventLimit(t *testing.T) {
	reader := pathAPIReader{summaries: []rolloutindex.TrajectorySummary{{
		Trajectory: rolloutindex.IndexedRecord[*model.Trajectory]{Value: &model.Trajectory{ID: "large", GroupID: "group"}},
		EventCount: MaxGroupPathEvents + 1,
	}}}
	response := indexedRequest(t, NewIndexedHandler(reader, "secret"), http.MethodGet, "/api/v1/indexed/paths?trajectory=source&group_id=group", true)
	if response.Code != http.StatusRequestEntityTooLarge || decodeIndexedResponse(t, response)["code"] != "group_paths_too_large" {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

type bytePathReader struct{ pathAPIReader }

func (bytePathReader) Events(_ context.Context, query rolloutindex.EventQuery) (rolloutindex.EventPage, error) {
	return rolloutindex.EventPage{
		Events:   []rolloutindex.IndexedRecord[*model.Event]{{Value: &model.Event{ID: query.TrajectoryID + "-event", TrajectoryID: query.TrajectoryID, Sequence: 0, Kind: "tool"}}},
		Total:    1,
		RawBytes: MaxGroupPathRawBytes/2 + 1,
	}, nil
}

func TestGroupPathsRejectCumulativeRawByteMaxPlusOne(t *testing.T) {
	summary := func(id string) rolloutindex.TrajectorySummary {
		return rolloutindex.TrajectorySummary{Trajectory: rolloutindex.IndexedRecord[*model.Trajectory]{Value: &model.Trajectory{ID: id, GroupID: "group"}}, EventCount: 1}
	}
	api := indexedAPI{reader: bytePathReader{}}
	_, _, _, err := api.loadGroupPaths(t.Context(), "source", []rolloutindex.TrajectorySummary{summary("one"), summary("two")})
	if !errors.Is(err, errGroupPathsTooLarge) {
		t.Fatalf("error = %v, want errGroupPathsTooLarge", err)
	}
}

func TestIndexedPathsRequiresExactGroupQuery(t *testing.T) {
	reader := pathAPIReader{}
	for _, target := range []string{
		"/api/v1/indexed/paths?trajectory=source",
		"/api/v1/indexed/paths?trajectory=source&group_id=group&extra=x",
	} {
		response := indexedRequest(t, NewIndexedHandler(reader, "secret"), http.MethodGet, target, true)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("target %q status = %d body=%s", target, response.Code, response.Body.String())
		}
	}
}
