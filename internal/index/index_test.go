package index

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unlatch-ai/rlviz/internal/model"
)

func openTestIndex(t *testing.T) *Index {
	t.Helper()
	idx, err := Open(filepath.Join(t.TempDir(), "rollouts.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := idx.Close(); err != nil {
			t.Error(err)
		}
	})
	return idx
}

func TestReplaceAndQueryRoundTrip(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	stream := fixture(t, 6, true)
	source := Source{ID: "source-1", Path: "/traces/run.ndjson", Fingerprint: "sha256:one", Size: int64(len(stream)), ModTime: time.Unix(123, 456).UTC()}
	info, err := idx.Replace(ctx, source, bytes.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	if info.Records != 12 || info.Warnings != 2 || !bytes.Equal(info.CompleteRaw, []byte(`{"record_type":"complete","records":12,"warnings":2}`)) {
		t.Fatalf("unexpected source info: %#v", info)
	}

	gotSource, err := idx.Source(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotSource.Fingerprint != source.Fingerprint || !gotSource.ModTime.Equal(source.ModTime) {
		t.Fatalf("source metadata changed: %#v", gotSource)
	}
	contextValue, err := idx.TrajectoryContext(ctx, source.ID, "trajectory-a")
	if err != nil {
		t.Fatal(err)
	}
	if contextValue.Run.Value.ID != "run-1" || contextValue.Case.Value.ID != "case-1" || contextValue.Group.Value.ID != "group-1" || contextValue.Trajectory.Value.ID != "trajectory-a" {
		t.Fatalf("bad context: %#v", contextValue)
	}
	if contextValue.Run.ByteOffset != 0 || contextValue.Run.ByteLength != int64(len(contextValue.Run.Raw)) {
		t.Fatalf("bad run location: %#v", contextValue.Run)
	}
	groups, err := idx.Groups(ctx, source.ID)
	if err != nil || len(groups) != 1 || groups[0].Value.ID != "group-1" {
		t.Fatalf("groups=%#v err=%v", groups, err)
	}
	trajectories, err := idx.Trajectories(ctx, source.ID)
	if err != nil || len(trajectories) != 1 || trajectories[0].Value.ID != "trajectory-a" {
		t.Fatalf("trajectories=%#v err=%v", trajectories, err)
	}

	page, err := idx.Events(ctx, EventQuery{SourceID: source.ID, TrajectoryID: "trajectory-a", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 6 || len(page.Events) != 2 || page.NextSequence == nil || *page.NextSequence != 1 {
		t.Fatalf("bad first page: %#v", page)
	}
	first := page.Events[0]
	if first.Value.Source == nil || first.Value.Source.Path != source.Path || first.Value.Source.Line == nil || *first.Value.Source.Line != first.Line ||
		first.Value.Source.ByteOffset == nil || *first.Value.Source.ByteOffset != first.ByteOffset || first.Value.Source.ByteLength == nil || *first.Value.Source.ByteLength != first.ByteLength {
		t.Fatalf("direct provenance not derived: %#v", first)
	}
	next, err := idx.Events(ctx, EventQuery{SourceID: source.ID, TrajectoryID: "trajectory-a", AfterSequence: page.NextSequence, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if next.Total != 6 || len(next.Events) != 4 || next.NextSequence != nil {
		t.Fatalf("bad second page: %#v", next)
	}

	signals, err := idx.Signals(ctx, source.ID, "trajectory-a")
	if err != nil || len(signals) != 1 || signals[0].Value.Name != "reward" {
		t.Fatalf("signals=%#v err=%v", signals, err)
	}
	artifacts, err := idx.Artifacts(ctx, source.ID, "trajectory-a")
	if err != nil || len(artifacts) != 1 || artifacts[0].Value.MediaType != "text/plain" {
		t.Fatalf("artifacts=%#v err=%v", artifacts, err)
	}
	records, err := idx.Records(ctx, source.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 13 || !bytes.Equal(records[0].Raw, contextValue.Run.Raw) || records[0].ByteOffset != 0 {
		t.Fatalf("lossless records failed: %#v", records[:1])
	}
}

func TestChildPagesBoundMaxPlusOneAndArtifactDirectLookup(t *testing.T) {
	idx := openTestIndex(t)
	stream := fixture(t, 0, false)
	if _, err := idx.Replace(t.Context(), Source{ID: "bounded"}, bytes.NewReader(stream)); err != nil {
		t.Fatal(err)
	}
	_, err := idx.db.ExecContext(t.Context(), `WITH RECURSIVE generated(n) AS (
	    SELECT 0 UNION ALL SELECT n+1 FROM generated WHERE n < ?
	  ) INSERT INTO artifacts
	  (source_id,id,trajectory_id,event_id,name,media_type,path,sha256,line,byte_offset,byte_length,raw)
	  SELECT 'bounded','artifact-'||n,'trajectory-a','','artifact-'||n,'text/plain','','',n,0,2,
	    json_object('record_type','artifact','id','artifact-'||n,'trajectory_id','trajectory-a','name','artifact-'||n,'media_type','text/plain','content','x')
	  FROM generated`, MaxQueryRecords)
	if err != nil {
		t.Fatal(err)
	}
	page, err := idx.ArtifactsPage(t.Context(), "bounded", "trajectory-a", 0, MaxQueryRecords)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != MaxQueryRecords+1 || len(page.Items) != MaxQueryRecords {
		t.Fatalf("artifact page total/items = %d/%d", page.Total, len(page.Items))
	}
	artifactTail, err := idx.ArtifactsPage(t.Context(), "bounded", "trajectory-a", int64(MaxQueryRecords), 10)
	if err != nil || len(artifactTail.Items) != 1 || artifactTail.Items[0].Value.ID != fmt.Sprintf("artifact-%d", MaxQueryRecords) || artifactTail.Offset != MaxQueryRecords {
		t.Fatalf("artifact tail = %#v err=%v", artifactTail, err)
	}
	item, err := idx.Artifact(t.Context(), "bounded", "trajectory-a", fmt.Sprintf("artifact-%d", MaxQueryRecords))
	if err != nil || item.Value == nil || item.Value.ID != fmt.Sprintf("artifact-%d", MaxQueryRecords) {
		t.Fatalf("direct artifact = %#v err=%v", item, err)
	}

	_, err = idx.db.ExecContext(t.Context(), `WITH RECURSIVE generated(n) AS (
	    SELECT 0 UNION ALL SELECT n+1 FROM generated WHERE n < ?
	  ) INSERT INTO signals
	  (source_id,id,trajectory_id,event_id,name,unit,line,byte_offset,byte_length,raw)
	  SELECT 'bounded','signal-'||n,'trajectory-a','','metric-'||n,'',n,0,2,
	    json_object('record_type','signal','id','signal-'||n,'trajectory_id','trajectory-a','name','metric-'||n,'value',n)
	  FROM generated`, MaxQueryRecords)
	if err != nil {
		t.Fatal(err)
	}
	signals, err := idx.SignalsPage(t.Context(), "bounded", "trajectory-a", 0, MaxQueryRecords)
	if err != nil || signals.Total != MaxQueryRecords+1 || len(signals.Items) != MaxQueryRecords {
		t.Fatalf("signal page total/items = %d/%d err=%v", signals.Total, len(signals.Items), err)
	}
	signalTail, err := idx.SignalsPage(t.Context(), "bounded", "trajectory-a", int64(MaxQueryRecords), 10)
	if err != nil || len(signalTail.Items) != 1 || signalTail.Items[0].Value.ID != fmt.Sprintf("signal-%d", MaxQueryRecords) || signalTail.Offset != MaxQueryRecords {
		t.Fatalf("signal tail = %#v err=%v", signalTail, err)
	}

	_, err = idx.db.ExecContext(t.Context(), `WITH RECURSIVE generated(n) AS (
	    SELECT 0 UNION ALL SELECT n+1 FROM generated WHERE n < ?
	  ) INSERT INTO trajectories
	  (source_id,id,group_id,parent_id,branch_id,status,termination,line,byte_offset,byte_length,raw)
	  SELECT 'bounded','trajectory-extra-'||n,'group-1','','','','',n+1,0,2,
	    json_object('record_type','trajectory','id','trajectory-extra-'||n,'group_id','group-1')
	  FROM generated`, MaxQueryRecords)
	if err != nil {
		t.Fatal(err)
	}
	trajectories, err := idx.TrajectoriesPage(t.Context(), "bounded", MaxQueryRecords)
	if err != nil || trajectories.Total != MaxQueryRecords+2 || len(trajectories.Items) != MaxQueryRecords {
		t.Fatalf("trajectory page total/items = %d/%d err=%v", trajectories.Total, len(trajectories.Items), err)
	}
	summaries, err := idx.GroupSummariesPage(t.Context(), "bounded", "group-1", MaxQueryRecords)
	if err != nil || summaries.Total != MaxQueryRecords+2 || len(summaries.Items) != MaxQueryRecords {
		t.Fatalf("summary page total/items = %d/%d err=%v", summaries.Total, len(summaries.Items), err)
	}
}

func TestEventPageRejectsRawByteMaxPlusOneBeforeDecoding(t *testing.T) {
	idx := openTestIndex(t)
	if _, err := idx.Replace(t.Context(), Source{ID: "byte-bound"}, bytes.NewReader(fixture(t, 0, false))); err != nil {
		t.Fatal(err)
	}
	const rows = 5
	_, err := idx.db.ExecContext(t.Context(), `WITH RECURSIVE generated(n) AS (
	    SELECT 0 UNION ALL SELECT n+1 FROM generated WHERE n < ?
	  ) INSERT INTO events
	  (source_id,id,trajectory_id,sequence,kind,timestamp,parent_id,branch_id,alignment_key,state_hash,search_text,
	   source_path,source_line,byte_offset,byte_length,line,record_byte_offset,record_byte_length,raw)
	  SELECT 'byte-bound','large-'||n,'trajectory-a',n,'tool','','','','','','',NULL,NULL,NULL,NULL,n,0,0,zeroblob(?)
	  FROM generated`, rows-1, MaxQueryRawBytes/rows+1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.Events(t.Context(), EventQuery{SourceID: "byte-bound", TrajectoryID: "trajectory-a", Limit: rows}); !errors.Is(err, ErrResultTooLarge) {
		t.Fatalf("error = %v, want ErrResultTooLarge before invalid raw decode", err)
	}
}

func TestGroupSummariesRejectRawByteMaxPlusOneBeforeDecoding(t *testing.T) {
	idx := openTestIndex(t)
	if _, err := idx.Replace(t.Context(), Source{ID: "group-byte-bound"}, bytes.NewReader(fixture(t, 0, false))); err != nil {
		t.Fatal(err)
	}
	const rows = 9
	_, err := idx.db.ExecContext(t.Context(), `WITH RECURSIVE generated(n) AS (
	    SELECT 0 UNION ALL SELECT n+1 FROM generated WHERE n < ?
	  ) INSERT INTO signals
	  (source_id,id,trajectory_id,event_id,name,unit,line,byte_offset,byte_length,raw)
	  SELECT 'group-byte-bound','large-signal-'||n,'trajectory-a','','metric-'||n,'',n,0,0,zeroblob(?)
	  FROM generated`, rows-1, MaxGroupSummaryRawBytes/rows+1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.GroupSummariesPage(t.Context(), "group-byte-bound", "group-1", 1); !errors.Is(err, ErrResultTooLarge) {
		t.Fatalf("error = %v, want ErrResultTooLarge before invalid raw decode", err)
	}
}

func TestAdapterDoesNotInventSourceOffsets(t *testing.T) {
	idx := openTestIndex(t)
	stream := fixture(t, 1, false)
	_, err := idx.Replace(context.Background(), Source{ID: "adapter", Path: "/raw/customer.bin", Adapter: "/plugins/customer", Size: int64(len(stream))}, bytes.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	page, err := idx.Events(context.Background(), EventQuery{SourceID: "adapter", TrajectoryID: "trajectory-a"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Events[0].Value.Source != nil {
		t.Fatalf("adapter provenance was invented: %#v", page.Events[0].Value.Source)
	}
}

func TestReplaceIsAtomic(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	valid := fixture(t, 3, false)
	if _, err := idx.Replace(ctx, Source{ID: "same", Fingerprint: "old"}, bytes.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	invalid := strings.Replace(string(fixture(t, 1, false)), `"id":"event-0"`, `"id":"trajectory-a"`, 1)
	if _, err := idx.Replace(ctx, Source{ID: "same", Fingerprint: "new"}, strings.NewReader(invalid)); err == nil {
		t.Fatal("invalid replacement succeeded")
	}
	info, err := idx.Source(ctx, "same")
	if err != nil {
		t.Fatal(err)
	}
	page, err := idx.Events(ctx, EventQuery{SourceID: "same", TrajectoryID: "trajectory-a"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Fingerprint != "old" || len(page.Events) != 3 {
		t.Fatalf("prior transaction was not preserved: %#v %#v", info, page)
	}
}

func TestEventFiltersSearchAndGroupSummaries(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	stream := groupFixture(t)
	if _, err := idx.Replace(ctx, Source{ID: "group"}, bytes.NewReader(stream)); err != nil {
		t.Fatal(err)
	}
	page, err := idx.Events(ctx, EventQuery{SourceID: "group", TrajectoryID: "trajectory-a", Kinds: []string{"tool"}, Query: "Needle%_", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Events) != 1 || page.Events[0].Value.Sequence != 1 || page.NextSequence == nil {
		t.Fatalf("filtered page: %#v", page)
	}
	page2, err := idx.Events(ctx, EventQuery{SourceID: "group", TrajectoryID: "trajectory-a", Kinds: []string{"tool"}, Query: "Needle%_", AfterSequence: page.NextSequence, Limit: 1})
	if err != nil || page2.Total != 2 || len(page2.Events) != 1 || page2.Events[0].Value.Sequence != 3 {
		t.Fatalf("filtered page 2: %#v err=%v", page2, err)
	}
	summaries, err := idx.GroupSummaries(ctx, "group", "group-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 || summaries[0].EventCount != 4 || summaries[0].SignalCount != 2 || summaries[0].ArtifactCount != 2 ||
		summaries[1].EventCount != 1 || summaries[1].FirstSequence == nil || *summaries[1].FirstSequence != 8 {
		t.Fatalf("bad summaries: %#v", summaries)
	}
	trajectories, err := idx.Trajectories(ctx, "group")
	if err != nil || len(trajectories) != 2 || trajectories[0].Value.ID != "trajectory-a" || trajectories[1].Value.ID != "trajectory-b" {
		t.Fatalf("canonical trajectory order=%#v err=%v", trajectories, err)
	}
}

func TestGroupMetricsPreserveSignalsAndAggregateMissingValues(t *testing.T) {
	idx := openTestIndex(t)
	if _, err := idx.Replace(t.Context(), Source{ID: "metrics"}, bytes.NewReader(metricGroupFixture(t))); err != nil {
		t.Fatal(err)
	}
	summaries, err := idx.GroupSummaries(t.Context(), "metrics", "group-metrics")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 3 {
		t.Fatalf("summary count=%d", len(summaries))
	}
	passed := summaries[0]
	if passed.Success == nil || !*passed.Success || passed.Reward == nil || *passed.Reward != 1.5 ||
		passed.TokenCount == nil || *passed.TokenCount != 9007199254740993 || passed.LatencyMS == nil || *passed.LatencyMS != 250 {
		t.Fatalf("passed metrics=%#v", passed)
	}
	if string(passed.Signals["token_count"]) != "9007199254740993" {
		t.Fatalf("token signal lost precision: %s", passed.Signals["token_count"])
	}
	// pass has precedence over success; the lower-priority contradictory signal
	// must not change the classification.
	if string(passed.Signals["success"]) != "false" {
		t.Fatalf("generic success signal=%s", passed.Signals["success"])
	}
	failed, unknown := summaries[1], summaries[2]
	if failed.Success == nil || *failed.Success || failed.ErrorCount != 4 || unknown.Success != nil || unknown.Reward != nil {
		t.Fatalf("failed/unknown metrics=%#v %#v", failed, unknown)
	}
	aggregates := AggregateGroup(summaries)
	if aggregates.Count != 3 || aggregates.Success != 1 || aggregates.Failure != 1 || aggregates.Unknown != 1 {
		t.Fatalf("outcomes=%#v", aggregates)
	}
	if aggregates.Reward == nil || aggregates.Reward.Min != -1 || aggregates.Reward.Max != 1.5 || aggregates.Reward.Mean != 0.25 {
		t.Fatalf("reward aggregate=%#v", aggregates.Reward)
	}
	if aggregates.EventCount == nil || aggregates.EventCount.Min != 0 || aggregates.EventCount.Max != 2 ||
		aggregates.ErrorCount == nil || aggregates.ErrorCount.Min != 0 || aggregates.ErrorCount.Max != 4 ||
		aggregates.TokenCount == nil || aggregates.TokenCount.Min != 10 || aggregates.TokenCount.Max != 9007199254740993 ||
		aggregates.LatencyMS == nil || aggregates.LatencyMS.Min != 100 || aggregates.LatencyMS.Max != 250 {
		t.Fatalf("range aggregates=%#v", aggregates)
	}
}

func TestGroupMetricsCanonicalizeCaseVariants(t *testing.T) {
	idx := openTestIndex(t)
	stream := metricGroupFixture(t)
	stream = bytes.ReplaceAll(stream, []byte(`"name":"reward"`), []byte(`"name":"ReWaRd"`))
	stream = bytes.ReplaceAll(stream, []byte(`"name":"pass"`), []byte(`"name":"PASS"`))
	if _, err := idx.Replace(t.Context(), Source{ID: "case-metrics"}, bytes.NewReader(stream)); err != nil {
		t.Fatal(err)
	}
	summaries, err := idx.GroupSummaries(t.Context(), "case-metrics", "group-metrics")
	if err != nil {
		t.Fatal(err)
	}
	if summaries[0].Reward == nil || *summaries[0].Reward != 1.5 || summaries[0].Success == nil || !*summaries[0].Success {
		t.Fatalf("case-variant metrics disappeared: %#v", summaries[0])
	}
	if _, ok := summaries[0].Signals["reward"]; !ok {
		t.Fatalf("signals were not canonicalized: %#v", summaries[0].Signals)
	}
}

func TestSourceStatusCleanupAndCascade(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	when := time.Unix(22, 33).UTC()
	for _, source := range []Source{{ID: "keep", Fingerprint: "a", ModTime: when}, {ID: "drop", Fingerprint: "b", ModTime: when}} {
		if _, err := idx.Replace(ctx, source, bytes.NewReader(fixture(t, 1, false))); err != nil {
			t.Fatal(err)
		}
	}
	status, err := idx.Status(ctx, Source{ID: "keep", Fingerprint: "a", ModTime: when})
	if err != nil || status.State != CacheFresh {
		t.Fatalf("fresh status=%#v err=%v", status, err)
	}
	status, err = idx.Status(ctx, Source{ID: "keep", Fingerprint: "changed", ModTime: when})
	if err != nil || status.State != CacheStale {
		t.Fatalf("stale status=%#v err=%v", status, err)
	}
	status, err = idx.Status(ctx, Source{ID: "missing"})
	if err != nil || status.State != CacheMissing {
		t.Fatalf("missing status=%#v err=%v", status, err)
	}
	removed, err := idx.Cleanup(ctx, []string{"keep"})
	if err != nil || removed != 1 {
		t.Fatalf("cleanup removed=%d err=%v", removed, err)
	}
	if _, err := idx.Source(ctx, "drop"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("drop source error=%v", err)
	}
	var children int
	if err := idx.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE source_id='drop'`).Scan(&children); err != nil || children != 0 {
		t.Fatalf("cascade count=%d err=%v", children, err)
	}
}

func TestIndexesTenThousandEvents(t *testing.T) {
	idx := openTestIndex(t)
	stream := fixture(t, 10_000, false)
	info, err := idx.Replace(context.Background(), Source{ID: "large", Size: int64(len(stream))}, bytes.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	if info.Records != 10_004 {
		t.Fatalf("records=%d", info.Records)
	}
	after := int64(9_899)
	page, err := idx.Events(context.Background(), EventQuery{SourceID: "large", TrajectoryID: "trajectory-a", AfterSequence: &after, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 10_000 || len(page.Events) != 100 || page.Events[0].Value.Sequence != 9_900 || page.Events[99].Value.Sequence != 9_999 {
		t.Fatalf("bad tail page: total=%d len=%d", page.Total, len(page.Events))
	}
}

func TestSQLitePragmas(t *testing.T) {
	idx := openTestIndex(t)
	var journal string
	var foreignKeys, busy, version int
	if err := idx.db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if err := idx.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if err := idx.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if err := idx.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if strings.ToLower(journal) != "wal" || foreignKeys != 1 || busy != 5000 || version != 4 {
		t.Fatalf("pragmas: journal=%s foreign=%d busy=%d version=%d", journal, foreignKeys, busy, version)
	}
}

func TestOpenMigratesPreProgressiveSourceState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE sources (
      id TEXT PRIMARY KEY, path TEXT NOT NULL, adapter TEXT NOT NULL,
      fingerprint TEXT NOT NULL, size INTEGER NOT NULL, mod_time_ns INTEGER NOT NULL,
      indexed_at_ns INTEGER NOT NULL, records INTEGER NOT NULL, warnings INTEGER NOT NULL,
      complete_raw BLOB NOT NULL
    ); PRAGMA user_version=2;`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	idx, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	for _, column := range []string{"index_state", "index_error"} {
		present, err := idx.sourceColumn(t.Context(), column)
		if err != nil || !present {
			t.Fatalf("column %s present=%v err=%v", column, present, err)
		}
	}
	var version int
	if err := idx.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 4 {
		t.Fatalf("version=%d err=%v", version, err)
	}
	var presentationTable string
	if err := idx.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='source_presentations'`).Scan(&presentationTable); err != nil || presentationTable != "source_presentations" {
		t.Fatalf("presentation migration=%q err=%v", presentationTable, err)
	}
}

func TestPresentationPersistsAcrossSourceReplacementAndClearsExplicitly(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	stream := fixture(t, 1, false)
	source := Source{ID: "presented", Path: "/trace.ndjson", Fingerprint: "first", Size: int64(len(stream)), ModTime: time.Unix(1, 0)}
	if _, err := idx.Replace(ctx, source, bytes.NewReader(stream)); err != nil {
		t.Fatal(err)
	}
	config := json.RawMessage(`{ "group": { "columns": ["reward"] }, "api_version": "rlviz.dev/v1alpha1" }`)
	if err := idx.SetPresentation(ctx, source.ID, config); err != nil {
		t.Fatal(err)
	}
	got, err := idx.Presentation(ctx, source.ID)
	if err != nil || string(got) != `{"api_version":"rlviz.dev/v1alpha1","group":{"columns":["reward"]}}` {
		t.Fatalf("normalized presentation=%s err=%v", got, err)
	}
	source.Fingerprint = "second"
	if _, err := idx.Replace(ctx, source, bytes.NewReader(stream)); err != nil {
		t.Fatal(err)
	}
	got, err = idx.Presentation(ctx, source.ID)
	if err != nil || len(got) == 0 {
		t.Fatalf("presentation after refresh=%s err=%v", got, err)
	}
	if err := idx.SetPresentation(ctx, source.ID, nil); err != nil {
		t.Fatal(err)
	}
	got, err = idx.Presentation(ctx, source.ID)
	if err != nil || got != nil {
		t.Fatalf("presentation after clear=%s err=%v", got, err)
	}
}

func TestDatabaseFileIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	path := filepath.Join(t.TempDir(), "private", "index.sqlite")
	idx, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o, want 600", info.Mode().Perm())
	}
}

func fixture(t *testing.T, events int, extras bool) []byte {
	t.Helper()
	records := []any{
		&model.Run{RecordType: model.RecordRun, ID: "run-1", Name: "test"},
		&model.Case{RecordType: model.RecordCase, ID: "case-1", RunID: "run-1"},
		&model.Group{RecordType: model.RecordGroup, ID: "group-1", CaseID: "case-1"},
		&model.Trajectory{RecordType: model.RecordTrajectory, ID: "trajectory-a", GroupID: "group-1", Status: "complete"},
	}
	for n := 0; n < events; n++ {
		records = append(records, &model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("event-%d", n), TrajectoryID: "trajectory-a", Sequence: int64(n), Kind: "message", Data: map[string]any{"text": fmt.Sprintf("event %d", n)}})
	}
	if extras {
		records = append(records,
			&model.Signal{RecordType: model.RecordSignal, ID: "signal-1", TrajectoryID: "trajectory-a", Name: "reward", Value: 1.0},
			&model.Artifact{RecordType: model.RecordArtifact, ID: "artifact-1", TrajectoryID: "trajectory-a", Name: "log", MediaType: "text/plain", Text: "ok"})
	}
	return encodeFixture(t, records, 2)
}

func groupFixture(t *testing.T) []byte {
	records := []any{
		&model.Run{RecordType: model.RecordRun, ID: "run-1"},
		&model.Case{RecordType: model.RecordCase, ID: "case-1", RunID: "run-1"},
		&model.Group{RecordType: model.RecordGroup, ID: "group-1", CaseID: "case-1"},
		&model.Trajectory{RecordType: model.RecordTrajectory, ID: "trajectory-a", GroupID: "group-1"},
	}
	for n, kind := range []string{"message", "tool", "message", "tool"} {
		records = append(records, &model.Event{RecordType: model.RecordEvent, ID: fmt.Sprintf("a-%d", n), TrajectoryID: "trajectory-a", Sequence: int64(n), Kind: kind, Data: map[string]any{"text": map[bool]string{true: "Needle%_", false: "other"}[kind == "tool"]}})
	}
	records = append(records,
		&model.Signal{RecordType: model.RecordSignal, ID: "s-1", TrajectoryID: "trajectory-a", Name: "reward", Value: 1},
		&model.Signal{RecordType: model.RecordSignal, ID: "s-2", TrajectoryID: "trajectory-a", Name: "score", Value: 2},
		&model.Artifact{RecordType: model.RecordArtifact, ID: "x-1", TrajectoryID: "trajectory-a", MediaType: "text/plain", Text: "one"},
		&model.Artifact{RecordType: model.RecordArtifact, ID: "x-2", TrajectoryID: "trajectory-a", MediaType: "text/plain", Text: "two"},
		&model.Trajectory{RecordType: model.RecordTrajectory, ID: "trajectory-b", GroupID: "group-1", ParentID: "trajectory-a"},
		&model.Event{RecordType: model.RecordEvent, ID: "b-8", TrajectoryID: "trajectory-b", Sequence: 8, Kind: "error"})
	return encodeFixture(t, records, 0)
}

func metricGroupFixture(t *testing.T) []byte {
	records := []any{
		&model.Run{RecordType: model.RecordRun, ID: "run-metrics"},
		&model.Case{RecordType: model.RecordCase, ID: "case-metrics", RunID: "run-metrics"},
		&model.Group{RecordType: model.RecordGroup, ID: "group-metrics", CaseID: "case-metrics"},
		&model.Trajectory{RecordType: model.RecordTrajectory, ID: "passed", GroupID: "group-metrics", Status: "completed", Termination: "answer"},
		&model.Trajectory{RecordType: model.RecordTrajectory, ID: "failed", GroupID: "group-metrics", Status: "failed", Termination: "error"},
		&model.Trajectory{RecordType: model.RecordTrajectory, ID: "unknown", GroupID: "group-metrics", Status: "running"},
		&model.Event{RecordType: model.RecordEvent, ID: "p-0", TrajectoryID: "passed", Sequence: 0, Kind: "message"},
		&model.Event{RecordType: model.RecordEvent, ID: "p-1", TrajectoryID: "passed", Sequence: 1, Kind: "generation"},
		&model.Event{RecordType: model.RecordEvent, ID: "f-0", TrajectoryID: "failed", Sequence: 0, Kind: "error"},
		&model.Signal{RecordType: model.RecordSignal, ID: "p-reward", TrajectoryID: "passed", Name: "reward", Value: json.Number("1.5")},
		&model.Signal{RecordType: model.RecordSignal, ID: "p-success", TrajectoryID: "passed", Name: "success", Value: false},
		&model.Signal{RecordType: model.RecordSignal, ID: "p-pass", TrajectoryID: "passed", Name: "pass", Value: true},
		&model.Signal{RecordType: model.RecordSignal, ID: "p-tokens", TrajectoryID: "passed", Name: "token_count", Value: json.Number("9007199254740993")},
		&model.Signal{RecordType: model.RecordSignal, ID: "p-latency", TrajectoryID: "passed", Name: "latency_seconds", Value: json.Number("0.25")},
		&model.Signal{RecordType: model.RecordSignal, ID: "f-reward", TrajectoryID: "failed", Name: "reward", Value: json.Number("-1")},
		&model.Signal{RecordType: model.RecordSignal, ID: "f-pass", TrajectoryID: "failed", Name: "pass", Value: json.Number("0")},
		&model.Signal{RecordType: model.RecordSignal, ID: "f-errors", TrajectoryID: "failed", Name: "error_count", Value: json.Number("4")},
		&model.Signal{RecordType: model.RecordSignal, ID: "f-tokens", TrajectoryID: "failed", Name: "total_tokens", Value: json.Number("10")},
		&model.Signal{RecordType: model.RecordSignal, ID: "f-duration", TrajectoryID: "failed", Name: "duration", Value: json.Number("0.1"), Unit: "s"},
		&model.Signal{RecordType: model.RecordSignal, ID: "u-score", TrajectoryID: "unknown", Name: "score", Value: json.Number("0.8")},
	}
	return encodeFixture(t, records, 0)
}

func encodeFixture(t *testing.T, records []any, warnings int64) []byte {
	t.Helper()
	var out bytes.Buffer
	for _, record := range records {
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		out.Write(data)
		out.WriteByte('\n')
	}
	data, err := json.Marshal(&model.Complete{RecordType: model.RecordComplete, Records: int64(len(records)), Warnings: warnings})
	if err != nil {
		t.Fatal(err)
	}
	out.Write(data)
	out.WriteByte('\n')
	return out.Bytes()
}
