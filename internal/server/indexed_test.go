package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	rolloutindex "github.com/unlatch-ai/rlviz/internal/index"
	"github.com/unlatch-ai/rlviz/internal/model"
)

func testIndexedHandler(t *testing.T) http.Handler {
	t.Helper()
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "rlviz.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	path := filepath.Join("..", "..", "fixtures", "canonical", "group.ndjson")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Replace(t.Context(), rolloutindex.Source{
		ID: "source-group", Path: path, Fingerprint: "fixture", Size: info.Size(), ModTime: time.Unix(1, 0),
	}, file)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetPresentation(t.Context(), "source-group", json.RawMessage(`{"api_version":"rlviz.dev/v1alpha1","fields":{"reward":{"label":"Return"}}}`)); err != nil {
		t.Fatal(err)
	}
	return NewIndexedHandler(store, "secret")
}

func indexedRequest(t *testing.T, handler http.Handler, method, target string, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, nil)
	if authenticated {
		request.Header.Set("Authorization", "Bearer secret")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	return response
}

func decodeIndexedResponse(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
	return payload
}

func TestIndexedReadsRequireAuthentication(t *testing.T) {
	handler := testIndexedHandler(t)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/trajectory?trajectory=source-group", false)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if code := decodeIndexedResponse(t, response)["code"]; code != "unauthorized" {
		t.Fatalf("code = %v", code)
	}
}

func TestIndexedTrajectoryReturnsSourcePresentation(t *testing.T) {
	response := indexedRequest(t, testIndexedHandler(t), http.MethodGet, "/api/v1/indexed/trajectory?trajectory=source-group", true)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	presentation, ok := decodeIndexedResponse(t, response)["presentation"].(map[string]any)
	if !ok || presentation["api_version"] != "rlviz.dev/v1alpha1" {
		t.Fatalf("presentation=%#v", presentation)
	}
}

func TestIndexedAnalysisIsAuthenticatedCachedAndProvenanced(t *testing.T) {
	handler := testIndexedHandler(t)
	target := "/api/v1/indexed/analysis?trajectory=source-group&trajectory_id=traj-success&analyzer=loop-retry"
	unauthorized := indexedRequest(t, handler, http.MethodGet, target, false)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
	first := indexedRequest(t, handler, http.MethodGet, target, true)
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", first.Code, first.Body.String())
	}
	payload := decodeIndexedResponse(t, first)
	if payload["cached"] != false || payload["analyzed_at"] == "" {
		t.Fatalf("analysis metadata = %#v", payload)
	}
	analysis := payload["analysis"].(map[string]any)
	provenance := analysis["provenance"].(map[string]any)
	if provenance["name"] != "builtin.loop-retry" || provenance["digest"] == "" || provenance["input_digest"] == "" {
		t.Fatalf("provenance = %#v", provenance)
	}

	second := indexedRequest(t, handler, http.MethodGet, target, true)
	secondPayload := decodeIndexedResponse(t, second)
	if secondPayload["cached"] != true || secondPayload["analyzed_at"] != payload["analyzed_at"] {
		t.Fatalf("cached response = %#v", secondPayload)
	}
}

func TestIndexedAnalysisRejectsUnknownAnalyzerAndQuery(t *testing.T) {
	handler := testIndexedHandler(t)
	for _, target := range []string{
		"/api/v1/indexed/analysis?trajectory=source-group&analyzer=semantic-magic",
		"/api/v1/indexed/analysis?trajectory=source-group&analyzer=loop-retry&extra=x",
	} {
		response := indexedRequest(t, handler, http.MethodGet, target, true)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", target, response.Code, response.Body.String())
		}
	}
}

type oversizedAnalysisReader struct{ IndexedReader }

func (oversizedAnalysisReader) Source(context.Context, string) (rolloutindex.SourceInfo, error) {
	return rolloutindex.SourceInfo{Source: rolloutindex.Source{ID: "source"}}, nil
}

func (oversizedAnalysisReader) TrajectoryContext(context.Context, string, string) (rolloutindex.TrajectoryContext, error) {
	return rolloutindex.TrajectoryContext{}, nil
}

func (oversizedAnalysisReader) LoopRetryAnalysis(context.Context, string, string) (rolloutindex.AnalysisResult, error) {
	return rolloutindex.AnalysisResult{}, fmt.Errorf("%w: adversarial max+1 input", rolloutindex.ErrResultTooLarge)
}

func TestIndexedAnalysisMapsOversizedInputTo413(t *testing.T) {
	handler := NewIndexedHandler(oversizedAnalysisReader{}, "secret")
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/analysis?trajectory=source&trajectory_id=large", true)
	if response.Code != http.StatusRequestEntityTooLarge || decodeIndexedResponse(t, response)["code"] != "result_too_large" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type oversizedTrajectoryReader struct{ IndexedReader }

func (oversizedTrajectoryReader) Source(context.Context, string) (rolloutindex.SourceInfo, error) {
	return rolloutindex.SourceInfo{Source: rolloutindex.Source{ID: "source"}}, nil
}

func (oversizedTrajectoryReader) TrajectoryContext(context.Context, string, string) (rolloutindex.TrajectoryContext, error) {
	return rolloutindex.TrajectoryContext{}, nil
}

func (oversizedTrajectoryReader) Events(context.Context, rolloutindex.EventQuery) (rolloutindex.EventPage, error) {
	return rolloutindex.EventPage{RawBytes: MaxTrajectoryRawBytes}, nil
}

func (oversizedTrajectoryReader) SignalsPage(context.Context, string, string, int64, int) (rolloutindex.RecordPage[*model.Signal], error) {
	return rolloutindex.RecordPage[*model.Signal]{RawBytes: 1}, nil
}

func (oversizedTrajectoryReader) ArtifactsPage(context.Context, string, string, int64, int) (rolloutindex.RecordPage[*model.Artifact], error) {
	return rolloutindex.RecordPage[*model.Artifact]{}, nil
}

func TestIndexedTrajectoryRejectsCumulativeRawByteMaxPlusOne(t *testing.T) {
	handler := NewIndexedHandler(oversizedTrajectoryReader{}, "secret")
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/trajectory?trajectory=source&trajectory_id=large", true)
	if response.Code != http.StatusRequestEntityTooLarge || decodeIndexedResponse(t, response)["code"] != "trajectory_too_large" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestIndexedTrajectoryInfersFirstTrajectoryAndReturnsFirstPage(t *testing.T) {
	handler := testIndexedHandler(t)
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/trajectory?trajectory=source-group&limit=1", true)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	payload := decodeIndexedResponse(t, response)
	context := payload["context"].(map[string]any)
	trajectory := context["trajectory"].(map[string]any)["value"].(map[string]any)
	if trajectory["id"] != "traj-success" {
		t.Fatalf("trajectory = %v", trajectory["id"])
	}
	page := payload["page"].(map[string]any)
	if page["count"] != float64(1) || page["total"] != float64(2) || page["limit"] != float64(1) || page["has_more"] != true {
		t.Fatalf("page = %#v", page)
	}
	if events := payload["events"].([]any); len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if payload["run"].(map[string]any)["id"] != "run-group" || payload["trajectory"].(map[string]any)["id"] != "traj-success" {
		t.Fatalf("canonical context = %#v", payload)
	}
	if payload["events"].([]any)[0].(map[string]any)["raw"] == nil || len(payload["signals"].([]any)) != 2 {
		t.Fatalf("canonical document payload = %#v", payload)
	}
}

func TestIndexedEventsPaginationSearchAndKindFilter(t *testing.T) {
	handler := testIndexedHandler(t)
	first := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/events?trajectory=source-group&trajectory_id=traj-success&limit=1", true)
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", first.Code, first.Body.String())
	}
	firstPayload := decodeIndexedResponse(t, first)
	firstPage := firstPayload["page"].(map[string]any)
	if source := firstPayload["source"].(map[string]any); source["id"] != "source-group" || source["index_state"] != "complete" {
		t.Fatalf("source state = %#v", source)
	}
	if firstPage["total"] != float64(2) || firstPage["next_sequence"] != float64(0) {
		t.Fatalf("first page = %#v", firstPage)
	}

	second := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/events?trajectory=source-group&trajectory_id=traj-success&limit=1&after_sequence=0", true)
	secondPayload := decodeIndexedResponse(t, second)
	events := secondPayload["events"].([]any)
	event := events[0].(map[string]any)
	if event["id"] != "evt-success-answer" {
		t.Fatalf("second event = %#v", event)
	}
	if secondPayload["page"].(map[string]any)["has_more"] != false {
		t.Fatalf("second page = %#v", secondPayload["page"])
	}

	filtered := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/events?trajectory=source-group&trajectory_id=traj-success&kind=tool&q=Paris", true)
	filteredPayload := decodeIndexedResponse(t, filtered)
	if filteredPayload["page"].(map[string]any)["total"] != float64(1) || len(filteredPayload["events"].([]any)) != 1 {
		t.Fatalf("filtered response = %#v", filteredPayload)
	}
}

func TestIndexedQueryValidationIsStrict(t *testing.T) {
	handler := testIndexedHandler(t)
	tests := []string{
		"/api/v1/indexed/events?trajectory=source-group&limit=201",
		"/api/v1/indexed/events?trajectory=source-group&after_sequence=-1",
		"/api/v1/indexed/events?trajectory=source-group&unknown=x",
		"/api/v1/indexed/events?trajectory=source-group&trajectory=other",
	}
	for _, target := range tests {
		response := indexedRequest(t, handler, http.MethodGet, target, true)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d body=%s", target, response.Code, response.Body.String())
		}
		if code := decodeIndexedResponse(t, response)["code"]; code != "invalid_query" {
			t.Errorf("%s code = %v", target, code)
		}
	}
}

func TestIndexedMissingSourceAndTrajectoryAreDistinct(t *testing.T) {
	handler := testIndexedHandler(t)
	tests := []struct {
		target string
		code   string
	}{
		{"/api/v1/indexed/trajectory?trajectory=missing", "source_not_found"},
		{"/api/v1/indexed/trajectory?trajectory=source-group&trajectory_id=missing", "trajectory_not_found"},
	}
	for _, test := range tests {
		response := indexedRequest(t, handler, http.MethodGet, test.target, true)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d body=%s", test.target, response.Code, response.Body.String())
		}
		if code := decodeIndexedResponse(t, response)["code"]; code != test.code {
			t.Fatalf("%s code = %v", test.target, code)
		}
	}
}

func TestIndexedGroupSignalsAndArtifacts(t *testing.T) {
	handler := testIndexedHandler(t)
	group := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/group?trajectory=source-group&group_id=group-search", true)
	groupPayload := decodeIndexedResponse(t, group)
	if group.Code != http.StatusOK || groupPayload["count"] != float64(2) || groupPayload["total"] != float64(2) {
		t.Fatalf("group status/payload = %d %#v", group.Code, groupPayload)
	}
	aggregates := groupPayload["aggregates"].(map[string]any)
	if aggregates["success"] != float64(1) || aggregates["failure"] != float64(1) || aggregates["unknown"] != float64(0) {
		t.Fatalf("group outcomes = %#v", aggregates)
	}
	reward := aggregates["reward"].(map[string]any)
	if reward["min"] != float64(0) || reward["max"] != float64(1) || reward["mean"] != float64(0.5) {
		t.Fatalf("reward distribution = %#v", reward)
	}
	trajectories := groupPayload["trajectories"].([]any)
	first := trajectories[0].(map[string]any)
	if first["success"] != true || first["reward"] != float64(1) || first["status"] != "completed" || first["termination"] != "answer" {
		t.Fatalf("normalized trajectory = %#v", first)
	}

	signals := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/signals?trajectory=source-group&trajectory_id=traj-success", true)
	signalPayload := decodeIndexedResponse(t, signals)
	if signals.Code != http.StatusOK || signalPayload["count"] != float64(2) || signalPayload["total"] != float64(2) {
		t.Fatalf("signals status/payload = %d %#v", signals.Code, signalPayload)
	}
	firstSignal := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/signals?trajectory=source-group&trajectory_id=traj-success&limit=1", true)
	firstSignalPayload := decodeIndexedResponse(t, firstSignal)
	firstSignalPage := firstSignalPayload["page"].(map[string]any)
	if firstSignalPage["next_offset"] != float64(1) || firstSignalPage["has_more"] != true {
		t.Fatalf("first signal page = %#v", firstSignalPage)
	}
	secondSignal := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/signals?trajectory=source-group&trajectory_id=traj-success&limit=1&offset=1", true)
	secondSignalPayload := decodeIndexedResponse(t, secondSignal)
	secondSignalPage := secondSignalPayload["page"].(map[string]any)
	if secondSignalPayload["count"] != float64(1) || secondSignalPage["offset"] != float64(1) || secondSignalPage["has_more"] != false {
		t.Fatalf("second signal page = %#v", secondSignalPayload)
	}

	artifacts := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/artifacts?trajectory=source-group&trajectory_id=traj-success", true)
	artifactPayload := decodeIndexedResponse(t, artifacts)
	if artifacts.Code != http.StatusOK || artifactPayload["count"] != float64(0) || artifactPayload["total"] != float64(0) {
		t.Fatalf("artifacts status/payload = %d %#v", artifacts.Code, artifactPayload)
	}
}

func TestIndexedGroupAggregatesIncludeUnknownOutcomes(t *testing.T) {
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	stream := []byte("{\"record_type\":\"run\",\"id\":\"r\"}\n" +
		"{\"record_type\":\"case\",\"id\":\"c\",\"run_id\":\"r\"}\n" +
		"{\"record_type\":\"group\",\"id\":\"g\",\"case_id\":\"c\"}\n" +
		"{\"record_type\":\"trajectory\",\"id\":\"p\",\"group_id\":\"g\"}\n" +
		"{\"record_type\":\"trajectory\",\"id\":\"f\",\"group_id\":\"g\"}\n" +
		"{\"record_type\":\"trajectory\",\"id\":\"u\",\"group_id\":\"g\"}\n" +
		"{\"record_type\":\"signal\",\"id\":\"p-pass\",\"trajectory_id\":\"p\",\"name\":\"pass\",\"value\":true}\n" +
		"{\"record_type\":\"signal\",\"id\":\"f-pass\",\"trajectory_id\":\"f\",\"name\":\"pass\",\"value\":false}\n" +
		"{\"record_type\":\"signal\",\"id\":\"p-reward\",\"trajectory_id\":\"p\",\"name\":\"reward\",\"value\":2}\n" +
		"{\"record_type\":\"signal\",\"id\":\"f-reward\",\"trajectory_id\":\"f\",\"name\":\"reward\",\"value\":-1}\n" +
		"{\"record_type\":\"complete\",\"records\":10,\"warnings\":0}\n")
	if _, err := store.Replace(t.Context(), rolloutindex.Source{ID: "mixed"}, bytes.NewReader(stream)); err != nil {
		t.Fatal(err)
	}
	handler := NewIndexedHandler(store, "secret")
	response := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/group?trajectory=mixed&group_id=g", true)
	payload := decodeIndexedResponse(t, response)
	aggregates := payload["aggregates"].(map[string]any)
	if response.Code != http.StatusOK || aggregates["count"] != float64(3) || aggregates["success"] != float64(1) ||
		aggregates["failure"] != float64(1) || aggregates["unknown"] != float64(1) {
		t.Fatalf("mixed aggregates = %#v", aggregates)
	}
	if reward := aggregates["reward"].(map[string]any); reward["min"] != float64(-1) || reward["max"] != float64(2) || reward["mean"] != float64(0.5) {
		t.Fatalf("mixed reward = %#v", reward)
	}
}

func TestIndexedMethodAndRouteErrorsAreStructured(t *testing.T) {
	handler := testIndexedHandler(t)
	method := indexedRequest(t, handler, http.MethodPost, "/api/v1/indexed/events", true)
	if method.Code != http.StatusMethodNotAllowed || method.Header().Get("Allow") != http.MethodGet || decodeIndexedResponse(t, method)["code"] != "method_not_allowed" {
		t.Fatalf("method response = %d %s", method.Code, method.Body.String())
	}
	missing := indexedRequest(t, handler, http.MethodGet, "/api/v1/indexed/missing", true)
	if missing.Code != http.StatusNotFound || decodeIndexedResponse(t, missing)["code"] != "endpoint_not_found" {
		t.Fatalf("missing response = %d %s", missing.Code, missing.Body.String())
	}
}
