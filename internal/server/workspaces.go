package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maximumWorkspaceLanes   = 32
	maximumWorkspaceDetails = 32
	workspaceWaitTimeout    = 20 * time.Second
)

type WorkspaceAxis struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type WorkspaceLane struct {
	ID           string        `json:"id"`
	SourceID     string        `json:"sourceId"`
	TrajectoryID string        `json:"trajectoryId"`
	Band         string        `json:"band"`
	Selected     int64         `json:"selected"`
	Depth        int           `json:"depth"`
	Fidelity     int           `json:"fidelity"`
	Axis         WorkspaceAxis `json:"axis"`
	DescentStack []any         `json:"descentStack"`
}

// Workspace is the bounded logical state shared by the CLI and browser. Exact
// dock geometry remains device-local and is intentionally excluded.
type Workspace struct {
	Version        int             `json:"version"`
	RailExpanded   bool            `json:"railExpanded"`
	RailQuery      string          `json:"railQuery"`
	RailSelected   int             `json:"railSelected"`
	CollectionView string          `json:"collectionView"`
	GuideOpen      bool            `json:"guideOpen"`
	SettingsOpen   bool            `json:"settingsOpen"`
	Lanes          []WorkspaceLane `json:"lanes"`
	DetailOpen     bool            `json:"detailOpen"`
	DetailCompact  bool            `json:"detailCompact"`
	DetailPinned   string          `json:"detailPinned,omitempty"`
	Details        []string        `json:"details"`
	Direction      string          `json:"direction"`
	Reference      string          `json:"reference,omitempty"`
	Active         string          `json:"active"`
}

type WorkspaceEnvelope struct {
	ID        string    `json:"workspace_id"`
	Revision  int64     `json:"revision"`
	Workspace Workspace `json:"workspace"`
}

type workspaceEntry struct {
	envelope WorkspaceEnvelope
	changed  chan struct{}
}

type WorkspaceStore struct {
	mu      sync.Mutex
	entries map[string]*workspaceEntry
}

func NewWorkspaceStore() *WorkspaceStore {
	return &WorkspaceStore{entries: make(map[string]*workspaceEntry)}
}

func (store *WorkspaceStore) Create(workspace Workspace) (WorkspaceEnvelope, error) {
	normalized, err := normalizeWorkspace(workspace)
	if err != nil {
		return WorkspaceEnvelope{}, err
	}
	id, err := workspaceID()
	if err != nil {
		return WorkspaceEnvelope{}, err
	}
	envelope := WorkspaceEnvelope{ID: id, Revision: 1, Workspace: normalized}
	store.mu.Lock()
	store.entries[id] = &workspaceEntry{envelope: envelope, changed: make(chan struct{})}
	store.mu.Unlock()
	return envelope, nil
}

func (store *WorkspaceStore) Get(id string) (WorkspaceEnvelope, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, ok := store.entries[id]
	if !ok {
		return WorkspaceEnvelope{}, false
	}
	return entry.envelope, true
}

func (store *WorkspaceStore) Replace(id string, workspace Workspace) (WorkspaceEnvelope, error) {
	normalized, err := normalizeWorkspace(workspace)
	if err != nil {
		return WorkspaceEnvelope{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, ok := store.entries[id]
	if !ok {
		return WorkspaceEnvelope{}, errors.New("workspace not found")
	}
	entry.envelope.Revision++
	entry.envelope.Workspace = normalized
	close(entry.changed)
	entry.changed = make(chan struct{})
	return entry.envelope, nil
}

func (store *WorkspaceStore) Wait(ctx context.Context, id string, after int64) (WorkspaceEnvelope, bool) {
	for {
		store.mu.Lock()
		entry, ok := store.entries[id]
		if !ok {
			store.mu.Unlock()
			return WorkspaceEnvelope{}, false
		}
		if entry.envelope.Revision > after {
			envelope := entry.envelope
			store.mu.Unlock()
			return envelope, true
		}
		changed := entry.changed
		store.mu.Unlock()
		timer := time.NewTimer(workspaceWaitTimeout)
		select {
		case <-ctx.Done():
			timer.Stop()
			return WorkspaceEnvelope{}, false
		case <-changed:
			timer.Stop()
		case <-timer.C:
			return store.Get(id)
		}
	}
}

func workspaceID() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create workspace id: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func normalizeWorkspace(workspace Workspace) (Workspace, error) {
	workspace.Version = 3
	workspace.RailQuery = strings.TrimSpace(workspace.RailQuery)
	if len(workspace.RailQuery) > 500 {
		return Workspace{}, errors.New("rail query exceeds 500 characters")
	}
	if workspace.CollectionView == "" {
		workspace.CollectionView = "rollouts"
	}
	if workspace.CollectionView != "rollouts" && workspace.CollectionView != "trials" {
		return Workspace{}, errors.New("collectionView must be rollouts or trials")
	}
	if workspace.Direction == "" {
		workspace.Direction = "rows"
	}
	if workspace.Direction != "rows" && workspace.Direction != "columns" {
		return Workspace{}, errors.New("direction must be rows or columns")
	}
	if len(workspace.Lanes) > maximumWorkspaceLanes || len(workspace.Details) > maximumWorkspaceDetails {
		return Workspace{}, errors.New("workspace exceeds lane or detail limit")
	}
	ids := make(map[string]bool, len(workspace.Lanes))
	focus := 0
	for index := range workspace.Lanes {
		lane := &workspace.Lanes[index]
		if lane.SourceID == "" || lane.TrajectoryID == "" {
			return Workspace{}, errors.New("workspace lanes require sourceId and trajectoryId")
		}
		lane.ID = url.PathEscape(lane.SourceID) + ":" + url.PathEscape(lane.TrajectoryID)
		if ids[lane.ID] {
			return Workspace{}, fmt.Errorf("duplicate workspace lane %q", lane.ID)
		}
		ids[lane.ID] = true
		if lane.Band == "" {
			lane.Band = "focus"
		}
		if lane.Band != "focus" && lane.Band != "context" {
			return Workspace{}, errors.New("lane band must be focus or context")
		}
		if lane.Band == "focus" {
			focus++
			if focus > 2 {
				lane.Band = "context"
			}
		}
		if lane.Depth < 1 || lane.Depth > 4 {
			lane.Depth = 1
		}
		if lane.Fidelity < 0 || lane.Fidelity > 2 {
			lane.Fidelity = 1
		}
		if lane.Axis.End < lane.Axis.Start {
			lane.Axis = WorkspaceAxis{Start: 0, End: 1}
		}
		if lane.DescentStack == nil {
			lane.DescentStack = []any{}
		}
	}
	details := make([]string, 0, len(workspace.Details))
	seenDetails := make(map[string]bool)
	for _, id := range workspace.Details {
		if ids[id] && !seenDetails[id] {
			details = append(details, id)
			seenDetails[id] = true
		}
	}
	workspace.Details = details
	if workspace.DetailPinned != "" && !ids[workspace.DetailPinned] {
		workspace.DetailPinned = ""
	}
	if len(workspace.Lanes) == 0 {
		workspace.DetailOpen = false
		workspace.DetailPinned = ""
	}
	if workspace.Active == "" {
		workspace.Active = "rail"
	}
	return workspace, nil
}

func workspaceHandler(store *WorkspaceStore, token string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !authorized(request, token) {
			writeJSONError(response, http.StatusUnauthorized, "unauthorized", errors.New("valid daemon token required"))
			return
		}
		if store == nil {
			writeJSONError(response, http.StatusServiceUnavailable, "workspace_unavailable", errors.New("workspace control is unavailable"))
			return
		}
		path := strings.TrimPrefix(request.URL.Path, "/api/v1/workspaces")
		if path == "" || path == "/" {
			if request.Method != http.MethodPost {
				response.Header().Set("Allow", http.MethodPost)
				writeJSONError(response, http.StatusMethodNotAllowed, "method_not_allowed", errors.New("workspace collection requires POST"))
				return
			}
			var workspace Workspace
			if !decodeWorkspace(response, request, &workspace) {
				return
			}
			envelope, err := store.Create(workspace)
			if err != nil {
				writeJSONError(response, http.StatusBadRequest, "invalid_workspace", err)
				return
			}
			writeJSON(response, http.StatusCreated, envelope)
			return
		}
		id := strings.Trim(path, "/")
		if id == "" || strings.Contains(id, "/") {
			writeJSONError(response, http.StatusNotFound, "workspace_not_found", errors.New("workspace not found"))
			return
		}
		switch request.Method {
		case http.MethodGet:
			after, err := strconv.ParseInt(request.URL.Query().Get("after"), 10, 64)
			if request.URL.Query().Get("after") == "" {
				after = -1
				err = nil
			}
			if err != nil || after < -1 {
				writeJSONError(response, http.StatusBadRequest, "invalid_query", errors.New("after must be a non-negative revision"))
				return
			}
			envelope, ok := store.Wait(request.Context(), id, after)
			if !ok {
				if request.Context().Err() != nil {
					return
				}
				writeJSONError(response, http.StatusNotFound, "workspace_not_found", errors.New("workspace not found"))
				return
			}
			writeJSON(response, http.StatusOK, envelope)
		case http.MethodPut:
			var workspace Workspace
			if !decodeWorkspace(response, request, &workspace) {
				return
			}
			envelope, err := store.Replace(id, workspace)
			if err != nil {
				status := http.StatusBadRequest
				code := "invalid_workspace"
				if err.Error() == "workspace not found" {
					status, code = http.StatusNotFound, "workspace_not_found"
				}
				writeJSONError(response, status, code, err)
				return
			}
			writeJSON(response, http.StatusOK, envelope)
		default:
			response.Header().Set("Allow", "GET, PUT")
			writeJSONError(response, http.StatusMethodNotAllowed, "method_not_allowed", errors.New("workspace requires GET or PUT"))
		}
	})
}

func decodeWorkspace(response http.ResponseWriter, request *http.Request, workspace *Workspace) bool {
	request.Body = http.MaxBytesReader(response, request.Body, 256<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(workspace); err != nil {
		writeJSONError(response, http.StatusBadRequest, "invalid_request", err)
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("request contains multiple JSON values")
		}
		writeJSONError(response, http.StatusBadRequest, "invalid_request", err)
		return false
	}
	return true
}
