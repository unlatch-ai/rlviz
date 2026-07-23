package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func workspaceRequest(t *testing.T, handler http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, target, &payload)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestWorkspaceHandlerCreatesReplacesAndWaits(t *testing.T) {
	handler := workspaceHandler(NewWorkspaceStore(), "secret")
	initial := Workspace{RailExpanded: true, CollectionView: "trials", GuideOpen: true, Direction: "rows", Active: "rail"}
	created := workspaceRequest(t, handler, http.MethodPost, "/api/v1/workspaces", initial)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var envelope WorkspaceEnvelope
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.ID == "" || envelope.Revision != 1 || envelope.Workspace.CollectionView != "trials" {
		t.Fatalf("created workspace=%#v", envelope)
	}
	updated := initial
	updated.CollectionView = "rollouts"
	updated.Lanes = []WorkspaceLane{{SourceID: "source", TrajectoryID: "rollout/one", Band: "focus", Depth: 1, Fidelity: 2, Axis: WorkspaceAxis{End: 10}}}
	replaced := workspaceRequest(t, handler, http.MethodPut, "/api/v1/workspaces/"+envelope.ID, updated)
	if replaced.Code != http.StatusOK {
		t.Fatalf("replace status=%d body=%s", replaced.Code, replaced.Body.String())
	}
	if err := json.Unmarshal(replaced.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Revision != 2 || envelope.Workspace.Lanes[0].ID != "source:rollout%2Fone" {
		t.Fatalf("replaced workspace=%#v", envelope)
	}
	fetched := workspaceRequest(t, handler, http.MethodGet, "/api/v1/workspaces/"+envelope.ID+"?after=1", nil)
	if fetched.Code != http.StatusOK || !bytes.Contains(fetched.Body.Bytes(), []byte(`"revision":2`)) {
		t.Fatalf("get status=%d body=%s", fetched.Code, fetched.Body.String())
	}
}

func TestWorkspaceHandlerRejectsInvalidAndUnauthenticatedInput(t *testing.T) {
	handler := workspaceHandler(NewWorkspaceStore(), "secret")
	invalid := workspaceRequest(t, handler, http.MethodPost, "/api/v1/workspaces", Workspace{CollectionView: "unknown"})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status=%d body=%s", invalid.Code, invalid.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", bytes.NewBufferString(`{}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", response.Code)
	}
}

func TestNormalizeWorkspaceClearsOrphanedSharedDetail(t *testing.T) {
	workspace, err := normalizeWorkspace(Workspace{
		DetailOpen:    true,
		DetailCompact: true,
		DetailPinned:  "missing",
		Active:        "detail",
	})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.DetailOpen || workspace.DetailPinned != "" {
		t.Fatalf("orphaned shared detail survived normalization: %#v", workspace)
	}
}
