package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	rolloutindex "github.com/unlatch-ai/rlviz/internal/index"
	"github.com/unlatch-ai/rlviz/internal/model"
)

const (
	DefaultIndexedPageLimit = 100
	MaxIndexedPageLimit     = 200
	MaxCompleteChildRecords = 1000
	MaxTrajectoryRawBytes   = 64 << 20
)

// IndexedReader is the read-only query surface needed by the HTTP API. *index.Index
// implements it directly, while tests and alternate stores can provide small fakes.
type IndexedReader interface {
	Source(context.Context, string) (rolloutindex.SourceInfo, error)
	TrajectoryContext(context.Context, string, string) (rolloutindex.TrajectoryContext, error)
	Events(context.Context, rolloutindex.EventQuery) (rolloutindex.EventPage, error)
	SignalsPage(context.Context, string, string, int64, int) (rolloutindex.RecordPage[*model.Signal], error)
	ArtifactsPage(context.Context, string, string, int64, int) (rolloutindex.RecordPage[*model.Artifact], error)
	Artifact(context.Context, string, string, string) (rolloutindex.IndexedRecord[*model.Artifact], error)
	GroupSummariesPage(context.Context, string, string, int) (rolloutindex.SummaryPage, error)
	FirstTrajectory(context.Context, string) (rolloutindex.IndexedRecord[*model.Trajectory], error)
	LoopRetryAnalysis(context.Context, string, string) (rolloutindex.AnalysisResult, error)
}

type indexedPresentationReader interface {
	Presentation(context.Context, string) (json.RawMessage, error)
}

type pageMetadata struct {
	Count         int    `json:"count"`
	Total         int64  `json:"total"`
	Limit         int    `json:"limit"`
	AfterSequence *int64 `json:"after_sequence,omitempty"`
	NextSequence  *int64 `json:"next_sequence,omitempty"`
	HasMore       bool   `json:"has_more"`
	Offset        int64  `json:"offset,omitempty"`
	NextOffset    *int64 `json:"next_offset,omitempty"`
}

type indexedAPI struct {
	reader IndexedReader
	token  string
}

// NewIndexedHandler serves authenticated, read-only queries over the persistent
// rollout index. It is separate from NewRegistryHandler so foreground and legacy
// in-memory behavior remains unchanged.
func NewIndexedHandler(reader IndexedReader, token string) http.Handler {
	api := &indexedAPI{reader: reader, token: token}
	return http.HandlerFunc(api.serveHTTP)
}

func (api *indexedAPI) serveHTTP(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", http.MethodGet)
		writeJSONError(response, http.StatusMethodNotAllowed, "method_not_allowed", errors.New("indexed reads require GET"))
		return
	}
	if !authorized(request, api.token) {
		writeJSONError(response, http.StatusUnauthorized, "unauthorized", errors.New("valid daemon token required"))
		return
	}
	if api.reader == nil {
		writeJSONError(response, http.StatusServiceUnavailable, "index_unavailable", errors.New("rollout index is unavailable"))
		return
	}

	switch request.URL.Path {
	case "/api/v1/indexed/trajectory":
		api.trajectory(response, request)
	case "/api/v1/indexed/events":
		api.events(response, request)
	case "/api/v1/indexed/signals":
		api.signals(response, request)
	case "/api/v1/indexed/artifacts":
		api.artifacts(response, request)
	case "/api/v1/indexed/artifact/content":
		api.artifactContent(response, request)
	case "/api/v1/indexed/group":
		api.group(response, request)
	case "/api/v1/indexed/compare":
		api.compare(response, request)
	case "/api/v1/indexed/paths":
		api.paths(response, request)
	case "/api/v1/indexed/analysis":
		api.analysis(response, request)
	default:
		writeJSONError(response, http.StatusNotFound, "endpoint_not_found", errors.New("indexed endpoint not found"))
	}
}

func (api *indexedAPI) analysis(response http.ResponseWriter, request *http.Request) {
	params, ok := api.trajectoryParams(response, request, map[string]bool{"trajectory": true, "trajectory_id": true, "analyzer": true})
	if !ok {
		return
	}
	analyzer, err := optionalSingle(request.URL.Query(), "analyzer")
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	if analyzer != "" && analyzer != "loop-retry" && analyzer != "builtin.loop-retry" {
		writeJSONError(response, http.StatusBadRequest, "unknown_analyzer", fmt.Errorf("unsupported analyzer %q", analyzer))
		return
	}
	result, err := api.reader.LoopRetryAnalysis(request.Context(), params.sourceID, params.trajectoryID)
	if err != nil {
		api.writeReadError(response, "analysis_failed", err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (api *indexedAPI) trajectory(response http.ResponseWriter, request *http.Request) {
	params, ok := api.trajectoryParams(response, request, map[string]bool{"trajectory": true, "trajectory_id": true, "limit": true})
	if !ok {
		return
	}
	limit, err := parseLimit(request.URL.Query())
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	ctx := params.context
	events, err := api.reader.Events(request.Context(), rolloutindex.EventQuery{SourceID: params.sourceID, TrajectoryID: params.trajectoryID, Limit: limit})
	if err != nil {
		api.writeReadError(response, "index_query_failed", err)
		return
	}
	signals, err := api.reader.SignalsPage(request.Context(), params.sourceID, params.trajectoryID, 0, DefaultIndexedPageLimit)
	if err != nil {
		api.writeReadError(response, "index_query_failed", err)
		return
	}
	artifacts, err := api.reader.ArtifactsPage(request.Context(), params.sourceID, params.trajectoryID, 0, DefaultIndexedPageLimit)
	if err != nil {
		api.writeReadError(response, "index_query_failed", err)
		return
	}
	var presentationConfig json.RawMessage
	if reader, ok := api.reader.(indexedPresentationReader); ok {
		presentationConfig, err = reader.Presentation(request.Context(), params.sourceID)
		if err != nil {
			api.writeReadError(response, "index_query_failed", err)
			return
		}
	}
	rawBytes := events.RawBytes + signals.RawBytes + artifacts.RawBytes
	if rawBytes > MaxTrajectoryRawBytes {
		writeJSONError(response, http.StatusRequestEntityTooLarge, "trajectory_too_large", fmt.Errorf("trajectory page is %d raw bytes; maximum is %d", rawBytes, MaxTrajectoryRawBytes))
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"source":           params.source,
		"context":          ctx,
		"run":              ctx.Run.Value,
		"case":             ctx.Case.Value,
		"group":            ctx.Group.Value,
		"trajectory":       ctx.Trajectory.Value,
		"presentation":     presentationConfig,
		"events":           canonicalEvents(events.Events),
		"event_provenance": eventProvenance(events.Events),
		"signals":          canonicalSignals(signals.Items),
		"artifacts":        canonicalArtifacts(artifacts.Items),
		"signal_page":      boundedPage(0, len(signals.Items), signals.Total, DefaultIndexedPageLimit),
		"artifact_page":    boundedPage(0, len(artifacts.Items), artifacts.Total, DefaultIndexedPageLimit),
		"page":             pageMetadata{Count: len(events.Events), Total: events.Total, Limit: limit, NextSequence: events.NextSequence, HasMore: events.NextSequence != nil},
	})
}

func (api *indexedAPI) events(response http.ResponseWriter, request *http.Request) {
	allowed := map[string]bool{"trajectory": true, "trajectory_id": true, "after_sequence": true, "limit": true, "kind": true, "q": true}
	params, ok := api.trajectoryParams(response, request, allowed)
	if !ok {
		return
	}
	values := request.URL.Query()
	limit, err := parseLimit(values)
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	after, err := optionalNonnegativeInt64(values, "after_sequence")
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	kinds, err := parseKinds(values["kind"])
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	query, err := optionalSingle(values, "q")
	if err != nil || len(query) > 256 {
		if err == nil {
			err = errors.New("q must be at most 256 characters")
		}
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	base := rolloutindex.EventQuery{SourceID: params.sourceID, TrajectoryID: params.trajectoryID, Kinds: kinds, Query: query}
	requested := base
	requested.AfterSequence = after
	requested.Limit = limit
	page, err := api.reader.Events(request.Context(), requested)
	if err != nil {
		api.writeReadError(response, "index_query_failed", err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"source":           params.source,
		"events":           canonicalEvents(page.Events),
		"event_provenance": eventProvenance(page.Events),
		"page":             pageMetadata{Count: len(page.Events), Total: page.Total, Limit: limit, AfterSequence: after, NextSequence: page.NextSequence, HasMore: page.NextSequence != nil},
	})
}

func (api *indexedAPI) signals(response http.ResponseWriter, request *http.Request) {
	params, ok := api.trajectoryParams(response, request, map[string]bool{"trajectory": true, "trajectory_id": true, "limit": true, "offset": true})
	if !ok {
		return
	}
	limit, err := parseLimit(request.URL.Query())
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	offset, err := parseOffset(request.URL.Query())
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	page, err := api.reader.SignalsPage(request.Context(), params.sourceID, params.trajectoryID, offset, limit)
	if err != nil {
		api.writeReadError(response, "index_query_failed", err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"signals": canonicalSignals(page.Items), "count": len(page.Items), "total": page.Total, "page": boundedPage(offset, len(page.Items), page.Total, limit)})
}

func (api *indexedAPI) artifacts(response http.ResponseWriter, request *http.Request) {
	params, ok := api.trajectoryParams(response, request, map[string]bool{"trajectory": true, "trajectory_id": true, "limit": true, "offset": true})
	if !ok {
		return
	}
	limit, err := parseLimit(request.URL.Query())
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	offset, err := parseOffset(request.URL.Query())
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	page, err := api.reader.ArtifactsPage(request.Context(), params.sourceID, params.trajectoryID, offset, limit)
	if err != nil {
		api.writeReadError(response, "index_query_failed", err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"artifacts": canonicalArtifacts(page.Items), "count": len(page.Items), "total": page.Total, "page": boundedPage(offset, len(page.Items), page.Total, limit)})
}

func (api *indexedAPI) group(response http.ResponseWriter, request *http.Request) {
	values := request.URL.Query()
	if err := validateQuery(values, map[string]bool{"trajectory": true, "trajectory_id": true, "group_id": true}); err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	sourceID, err := requiredSingle(values, "trajectory")
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	source, err := api.reader.Source(request.Context(), sourceID)
	if err != nil {
		api.writeReadError(response, "source_not_found", err)
		return
	}
	groupID, err := optionalSingle(values, "group_id")
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return
	}
	if groupID == "" {
		trajectoryID, resolveErr := api.resolveTrajectoryID(request.Context(), sourceID, values)
		if resolveErr != nil {
			api.writeReadError(response, "trajectory_not_found", resolveErr)
			return
		}
		ctx, contextErr := api.reader.TrajectoryContext(request.Context(), sourceID, trajectoryID)
		if contextErr != nil {
			api.writeReadError(response, "trajectory_not_found", contextErr)
			return
		}
		groupID = ctx.Group.Value.ID
	}
	summaryPage, err := api.reader.GroupSummariesPage(request.Context(), sourceID, groupID, MaxCompleteChildRecords)
	if err != nil {
		api.writeReadError(response, "group_not_found", err)
		return
	}
	if summaryPage.Total > int64(len(summaryPage.Items)) {
		writeJSONError(response, http.StatusRequestEntityTooLarge, "group_too_large", fmt.Errorf("group has %d trajectories; maximum is %d", summaryPage.Total, MaxCompleteChildRecords))
		return
	}
	summaries := summaryPage.Items
	if len(summaries) == 0 {
		writeJSONError(response, http.StatusNotFound, "group_not_found", fmt.Errorf("group %q was not found", groupID))
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"source": source, "group_id": groupID, "trajectories": summaries,
		"aggregates": rolloutindex.AggregateGroup(summaries), "count": len(summaries), "total": len(summaries)})
}

type resolvedTrajectoryParams struct {
	sourceID     string
	trajectoryID string
	source       rolloutindex.SourceInfo
	context      rolloutindex.TrajectoryContext
}

func (api *indexedAPI) trajectoryParams(response http.ResponseWriter, request *http.Request, allowed map[string]bool) (resolvedTrajectoryParams, bool) {
	values := request.URL.Query()
	if err := validateQuery(values, allowed); err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return resolvedTrajectoryParams{}, false
	}
	sourceID, err := requiredSingle(values, "trajectory")
	if err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_query", err)
		return resolvedTrajectoryParams{}, false
	}
	source, err := api.reader.Source(request.Context(), sourceID)
	if err != nil {
		api.writeReadError(response, "source_not_found", err)
		return resolvedTrajectoryParams{}, false
	}
	trajectoryID, err := api.resolveTrajectoryID(request.Context(), sourceID, values)
	if err != nil {
		api.writeReadError(response, "trajectory_not_found", err)
		return resolvedTrajectoryParams{}, false
	}
	trajectoryContext, err := api.reader.TrajectoryContext(request.Context(), sourceID, trajectoryID)
	if err != nil {
		api.writeReadError(response, "trajectory_not_found", err)
		return resolvedTrajectoryParams{}, false
	}
	return resolvedTrajectoryParams{sourceID: sourceID, trajectoryID: trajectoryID, source: source, context: trajectoryContext}, true
}

func (api *indexedAPI) resolveTrajectoryID(ctx context.Context, sourceID string, values url.Values) (string, error) {
	id, err := optionalSingle(values, "trajectory_id")
	if err != nil || id != "" {
		return id, err
	}
	trajectory, err := api.reader.FirstTrajectory(ctx, sourceID)
	if err != nil {
		return "", err
	}
	if trajectory.Value == nil || trajectory.Value.ID == "" {
		return "", rolloutindex.ErrNotFound
	}
	return trajectory.Value.ID, nil
}

func boundedPage(offset int64, count int, total int64, limit int) pageMetadata {
	page := pageMetadata{Count: count, Total: total, Limit: limit, Offset: offset, HasMore: offset+int64(count) < total}
	if page.HasMore {
		next := offset + int64(count)
		page.NextOffset = &next
	}
	return page
}

func parseOffset(values url.Values) (int64, error) {
	raw, err := optionalSingle(values, "offset")
	if err != nil || raw == "" {
		return 0, err
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, errors.New("offset must be a non-negative integer")
	}
	return value, nil
}

type indexedProvenance struct {
	ID         string `json:"id"`
	Line       int64  `json:"line"`
	ByteOffset int64  `json:"byte_offset"`
	ByteLength int64  `json:"byte_length"`
}

func canonicalEvents(items []rolloutindex.IndexedRecord[*model.Event]) []*model.Event {
	result := make([]*model.Event, 0, len(items))
	for _, item := range items {
		if item.Value == nil {
			continue
		}
		value := *item.Value
		if len(value.Raw) == 0 {
			value.Raw = append([]byte(nil), item.Raw...)
		}
		result = append(result, &value)
	}
	return result
}

func eventProvenance(items []rolloutindex.IndexedRecord[*model.Event]) []indexedProvenance {
	result := make([]indexedProvenance, 0, len(items))
	for _, item := range items {
		if item.Value != nil {
			result = append(result, indexedProvenance{ID: item.Value.ID, Line: item.Line, ByteOffset: item.ByteOffset, ByteLength: item.ByteLength})
		}
	}
	return result
}

func canonicalSignals(items []rolloutindex.IndexedRecord[*model.Signal]) []*model.Signal {
	result := make([]*model.Signal, 0, len(items))
	for _, item := range items {
		if item.Value != nil {
			result = append(result, item.Value)
		}
	}
	return result
}

func canonicalArtifacts(items []rolloutindex.IndexedRecord[*model.Artifact]) []*model.Artifact {
	result := make([]*model.Artifact, 0, len(items))
	for _, item := range items {
		if item.Value != nil {
			result = append(result, item.Value)
		}
	}
	return result
}

func parseLimit(values url.Values) (int, error) {
	raw, err := optionalSingle(values, "limit")
	if err != nil {
		return 0, err
	}
	if raw == "" {
		return DefaultIndexedPageLimit, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > MaxIndexedPageLimit {
		return 0, fmt.Errorf("limit must be an integer between 1 and %d", MaxIndexedPageLimit)
	}
	return value, nil
}

func optionalNonnegativeInt64(values url.Values, name string) (*int64, error) {
	raw, err := optionalSingle(values, name)
	if err != nil || raw == "" {
		return nil, err
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return nil, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return &value, nil
}

func parseKinds(values []string) ([]string, error) {
	if len(values) > 20 {
		return nil, errors.New("kind may be repeated at most 20 times")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		if value == "" || strings.TrimSpace(value) != value || len(value) > 64 {
			return nil, errors.New("kind values must be non-empty and at most 64 characters")
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, nil
}

func validateQuery(values url.Values, allowed map[string]bool) error {
	for name := range values {
		if !allowed[name] {
			return fmt.Errorf("unknown query parameter %q", name)
		}
	}
	return nil
}

func requiredSingle(values url.Values, name string) (string, error) {
	value, err := optionalSingle(values, name)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func optionalSingle(values url.Values, name string) (string, error) {
	items := values[name]
	if len(items) > 1 {
		return "", fmt.Errorf("%s may only be provided once", name)
	}
	if len(items) == 0 {
		return "", nil
	}
	if strings.TrimSpace(items[0]) != items[0] {
		return "", fmt.Errorf("%s may not contain surrounding whitespace", name)
	}
	return items[0], nil
}

func (api *indexedAPI) writeReadError(response http.ResponseWriter, notFoundCode string, err error) {
	if errors.Is(err, rolloutindex.ErrNotFound) {
		writeJSONError(response, http.StatusNotFound, notFoundCode, err)
		return
	}
	if errors.Is(err, rolloutindex.ErrResultTooLarge) {
		writeJSONError(response, http.StatusRequestEntityTooLarge, "result_too_large", err)
		return
	}
	writeJSONError(response, http.StatusInternalServerError, "index_query_failed", err)
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
