package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/unlatch-ai/rolloutviz/internal/daemon"
	"github.com/unlatch-ai/rolloutviz/internal/server"
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

	registry := server.NewRegistry()
	var httpServer *http.Server
	stop := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if httpServer != nil {
			_ = httpServer.Shutdown(ctx)
		}
	}
	loader := func(ctx context.Context, path, adapter string) (string, server.Document, error) {
		return LoadSource(ctx, path, adapter)
	}
	httpServer = &http.Server{
		Handler:           server.NewRegistryHandler(registry, token, loader, stop),
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
