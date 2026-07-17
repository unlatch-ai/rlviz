package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/unlatch-ai/rlviz/internal/daemon"
	rolloutindex "github.com/unlatch-ai/rlviz/internal/index"
	"github.com/unlatch-ai/rlviz/internal/server"
	"github.com/unlatch-ai/rlviz/internal/watch"
)

// RunDaemon serves the authenticated per-user registry until stopped through
// the private API or an operating-system signal.
func RunDaemon(paths daemon.Paths, version string) error {
	if err := paths.EnsureRuntimeDir(); err != nil {
		return err
	}
	listener, err := server.ListenLoopback(0)
	if err != nil {
		return err
	}
	defer listener.Close()
	token, err := daemon.GenerateToken()
	if err != nil {
		return err
	}
	store, err := rolloutindex.Open(paths.IndexFile)
	if err != nil {
		return err
	}
	defer store.Close()
	sourceWatcher := watch.New(500 * time.Millisecond)
	defer sourceWatcher.Close()
	watchContext, cancelWatches := context.WithCancel(context.Background())
	defer cancelWatches()
	sourceIndexer := NewSourceIndexer(watchContext, store)

	var httpServer *http.Server
	stop := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if httpServer != nil {
			_ = httpServer.Shutdown(ctx)
		}
	}
	registrar := func(ctx context.Context, path, adapter string, presentationConfig json.RawMessage) (server.Registration, error) {
		indexed, err := sourceIndexer.Index(ctx, path, adapter)
		if err != nil {
			return server.Registration{}, err
		}
		source := indexed.Info.Source
		if err := sourceWatcher.Add(watchContext, source.ID, source.Path, func(changeContext context.Context, _ watch.Change) error {
			_, refreshErr := sourceIndexer.Index(changeContext, source.Path, adapter)
			if refreshErr != nil {
				fmt.Fprintf(os.Stderr, "refresh source %s: %v\n", source.Path, refreshErr)
			}
			return refreshErr
		}); err != nil {
			return server.Registration{}, fmt.Errorf("watch source: %w", err)
		}
		if err := store.SetPresentation(ctx, source.ID, presentationConfig); err != nil {
			return server.Registration{}, err
		}
		return server.Registration{
			SourceID: source.ID, Path: source.Path,
			URL: "/?trajectory=" + source.ID + "&indexed=1",
		}, nil
	}
	httpServer = &http.Server{
		Handler:           server.NewPersistentHandler(store, token, registrar, stop),
		ReadHeaderTimeout: 5 * time.Second,
	}
	metadata := daemon.Metadata{
		PID: os.Getpid(), Address: listener.Addr().String(), Token: token, Version: version,
	}
	if err := daemon.WriteMetadata(paths, metadata); err != nil {
		return err
	}
	defer daemon.RemoveMetadata(paths)

	signalContext, cancelSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelSignals()
	go func() {
		<-signalContext.Done()
		stop()
	}()

	if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve daemon: %w", err)
	}
	return nil
}
