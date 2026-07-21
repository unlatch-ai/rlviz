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

	"github.com/TheSnakeFang/rlviz/internal/analyzers"
	"github.com/TheSnakeFang/rlviz/internal/model"
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

type adapterProtocolError struct{ err error }

func (err *adapterProtocolError) Error() string { return err.err.Error() }
func (err *adapterProtocolError) Unwrap() error { return err.err }

func protocolError(err error) error {
	return &adapterProtocolError{err: err}
}

type preparedCommand struct {
	cmd     *exec.Cmd
	plugin  *Plugin
	ctx     context.Context
	cancel  context.CancelFunc
	cleanup func()
	timeout time.Duration
	stderr  *boundedBuffer
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

// Analyze executes a trusted external analyzer against one normalized
// trajectory. The process receives the v1alpha1 input in a private request
// file and must emit exactly one bounded v1alpha1 output object.
func (h *Host) Analyze(ctx context.Context, plugin *Plugin, input analyzers.Input) (analyzers.Output, string, error) {
	var output analyzers.Output
	raw, stderr, err := h.analyzeBytes(ctx, plugin, input)
	if err != nil {
		return output, stderr, err
	}
	output, err = decodeAnalyzerOutput(plugin, input, raw)
	return output, stderr, err
}

func decodeAnalyzerOutput(plugin *Plugin, input analyzers.Input, raw []byte) (analyzers.Output, error) {
	var output analyzers.Output
	input = analyzers.NormalizeInput(input)
	inputDigest, err := analyzers.InputDigest(input)
	if err != nil {
		return output, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return output, fmt.Errorf("invalid analyzer output: %w", err)
	}
	if err := ensureSingleJSON(decoder, "analyzer"); err != nil {
		return output, err
	}
	expected := analyzers.Provenance{Name: plugin.Manifest.Name, Version: plugin.Manifest.Version, Digest: "sha256:" + plugin.Digest, InputDigest: inputDigest}
	if err := analyzers.ValidateOutput(output, input, expected); err != nil {
		return output, fmt.Errorf("invalid analyzer output: %w", err)
	}
	return output, nil
}

func (h *Host) analyzeBytes(ctx context.Context, plugin *Plugin, input analyzers.Input) ([]byte, string, error) {
	if err := requirePluginKind(plugin, "Analyzer"); err != nil {
		return nil, "", err
	}
	input = analyzers.NormalizeInput(input)
	if err := analyzers.ValidateInput(input); err != nil {
		return nil, "", err
	}
	inputDigest, err := analyzers.InputDigest(input)
	if err != nil {
		return nil, "", err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, "", fmt.Errorf("encode analyzer input: %w", err)
	}
	if len(data) > analyzers.MaxInputBytes {
		return nil, "", fmt.Errorf("encoded analyzer input exceeds %d bytes", analyzers.MaxInputBytes)
	}
	prepared, err := h.prepareAnalyzer(ctx, plugin, data, inputDigest)
	if err != nil {
		return nil, "", err
	}
	defer prepared.cleanup()
	stdout := &boundedBuffer{limit: boundedLimit(h.MaxStdoutBytes, analyzers.MaxOutputBytes)}
	prepared.cmd.Stdout = stdout
	err = prepared.cmd.Run()
	stderr := prepared.stderr.String()
	if prepared.ctx.Err() != nil {
		return nil, stderr, processContextError("analyzer", prepared.plugin, prepared.timeout, prepared.ctx.Err())
	}
	if stdout.exceeded {
		return nil, stderr, fmt.Errorf("analyzer stdout exceeded %d bytes", stdout.limit)
	}
	if prepared.stderr.exceeded {
		return nil, stderr, fmt.Errorf("analyzer stderr exceeded %d bytes", prepared.stderr.limit)
	}
	if err != nil {
		return nil, stderr, fmt.Errorf("analyzer %s analyze failed: %w%s", prepared.plugin.Manifest.Name, err, diagnosticSuffix(stderr))
	}
	return append([]byte(nil), stdout.Bytes()...), stderr, nil
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
	if err := requirePluginKind(plugin, "Adapter"); err != nil {
		return ProbeResponse{}, "", err
	}
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
		return ProbeResponse{}, result.Stderr, protocolError(fmt.Errorf("invalid probe response: %w", err))
	}
	if err := ensureEOF(dec); err != nil {
		return ProbeResponse{}, result.Stderr, protocolError(err)
	}
	if wire.Supported == nil || wire.Confidence == nil {
		return ProbeResponse{}, result.Stderr, protocolError(errors.New("probe response requires supported and confidence"))
	}
	response := ProbeResponse{Supported: *wire.Supported, Confidence: *wire.Confidence, Format: wire.Format, Reason: wire.Reason}
	if math.IsNaN(response.Confidence) || math.IsInf(response.Confidence, 0) || response.Confidence < 0 || response.Confidence > 1 {
		return response, result.Stderr, protocolError(errors.New("probe confidence must be between 0 and 1"))
	}
	if response.Supported && response.Format == "" {
		return response, result.Stderr, protocolError(errors.New("supported probe response requires format"))
	}
	return response, result.Stderr, nil
}

func (h *Host) Stream(ctx context.Context, plugin *Plugin, request Request, visit func(*model.Record) error) (string, error) {
	if err := requirePluginKind(plugin, "Adapter"); err != nil {
		return "", err
	}
	request.Operation = "stream"
	request.APIVersion = APIVersion
	prepared, err := h.prepare(ctx, plugin, request)
	if err != nil {
		return "", err
	}
	defer prepared.cleanup()
	stdout, err := prepared.cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := prepared.cmd.Start(); err != nil {
		return "", fmt.Errorf("start adapter %s: %w", prepared.plugin.Manifest.Name, err)
	}
	limited := &boundedReader{reader: stdout, limit: positive(h.MaxStdoutBytes, 32<<20)}
	reader := io.Reader(limited)
	decodeErr := model.DecodeContext(prepared.ctx, reader, func(record *model.Record) error {
		if err := validateRecordProvenance(record, request.Source.Root); err != nil {
			return err
		}
		return visit(record)
	})
	if decodeErr != nil {
		prepared.cancel()
	}
	waitErr := prepared.cmd.Wait()
	stderr := prepared.stderr.String()
	if prepared.ctx.Err() != nil && (decodeErr == nil || errors.Is(decodeErr, context.Canceled) || errors.Is(decodeErr, context.DeadlineExceeded)) {
		return stderr, adapterContextError(prepared.plugin, prepared.timeout, prepared.ctx.Err())
	}
	if decodeErr != nil {
		return stderr, fmt.Errorf("invalid canonical stream: %w", decodeErr)
	}
	if limited.exceeded {
		return stderr, fmt.Errorf("adapter stdout exceeded %d bytes", limited.limit)
	}
	if prepared.stderr.exceeded {
		return stderr, fmt.Errorf("adapter stderr exceeded %d bytes", prepared.stderr.limit)
	}
	if waitErr != nil {
		return stderr, fmt.Errorf("adapter %s %s failed: %w%s", prepared.plugin.Manifest.Name, request.Operation, waitErr, diagnosticSuffix(stderr))
	}
	return stderr, nil
}

func requirePluginKind(plugin *Plugin, kind string) error {
	if plugin == nil {
		return errors.New("plugin is nil")
	}
	if plugin.Manifest.Kind != kind {
		return fmt.Errorf("plugin %q has kind %s, want %s", plugin.Manifest.Name, plugin.Manifest.Kind, kind)
	}
	return nil
}

func (h *Host) run(ctx context.Context, plugin *Plugin, request Request) (Result, error) {
	var result Result
	prepared, err := h.prepare(ctx, plugin, request)
	if err != nil {
		return result, err
	}
	defer prepared.cleanup()
	stdout := &boundedBuffer{limit: positive(h.MaxStdoutBytes, 32<<20)}
	prepared.cmd.Stdout = stdout
	err = prepared.cmd.Run()
	result.Stdout = stdout.Bytes()
	result.Stderr = prepared.stderr.String()
	if prepared.ctx.Err() != nil {
		return result, adapterContextError(prepared.plugin, prepared.timeout, prepared.ctx.Err())
	}
	if stdout.exceeded {
		return result, fmt.Errorf("adapter stdout exceeded %d bytes", stdout.limit)
	}
	if prepared.stderr.exceeded {
		return result, fmt.Errorf("adapter stderr exceeded %d bytes", prepared.stderr.limit)
	}
	if err != nil {
		return result, fmt.Errorf("adapter %s %s failed: %w%s", prepared.plugin.Manifest.Name, request.Operation, err, diagnosticSuffix(result.Stderr))
	}
	return result, nil
}

func (h *Host) prepare(ctx context.Context, plugin *Plugin, request Request) (*preparedCommand, error) {
	if plugin == nil {
		return nil, errors.New("plugin is nil")
	}
	if h.Trust == nil {
		return nil, errors.New("trust store is required")
	}
	current, err := Load(plugin.Path)
	if err != nil {
		return nil, err
	}
	if current.Digest != plugin.Digest {
		return nil, errors.New("plugin changed after it was loaded; reload and trust it again")
	}
	plugin = current
	if err := h.Trust.Require(current); err != nil {
		return nil, err
	}
	snapshot, cleanupSnapshot, err := snapshotPlugin(current)
	if err != nil {
		return nil, fmt.Errorf("snapshot trusted plugin: %w", err)
	}
	if snapshot.Digest != current.Digest {
		cleanupSnapshot()
		return nil, errors.New("plugin changed while creating its execution snapshot; reload and trust it again")
	}
	plugin = snapshot
	if err := validateRequest(request); err != nil {
		cleanupSnapshot()
		return nil, err
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
	requestPath, cleanup, err := writeRequest(plugin.Path, request)
	if err != nil {
		cancel()
		cleanupSnapshot()
		return nil, err
	}
	cleanupAll := func() {
		cancel()
		cleanup()
		cleanupSnapshot()
	}
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
	stderr := &boundedBuffer{limit: positive(h.MaxStderrBytes, 1<<20)}
	cmd.Stderr = stderr
	return &preparedCommand{cmd: cmd, plugin: plugin, ctx: ctx, cancel: cancel, cleanup: cleanupAll, timeout: timeout, stderr: stderr}, nil
}

func (h *Host) prepareAnalyzer(ctx context.Context, plugin *Plugin, request []byte, inputDigest string) (*preparedCommand, error) {
	if plugin == nil {
		return nil, errors.New("plugin is nil")
	}
	if h.Trust == nil {
		return nil, errors.New("trust store is required")
	}
	current, err := Load(plugin.Path)
	if err != nil {
		return nil, err
	}
	if current.Digest != plugin.Digest {
		return nil, errors.New("plugin changed after it was loaded; reload and trust it again")
	}
	if err := h.Trust.Require(current); err != nil {
		return nil, err
	}
	snapshot, cleanupSnapshot, err := snapshotPlugin(current)
	if err != nil {
		return nil, fmt.Errorf("snapshot trusted plugin: %w", err)
	}
	if snapshot.Digest != current.Digest {
		cleanupSnapshot()
		return nil, errors.New("plugin changed while creating its execution snapshot; reload and trust it again")
	}
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	processCtx, cancel := context.WithTimeout(ctx, timeout)
	requestPath, cleanupRequest, err := writePayload(request)
	if err != nil {
		cancel()
		cleanupSnapshot()
		return nil, err
	}
	cleanup := func() {
		cancel()
		cleanupRequest()
		cleanupSnapshot()
	}
	command := snapshot.Manifest.Command
	program := command[0]
	if strings.ContainsRune(program, filepath.Separator) && !filepath.IsAbs(program) {
		program = filepath.Join(snapshot.Path, program)
	}
	args := append([]string{}, command[1:]...)
	args = append(args, analyzers.OperationAnalyze, "--request", requestPath)
	cmd := exec.CommandContext(processCtx, program, args...)
	cmd.Dir = snapshot.Path
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(),
		"PYTHONDONTWRITEBYTECODE=1",
		"RLVIZ_ANALYZER_NAME="+snapshot.Manifest.Name,
		"RLVIZ_ANALYZER_VERSION="+snapshot.Manifest.Version,
		"RLVIZ_ANALYZER_DIGEST=sha256:"+snapshot.Digest,
		"RLVIZ_ANALYZER_INPUT_DIGEST="+inputDigest,
	)
	configureProcess(cmd)
	stderr := &boundedBuffer{limit: boundedLimit(h.MaxStderrBytes, 1<<20)}
	cmd.Stderr = stderr
	return &preparedCommand{cmd: cmd, plugin: snapshot, ctx: processCtx, cancel: cancel, cleanup: cleanup, timeout: timeout, stderr: stderr}, nil
}

func adapterContextError(plugin *Plugin, timeout time.Duration, err error) error {
	return processContextError("adapter", plugin, timeout, err)
}

func processContextError(kind string, plugin *Plugin, timeout time.Duration, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s %s timed out after %s: %w", kind, plugin.Manifest.Name, timeout, err)
	}
	return fmt.Errorf("%s %s canceled: %w", kind, plugin.Manifest.Name, err)
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
	var data bytes.Buffer
	enc := json.NewEncoder(&data)
	enc.SetEscapeHTML(false)
	err := enc.Encode(request)
	if err != nil {
		return "", func() {}, err
	}
	return writePayload(data.Bytes())
}

func writePayload(data []byte) (string, func(), error) {
	f, err := os.CreateTemp("", "rlviz-request-*.json")
	if err != nil {
		return "", func() {}, err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
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
	buffer   bytes.Buffer
	limit    int64
	exceeded bool
}

type boundedReader struct {
	reader   io.Reader
	limit    int64
	read     int64
	exceeded bool
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if r.read < r.limit {
		remaining := r.limit - r.read
		if int64(len(p)) > remaining {
			p = p[:remaining]
		}
		n, err := r.reader.Read(p)
		r.read += int64(n)
		return n, err
	}
	var extra [1]byte
	n, err := r.reader.Read(extra[:])
	if n > 0 {
		r.exceeded = true
		return 0, fmt.Errorf("adapter stdout exceeded %d bytes", r.limit)
	}
	return 0, err
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - int64(b.buffer.Len())
	if remaining <= 0 {
		b.exceeded = true
		return len(p), nil
	}
	write := p
	if int64(len(write)) > remaining {
		write = write[:remaining]
		b.exceeded = true
	}
	_, _ = b.buffer.Write(write)
	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte  { return b.buffer.Bytes() }
func (b *boundedBuffer) String() string { return b.buffer.String() }
func positive(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}
func boundedLimit(configured, protocol int64) int64 {
	if configured <= 0 || configured > protocol {
		return protocol
	}
	return configured
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

func ensureSingleJSON(dec *json.Decoder, kind string) error {
	var extra any
	if err := dec.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("invalid trailing %s output: %w", kind, err)
	}
	return fmt.Errorf("%s stdout must contain exactly one JSON object", kind)
}
