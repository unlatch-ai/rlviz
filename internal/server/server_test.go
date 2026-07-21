package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

func TestListenLoopback(t *testing.T) {
	listener, err := ListenLoopback(0)
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("loopback listeners are unavailable in this test environment: %v", err)
		}
		t.Fatalf("ListenLoopback() error = %v", err)
	}
	defer listener.Close()
	host, _, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		t.Fatalf("listener address %q is not loopback", listener.Addr())
	}
}

func TestTrajectoryAPI(t *testing.T) {
	document := Document{
		Trajectory: &model.Trajectory{RecordType: model.RecordTrajectory, ID: "traj-1", GroupID: "group-1"},
		Events:     []*model.Event{{RecordType: model.RecordEvent, ID: "evt-1", TrajectoryID: "traj-1"}},
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/trajectory", nil)
	response := httptest.NewRecorder()
	NewHandler(document).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var got Document
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Trajectory.ID != "traj-1" || len(got.Events) != 1 {
		t.Fatalf("response = %#v", got)
	}
}

func TestForegroundDocumentReturnsPresentation(t *testing.T) {
	document := Document{
		Trajectory:   &model.Trajectory{ID: "trajectory"},
		Presentation: json.RawMessage(`{"api_version":"rlviz.dev/v1alpha1","group":{"columns":["reward"]}}`),
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/trajectory", nil)
	response := httptest.NewRecorder()
	NewHandler(document).ServeHTTP(response, request)
	var payload map[string]any
	decodeErr := json.NewDecoder(response.Body).Decode(&payload)
	if response.Code != http.StatusOK || decodeErr != nil {
		t.Fatalf("status=%d body=%s err=%v", response.Code, response.Body.String(), decodeErr)
	}
	config, ok := payload["presentation"].(map[string]any)
	if !ok || config["api_version"] != "rlviz.dev/v1alpha1" {
		t.Fatalf("presentation=%#v", payload["presentation"])
	}
}

func TestDaemonSourceRegistrationRequiresTokenAndUsesStableURL(t *testing.T) {
	registry := NewRegistry()
	loader := func(_ context.Context, path, adapter string) (string, Document, error) {
		return "/resolved/trace.ndjson", Document{Trajectory: &model.Trajectory{ID: "traj-registered"}}, nil
	}
	handler := NewRegistryHandler(registry, "secret", loader, nil)

	unauthorized := httptest.NewRequest(http.MethodPost, "/api/v1/sources", bytes.NewBufferString(`{"path":"trace.ndjson"}`))
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorizedResponse.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/sources", bytes.NewBufferString(`{"path":"trace.ndjson"}`))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var registration Registration
	if err := json.NewDecoder(response.Body).Decode(&registration); err != nil {
		t.Fatal(err)
	}
	if registration.SourceID != SourceID("/resolved/trace.ndjson\x00builtin:canonical") || registration.URL != "/?trajectory="+registration.SourceID {
		t.Fatalf("registration = %#v", registration)
	}

	trajectoryRequest := httptest.NewRequest(http.MethodGet, "/api/v1/trajectory?trajectory="+registration.SourceID, nil)
	trajectoryResponse := httptest.NewRecorder()
	handler.ServeHTTP(trajectoryResponse, trajectoryRequest)
	if trajectoryResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized trajectory status = %d", trajectoryResponse.Code)
	}
	trajectoryRequest = httptest.NewRequest(http.MethodGet, "/api/v1/trajectory?trajectory="+registration.SourceID, nil)
	trajectoryRequest.Header.Set("Authorization", "Bearer secret")
	trajectoryResponse = httptest.NewRecorder()
	handler.ServeHTTP(trajectoryResponse, trajectoryRequest)
	if trajectoryResponse.Code != http.StatusOK || !strings.Contains(trajectoryResponse.Body.String(), "traj-registered") {
		t.Fatalf("trajectory status/body = %d %s", trajectoryResponse.Code, trajectoryResponse.Body.String())
	}
}

func TestAdapterIdentityKeepsViewsSeparate(t *testing.T) {
	registry := NewRegistry()
	loader := func(_ context.Context, path, adapter string) (string, Document, error) {
		return "/resolved/trace", Document{Trajectory: &model.Trajectory{ID: adapter}}, nil
	}
	handler := NewRegistryHandler(registry, "secret", loader, nil)
	var ids []string
	for _, adapter := range []string{"adapter-a", "adapter-b"} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sources", bytes.NewBufferString(`{"path":"trace","adapter":"`+adapter+`"}`))
		request.Header.Set("Authorization", "Bearer secret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
		}
		var registration Registration
		if err := json.NewDecoder(response.Body).Decode(&registration); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, registration.SourceID)
	}
	if ids[0] == ids[1] || registry.Count() != 2 {
		t.Fatalf("ids = %v count = %d", ids, registry.Count())
	}
}

func TestRegistryEvictsLeastRecentlyUsedSources(t *testing.T) {
	registry := NewRegistry()
	for index := 0; index <= MaxRegisteredSources; index++ {
		registry.Put(fmt.Sprintf("source-%d", index), Document{})
	}
	if registry.Count() != MaxRegisteredSources {
		t.Fatalf("count = %d", registry.Count())
	}
	if _, ok := registry.Get(SourceID("source-0")); ok {
		t.Fatal("oldest source was not evicted")
	}
}

func TestLoadCanonicalNDJSONRejectsOversizedSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.ndjson")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(MaxCanonicalSourceBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCanonicalNDJSON(path); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("error = %v", err)
	}
}

func TestDaemonSourceRegistrationRejectsTrailingJSON(t *testing.T) {
	registry := NewRegistry()
	handler := NewRegistryHandler(registry, "secret", func(_ context.Context, path, adapter string) (string, Document, error) {
		return path, Document{}, nil
	}, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sources", bytes.NewBufferString(`{"path":"trace"} {"path":"other"}`))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestEmbeddedViewerAndSecurityHeaders(t *testing.T) {
	document := Document{Trajectory: &model.Trajectory{ID: "traj-1"}, Events: []*model.Event{}}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	NewHandler(document).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `id="root"`) || !strings.Contains(string(body), `/assets/`) {
		t.Fatalf("root did not serve compiled viewer: %s", body)
	}
	if got := response.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") || !strings.Contains(got, "object-src 'none'") {
		t.Fatalf("unexpected content security policy %q", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}

func TestUnknownAPIRouteDoesNotServeViewer(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	response := httptest.NewRecorder()
	NewHandler(Document{}).ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestLoadCanonicalNDJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trajectory.jsonl")
	data := []byte("{\"record_type\":\"run\",\"id\":\"run-1\"}\n" +
		"{\"record_type\":\"case\",\"id\":\"case-1\",\"run_id\":\"run-1\"}\n" +
		"{\"record_type\":\"group\",\"id\":\"group-1\",\"case_id\":\"case-1\"}\n" +
		"{\"record_type\":\"trajectory\",\"id\":\"traj-1\",\"group_id\":\"group-1\"}\n" +
		"{\"record_type\":\"event\",\"id\":\"evt-1\",\"trajectory_id\":\"traj-1\",\"sequence\":1,\"kind\":\"message\"}\n" +
		"{\"record_type\":\"complete\",\"records\":5,\"warnings\":0}\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := LoadCanonicalNDJSON(path)
	if err != nil {
		t.Fatalf("LoadCanonicalNDJSON() error = %v", err)
	}
	if document.Trajectory.ID != "traj-1" || len(document.Events) != 1 {
		t.Fatalf("document = %#v", document)
	}
	if len(document.Events[0].Raw) == 0 {
		t.Fatal("event raw source record was not preserved")
	}
}

func TestLoadCanonicalNDJSONRejectsMalformedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trajectory.jsonl")
	if err := os.WriteFile(path, []byte("{not json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCanonicalNDJSON(path); err == nil {
		t.Fatal("LoadCanonicalNDJSON() accepted malformed JSON")
	}
}

func TestLoadCanonicalFixture(t *testing.T) {
	document, err := LoadCanonicalNDJSON(filepath.Join("..", "..", "fixtures", "canonical", "linear.ndjson"))
	if err != nil {
		t.Fatalf("LoadCanonicalNDJSON() error = %v", err)
	}
	if document.Trajectory.ID != "traj-linear" {
		t.Fatalf("trajectory id = %q, want traj-linear", document.Trajectory.ID)
	}
	if document.Run == nil || document.Run.ID != "run-linear" || document.Case == nil || document.Case.ID != "case-file" || document.Group == nil || document.Group.ID != "group-file" {
		t.Fatalf("related context was not resolved: %#v", document)
	}
	if len(document.Signals) != 2 || len(document.Artifacts) != 1 {
		t.Fatalf("signals/artifacts = %d/%d, want 2/1", len(document.Signals), len(document.Artifacts))
	}
	if len(document.Events) != 3 {
		t.Fatalf("event count = %d, want 3", len(document.Events))
	}
	for _, event := range document.Events {
		if len(event.Raw) == 0 {
			t.Fatalf("event %q has no raw payload or canonical provenance", event.ID)
		}
	}
	if source := document.Events[0].Source; source == nil || source.ByteOffset == nil || *source.ByteOffset != 0 || source.ByteLength == nil || *source.ByteLength != 72 {
		t.Fatalf("zero source offset was not preserved: %#v", source)
	}
}

func TestLoadGroupFixtureScopesSelectedTrajectory(t *testing.T) {
	document, err := LoadCanonicalNDJSON(filepath.Join("..", "..", "fixtures", "canonical", "group.ndjson"))
	if err != nil {
		t.Fatalf("LoadCanonicalNDJSON() error = %v", err)
	}
	if document.Trajectory.ID != "traj-success" || len(document.Events) != 2 || len(document.Signals) != 2 {
		t.Fatalf("selected trajectory scope = %q with %d events/%d signals", document.Trajectory.ID, len(document.Events), len(document.Signals))
	}
	for _, event := range document.Events {
		if event.TrajectoryID != document.Trajectory.ID {
			t.Fatalf("event %q belongs to %q", event.ID, event.TrajectoryID)
		}
	}
	for _, signal := range document.Signals {
		if signal.TrajectoryID != document.Trajectory.ID {
			t.Fatalf("signal %q belongs to %q", signal.ID, signal.TrajectoryID)
		}
	}
}
