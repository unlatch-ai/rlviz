package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

func TestSourceIndexerReturnsInitialBatchAndSerializesDuplicates(t *testing.T) {
	store := openAppIndex(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	indexer := NewSourceIndexer(ctx, store)
	path := filepath.Join(t.TempDir(), "growing.ndjson")
	writeAppCanonical(t, path, appCanonicalRecords(3, false))

	var wg sync.WaitGroup
	results := make(chan IndexedSource, 2)
	errors := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := indexer.Index(context.Background(), path, "")
			results <- result
			errors <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	var sourceID string
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for result := range results {
		if result.Info.IndexState != rolloutindex.Indexing || !result.Refreshed {
			t.Fatalf("initial result = %#v", result)
		}
		if sourceID != "" && result.Info.ID != sourceID {
			t.Fatal("duplicate calls returned different sources")
		}
		sourceID = result.Info.ID
	}
	page, err := store.Events(t.Context(), rolloutindex.EventQuery{SourceID: sourceID, TrajectoryID: "trajectory-app", Limit: 100})
	if err != nil || page.Total != 3 {
		t.Fatalf("initial events=%d err=%v", page.Total, err)
	}

	appendAppCanonical(t, path, []any{model.Event{RecordType: model.RecordEvent, ID: "event-3", TrajectoryID: "trajectory-app", Sequence: 3, Kind: "message"}})
	appEventually(t, func() bool {
		page, queryErr := store.Events(t.Context(), rolloutindex.EventQuery{SourceID: sourceID, TrajectoryID: "trajectory-app", Limit: 100})
		return queryErr == nil && page.Total == 4
	})
	appendAppCanonical(t, path, []any{model.Complete{RecordType: model.RecordComplete, Records: 8, Warnings: 0}})
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer waitCancel()
	if err := indexer.waitIdle(waitCtx, sourceID); err != nil {
		t.Fatal(err)
	}
	info, err := store.Source(t.Context(), sourceID)
	if err != nil || info.IndexState != rolloutindex.IndexComplete || info.Records != 8 {
		t.Fatalf("completed info=%#v err=%v", info, err)
	}
}

func TestSourceIndexerRestartsAfterRegeneration(t *testing.T) {
	store := openAppIndex(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	indexer := NewSourceIndexer(ctx, store)
	path := filepath.Join(t.TempDir(), "regenerated.ndjson")
	writeAppCanonical(t, path, appCanonicalRecords(2, false))
	initial, err := indexer.Index(t.Context(), path, "")
	if err != nil {
		t.Fatal(err)
	}
	writeAppCanonical(t, path, appCanonicalRecords(5, true))
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer waitCancel()
	if err := indexer.waitIdle(waitCtx, initial.Info.ID); err != nil {
		t.Fatal(err)
	}
	info, err := store.Source(t.Context(), initial.Info.ID)
	if err != nil || info.IndexState != rolloutindex.IndexComplete {
		t.Fatalf("regenerated info=%#v err=%v", info, err)
	}
	page, err := store.Events(t.Context(), rolloutindex.EventQuery{SourceID: info.ID, TrajectoryID: "trajectory-app", Limit: 100})
	if err != nil || page.Total != 5 {
		t.Fatalf("regenerated events=%d err=%v", page.Total, err)
	}
}

func TestSourceIndexerInvalidRefreshPreservesValidCache(t *testing.T) {
	store := openAppIndex(t)
	path := filepath.Join(t.TempDir(), "refresh.ndjson")
	writeAppCanonical(t, path, appCanonicalRecords(3, true))
	valid, err := IndexSource(t.Context(), store, path, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{invalid}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	indexer := NewSourceIndexer(context.Background(), store)
	returned, err := indexer.Index(t.Context(), path, "")
	if err != nil {
		t.Fatal(err)
	}
	if returned.Info.IndexState == rolloutindex.IndexComplete {
		t.Fatalf("stale source was presented as complete: %#v", returned.Info)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := indexer.waitIdle(waitCtx, valid.Info.ID); err != nil {
		t.Fatal(err)
	}
	failed, err := store.Source(t.Context(), valid.Info.ID)
	if err != nil || failed.IndexState != rolloutindex.IndexFailed || failed.IndexError == "" {
		t.Fatalf("failed refresh=%#v err=%v", failed, err)
	}
	page, err := store.Events(t.Context(), rolloutindex.EventQuery{SourceID: valid.Info.ID, TrajectoryID: "trajectory-app", Limit: 100})
	if err != nil || page.Total != 3 {
		t.Fatalf("valid cache was replaced: total=%d err=%v", page.Total, err)
	}
}

func openAppIndex(t *testing.T) *rolloutindex.Index {
	t.Helper()
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func appCanonicalRecords(events int, complete bool) []any {
	records := []any{
		model.Run{RecordType: model.RecordRun, ID: "run-app"},
		model.Case{RecordType: model.RecordCase, ID: "case-app", RunID: "run-app"},
		model.Group{RecordType: model.RecordGroup, ID: "group-app", CaseID: "case-app"},
		model.Trajectory{RecordType: model.RecordTrajectory, ID: "trajectory-app", GroupID: "group-app"},
	}
	for n := 0; n < events; n++ {
		records = append(records, model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("event-%d", n), TrajectoryID: "trajectory-app", Sequence: int64(n), Kind: "message"})
	}
	if complete {
		records = append(records, model.Complete{RecordType: model.RecordComplete, Records: int64(len(records)), Warnings: 0})
	}
	return records
}

func writeAppCanonical(t *testing.T, path string, records []any) {
	t.Helper()
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func appendAppCanonical(t *testing.T, path string, records []any) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			file.Close()
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func appEventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met")
}
