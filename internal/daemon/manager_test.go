package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type launcherFunc func(LaunchConfig) (int, error)

func (function launcherFunc) Start(config LaunchConfig) (int, error) {
	return function(config)
}

type transportFunc func(*http.Request) (*http.Response, error)

func (function transportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestLoadLiveMetadataRemovesStaleRecord(t *testing.T) {
	paths := PathsAt(filepath.Join(t.TempDir(), "runtime"))
	if err := WriteMetadata(paths, testMetadata(t, "127.0.0.1:7317")); err != nil {
		t.Fatal(err)
	}
	client := Client{HTTP: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}}
	if _, err := LoadLiveMetadata(context.Background(), paths, client); !errors.Is(err, ErrDaemonUnavailable) {
		t.Fatalf("LoadLiveMetadata error = %v, want ErrDaemonUnavailable", err)
	}
	if _, err := ReadMetadata(paths); !errors.Is(err, ErrNoMetadata) {
		t.Fatalf("ReadMetadata after stale cleanup = %v, want ErrNoMetadata", err)
	}
}

func TestManagerEnsureStartsAndThenReusesDaemon(t *testing.T) {
	paths := PathsAt(filepath.Join(t.TempDir(), "runtime"))
	metadata := testMetadata(t, "127.0.0.1:1")
	server := newLoopbackTestServer(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+metadata.Token {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = jsonResponse(response, Status{Status: "running", PID: metadata.PID, Version: metadata.Version})
	}))
	defer server.Close()
	metadata.Address = strings.TrimPrefix(server.URL, "http://")
	var starts atomic.Int32
	manager := Manager{
		Paths:      paths,
		Client:     Client{HTTP: server.Client()},
		Executable: "/ignored/rlviz",
		Args:       []string{"daemon", "serve"},
		Version:    metadata.Version,
		Launcher: launcherFunc(func(config LaunchConfig) (int, error) {
			starts.Add(1)
			if config.LogPath != paths.LogFile {
				t.Errorf("log path = %q, want %q", config.LogPath, paths.LogFile)
			}
			return metadata.PID, WriteMetadata(paths, metadata)
		}),
		StartupTimeout: time.Second,
		PollInterval:   time.Millisecond,
	}
	first, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !first.Started {
		t.Fatal("first Ensure did not report startup")
	}
	second, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Started {
		t.Fatal("second Ensure reported startup instead of reuse")
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("launcher starts = %d, want 1", got)
	}
}

func TestManagerEnsureReplacesOlderDaemonVersion(t *testing.T) {
	paths := PathsAt(filepath.Join(t.TempDir(), "runtime"))
	metadata := testMetadata(t, "127.0.0.1:1")
	metadata.Version = "0.1.0"
	server := newLoopbackTestServer(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+metadata.Token {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == StatusPath:
			current, err := ReadMetadata(paths)
			if err != nil {
				http.Error(response, err.Error(), http.StatusServiceUnavailable)
				return
			}
			_ = jsonResponse(response, Status{Status: "running", PID: current.PID, Version: current.Version})
		case request.Method == http.MethodPost && request.URL.Path == StopPath:
			_ = RemoveMetadata(paths)
			response.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	metadata.Address = strings.TrimPrefix(server.URL, "http://")
	if err := WriteMetadata(paths, metadata); err != nil {
		t.Fatal(err)
	}
	var starts atomic.Int32
	manager := Manager{
		Paths: paths, Client: Client{HTTP: server.Client()}, Executable: "/ignored/rlviz",
		Args: []string{"daemon", "serve"}, Version: "0.2.0",
		Launcher: launcherFunc(func(LaunchConfig) (int, error) {
			starts.Add(1)
			upgraded := metadata
			upgraded.Version = "0.2.0"
			return upgraded.PID, WriteMetadata(paths, upgraded)
		}),
		StartupTimeout: time.Second, PollInterval: time.Millisecond,
	}
	result, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Started || result.Metadata.Version != "0.2.0" || starts.Load() != 1 {
		t.Fatalf("upgrade result = %#v, starts=%d", result, starts.Load())
	}
}

func jsonResponse(response http.ResponseWriter, value any) error {
	response.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(response).Encode(value)
}
