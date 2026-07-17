package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/unlatch-ai/rlviz/internal/model"
	"github.com/unlatch-ai/rlviz/internal/presentation"
	webassets "github.com/unlatch-ai/rlviz/web"
)

const fallbackUI = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>RLViz</title><style>body{font:16px system-ui;margin:3rem;max-width:70ch}code{background:#eee;padding:.15rem .3rem}</style></head>
<body><h1>RLViz</h1><p>The local viewer is running.</p><p>Trajectory data is available at <code>/api/v1/trajectory</code>.</p></body></html>`

const MaxCanonicalSourceBytes int64 = 32 << 20

type Document struct {
	Trajectory   *model.Trajectory `json:"trajectory"`
	Events       []*model.Event    `json:"events"`
	Run          *model.Run        `json:"run,omitempty"`
	Case         *model.Case       `json:"case,omitempty"`
	Group        *model.Group      `json:"group,omitempty"`
	Signals      []*model.Signal   `json:"signals,omitempty"`
	Artifacts    []*model.Artifact `json:"artifacts,omitempty"`
	Presentation json.RawMessage   `json:"presentation"`
}

type SourceLoader func(ctx context.Context, path, adapter string) (resolvedPath string, document Document, err error)

type DiagnosticError interface {
	error
	DiagnosticCode() string
	DiagnosticFields() map[string]any
}

type Registration struct {
	SourceID string `json:"source_id"`
	Path     string `json:"path"`
	URL      string `json:"url"`
}

// LoadCanonicalNDJSON reads canonical records without changing the source.
func LoadCanonicalNDJSON(path string) (Document, error) {
	file, err := os.Open(path)
	if err != nil {
		return Document{}, fmt.Errorf("open canonical trajectory: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Document{}, fmt.Errorf("stat canonical trajectory: %w", err)
	}
	if info.Size() > MaxCanonicalSourceBytes {
		return Document{}, fmt.Errorf("canonical trajectory is %d bytes; maximum is %d", info.Size(), MaxCanonicalSourceBytes)
	}

	records := make([]*model.Record, 0)
	limited := &io.LimitedReader{R: file, N: MaxCanonicalSourceBytes + 1}
	decodeErr := model.Decode(limited, func(record *model.Record) error {
		records = append(records, record)
		return nil
	})
	if limited.N == 0 {
		return Document{}, fmt.Errorf("canonical trajectory exceeds maximum of %d bytes", MaxCanonicalSourceBytes)
	}
	if decodeErr != nil {
		return Document{}, fmt.Errorf("decode canonical trajectory: %w", decodeErr)
	}
	return DocumentFromRecords(records)
}

// DocumentFromRecords selects the first trajectory in a validated canonical
// stream and resolves its run, case, group, events, signals, and artifacts.
func DocumentFromRecords(records []*model.Record) (Document, error) {
	document := Document{
		Events: make([]*model.Event, 0), Signals: make([]*model.Signal, 0),
		Artifacts: make([]*model.Artifact, 0),
	}
	allEvents := make([]*model.Event, 0)
	allSignals := make([]*model.Signal, 0)
	allArtifacts := make([]*model.Artifact, 0)
	runs := make(map[string]*model.Run)
	cases := make(map[string]*model.Case)
	groups := make(map[string]*model.Group)
	for _, record := range records {
		if record == nil {
			return Document{}, fmt.Errorf("canonical record is nil")
		}
		switch value := record.Value.(type) {
		case *model.Run:
			runs[value.ID] = value
		case *model.Case:
			cases[value.ID] = value
		case *model.Group:
			groups[value.ID] = value
		case *model.Trajectory:
			if document.Trajectory == nil {
				document.Trajectory = value
			}
		case *model.Event:
			// Keep the canonical source line available to the inspector even
			// when the adapter did not provide a separate raw payload.
			if len(value.Raw) == 0 {
				value.Raw = append(json.RawMessage(nil), record.Raw...)
			}
			allEvents = append(allEvents, value)
		case *model.Signal:
			allSignals = append(allSignals, value)
		case *model.Artifact:
			allArtifacts = append(allArtifacts, value)
		}
	}
	if document.Trajectory == nil {
		return Document{}, fmt.Errorf("canonical trajectory contains no trajectory record")
	}
	for _, event := range allEvents {
		if event.TrajectoryID == document.Trajectory.ID {
			document.Events = append(document.Events, event)
		}
	}
	for _, signal := range allSignals {
		if signal.TrajectoryID == document.Trajectory.ID {
			document.Signals = append(document.Signals, signal)
		}
	}
	for _, artifact := range allArtifacts {
		if artifact.TrajectoryID == document.Trajectory.ID {
			document.Artifacts = append(document.Artifacts, artifact)
		}
	}
	document.Group = groups[document.Trajectory.GroupID]
	if document.Group != nil {
		document.Case = cases[document.Group.CaseID]
	}
	if document.Case != nil {
		document.Run = runs[document.Case.RunID]
	}
	return document, nil
}

func ListenLoopback(port int) (net.Listener, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("port must be between 0 and 65535")
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("listen on loopback: %w", err)
	}
	return listener, nil
}

func NewHandler(document Document) http.Handler {
	registry := NewRegistry()
	registry.Put("foreground", document)
	return NewRegistryHandler(registry, "", nil, nil)
}

// NewRegistryHandler serves a multi-source daemon. Mutation endpoints require
// a bearer token. Foreground mode passes an empty token and remains directly
// readable; daemon trajectory reads use the same bearer token as registration.
func NewRegistryHandler(registry *Registry, token string, loader SourceLoader, stop func()) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/trajectory", func(response http.ResponseWriter, request *http.Request) {
		if token != "" && !authorized(request, token) {
			writeJSONError(response, http.StatusUnauthorized, "unauthorized", errors.New("valid daemon token required"))
			return
		}
		document, err := registry.Require(request.URL.Query().Get("trajectory"))
		if err != nil {
			writeJSONError(response, http.StatusNotFound, "trajectory_not_found", err)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(response).Encode(document); err != nil {
			http.Error(response, "encode trajectory response", http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("GET /api/v1/health", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"status":"ok"}`+"\n")
	})
	mux.HandleFunc("GET /api/v1/daemon/status", func(response http.ResponseWriter, request *http.Request) {
		if !authorized(request, token) {
			writeJSONError(response, http.StatusUnauthorized, "unauthorized", errors.New("valid daemon token required"))
			return
		}
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(response, `{"status":"ok"}`+"\n")
	})
	mux.HandleFunc("POST /api/v1/sources", func(response http.ResponseWriter, request *http.Request) {
		if !authorized(request, token) {
			writeJSONError(response, http.StatusUnauthorized, "unauthorized", errors.New("valid daemon token required"))
			return
		}
		if loader == nil {
			writeJSONError(response, http.StatusNotImplemented, "registration_unavailable", errors.New("source registration is unavailable"))
			return
		}
		request.Body = http.MaxBytesReader(response, request.Body, 1<<20)
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		var input struct {
			Path         string          `json:"path"`
			Adapter      string          `json:"adapter,omitempty"`
			Presentation json.RawMessage `json:"presentation"`
		}
		if err := decoder.Decode(&input); err != nil {
			writeJSONError(response, http.StatusBadRequest, "invalid_request", err)
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			if err == nil {
				err = errors.New("request contains multiple JSON values")
			}
			writeJSONError(response, http.StatusBadRequest, "invalid_request", err)
			return
		}
		normalizedPresentation, err := presentation.NormalizeJSON(input.Presentation)
		if err != nil {
			writeJSONError(response, http.StatusBadRequest, "invalid_presentation", err)
			return
		}
		resolved, document, err := loader(request.Context(), input.Path, input.Adapter)
		if err != nil {
			writeSourceError(response, err)
			return
		}
		document.Presentation = normalizedPresentation
		identity := resolved + "\x00builtin:canonical"
		if input.Adapter != "" {
			adapter, resolveErr := filepath.Abs(input.Adapter)
			if resolveErr != nil {
				writeSourceError(response, resolveErr)
				return
			}
			if evaluated, evaluateErr := filepath.EvalSymlinks(adapter); evaluateErr == nil {
				adapter = evaluated
			}
			identity = resolved + "\x00adapter:" + adapter
		}
		id := registry.PutWithIdentity(identity, resolved, document)
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Cache-Control", "no-store")
		response.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(response).Encode(Registration{SourceID: id, Path: resolved, URL: "/?trajectory=" + id})
	})
	mux.HandleFunc("POST /api/v1/daemon/stop", func(response http.ResponseWriter, request *http.Request) {
		if !authorized(request, token) {
			writeJSONError(response, http.StatusUnauthorized, "unauthorized", errors.New("valid daemon token required"))
			return
		}
		if stop == nil {
			writeJSONError(response, http.StatusNotImplemented, "stop_unavailable", errors.New("daemon stop is unavailable"))
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"status":"stopping"}`+"\n")
		go stop()
	})
	mux.Handle("GET /", viewerHandler())
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		mux.ServeHTTP(response, request)
	})
}

func authorized(request *http.Request, token string) bool {
	return token != "" && request.Header.Get("Authorization") == "Bearer "+token
}

func writeJSONError(response http.ResponseWriter, status int, code string, err error) {
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(map[string]string{"code": code, "error": err.Error()})
}

func writeSourceError(response http.ResponseWriter, err error) {
	payload := map[string]any{"code": "source_invalid", "error": err.Error()}
	var diagnostic DiagnosticError
	if errors.As(err, &diagnostic) {
		payload["code"] = diagnostic.DiagnosticCode()
		for key, value := range diagnostic.DiagnosticFields() {
			payload[key] = value
		}
	}
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(response).Encode(payload)
}

func viewerHandler() http.Handler {
	dist, err := fs.Sub(webassets.Dist, "dist")
	if err != nil {
		return fallbackHandler()
	}
	if _, err := fs.Stat(dist, "index.html"); err != nil {
		return fallbackHandler()
	}

	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/") {
			http.NotFound(response, request)
			return
		}
		name := strings.TrimPrefix(request.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if info, err := fs.Stat(dist, name); err == nil && !info.IsDir() {
			files.ServeHTTP(response, request)
			return
		}

		// Viewer URLs are client-side deep links. Unknown asset-like paths stay
		// 404s, while route paths receive the application shell.
		if strings.Contains(name, ".") {
			http.NotFound(response, request)
			return
		}
		request.URL.Path = "/"
		files.ServeHTTP(response, request)
	})
}

func fallbackHandler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(response, fallbackUI)
	})
}
