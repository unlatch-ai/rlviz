package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/model"
	"github.com/TheSnakeFang/rlviz/internal/plugins"
	"github.com/TheSnakeFang/rlviz/internal/server"
)

type IndexedSource struct {
	Info      rolloutindex.SourceInfo
	Refreshed bool
}

// IndexSource validates a source and transactionally refreshes its persistent
// canonical index when the source or selected adapter changed.
func IndexSource(ctx context.Context, store *rolloutindex.Index, path, adapterPath string) (IndexedSource, error) {
	if store == nil {
		return IndexedSource{}, errors.New("rollout index is required")
	}
	resolved, err := ValidateSource(path)
	if err != nil {
		return IndexedSource{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return IndexedSource{}, err
	}

	if adapterPath == "" {
		source := rolloutindex.Source{
			ID: server.SourceID(resolved + "\x00builtin:canonical"), Path: resolved,
			Fingerprint: "canonical:" + plugins.APIVersion, Size: info.Size(), ModTime: info.ModTime(),
		}
		if cached, ok, err := freshSource(ctx, store, source); err != nil {
			return IndexedSource{}, err
		} else if ok {
			return IndexedSource{Info: cached}, nil
		}
		file, err := os.Open(resolved)
		if err != nil {
			return IndexedSource{}, fmt.Errorf("open canonical source: %w", err)
		}
		defer file.Close()
		indexed, err := store.Replace(ctx, source, file)
		if err != nil {
			return IndexedSource{}, &UnsupportedFormatError{Path: resolved, Cause: err}
		}
		return IndexedSource{Info: indexed, Refreshed: true}, nil
	}

	plugin, err := plugins.Load(adapterPath)
	if err != nil {
		return IndexedSource{}, fmt.Errorf("load adapter: %w", err)
	}
	trust, err := plugins.DefaultTrustStore()
	if err != nil {
		return IndexedSource{}, fmt.Errorf("locate adapter trust store: %w", err)
	}
	host := plugins.NewHost(trust)
	probeRequest, err := plugins.NewRequest("probe", resolved, "")
	if err != nil {
		return IndexedSource{}, err
	}
	probe, diagnostics, err := host.Probe(ctx, plugin, probeRequest)
	if err != nil {
		if errors.Is(err, plugins.ErrUntrusted) {
			return IndexedSource{}, &PluginUntrustedError{Path: plugin.Path, Digest: plugin.Digest, Cause: err}
		}
		return IndexedSource{}, withDiagnostics(err, diagnostics)
	}
	if !probe.Supported {
		return IndexedSource{}, fmt.Errorf("adapter %q does not support source: %s", plugin.Manifest.Name, probe.Reason)
	}
	source := rolloutindex.Source{
		ID: server.SourceID(resolved + "\x00adapter:" + plugin.Path), Path: resolved,
		Adapter: plugin.Path, Fingerprint: plugin.Digest, Size: info.Size(), ModTime: info.ModTime(),
	}
	if cached, ok, err := freshSource(ctx, store, source); err != nil {
		return IndexedSource{}, err
	} else if ok {
		return IndexedSource{Info: cached}, nil
	}

	temporary, err := os.CreateTemp("", "rlviz-adapter-stream-*.ndjson")
	if err != nil {
		return IndexedSource{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	defer temporary.Close()
	if err := temporary.Chmod(0o600); err != nil {
		return IndexedSource{}, err
	}
	writer := bufio.NewWriterSize(temporary, 64<<10)
	streamRequest, err := plugins.NewRequest("stream", resolved, probeRequest.Source.Root)
	if err != nil {
		return IndexedSource{}, err
	}
	diagnostics, err = host.Stream(ctx, plugin, streamRequest, func(record *model.Record) error {
		if _, err := writer.Write(record.Raw); err != nil {
			return err
		}
		return writer.WriteByte('\n')
	})
	if err != nil {
		return IndexedSource{}, withDiagnostics(err, diagnostics)
	}
	if err := writer.Flush(); err != nil {
		return IndexedSource{}, err
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return IndexedSource{}, err
	}
	indexed, err := store.Replace(ctx, source, temporary)
	if err != nil {
		return IndexedSource{}, fmt.Errorf("index adapter output: %w", err)
	}
	return IndexedSource{Info: indexed, Refreshed: true}, nil
}

func freshSource(ctx context.Context, store *rolloutindex.Index, source rolloutindex.Source) (rolloutindex.SourceInfo, bool, error) {
	status, err := store.Status(ctx, source)
	if err != nil {
		return rolloutindex.SourceInfo{}, false, err
	}
	if status.State == rolloutindex.CacheFresh && status.Cached != nil {
		return *status.Cached, true, nil
	}
	return rolloutindex.SourceInfo{}, false, nil
}
