package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/unlatch-ai/rolloutviz/internal/model"
)

type Source struct {
	Path       string `json:"path"`
	Root       string `json:"root"`
	Kind       string `json:"kind,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
}

type Limits struct {
	ProbeBytes int64 `json:"probe_bytes,omitempty"`
	TimeoutMS  int64 `json:"timeout_ms,omitempty"`
}

type Request struct {
	APIVersion string         `json:"api_version"`
	Operation  string         `json:"operation"`
	Source     Source         `json:"source"`
	Options    map[string]any `json:"options,omitempty"`
	Limits     Limits         `json:"limits,omitempty"`
}

type ProbeResponse struct {
	Supported  bool    `json:"supported"`
	Confidence float64 `json:"confidence"`
	Format     string  `json:"format,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}

type Result struct {
	Stdout []byte
	Stderr string
}

type Host struct {
	Trust          *TrustStore
	Timeout        time.Duration
	MaxStdoutBytes int64
	MaxStderrBytes int64
}

func NewHost(trust *TrustStore) *Host {
	return &Host{Trust: trust, Timeout: 10 * time.Second, MaxStdoutBytes: 32 << 20, MaxStderrBytes: 1 << 20}
}

// NewRequest resolves source and root symlinks and guarantees source is inside
// root. If root is empty, a file's parent or the directory itself is used.
func NewRequest(operation, sourcePath, root string) (Request, error) {
	if operation != "probe" && operation != "stream" {
		return Request{}, fmt.Errorf("unsupported operation %q", operation)
	}
	source, err := filepath.Abs(sourcePath)
	if err != nil {
		return Request{}, err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return Request{}, fmt.Errorf("resolve source: %w", err)
	}
	info, err := os.Stat(source)
	if err != nil {
		return Request{}, err
	}
	kind := "file"
	if info.IsDir() {
		kind = "directory"
	} else if !info.Mode().IsRegular() {
		return Request{}, errors.New("source must be a regular file or directory")
	}
	if root == "" {
		if kind == "directory" {
			root = source
		} else {
			root = filepath.Dir(source)
		}
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return Request{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Request{}, fmt.Errorf("resolve root: %w", err)
	}
	if !within(root, source) {
		return Request{}, fmt.Errorf("source %s escapes root %s", source, root)
	}
	return Request{APIVersion: APIVersion, Operation: operation, Source: Source{Path: source, Root: root, Kind: kind, SizeBytes: info.Size(), ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano)}}, nil
}

func (h *Host) Probe(ctx context.Context, plugin *Plugin, request Request) (ProbeResponse, string, error) {
	request.Operation = "probe"
	request.APIVersion = APIVersion
	result, err := h.run(ctx, plugin, request)
	if err != nil {
		return ProbeResponse{}, result.Stderr, err
	}
	var wire struct {
		Supported  *bool    `json:"supported"`
		Confidence *float64 `json:"confidence"`
		Format     string   `json:"format,omitempty"`
		Reason     string   `json:"reason,omitempty"`
	}
	dec := json.NewDecoder(bytes.NewReader(result.Stdout))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&wire); err != nil {
		return ProbeResponse{}, result.Stderr, fmt.Errorf("invalid probe response: %w", err)
	}
	if err := ensureEOF(dec); err != nil {
		return ProbeResponse{}, result.Stderr, err
	}
	if wire.Supported == nil || wire.Confidence == nil {
		return ProbeResponse{}, result.Stderr, errors.New("probe response requires supported and confidence")
	}
	response := ProbeResponse{Supported: *wire.Supported, Confidence: *wire.Confidence, Format: wire.Format, Reason: wire.Reason}
	if math.IsNaN(response.Confidence) || math.IsInf(response.Confidence, 0) || response.Confidence < 0 || response.Confidence > 1 {
		return response, result.Stderr, errors.New("probe confidence must be between 0 and 1")
	}
	if response.Supported && response.Format == "" {
		return response, result.Stderr, errors.New("supported probe response requires format")
	}
	return response, result.Stderr, nil
}

func (h *Host) Stream(ctx context.Context, plugin *Plugin, request Request, visit func(*model.Record) error) (string, error) {
	request.Operation = "stream"
	request.APIVersion = APIVersion
	result, err := h.run(ctx, plugin, request)
	if err != nil {
		return result.Stderr, err
	}
	if err := model.Decode(bytes.NewReader(result.Stdout), func(record *model.Record) error {
		if err := validateRecordProvenance(record, request.Source.Root); err != nil {
			return err
		}
		return visit(record)
	}); err != nil {
		return result.Stderr, fmt.Errorf("invalid canonical stream: %w", err)
	}
	return result.Stderr, nil
}

func (h *Host) run(ctx context.Context, plugin *Plugin, request Request) (Result, error) {
	var result Result
	if plugin == nil {
		return result, errors.New("plugin is nil")
	}
	if h.Trust == nil {
		return result, errors.New("trust store is required")
	}
	current, err := Load(plugin.Path)
	if err != nil {
		return result, err
	}
	if current.Digest != plugin.Digest {
		return result, errors.New("plugin changed after it was loaded; reload and trust it again")
	}
	plugin = current
	if err := h.Trust.Require(current); err != nil {
		return result, err
	}
	snapshot, cleanupSnapshot, err := snapshotPlugin(current)
	if err != nil {
		return result, fmt.Errorf("snapshot trusted plugin: %w", err)
	}
	defer cleanupSnapshot()
	if snapshot.Digest != current.Digest {
		return result, errors.New("plugin changed while creating its execution snapshot; reload and trust it again")
	}
	plugin = snapshot
	if err := validateRequest(request); err != nil {
		return result, err
	}
	timeout := h.Timeout
	if request.Limits.TimeoutMS > 0 {
		requested := time.Duration(request.Limits.TimeoutMS) * time.Millisecond
		if timeout <= 0 || requested < timeout {
			timeout = requested
		}
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	requestPath, cleanup, err := writeRequest(plugin.Path, request)
	if err != nil {
		return result, err
	}
	defer cleanup()
	command := plugin.Manifest.Command
	program := command[0]
	if strings.ContainsRune(program, filepath.Separator) && !filepath.IsAbs(program) {
		program = filepath.Join(plugin.Path, program)
	}
	args := append([]string{}, command[1:]...)
	args = append(args, request.Operation, "--request", requestPath)
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = plugin.Path
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	configureProcess(cmd)
	stdout := &boundedBuffer{limit: positive(h.MaxStdoutBytes, 32<<20)}
	stderr := &boundedBuffer{limit: positive(h.MaxStderrBytes, 1<<20)}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	result.Stdout = stdout.Bytes()
	result.Stderr = stderr.String()
	if ctx.Err() != nil {
		return result, fmt.Errorf("adapter %s timed out after %s: %w", plugin.Manifest.Name, timeout, ctx.Err())
	}
	if stdout.exceeded {
		return result, fmt.Errorf("adapter stdout exceeded %d bytes", stdout.limit)
	}
	if stderr.exceeded {
		return result, fmt.Errorf("adapter stderr exceeded %d bytes", stderr.limit)
	}
	if err != nil {
		return result, fmt.Errorf("adapter %s %s failed: %w%s", plugin.Manifest.Name, request.Operation, err, diagnosticSuffix(result.Stderr))
	}
	return result, nil
}

func validateRequest(request Request) error {
	if request.APIVersion != APIVersion {
		return fmt.Errorf("request api_version must be %q", APIVersion)
	}
	if request.Operation != "probe" && request.Operation != "stream" {
		return fmt.Errorf("unsupported operation %q", request.Operation)
	}
	if !filepath.IsAbs(request.Source.Path) || !filepath.IsAbs(request.Source.Root) {
		return errors.New("request source path and root must be absolute")
	}
	path, err := filepath.EvalSymlinks(request.Source.Path)
	if err != nil {
		return fmt.Errorf("resolve request source: %w", err)
	}
	root, err := filepath.EvalSymlinks(request.Source.Root)
	if err != nil {
		return fmt.Errorf("resolve request root: %w", err)
	}
	if !within(root, path) {
		return errors.New("request source escapes registered root")
	}
	return nil
}

func writeRequest(_ string, request Request) (string, func(), error) {
	f, err := os.CreateTemp("", "rolloutviz-request-*.json")
	if err != nil {
		return "", func() {}, err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		cleanup()
		return "", func() {}, err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(request); err != nil {
		f.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

type boundedBuffer struct {
	bytes.Buffer
	limit    int64
	exceeded bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - int64(b.Len())
	if remaining <= 0 {
		b.exceeded = true
		return len(p), nil
	}
	write := p
	if int64(len(write)) > remaining {
		write = write[:remaining]
		b.exceeded = true
	}
	_, _ = b.Buffer.Write(write)
	return len(p), nil
}
func positive(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}
func diagnosticSuffix(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return ""
	}
	return ": " + stderr
}
func ensureEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("invalid trailing probe output: %w", err)
	}
	return errors.New("probe stdout must contain exactly one JSON object")
}
