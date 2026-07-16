package plugins

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/unlatch-ai/rolloutviz/internal/model"
)

type ValidationReport struct {
	Plugin        string `json:"plugin"`
	Digest        string `json:"digest"`
	Format        string `json:"format"`
	Records       int    `json:"records"`
	Warnings      int64  `json:"warnings"`
	Deterministic bool   `json:"deterministic"`
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
		return report, err
	}
	p1, _, err := h.Probe(ctx, plugin, probeReq)
	if err != nil {
		return report, fmt.Errorf("probe pass 1: %w", err)
	}
	p2, _, err := h.Probe(ctx, plugin, probeReq)
	if err != nil {
		return report, fmt.Errorf("probe pass 2: %w", err)
	}
	if p1 != p2 {
		return report, fmt.Errorf("probe is nondeterministic: first=%+v second=%+v", p1, p2)
	}
	if !p1.Supported {
		return report, fmt.Errorf("adapter does not support source: %s", p1.Reason)
	}
	report.Format = p1.Format
	streamReq, err := NewRequest("stream", sourcePath, root)
	if err != nil {
		return report, err
	}
	first, err := h.run(ctx, plugin, streamReq)
	if err != nil {
		return report, fmt.Errorf("stream pass 1: %w", err)
	}
	second, err := h.run(ctx, plugin, streamReq)
	if err != nil {
		return report, fmt.Errorf("stream pass 2: %w", err)
	}
	if !bytes.Equal(first.Stdout, second.Stdout) {
		return report, fmt.Errorf("stream is nondeterministic: repeated output differs")
	}
	err = decodeReport(first.Stdout, streamReq.Source.Root, &report)
	if err != nil {
		return report, err
	}
	report.Deterministic = true
	return report, nil
}

func decodeReport(data []byte, root string, report *ValidationReport) error {
	return model.Decode(bytes.NewReader(data), func(record *model.Record) error {
		if err := validateRecordProvenance(record, root); err != nil {
			return err
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
		return fmt.Errorf("event %q source path: %w", event.ID, err)
	}
	if !within(root, resolved) {
		return fmt.Errorf("event %q source path escapes registered root", event.ID)
	}
	if event.Source.ByteOffset == nil {
		return nil
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	end := *event.Source.ByteOffset
	if event.Source.ByteLength != nil {
		length := *event.Source.ByteLength
		if length > info.Size()-end {
			return fmt.Errorf("event %q source byte range exceeds file size %d", event.ID, info.Size())
		}
		end += length
	}
	if end > info.Size() {
		return fmt.Errorf("event %q source byte range ends at %d past file size %d", event.ID, end, info.Size())
	}
	return nil
}
