package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TheSnakeFang/rlviz/internal/daemon"
	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/presentation"
	"github.com/TheSnakeFang/rlviz/internal/server"
)

type Viewer struct {
	SourcePath   string
	AdapterPath  string
	Presentation json.RawMessage
	Port         int
}

type RunningViewer struct {
	URL         string
	SourcePath  string
	Listener    net.Listener
	Server      *http.Server
	cleanup     func()
	cleanupOnce sync.Once
}

// StartViewer validates and parses a source before opening a loopback listener.
// The caller owns the returned server lifecycle.
func StartViewer(config Viewer) (*RunningViewer, error) {
	if _, err := presentation.NormalizeJSON(config.Presentation); err != nil {
		return nil, fmt.Errorf("validate presentation configuration: %w", err)
	}
	listener, err := server.ListenLoopback(config.Port)
	if err != nil {
		return nil, err
	}
	viewer, err := startViewer(config, listener)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	return viewer, nil
}

func startViewer(config Viewer, listener net.Listener) (*RunningViewer, error) {
	presentationConfig, err := presentation.NormalizeJSON(config.Presentation)
	if err != nil {
		return nil, fmt.Errorf("validate presentation configuration: %w", err)
	}
	temporaryDir, err := os.MkdirTemp("", "rlviz-serve-")
	if err != nil {
		return nil, fmt.Errorf("create foreground index: %w", err)
	}
	cleanupDir := func() { _ = os.RemoveAll(temporaryDir) }
	store, err := rolloutindex.Open(filepath.Join(temporaryDir, "index.sqlite"))
	if err != nil {
		cleanupDir()
		return nil, err
	}
	cleanup := func() { _ = store.Close(); cleanupDir() }
	indexed, err := IndexSource(context.Background(), store, config.SourcePath, config.AdapterPath)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := store.SetPresentation(context.Background(), indexed.Info.Source.ID, presentationConfig); err != nil {
		cleanup()
		return nil, err
	}
	token, err := daemon.GenerateToken()
	if err != nil {
		cleanup()
		return nil, err
	}
	httpServer := &http.Server{
		Handler:           server.NewPersistentHandler(store, token, nil, nil),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return &RunningViewer{
		URL:        fmt.Sprintf("http://%s/?trajectory=%s&indexed=1#token=%s", listener.Addr().String(), indexed.Info.Source.ID, token),
		SourcePath: indexed.Info.Source.Path,
		Listener:   listener,
		Server:     httpServer,
		cleanup:    cleanup,
	}, nil
}

func (viewer *RunningViewer) Serve() error {
	defer viewer.runCleanup()
	err := viewer.Server.Serve(viewer.Listener)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (viewer *RunningViewer) Shutdown(ctx context.Context) error {
	err := viewer.Server.Shutdown(ctx)
	viewer.runCleanup()
	return err
}

func (viewer *RunningViewer) runCleanup() {
	viewer.cleanupOnce.Do(func() {
		if viewer.cleanup != nil {
			viewer.cleanup()
		}
	})
}
