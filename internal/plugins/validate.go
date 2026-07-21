package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheSnakeFang/rlviz/internal/analyzers"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

type ValidationReport struct {
	Plugin        string `json:"plugin"`
	Digest        string `json:"digest"`
	Format        string `json:"format"`
	Records       int    `json:"records"`
	Warnings      int64  `json:"warnings"`
	Deterministic bool   `json:"deterministic"`
}

type AnalyzerValidationReport struct {
	Plugin        string `json:"plugin"`
	Digest        string `json:"digest"`
	Findings      int    `json:"findings"`
	Signals       int    `json:"signals"`
	Deterministic bool   `json:"deterministic"`
}

const (
	ValidationPhaseSource = "source"
	ValidationPhaseProbe  = "probe"
	ValidationPhaseStream = "stream"

	ValidationKindExecution        = "execution"
	ValidationKindProtocol         = "protocol"
	ValidationKindProvenance       = "provenance"
	ValidationKindUnsupported      = "unsupported"
	ValidationKindNondeterministic = "nondeterministic"
)

// AdapterValidationError identifies the stable validation phase and, when the
// canonical decoder reached a record, its source location and identity.
type AdapterValidationError struct {
	Phase      string
	Kind       string
	Pass       int
	Line       int64
	RecordType model.RecordType
	RecordID   string
	Field      string
	Err        error
}

func (err *AdapterValidationError) Error() string { return err.Err.Error() }
func (err *AdapterValidationError) Unwrap() error { return err.Err }
func (err *AdapterValidationError) DiagnosticFields() map[string]any {
	fields := map[string]any{"phase": err.Phase, "kind": err.Kind}
	if err.Pass != 0 {
		fields["pass"] = err.Pass
	}
	if err.Line != 0 {
		fields["line"] = err.Line
	}
	if err.RecordType != "" {
		fields["record_type"] = err.RecordType
	}
	if err.RecordID != "" {
		fields["record_id"] = err.RecordID
	}
	if err.Field != "" {
		fields["field"] = err.Field
	}
	return fields
}

func adapterValidationError(phase, kind string, pass int, err error) error {
	if err == nil {
		return nil
	}
	failure := &AdapterValidationError{Phase: phase, Kind: kind, Pass: pass, Err: err}
	var recordError *model.RecordValidationError
	if errors.As(err, &recordError) {
		failure.Line = recordError.Line
		failure.RecordType = recordError.RecordType
		failure.RecordID = recordError.RecordID
		failure.Field = recordError.Field
		if strings.HasPrefix(recordError.Field, "source") {
			failure.Kind = ValidationKindProvenance
		}
	}
	return failure
}

func probeValidationKind(err error) string {
	var protocol *adapterProtocolError
	if errors.As(err, &protocol) {
		return ValidationKindProtocol
	}
	return ValidationKindExecution
}

// LoadAnalyzerInput reads one strict, bounded analyzer v1alpha1 request.
func LoadAnalyzerInput(path string) (analyzers.Input, error) {
	var input analyzers.Input
	file, err := os.Open(path)
	if err != nil {
		return input, err
	}
	defer file.Close()
	reader := io.LimitReader(file, analyzers.MaxInputBytes+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return input, err
	}
	if len(data) > analyzers.MaxInputBytes {
		return input, fmt.Errorf("analyzer input exceeds %d bytes", analyzers.MaxInputBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, fmt.Errorf("invalid analyzer input: %w", err)
	}
	if err := ensureSingleJSON(decoder, "analyzer input"); err != nil {
		return input, err
	}
	input = analyzers.NormalizeInput(input)
	if err := analyzers.ValidateInput(input); err != nil {
		return input, err
	}
	return input, nil
}

// ValidateAnalyzer executes the trusted analyzer twice and requires byte-level
// deterministic protocol output in addition to semantic validation.
func (h *Host) ValidateAnalyzer(ctx context.Context, plugin *Plugin, input analyzers.Input) (AnalyzerValidationReport, error) {
	report := AnalyzerValidationReport{}
	if plugin != nil {
		report.Plugin, report.Digest = plugin.Manifest.Name, plugin.Digest
	}
	first, _, err := h.analyzeBytes(ctx, plugin, input)
	if err != nil {
		return report, fmt.Errorf("analyze pass 1: %w", err)
	}
	second, _, err := h.analyzeBytes(ctx, plugin, input)
	if err != nil {
		return report, fmt.Errorf("analyze pass 2: %w", err)
	}
	if !bytes.Equal(first, second) {
		return report, errors.New("analyzer is nondeterministic: repeated output differs")
	}
	output, err := decodeAnalyzerOutput(plugin, input, first)
	if err != nil {
		return report, fmt.Errorf("validate analyzer output: %w", err)
	}
	report.Findings, report.Signals, report.Deterministic = len(output.Findings), len(output.Signals), true
	return report, nil
}

// ValidateAdapter probes and streams the same source twice. Exact stdout
// equality enforces stable IDs, ordering, payloads, and completion counts.
func (h *Host) ValidateAdapter(ctx context.Context, plugin *Plugin, sourcePath, root string) (ValidationReport, error) {
	report := ValidationReport{Deterministic: false}
	if plugin != nil {
		report.Plugin = plugin.Manifest.Name
		report.Digest = plugin.Digest
	}
	probeReq, err := NewRequest("probe", sourcePath, root)
	if err != nil {
		return report, adapterValidationError(ValidationPhaseSource, ValidationKindProvenance, 0, err)
	}
	p1, _, err := h.Probe(ctx, plugin, probeReq)
	if err != nil {
		return report, adapterValidationError(ValidationPhaseProbe, probeValidationKind(err), 1, fmt.Errorf("probe pass 1: %w", err))
	}
	p2, _, err := h.Probe(ctx, plugin, probeReq)
	if err != nil {
		return report, adapterValidationError(ValidationPhaseProbe, probeValidationKind(err), 2, fmt.Errorf("probe pass 2: %w", err))
	}
	if p1 != p2 {
		return report, adapterValidationError(ValidationPhaseProbe, ValidationKindNondeterministic, 0, fmt.Errorf("probe is nondeterministic: first=%+v second=%+v", p1, p2))
	}
	if !p1.Supported {
		return report, adapterValidationError(ValidationPhaseProbe, ValidationKindUnsupported, 0, fmt.Errorf("adapter does not support source: %s", p1.Reason))
	}
	report.Format = p1.Format
	streamReq, err := NewRequest("stream", sourcePath, root)
	if err != nil {
		return report, adapterValidationError(ValidationPhaseSource, ValidationKindProvenance, 0, err)
	}
	first, err := h.run(ctx, plugin, streamReq)
	if err != nil {
		return report, adapterValidationError(ValidationPhaseStream, ValidationKindExecution, 1, fmt.Errorf("stream pass 1: %w", err))
	}
	firstReport := report
	if err := decodeReport(first.Stdout, streamReq.Source.Root, &firstReport); err != nil {
		return report, adapterValidationError(ValidationPhaseStream, ValidationKindProtocol, 1, err)
	}
	second, err := h.run(ctx, plugin, streamReq)
	if err != nil {
		return report, adapterValidationError(ValidationPhaseStream, ValidationKindExecution, 2, fmt.Errorf("stream pass 2: %w", err))
	}
	secondReport := report
	if err := decodeReport(second.Stdout, streamReq.Source.Root, &secondReport); err != nil {
		return report, adapterValidationError(ValidationPhaseStream, ValidationKindProtocol, 2, err)
	}
	if !bytes.Equal(first.Stdout, second.Stdout) {
		return report, adapterValidationError(ValidationPhaseStream, ValidationKindNondeterministic, 0, errors.New("stream is nondeterministic: repeated output differs"))
	}
	report.Records, report.Warnings = firstReport.Records, firstReport.Warnings
	report.Deterministic = true
	return report, nil
}

func decodeReport(data []byte, root string, report *ValidationReport) error {
	return model.Decode(bytes.NewReader(data), func(record *model.Record) error {
		if err := validateRecordProvenance(record, root); err != nil {
			field := "source"
			var fieldError *model.FieldValidationError
			if errors.As(err, &fieldError) {
				field = fieldError.Field
			}
			return &model.RecordValidationError{Line: record.Line, RecordType: record.Type, RecordID: model.RecordID(record), Field: field, Err: err}
		}
		if record.Type == model.RecordComplete {
			report.Warnings = record.Value.(*model.Complete).Warnings
		} else {
			report.Records++
		}
		return nil
	})
}

func validateRecordProvenance(record *model.Record, root string) error {
	event, ok := record.Value.(*model.Event)
	if !ok || event.Source == nil {
		return nil
	}
	path := event.Source.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return &model.FieldValidationError{Field: "source.path", Err: fmt.Errorf("event %q source path: %w", event.ID, err)}
	}
	if !within(root, resolved) {
		return &model.FieldValidationError{Field: "source.path", Err: fmt.Errorf("event %q source path escapes registered root", event.ID)}
	}
	if event.Source.ByteOffset == nil {
		return nil
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return &model.FieldValidationError{Field: "source.path", Err: err}
	}
	end := *event.Source.ByteOffset
	if event.Source.ByteLength != nil {
		length := *event.Source.ByteLength
		if length > info.Size()-end {
			return &model.FieldValidationError{Field: "source.byte_length", Err: fmt.Errorf("event %q source byte range exceeds file size %d", event.ID, info.Size())}
		}
		end += length
	}
	if end > info.Size() {
		return &model.FieldValidationError{Field: "source.byte_offset", Err: fmt.Errorf("event %q source byte range ends at %d past file size %d", event.ID, end, info.Size())}
	}
	return nil
}
