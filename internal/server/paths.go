package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/TheSnakeFang/rlviz/internal/alignment"
	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

const (
	MaxGroupPathEvents   = 50_000
	MaxGroupPathRawBytes = 64 << 20
)

var errGroupPathsTooLarge = errors.New("group paths exceed event limit")

func (api *indexedAPI) paths(response http.ResponseWriter, request *http.Request) {
	values := request.URL.Query()
	if err := validateQuery(values, map[string]bool{"trajectory": true, "group_id": true}); err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	sourceID, err := requiredSingle(values, "trajectory")
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	groupID, err := requiredSingle(values, "group_id")
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	source, err := api.reader.Source(request.Context(), sourceID)
	if err != nil {
		api.writeReadError(response, "source_not_found", err)
		return
	}
	summaryPage, err := api.reader.GroupSummariesPage(request.Context(), sourceID, groupID, MaxCompleteChildRecords)
	if err != nil {
		api.writeReadError(response, "group_not_found", err)
		return
	}
	if summaryPage.Total > int64(len(summaryPage.Items)) {
		writeJSONError(response, http.StatusRequestEntityTooLarge, "group_paths_too_large", fmt.Errorf("%w: group has %d trajectories; maximum is %d", errGroupPathsTooLarge, summaryPage.Total, MaxCompleteChildRecords))
		return
	}
	summaries := summaryPage.Items
	if len(summaries) == 0 {
		writeJSONError(response, http.StatusNotFound, "group_not_found", fmt.Errorf("group %q was not found", groupID))
		return
	}
	paths, nativeCount, totalEvents, err := api.loadGroupPaths(request.Context(), sourceID, summaries)
	if errors.Is(err, errGroupPathsTooLarge) {
		writeJSONError(response, http.StatusRequestEntityTooLarge, "group_paths_too_large", err)
		return
	}
	if err != nil {
		api.writeReadError(response, "index_query_failed", err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"source": source, "group_id": groupID, "tree": alignment.BuildPathTree(paths),
		"source_native_branches": nativeCount > 0, "source_native_branch_count": nativeCount,
		"count": len(paths), "total_events": totalEvents,
	})
}

func (api *indexedAPI) loadGroupPaths(ctx context.Context, sourceID string, summaries []rolloutindex.TrajectorySummary) ([]alignment.PathTrajectory, int, int, error) {
	total := int64(0)
	for _, summary := range summaries {
		if summary.EventCount < 0 || total > int64(MaxGroupPathEvents)-summary.EventCount {
			return nil, 0, 0, fmt.Errorf("%w: maximum is %d events", errGroupPathsTooLarge, MaxGroupPathEvents)
		}
		total += summary.EventCount
	}
	paths := make([]alignment.PathTrajectory, 0, len(summaries))
	nativeCount := 0
	loaded := 0
	var rawBytes int64
	for _, summary := range summaries {
		trajectory := summary.Trajectory.Value
		if trajectory == nil || trajectory.ID == "" {
			return nil, 0, 0, errors.New("group contains an invalid trajectory")
		}
		if trajectory.ParentID != "" || trajectory.BranchID != "" {
			nativeCount++
		}
		events, eventNativeCount, eventRawBytes, err := api.loadPathEvents(ctx, sourceID, trajectory.ID, MaxGroupPathEvents-loaded, MaxGroupPathRawBytes-rawBytes)
		if err != nil {
			return nil, 0, 0, err
		}
		loaded += len(events)
		rawBytes += eventRawBytes
		nativeCount += eventNativeCount
		paths = append(paths, alignment.PathTrajectory{ID: trajectory.ID, Events: events})
	}
	return paths, nativeCount, loaded, nil
}

func (api *indexedAPI) loadPathEvents(ctx context.Context, sourceID, trajectoryID string, remaining int, remainingBytes int64) ([]model.Event, int, int64, error) {
	if remaining < 0 {
		return nil, 0, 0, fmt.Errorf("%w: maximum is %d events", errGroupPathsTooLarge, MaxGroupPathEvents)
	}
	if remainingBytes < 0 {
		return nil, 0, 0, fmt.Errorf("%w: maximum is %d raw bytes", errGroupPathsTooLarge, MaxGroupPathRawBytes)
	}
	result := make([]model.Event, 0)
	nativeCount := 0
	var rawBytes int64
	var after *int64
	for {
		limit := 1000
		if remaining-len(result) < limit {
			limit = remaining - len(result)
		}
		if limit <= 0 {
			// Query one item only to distinguish an exactly-full trajectory from
			// another event that would exceed the group bound.
			limit = 1
		}
		page, err := api.reader.Events(ctx, rolloutindex.EventQuery{SourceID: sourceID, TrajectoryID: trajectoryID, AfterSequence: after, Limit: limit})
		if err != nil {
			return nil, 0, 0, err
		}
		if page.Total < 0 || page.Total > int64(remaining) {
			return nil, 0, 0, fmt.Errorf("%w: trajectory %q has %d events with %d remaining", errGroupPathsTooLarge, trajectoryID, page.Total, remaining)
		}
		rawBytes += page.RawBytes
		if rawBytes > remainingBytes {
			return nil, 0, 0, fmt.Errorf("%w: maximum is %d raw bytes", errGroupPathsTooLarge, MaxGroupPathRawBytes)
		}
		for _, item := range page.Events {
			if len(result) >= remaining {
				return nil, 0, 0, fmt.Errorf("%w: maximum is %d events", errGroupPathsTooLarge, MaxGroupPathEvents)
			}
			if item.Value == nil {
				continue
			}
			event := *item.Value
			if event.ParentID != "" || event.BranchID != "" {
				nativeCount++
			}
			result = append(result, event)
		}
		if page.NextSequence == nil {
			return result, nativeCount, rawBytes, nil
		}
		if after != nil && *page.NextSequence <= *after {
			return nil, 0, 0, errors.New("event pagination did not advance")
		}
		next := *page.NextSequence
		after = &next
	}
}
