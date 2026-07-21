package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/TheSnakeFang/rlviz/internal/model"
	"github.com/TheSnakeFang/rlviz/internal/plugins"
	"github.com/TheSnakeFang/rlviz/internal/server"
)

type UnsupportedFormatError struct {
	Path  string
	Cause error
}

func (err *UnsupportedFormatError) Error() string {
	return fmt.Sprintf("unsupported canonical format at %q: %v", err.Path, err.Cause)
}

func (err *UnsupportedFormatError) Unwrap() error { return err.Cause }

func (err *UnsupportedFormatError) DiagnosticCode() string { return "unsupported_format" }

func (err *UnsupportedFormatError) DiagnosticFields() map[string]any {
	return map[string]any{
		"path":              err.Path,
		"suggested_command": AdapterScaffoldCommand(err.Path),
	}
}

// AdapterScaffoldCommand keeps unsupported-format diagnostics and inspect
// output on one source-aware, shell-safe next step.
func AdapterScaffoldCommand(source string) string {
	arguments := []string{"rlviz", "plugin", "init", "--type", "adapter", "--lang", "python", "--from", source, ".rlviz/plugins/local-adapter"}
	for index, argument := range arguments {
		if argument != "" && strings.IndexFunc(argument, func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && !strings.ContainsRune("_@%+=:,./-", r)
		}) == -1 {
			continue
		}
		arguments[index] = "'" + strings.ReplaceAll(argument, "'", `'"'"'`) + "'"
	}
	return strings.Join(arguments, " ")
}

type PluginUntrustedError struct {
	Path   string
	Digest string
	Cause  error
}

func (err *PluginUntrustedError) Error() string          { return err.Cause.Error() }
func (err *PluginUntrustedError) Unwrap() error          { return err.Cause }
func (err *PluginUntrustedError) DiagnosticCode() string { return "plugin_untrusted" }
func (err *PluginUntrustedError) DiagnosticFields() map[string]any {
	return map[string]any{
		"plugin": err.Path, "digest": err.Digest,
		"suggested_command": "rlviz plugin trust " + err.Path,
	}
}

// LoadSource opens a canonical source directly or translates a private format
// through an explicitly selected, trusted adapter.
func LoadSource(ctx context.Context, path, adapterPath string) (string, server.Document, error) {
	if adapterPath == "" {
		resolved, err := ValidateSource(path)
		if err != nil {
			return "", server.Document{}, err
		}
		document, err := server.LoadCanonicalNDJSON(resolved)
		if err != nil {
			return "", server.Document{}, &UnsupportedFormatError{Path: resolved, Cause: err}
		}
		return resolved, document, nil
	}

	plugin, err := plugins.Load(adapterPath)
	if err != nil {
		return "", server.Document{}, fmt.Errorf("load adapter: %w", err)
	}
	trust, err := plugins.DefaultTrustStore()
	if err != nil {
		return "", server.Document{}, fmt.Errorf("locate adapter trust store: %w", err)
	}
	host := plugins.NewHost(trust)
	probeRequest, err := plugins.NewRequest("probe", path, "")
	if err != nil {
		return "", server.Document{}, err
	}
	probe, diagnostics, err := host.Probe(ctx, plugin, probeRequest)
	if err != nil {
		if errors.Is(err, plugins.ErrUntrusted) {
			return "", server.Document{}, &PluginUntrustedError{Path: plugin.Path, Digest: plugin.Digest, Cause: err}
		}
		return "", server.Document{}, withDiagnostics(err, diagnostics)
	}
	if !probe.Supported {
		return "", server.Document{}, fmt.Errorf("adapter %q does not support source: %s", plugin.Manifest.Name, probe.Reason)
	}

	streamRequest, err := plugins.NewRequest("stream", path, probeRequest.Source.Root)
	if err != nil {
		return "", server.Document{}, err
	}
	records := make([]*model.Record, 0)
	diagnostics, err = host.Stream(ctx, plugin, streamRequest, func(record *model.Record) error {
		records = append(records, record)
		return nil
	})
	if err != nil {
		return "", server.Document{}, withDiagnostics(err, diagnostics)
	}
	document, err := server.DocumentFromRecords(records)
	if err != nil {
		return "", server.Document{}, err
	}
	return probeRequest.Source.Path, document, nil
}

func withDiagnostics(err error, diagnostics string) error {
	if diagnostics == "" || errors.Is(err, context.Canceled) {
		return err
	}
	return fmt.Errorf("%w: %s", err, diagnostics)
}
