package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/unlatch-ai/rlviz/internal/app"
	"github.com/unlatch-ai/rlviz/internal/model"
	"github.com/unlatch-ai/rlviz/internal/plugins"
)

const (
	inspectProbeBytes   int64 = 1 << 20
	inspectProbeRecords       = 64
)

type inspectShape struct {
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes"`
}

type inspectAdapter struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`
}

type inspectResult struct {
	Path        string          `json:"path"`
	Shape       inspectShape    `json:"shape"`
	Supported   bool            `json:"supported"`
	Format      string          `json:"format"`
	Adapter     *inspectAdapter `json:"adapter"`
	Confidence  float64         `json:"confidence"`
	Reason      string          `json:"reason"`
	Warnings    []string        `json:"warnings"`
	NextCommand string          `json:"next_command"`
}

func runInspect(arguments []string) {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	adapter := flags.String("adapter", "", "trusted adapter plugin path")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: rlviz inspect [--json] [--adapter PATH] SOURCE")
	}
	if err := flags.Parse(normalizeViewerArguments(arguments)); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 1 {
		flags.Usage()
		os.Exit(2)
	}

	store, err := plugins.DefaultTrustStore()
	if err != nil {
		fatalError("inspect", *jsonOutput, err)
	}
	result, err := inspectSource(context.Background(), flags.Arg(0), *adapter, store)
	if err != nil {
		fatalError("inspect", *jsonOutput, err)
	}
	writeOutput(result, *jsonOutput, formatInspectText(result))
}

func inspectSource(ctx context.Context, sourcePath, adapterPath string, trust *plugins.TrustStore) (inspectResult, error) {
	request, err := plugins.NewRequest("probe", sourcePath, "")
	if err != nil {
		return inspectResult{}, err
	}
	result := inspectResult{
		Path:     request.Source.Path,
		Shape:    inspectShape{Kind: request.Source.Kind, SizeBytes: request.Source.SizeBytes},
		Warnings: []string{},
	}
	if adapterPath == "" {
		return inspectCanonical(result)
	}

	plugin, err := plugins.Load(adapterPath)
	if err != nil {
		return inspectResult{}, fmt.Errorf("load adapter: %w", err)
	}
	result.Adapter = &inspectAdapter{Kind: "plugin", Name: plugin.Manifest.Name, Version: plugin.Manifest.Version, Path: plugin.Path}
	request.Limits.ProbeBytes = inspectProbeBytes
	probe, diagnostics, err := plugins.NewHost(trust).Probe(ctx, plugin, request)
	if err != nil {
		if errors.Is(err, plugins.ErrUntrusted) {
			return inspectResult{}, fmt.Errorf("%w; trust it with %s", err, shellCommand("rlviz", "plugin", "trust", plugin.Path))
		}
		return inspectResult{}, err
	}
	if strings.TrimSpace(diagnostics) != "" {
		result.Warnings = append(result.Warnings, "adapter diagnostics: "+strings.TrimSpace(diagnostics))
	}
	result.Supported = probe.Supported
	result.Format = probe.Format
	result.Confidence = probe.Confidence
	result.Reason = probe.Reason
	if result.Reason == "" {
		if result.Supported {
			result.Reason = "trusted adapter recognized the source"
		} else {
			result.Reason = "trusted adapter did not recognize the source"
		}
	}
	if result.Supported {
		result.NextCommand = shellCommand("rlviz", "open", "--adapter", plugin.Path, result.Path)
	} else {
		result.NextCommand = shellCommand("rlviz", "plugin", "validate", plugin.Path, result.Path)
	}
	return result, nil
}

func inspectCanonical(result inspectResult) (inspectResult, error) {
	result.Adapter = &inspectAdapter{Kind: "built_in", Name: "canonical-ndjson"}
	if result.Shape.Kind != "file" {
		return unsupportedCanonical(result, "the built-in canonical format requires a regular NDJSON file"), nil
	}
	file, err := os.Open(result.Path)
	if err != nil {
		return inspectResult{}, err
	}
	defer file.Close()

	reader := bufio.NewReader(io.LimitReader(file, inspectProbeBytes))
	records := 0
	hasTrajectory := false
	hasComplete := false
	complete := false
	validator := model.NewValidator()
	for records < inspectProbeRecords {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return inspectResult{}, fmt.Errorf("probe canonical source: %w", readErr)
		}
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			complete = result.Shape.SizeBytes <= inspectProbeBytes
			break
		}
		truncated := errors.Is(readErr, io.EOF) && result.Shape.SizeBytes > inspectProbeBytes
		if truncated {
			result.Warnings = append(result.Warnings, fmt.Sprintf("probe was limited to the first %d bytes", inspectProbeBytes))
			break
		}
		record, decodeErr := decodeProbeLine(line)
		if decodeErr != nil {
			return unsupportedCanonical(result, decodeErr.Error()), nil
		}
		if validateErr := validator.Add(record); validateErr != nil {
			return unsupportedCanonical(result, fmt.Sprintf("line %d: %v", records+1, validateErr)), nil
		}
		records++
		if record.Type == model.RecordTrajectory {
			hasTrajectory = true
		}
		if record.Type == model.RecordComplete {
			hasComplete = true
		}
		if errors.Is(readErr, io.EOF) {
			complete = true
			break
		}
	}
	if records == inspectProbeRecords && !complete {
		_, peekErr := reader.Peek(1)
		if errors.Is(peekErr, io.EOF) && result.Shape.SizeBytes <= inspectProbeBytes {
			complete = true
		} else {
			result.Warnings = append(result.Warnings, fmt.Sprintf("probe was limited to the first %d records", inspectProbeRecords))
		}
	}
	if records == 0 {
		return unsupportedCanonical(result, "source is empty"), nil
	}
	if !complete && hasComplete {
		return unsupportedCanonical(result, "canonical complete record is followed by data outside the probe boundary"), nil
	}
	if complete {
		if finishErr := validator.Finish(); finishErr != nil {
			return unsupportedCanonical(result, finishErr.Error()), nil
		}
	}
	if complete && !hasTrajectory {
		return unsupportedCanonical(result, "canonical source contains no trajectory record"), nil
	}

	result.Supported = true
	result.Format = "canonical-ndjson"
	result.Confidence = 0.9
	result.Reason = fmt.Sprintf("first %d record(s) are valid canonical NDJSON", records)
	if complete && hasTrajectory {
		result.Confidence = 1
		result.Reason = fmt.Sprintf("all %d record(s) are valid canonical NDJSON and include a trajectory", records)
	} else if hasTrajectory {
		result.Confidence = 0.99
		result.Reason = fmt.Sprintf("bounded probe found %d valid canonical record(s), including a trajectory", records)
	}
	result.NextCommand = shellCommand("rlviz", "open", result.Path)
	return result, nil
}

func decodeProbeLine(line []byte) (*model.Record, error) {
	decoder := model.NewDecoder(bytes.NewReader(line))
	record, err := decoder.Next()
	if err != nil {
		return nil, err
	}
	if _, err = decoder.Next(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("probe line contains multiple records")
		}
		return nil, err
	}
	return record, nil
}

func unsupportedCanonical(result inspectResult, reason string) inspectResult {
	result.Supported = false
	result.Format = ""
	result.Confidence = 0
	result.Reason = reason
	result.NextCommand = app.AdapterScaffoldCommand(result.Path)
	return result
}

func formatInspectText(result inspectResult) string {
	status := "unsupported"
	if result.Supported {
		status = fmt.Sprintf("%s (confidence %.2f)", result.Format, result.Confidence)
	}
	lines := []string{
		fmt.Sprintf("Source: %s", result.Path),
		fmt.Sprintf("Shape:  %s, %d bytes", result.Shape.Kind, result.Shape.SizeBytes),
		fmt.Sprintf("Result: %s", status),
		fmt.Sprintf("Reason: %s", result.Reason),
	}
	for _, warning := range result.Warnings {
		lines = append(lines, "Warning: "+warning)
	}
	lines = append(lines, "Next:   "+result.NextCommand)
	return strings.Join(lines, "\n")
}

func shellCommand(arguments ...string) string {
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		if argument != "" && strings.IndexFunc(argument, func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && !strings.ContainsRune("_@%+=:,./-", r)
		}) == -1 {
			quoted[index] = argument
		} else {
			quoted[index] = "'" + strings.ReplaceAll(argument, "'", "'\"'\"'") + "'"
		}
	}
	return strings.Join(quoted, " ")
}
