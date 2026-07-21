package index

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/analyzers"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

func TestLoopRetryAnalysisCachesByAnalyzerAndInputDigest(t *testing.T) {
	idx := openTestIndex(t)
	stream := analysisFixture(t, []string{"go test", "go test", "go test"})
	if _, err := idx.Replace(t.Context(), Source{ID: "analysis"}, bytes.NewReader(stream)); err != nil {
		t.Fatal(err)
	}

	first, err := idx.LoopRetryAnalysis(t.Context(), "analysis", "trajectory-analysis")
	if err != nil {
		t.Fatal(err)
	}
	if first.Cached || len(first.Output.Findings) != 1 || first.Output.Findings[0].Kind != "retry" {
		t.Fatalf("first result = %#v", first)
	}
	if first.Output.Provenance.Name != analyzers.LoopRetryName || first.Output.Provenance.Digest != analyzers.LoopRetryDigest || first.Output.Provenance.InputDigest == "" {
		t.Fatalf("provenance = %#v", first.Output.Provenance)
	}

	second, err := idx.LoopRetryAnalysis(t.Context(), "analysis", "trajectory-analysis")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached || !second.AnalyzedAt.Equal(first.AnalyzedAt) {
		t.Fatalf("second result = %#v", second)
	}
	if second.Output.Provenance != first.Output.Provenance {
		t.Fatalf("cached provenance changed: %#v %#v", first.Output.Provenance, second.Output.Provenance)
	}
	var rows int
	if err := idx.db.QueryRow(`SELECT COUNT(*) FROM analyzer_results WHERE source_id='analysis'`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("cache rows=%d err=%v", rows, err)
	}
}

func TestLoopRetryAnalysisInvalidatesOnSourceReplacement(t *testing.T) {
	idx := openTestIndex(t)
	if _, err := idx.Replace(t.Context(), Source{ID: "analysis"}, bytes.NewReader(analysisFixture(t, []string{"same", "same", "same"}))); err != nil {
		t.Fatal(err)
	}
	old, err := idx.LoopRetryAnalysis(t.Context(), "analysis", "trajectory-analysis")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.Replace(t.Context(), Source{ID: "analysis", Fingerprint: "changed"}, bytes.NewReader(analysisFixture(t, []string{"one", "two", "three"}))); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := idx.db.QueryRow(`SELECT COUNT(*) FROM analyzer_results WHERE source_id='analysis'`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("stale cache rows=%d err=%v", rows, err)
	}
	fresh, err := idx.LoopRetryAnalysis(t.Context(), "analysis", "trajectory-analysis")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Cached || len(fresh.Output.Findings) != 0 || fresh.Output.Provenance.InputDigest == old.Output.Provenance.InputDigest {
		t.Fatalf("fresh result = %#v", fresh)
	}
}

func TestLoopRetryAnalysisMissingTrajectory(t *testing.T) {
	idx := openTestIndex(t)
	if _, err := idx.Replace(t.Context(), Source{ID: "analysis"}, bytes.NewReader(analysisFixture(t, nil))); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.LoopRetryAnalysis(t.Context(), "analysis", "missing"); err != ErrNotFound {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestLoopRetryAnalysisRejectsMaxPlusOneEventsBeforeDecoding(t *testing.T) {
	idx := openTestIndex(t)
	if _, err := idx.Replace(t.Context(), Source{ID: "analysis"}, bytes.NewReader(analysisFixture(t, nil))); err != nil {
		t.Fatal(err)
	}
	_, err := idx.db.ExecContext(t.Context(), `WITH RECURSIVE generated(n) AS (
	    SELECT 0 UNION ALL SELECT n+1 FROM generated WHERE n < ?
	  ) INSERT INTO events
	  (source_id,id,trajectory_id,sequence,kind,timestamp,parent_id,branch_id,alignment_key,state_hash,search_text,
	   source_path,source_line,byte_offset,byte_length,line,record_byte_offset,record_byte_length,raw)
	  SELECT 'analysis','oversized-'||n,'trajectory-analysis',n,'tool','','','','','','',NULL,NULL,NULL,NULL,n,0,2,'{}'
	  FROM generated`, analyzers.MaxInputEvents)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.LoopRetryAnalysis(t.Context(), "analysis", "trajectory-analysis"); !errors.Is(err, ErrResultTooLarge) {
		t.Fatalf("error = %v, want ErrResultTooLarge", err)
	}
}

func analysisFixture(t *testing.T, actions []string) []byte {
	t.Helper()
	records := []any{
		model.Run{RecordType: model.RecordRun, ID: "run-analysis"},
		model.Case{RecordType: model.RecordCase, ID: "case-analysis", RunID: "run-analysis"},
		model.Group{RecordType: model.RecordGroup, ID: "group-analysis", CaseID: "case-analysis"},
		model.Trajectory{RecordType: model.RecordTrajectory, ID: "trajectory-analysis", GroupID: "group-analysis"},
	}
	for sequence, action := range actions {
		records = append(records, model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("event-%d", sequence), TrajectoryID: "trajectory-analysis", Sequence: int64(sequence), Kind: "tool", Input: map[string]any{"name": "shell", "command": action}})
	}
	records = append(records, model.Complete{RecordType: model.RecordComplete, Records: int64(len(records)), Warnings: 0})
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	return output.Bytes()
}
