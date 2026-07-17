package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/unlatch-ai/rlviz/internal/daemon"
	"github.com/unlatch-ai/rlviz/internal/presentation"
	"github.com/unlatch-ai/rlviz/internal/server"
)

type Viewer struct {
	SourcePath   string
	AdapterPath  string
	Presentation json.RawMessage
	Port         int
}

type RunningViewer struct {
	URL        string
	SourcePath string
	Listener   net.Listener
	Server     *http.Server
}

// StartViewer validates and parses a source before opening a loopback listener.
// The caller owns the returned server lifecycle.
func StartViewer(config Viewer) (*RunningViewer, error) {
	presentationConfig, err := presentation.NormalizeJSON(config.Presentation)
	if err != nil {
		return nil, fmt.Errorf("validate presentation configuration: %w", err)
	}
	path, document, err := LoadSource(context.Background(), config.SourcePath, config.AdapterPath)
	if err != nil {
		return nil, err
	}
	document.Presentation = presentationConfig
	listener, err := server.ListenLoopback(config.Port)
	if err != nil {
		return nil, err
	}
	token, err := daemon.GenerateToken()
	if err != nil {
		listener.Close()
		return nil, err
	}
	registry := server.NewRegistry()
	id := registry.Put(path, document)
	httpServer := &http.Server{
		Handler:           server.NewRegistryHandler(registry, token, nil, nil),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return &RunningViewer{
		URL:        fmt.Sprintf("http://%s/?trajectory=%s#token=%s", listener.Addr().String(), id, token),
		SourcePath: path,
		Listener:   listener,
		Server:     httpServer,
	}, nil
}

func (viewer *RunningViewer) Serve() error {
	err := viewer.Server.Serve(viewer.Listener)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (viewer *RunningViewer) Shutdown(ctx context.Context) error {
	return viewer.Server.Shutdown(ctx)
}
