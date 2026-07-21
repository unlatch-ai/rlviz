package index

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

func TestProgressiveFilePublishesBatchesWhileGrowing(t *testing.T) {
	idx := openTestIndex(t)
	path := filepath.Join(t.TempDir(), "growing.ndjson")
	writeProgressiveRecords(t, path, progressiveRecords(130, false), false)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	source := Source{ID: "growing", Path: path, Fingerprint: "canonical", Size: info.Size(), ModTime: info.ModTime()}
	ready := make(chan SourceInfo, 1)
	done := make(chan struct {
		info SourceInfo
		err  error
	}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		result, runErr := idx.ProgressiveFile(ctx, source, ProgressiveOptions{BatchRecords: 32, PollInterval: 5 * time.Millisecond, Ready: func(info SourceInfo) { ready <- info }})
		done <- struct {
			info SourceInfo
			err  error
		}{result, runErr}
	}()

	select {
	case first := <-ready:
		if first.IndexState != Indexing || first.CompleteRaw != nil {
			t.Fatalf("ready source = %#v", first)
		}
		if first.Records > 32 {
			t.Fatalf("initial batch exceeded bound: %d records", first.Records)
		}
		status, statusErr := idx.Status(t.Context(), source)
		if statusErr != nil || status.State != CacheStale {
			t.Fatalf("indexing status=%#v err=%v", status, statusErr)
		}
		page, queryErr := idx.Events(t.Context(), EventQuery{SourceID: source.ID, TrajectoryID: "trajectory-progress", Limit: 1000})
		if queryErr != nil || page.Total == 0 || page.Total > 130 {
			t.Fatalf("initial page total=%d err=%v", page.Total, queryErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("initial event batch was not published")
	}

	appendProgressiveRecords(t, path, progressiveEvents(130, 130))
	eventually(t, 3*time.Second, func() bool {
		page, queryErr := idx.Events(t.Context(), EventQuery{SourceID: source.ID, TrajectoryID: "trajectory-progress", Limit: 1000})
		return queryErr == nil && page.Total == 260
	})
	appendProgressiveRecords(t, path, []any{model.Complete{RecordType: model.RecordComplete, Records: 264, Warnings: 0}})
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.info.IndexState != IndexComplete || result.info.Records != 264 || len(result.info.CompleteRaw) == 0 {
			t.Fatalf("completed source = %#v", result.info)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("growing index did not complete")
	}
}

func TestProgressiveFileMarksInvalidTailFailed(t *testing.T) {
	idx := openTestIndex(t)
	path := filepath.Join(t.TempDir(), "invalid.ndjson")
	writeProgressiveRecords(t, path, progressiveRecords(2, false), false)
	info, _ := os.Stat(path)
	ready := make(chan SourceInfo, 1)
	done := make(chan error, 1)
	go func() {
		_, err := idx.ProgressiveFile(t.Context(), Source{ID: "invalid", Path: path, Size: info.Size(), ModTime: info.ModTime()}, ProgressiveOptions{PollInterval: 5 * time.Millisecond, Ready: func(info SourceInfo) { ready <- info }})
		done <- err
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("not ready")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.WriteString("{broken json}\n")
	_ = file.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("invalid tail succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("invalid tail was not rejected")
	}
	cached, err := idx.Source(t.Context(), "invalid")
	if err != nil || cached.IndexState != IndexFailed || cached.IndexError == "" {
		t.Fatalf("failed source=%#v err=%v", cached, err)
	}
}

func TestProgressiveFileTenThousandEvents(t *testing.T) {
	idx := openTestIndex(t)
	path := filepath.Join(t.TempDir(), "large.ndjson")
	writeProgressiveRecords(t, path, progressiveRecords(10_000, true), false)
	info, _ := os.Stat(path)
	ready := make(chan SourceInfo, 1)
	started := time.Now()
	result, err := idx.ProgressiveFile(t.Context(), Source{ID: "large-progressive", Path: path, Size: info.Size(), ModTime: info.ModTime()}, ProgressiveOptions{BatchRecords: 128, Ready: func(info SourceInfo) {
		select {
		case ready <- info:
		default:
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("10k indexing took %s", time.Since(started))
	if result.IndexState != IndexComplete || result.Records != 10_004 {
		t.Fatalf("result = %#v", result)
	}
	select {
	case first := <-ready:
		if first.Records <= 0 {
			t.Fatalf("ready info = %#v", first)
		}
	default:
		t.Fatal("ready callback was not invoked")
	}
	page, err := idx.Events(t.Context(), EventQuery{SourceID: result.ID, TrajectoryID: "trajectory-progress", Limit: 1000})
	if err != nil || page.Total != 10_000 {
		t.Fatalf("event total=%d err=%v", page.Total, err)
	}
}

func TestProgressiveFileDetectsRegeneration(t *testing.T) {
	idx := openTestIndex(t)
	path := filepath.Join(t.TempDir(), "regenerate.ndjson")
	writeProgressiveRecords(t, path, progressiveRecords(3, false), false)
	info, _ := os.Stat(path)
	ready := make(chan SourceInfo, 1)
	done := make(chan error, 1)
	go func() {
		_, err := idx.ProgressiveFile(t.Context(), Source{ID: "regenerate", Path: path, Size: info.Size(), ModTime: info.ModTime()}, ProgressiveOptions{PollInterval: 5 * time.Millisecond, Ready: func(info SourceInfo) { ready <- info }})
		done <- err
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("not ready")
	}
	writeProgressiveRecords(t, path, progressiveRecords(1, true), false)
	select {
	case err := <-done:
		if !errors.Is(err, ErrSourceChanged) {
			t.Fatalf("error=%v, want ErrSourceChanged", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("regeneration was not detected")
	}
}

func TestProgressiveFileCancellationMarksIncompleteState(t *testing.T) {
	idx := openTestIndex(t)
	path := filepath.Join(t.TempDir(), "canceled.ndjson")
	writeProgressiveRecords(t, path, progressiveRecords(1, false), false)
	info, _ := os.Stat(path)
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan SourceInfo, 1)
	done := make(chan error, 1)
	go func() {
		_, err := idx.ProgressiveFile(ctx, Source{ID: "canceled", Path: path, Size: info.Size(), ModTime: info.ModTime()}, ProgressiveOptions{PollInterval: 5 * time.Millisecond, Ready: func(info SourceInfo) { ready <- info }})
		done <- err
	}()
	select {
	case <-ready:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("not ready")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("indexing did not cancel")
	}
	cached, err := idx.Source(t.Context(), "canceled")
	if err != nil || cached.IndexState != IndexFailed || cached.IndexError != "indexing canceled" {
		t.Fatalf("canceled source=%#v err=%v", cached, err)
	}
}

func progressiveRecords(events int, complete bool) []any {
	records := []any{
		model.Run{RecordType: model.RecordRun, ID: "run-progress"},
		model.Case{RecordType: model.RecordCase, ID: "case-progress", RunID: "run-progress"},
		model.Group{RecordType: model.RecordGroup, ID: "group-progress", CaseID: "case-progress"},
		model.Trajectory{RecordType: model.RecordTrajectory, ID: "trajectory-progress", GroupID: "group-progress"},
	}
	records = append(records, progressiveEvents(0, events)...)
	if complete {
		records = append(records, model.Complete{RecordType: model.RecordComplete, Records: int64(len(records)), Warnings: 0})
	}
	return records
}

func progressiveEvents(start, count int) []any {
	records := make([]any, 0, count)
	for n := start; n < start+count; n++ {
		records = append(records, model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("event-%d", n), TrajectoryID: "trajectory-progress", Sequence: int64(n), Kind: "message", Data: map[string]any{"text": fmt.Sprintf("event %d", n)}})
	}
	return records
}

func writeProgressiveRecords(t *testing.T, path string, records []any, appendMode bool) {
	t.Helper()
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendMode {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data.Bytes()); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func appendProgressiveRecords(t *testing.T, path string, records []any) {
	writeProgressiveRecords(t, path, records, true)
}

func eventually(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met")
}
