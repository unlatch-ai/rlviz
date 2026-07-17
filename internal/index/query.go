package index

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/unlatch-ai/rlviz/internal/model"
	"github.com/unlatch-ai/rlviz/internal/presentation"
)

var ErrNotFound = errors.New("indexed rollout not found")

const (
	MaxQueryRecords         = 1000
	MaxGroupSummarySignals  = 10_000
	MaxQueryRawBytes        = 32 << 20
	MaxGroupSummaryRawBytes = 64 << 20
)

var ErrResultTooLarge = errors.New("indexed result exceeds resource limit")

func (i *Index) Source(ctx context.Context, id string) (SourceInfo, error) {
	return scanSource(i.db.QueryRowContext(ctx, `SELECT id,path,adapter,fingerprint,size,mod_time_ns,indexed_at_ns,records,warnings,complete_raw,index_state,index_error FROM sources WHERE id=?`, id))
}

func (i *Index) Sources(ctx context.Context) ([]SourceInfo, error) {
	rows, err := i.db.QueryContext(ctx, `SELECT id,path,adapter,fingerprint,size,mod_time_ns,indexed_at_ns,records,warnings,complete_raw,index_state,index_error FROM sources ORDER BY indexed_at_ns DESC,id`)
	if err != nil {
		return nil, fmt.Errorf("list indexed sources: %w", err)
	}
	defer rows.Close()
	var result []SourceInfo
	for rows.Next() {
		info, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, rows.Err()
}

type rowScanner interface{ Scan(...any) error }

func scanSource(row rowScanner) (SourceInfo, error) {
	var info SourceInfo
	var modNS, indexedNS int64
	var raw []byte
	err := row.Scan(&info.ID, &info.Path, &info.Adapter, &info.Fingerprint, &info.Size, &modNS, &indexedNS,
		&info.Records, &info.Warnings, &raw, &info.IndexState, &info.IndexError)
	if errors.Is(err, sql.ErrNoRows) {
		return SourceInfo{}, ErrNotFound
	}
	if err != nil {
		return SourceInfo{}, fmt.Errorf("read indexed source: %w", err)
	}
	if modNS != 0 {
		info.ModTime = time.Unix(0, modNS).UTC()
	}
	info.IndexedAt = time.Unix(0, indexedNS).UTC()
	info.CompleteRaw = append(json.RawMessage(nil), raw...)
	return info, nil
}

func (i *Index) Status(ctx context.Context, source Source) (SourceStatus, error) {
	cached, err := i.Source(ctx, source.ID)
	if errors.Is(err, ErrNotFound) {
		return SourceStatus{State: CacheMissing}, nil
	}
	if err != nil {
		return SourceStatus{}, err
	}
	state := CacheFresh
	if cached.IndexState != IndexComplete {
		state = CacheStale
	}
	if cached.Path != source.Path || cached.Adapter != source.Adapter || cached.Fingerprint != source.Fingerprint ||
		cached.Size != source.Size || !cached.ModTime.Equal(source.ModTime) {
		state = CacheStale
	}
	return SourceStatus{State: state, Cached: &cached}, nil
}

func (i *Index) Remove(ctx context.Context, sourceID string) error {
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("remove indexed source: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sources WHERE id=?`, sourceID); err != nil {
		return fmt.Errorf("remove indexed source: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM source_presentations WHERE source_id=?`, sourceID); err != nil {
		return fmt.Errorf("remove source presentation: %w", err)
	}
	return tx.Commit()
}

// SetPresentation stores normalized declarative presentation independently of
// source contents. A nil or JSON null configuration clears the prior value.
func (i *Index) SetPresentation(ctx context.Context, sourceID string, config json.RawMessage) error {
	if strings.TrimSpace(sourceID) == "" {
		return errors.New("source id is required")
	}
	var exists int
	if err := i.db.QueryRowContext(ctx, `SELECT 1 FROM sources WHERE id=?`, sourceID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("find source for presentation: %w", err)
	}
	normalized, err := presentation.NormalizeJSON(config)
	if err != nil {
		return fmt.Errorf("validate source presentation: %w", err)
	}
	if normalized == nil {
		_, err = i.db.ExecContext(ctx, `DELETE FROM source_presentations WHERE source_id=?`, sourceID)
	} else {
		_, err = i.db.ExecContext(ctx, `INSERT INTO source_presentations(source_id,config_json,updated_at_ns)
		  VALUES(?,?,?) ON CONFLICT(source_id) DO UPDATE SET config_json=excluded.config_json,updated_at_ns=excluded.updated_at_ns`,
			sourceID, []byte(normalized), time.Now().UTC().UnixNano())
	}
	if err != nil {
		return fmt.Errorf("store source presentation: %w", err)
	}
	return nil
}

// Presentation returns a validated normalized config, or nil when unset.
func (i *Index) Presentation(ctx context.Context, sourceID string) (json.RawMessage, error) {
	var data []byte
	err := i.db.QueryRowContext(ctx, `SELECT config_json FROM source_presentations WHERE source_id=?`, sourceID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read source presentation: %w", err)
	}
	normalized, err := presentation.NormalizeJSON(data)
	if err != nil {
		return nil, fmt.Errorf("read invalid source presentation: %w", err)
	}
	return normalized, nil
}

// Cleanup removes every cached source whose ID is not in keepIDs.
func (i *Index) Cleanup(ctx context.Context, keepIDs []string) (int64, error) {
	query := `DELETE FROM sources`
	args := make([]any, len(keepIDs))
	if len(keepIDs) != 0 {
		query += ` WHERE id NOT IN (` + strings.TrimRight(strings.Repeat("?,", len(keepIDs)), ",") + `)`
		for n := range keepIDs {
			args[n] = keepIDs[n]
		}
	}
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("clean indexed sources: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("clean indexed sources: %w", err)
	}
	presentationQuery := `DELETE FROM source_presentations`
	if len(keepIDs) != 0 {
		presentationQuery += ` WHERE source_id NOT IN (` + strings.TrimRight(strings.Repeat("?,", len(keepIDs)), ",") + `)`
	}
	if _, err := tx.ExecContext(ctx, presentationQuery, args...); err != nil {
		return 0, fmt.Errorf("clean source presentations: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func (i *Index) TrajectoryContext(ctx context.Context, sourceID, trajectoryID string) (TrajectoryContext, error) {
	row := i.db.QueryRowContext(ctx, `SELECT
	    r.raw,r.line,r.byte_offset,r.byte_length,c.raw,c.line,c.byte_offset,c.byte_length,
	    g.raw,g.line,g.byte_offset,g.byte_length,t.raw,t.line,t.byte_offset,t.byte_length
    FROM trajectories t JOIN groups g ON g.source_id=t.source_id AND g.id=t.group_id
    JOIN cases c ON c.source_id=g.source_id AND c.id=g.case_id
    JOIN runs r ON r.source_id=c.source_id AND r.id=c.run_id
    WHERE t.source_id=? AND t.id=?`, sourceID, trajectoryID)
	var result TrajectoryContext
	var runRaw, caseRaw, groupRaw, trajectoryRaw []byte
	err := row.Scan(&runRaw, &result.Run.Line, &result.Run.ByteOffset, &result.Run.ByteLength,
		&caseRaw, &result.Case.Line, &result.Case.ByteOffset, &result.Case.ByteLength,
		&groupRaw, &result.Group.Line, &result.Group.ByteOffset, &result.Group.ByteLength,
		&trajectoryRaw, &result.Trajectory.Line, &result.Trajectory.ByteOffset, &result.Trajectory.ByteLength)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, fmt.Errorf("read trajectory context: %w", err)
	}
	if err := decodeRaw(runRaw, &result.Run.Value, &result.Run.Raw); err != nil {
		return result, err
	}
	if err := decodeRaw(caseRaw, &result.Case.Value, &result.Case.Raw); err != nil {
		return result, err
	}
	if err := decodeRaw(groupRaw, &result.Group.Value, &result.Group.Raw); err != nil {
		return result, err
	}
	if err := decodeRaw(trajectoryRaw, &result.Trajectory.Value, &result.Trajectory.Raw); err != nil {
		return result, err
	}
	return result, nil
}

func (i *Index) Groups(ctx context.Context, sourceID string) ([]IndexedRecord[*model.Group], error) {
	rows, err := i.db.QueryContext(ctx, `SELECT raw,line,byte_offset,byte_length FROM groups WHERE source_id=? ORDER BY line`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("query groups: %w", err)
	}
	defer rows.Close()
	var result []IndexedRecord[*model.Group]
	for rows.Next() {
		var item IndexedRecord[*model.Group]
		var raw []byte
		if err := rows.Scan(&raw, &item.Line, &item.ByteOffset, &item.ByteLength); err != nil {
			return nil, err
		}
		if err := decodeRaw(raw, &item.Value, &item.Raw); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (i *Index) Trajectories(ctx context.Context, sourceID string) ([]IndexedRecord[*model.Trajectory], error) {
	page, err := i.TrajectoriesPage(ctx, sourceID, MaxQueryRecords)
	return page.Items, err
}

func (i *Index) TrajectoriesPage(ctx context.Context, sourceID string, limit int) (RecordPage[*model.Trajectory], error) {
	limit = boundedQueryLimit(limit)
	var page RecordPage[*model.Trajectory]
	if err := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM trajectories WHERE source_id=?`, sourceID).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count trajectories: %w", err)
	}
	rows, err := i.db.QueryContext(ctx, `SELECT raw,line,byte_offset,byte_length FROM trajectories WHERE source_id=? ORDER BY line LIMIT ?`, sourceID, limit)
	if err != nil {
		return page, fmt.Errorf("query trajectories: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item IndexedRecord[*model.Trajectory]
		var raw []byte
		if err := rows.Scan(&raw, &item.Line, &item.ByteOffset, &item.ByteLength); err != nil {
			return page, err
		}
		if err := decodeRaw(raw, &item.Value, &item.Raw); err != nil {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}

func (i *Index) FirstTrajectory(ctx context.Context, sourceID string) (IndexedRecord[*model.Trajectory], error) {
	page, err := i.TrajectoriesPage(ctx, sourceID, 1)
	if err != nil {
		return IndexedRecord[*model.Trajectory]{}, err
	}
	if len(page.Items) == 0 {
		return IndexedRecord[*model.Trajectory]{}, ErrNotFound
	}
	return page.Items[0], nil
}

func (i *Index) Events(ctx context.Context, query EventQuery) (EventPage, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	where := []string{"source_id=?", "trajectory_id=?"}
	args := []any{query.SourceID, query.TrajectoryID}
	if len(query.Kinds) != 0 {
		where = append(where, "kind IN ("+strings.TrimRight(strings.Repeat("?,", len(query.Kinds)), ",")+")")
		for _, kind := range query.Kinds {
			args = append(args, kind)
		}
	}
	if query.Query != "" {
		where = append(where, `search_text LIKE ? ESCAPE '\' COLLATE NOCASE`)
		args = append(args, "%"+escapeLike(query.Query)+"%")
	}
	var page EventPage
	if err := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE `+strings.Join(where, " AND "), args...).Scan(&page.Total); err != nil {
		return EventPage{}, fmt.Errorf("count events: %w", err)
	}
	if query.AfterSequence != nil {
		where = append(where, "sequence>?")
		args = append(args, *query.AfterSequence)
	}
	byteArgs := append(append([]any(nil), args...), limit+1)
	byteQuery := `SELECT COALESCE(SUM(raw_bytes),0) FROM (SELECT length(raw) raw_bytes FROM events WHERE ` + strings.Join(where, " AND ") + ` ORDER BY sequence LIMIT ?)`
	if err := i.db.QueryRowContext(ctx, byteQuery, byteArgs...).Scan(&page.RawBytes); err != nil {
		return EventPage{}, fmt.Errorf("size event page: %w", err)
	}
	if page.RawBytes > MaxQueryRawBytes {
		return EventPage{}, fmt.Errorf("%w: event page is %d raw bytes; maximum is %d", ErrResultTooLarge, page.RawBytes, MaxQueryRawBytes)
	}
	args = append(args, limit+1)
	rows, err := i.db.QueryContext(ctx, `SELECT raw,line,record_byte_offset,record_byte_length,source_path,source_line,byte_offset,byte_length
	    FROM events WHERE `+strings.Join(where, " AND ")+` ORDER BY sequence LIMIT ?`, args...)
	if err != nil {
		return EventPage{}, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()
	page.Events = make([]IndexedRecord[*model.Event], 0, limit)
	var readBytes int64
	for rows.Next() {
		var item IndexedRecord[*model.Event]
		var raw []byte
		var sourcePath sql.NullString
		var sourceLine, sourceOffset, sourceLength sql.NullInt64
		if err := rows.Scan(&raw, &item.Line, &item.ByteOffset, &item.ByteLength, &sourcePath, &sourceLine, &sourceOffset, &sourceLength); err != nil {
			return EventPage{}, err
		}
		readBytes += int64(len(raw))
		if readBytes > MaxQueryRawBytes {
			return EventPage{}, fmt.Errorf("%w: event page exceeded maximum %d raw bytes while reading", ErrResultTooLarge, MaxQueryRawBytes)
		}
		if err := decodeRaw(raw, &item.Value, &item.Raw); err != nil {
			return EventPage{}, err
		}
		if sourcePath.Valid {
			item.Value.Source = &model.SourceLocation{Path: sourcePath.String}
			if sourceLine.Valid {
				value := sourceLine.Int64
				item.Value.Source.Line = &value
			}
			if sourceOffset.Valid {
				value := sourceOffset.Int64
				item.Value.Source.ByteOffset = &value
			}
			if sourceLength.Valid {
				value := sourceLength.Int64
				item.Value.Source.ByteLength = &value
			}
		}
		page.Events = append(page.Events, item)
	}
	if err := rows.Err(); err != nil {
		return EventPage{}, err
	}
	page.RawBytes = readBytes
	if len(page.Events) > limit {
		page.RawBytes -= page.Events[limit].ByteLength
		if page.RawBytes < 0 {
			page.RawBytes = 0
		}
		page.Events = page.Events[:limit]
		next := page.Events[len(page.Events)-1].Value.Sequence
		page.NextSequence = &next
	}
	return page, nil
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

func (i *Index) Signals(ctx context.Context, sourceID, trajectoryID string) ([]IndexedRecord[*model.Signal], error) {
	page, err := i.SignalsPage(ctx, sourceID, trajectoryID, 0, MaxQueryRecords)
	return page.Items, err
}

func (i *Index) SignalsPage(ctx context.Context, sourceID, trajectoryID string, offset int64, limit int) (RecordPage[*model.Signal], error) {
	limit = boundedQueryLimit(limit)
	if offset < 0 {
		offset = 0
	}
	page := RecordPage[*model.Signal]{Offset: offset}
	if err := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM signals WHERE source_id=? AND trajectory_id=?`, sourceID, trajectoryID).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count signals: %w", err)
	}
	if err := i.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(raw_bytes),0) FROM (SELECT length(raw) raw_bytes FROM signals WHERE source_id=? AND trajectory_id=? ORDER BY line,id LIMIT ? OFFSET ?)`, sourceID, trajectoryID, limit, offset).Scan(&page.RawBytes); err != nil {
		return page, fmt.Errorf("size signal page: %w", err)
	}
	if page.RawBytes > MaxQueryRawBytes {
		return page, fmt.Errorf("%w: signal page is %d raw bytes; maximum is %d", ErrResultTooLarge, page.RawBytes, MaxQueryRawBytes)
	}
	rows, err := i.db.QueryContext(ctx, `SELECT raw,line,byte_offset,byte_length FROM signals WHERE source_id=? AND trajectory_id=? ORDER BY line,id LIMIT ? OFFSET ?`, sourceID, trajectoryID, limit, offset)
	if err != nil {
		return page, fmt.Errorf("query signals: %w", err)
	}
	defer rows.Close()
	var readBytes int64
	for rows.Next() {
		var item IndexedRecord[*model.Signal]
		var raw []byte
		if err := rows.Scan(&raw, &item.Line, &item.ByteOffset, &item.ByteLength); err != nil {
			return page, err
		}
		readBytes += int64(len(raw))
		if readBytes > MaxQueryRawBytes {
			return page, fmt.Errorf("%w: signal page exceeded maximum %d raw bytes while reading", ErrResultTooLarge, MaxQueryRawBytes)
		}
		if err := decodeRaw(raw, &item.Value, &item.Raw); err != nil {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	page.RawBytes = readBytes
	return page, rows.Err()
}

func (i *Index) Artifacts(ctx context.Context, sourceID, trajectoryID string) ([]IndexedRecord[*model.Artifact], error) {
	page, err := i.ArtifactsPage(ctx, sourceID, trajectoryID, 0, MaxQueryRecords)
	return page.Items, err
}

func (i *Index) ArtifactsPage(ctx context.Context, sourceID, trajectoryID string, offset int64, limit int) (RecordPage[*model.Artifact], error) {
	limit = boundedQueryLimit(limit)
	if offset < 0 {
		offset = 0
	}
	page := RecordPage[*model.Artifact]{Offset: offset}
	if err := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts WHERE source_id=? AND trajectory_id=?`, sourceID, trajectoryID).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count artifacts: %w", err)
	}
	if err := i.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(raw_bytes),0) FROM (SELECT length(raw) raw_bytes FROM artifacts WHERE source_id=? AND trajectory_id=? ORDER BY line,id LIMIT ? OFFSET ?)`, sourceID, trajectoryID, limit, offset).Scan(&page.RawBytes); err != nil {
		return page, fmt.Errorf("size artifact page: %w", err)
	}
	if page.RawBytes > MaxQueryRawBytes {
		return page, fmt.Errorf("%w: artifact page is %d raw bytes; maximum is %d", ErrResultTooLarge, page.RawBytes, MaxQueryRawBytes)
	}
	rows, err := i.db.QueryContext(ctx, `SELECT raw,line,byte_offset,byte_length FROM artifacts WHERE source_id=? AND trajectory_id=? ORDER BY line,id LIMIT ? OFFSET ?`, sourceID, trajectoryID, limit, offset)
	if err != nil {
		return page, fmt.Errorf("query artifacts: %w", err)
	}
	defer rows.Close()
	var readBytes int64
	for rows.Next() {
		var item IndexedRecord[*model.Artifact]
		var raw []byte
		if err := rows.Scan(&raw, &item.Line, &item.ByteOffset, &item.ByteLength); err != nil {
			return page, err
		}
		readBytes += int64(len(raw))
		if readBytes > MaxQueryRawBytes {
			return page, fmt.Errorf("%w: artifact page exceeded maximum %d raw bytes while reading", ErrResultTooLarge, MaxQueryRawBytes)
		}
		if err := decodeRaw(raw, &item.Value, &item.Raw); err != nil {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	page.RawBytes = readBytes
	return page, rows.Err()
}

func (i *Index) Artifact(ctx context.Context, sourceID, trajectoryID, artifactID string) (IndexedRecord[*model.Artifact], error) {
	var item IndexedRecord[*model.Artifact]
	var raw []byte
	err := i.db.QueryRowContext(ctx, `SELECT raw,line,byte_offset,byte_length FROM artifacts WHERE source_id=? AND trajectory_id=? AND id=?`, sourceID, trajectoryID, artifactID).
		Scan(&raw, &item.Line, &item.ByteOffset, &item.ByteLength)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrNotFound
	}
	if err != nil {
		return item, fmt.Errorf("read artifact: %w", err)
	}
	if err := decodeRaw(raw, &item.Value, &item.Raw); err != nil {
		return item, err
	}
	return item, nil
}

func boundedQueryLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > MaxQueryRecords {
		return MaxQueryRecords
	}
	return limit
}

func (i *Index) GroupSummaries(ctx context.Context, sourceID, groupID string) ([]TrajectorySummary, error) {
	page, err := i.GroupSummariesPage(ctx, sourceID, groupID, MaxQueryRecords)
	if err != nil {
		return nil, err
	}
	if page.Total > int64(len(page.Items)) {
		return nil, fmt.Errorf("%w: group has %d trajectories; maximum is %d", ErrResultTooLarge, page.Total, MaxQueryRecords)
	}
	return page.Items, nil
}

func (i *Index) GroupSummariesPage(ctx context.Context, sourceID, groupID string, limit int) (SummaryPage, error) {
	limit = boundedQueryLimit(limit)
	var page SummaryPage
	if err := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM trajectories WHERE source_id=? AND group_id=?`, sourceID, groupID).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count group trajectories: %w", err)
	}
	var signalCount int64
	if err := i.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM signals s JOIN trajectories t ON t.source_id=s.source_id AND t.id=s.trajectory_id WHERE t.source_id=? AND t.group_id=?`, sourceID, groupID).Scan(&signalCount); err != nil {
		return page, fmt.Errorf("count group signals: %w", err)
	}
	if signalCount > MaxGroupSummarySignals {
		return page, fmt.Errorf("%w: group has %d signals; maximum is %d", ErrResultTooLarge, signalCount, MaxGroupSummarySignals)
	}
	if err := i.db.QueryRowContext(ctx, `WITH selected AS (
	    SELECT source_id,id,raw FROM trajectories WHERE source_id=? AND group_id=? ORDER BY line LIMIT ?
	  ) SELECT COALESCE(SUM(length(t.raw)+COALESCE(length(s.raw),0)),0)
	  FROM selected t LEFT JOIN signals s ON s.source_id=t.source_id AND s.trajectory_id=t.id`, sourceID, groupID, limit).Scan(&page.RawBytes); err != nil {
		return page, fmt.Errorf("size group summaries: %w", err)
	}
	if page.RawBytes > MaxGroupSummaryRawBytes {
		return page, fmt.Errorf("%w: group summary query is %d raw bytes; maximum is %d", ErrResultTooLarge, page.RawBytes, MaxGroupSummaryRawBytes)
	}
	// Aggregate each child table before joining it. This prevents event/signal/
	// artifact multiplication while retaining all signal rows in one query.
	rows, err := i.db.QueryContext(ctx, `WITH
	    event_summary AS (SELECT source_id,trajectory_id,COUNT(*) count,MIN(sequence) first_sequence,MAX(sequence) last_sequence,
	      SUM(CASE WHEN kind='error' THEN 1 ELSE 0 END) error_count FROM events WHERE source_id=? GROUP BY source_id,trajectory_id),
	    signal_summary AS (SELECT source_id,trajectory_id,COUNT(*) count FROM signals WHERE source_id=? GROUP BY source_id,trajectory_id),
	    artifact_summary AS (SELECT source_id,trajectory_id,COUNT(*) count FROM artifacts WHERE source_id=? GROUP BY source_id,trajectory_id)
    SELECT t.id,t.raw,t.line,t.byte_offset,t.byte_length,
      COALESCE(e.count,0),e.first_sequence,e.last_sequence,COALESCE(e.error_count,0),
      COALESCE(ss.count,0),COALESCE(a.count,0),s.name,s.raw,s.line
    FROM trajectories t
    LEFT JOIN event_summary e ON e.source_id=t.source_id AND e.trajectory_id=t.id
    LEFT JOIN signal_summary ss ON ss.source_id=t.source_id AND ss.trajectory_id=t.id
    LEFT JOIN artifact_summary a ON a.source_id=t.source_id AND a.trajectory_id=t.id
    LEFT JOIN signals s ON s.source_id=t.source_id AND s.trajectory_id=t.id
	    WHERE t.rowid IN (SELECT rowid FROM trajectories WHERE source_id=? AND group_id=? ORDER BY line LIMIT ?)
	      AND t.source_id=? AND t.group_id=? ORDER BY t.line,s.line`, sourceID, sourceID, sourceID, sourceID, groupID, limit, sourceID, groupID)
	if err != nil {
		return page, fmt.Errorf("query group summaries: %w", err)
	}
	defer rows.Close()
	var result []TrajectorySummary
	byID := make(map[string]int)
	var readBytes int64
	for rows.Next() {
		var trajectoryID string
		var raw, signalRaw []byte
		var signalName sql.NullString
		var signalLine sql.NullInt64
		var first, last sql.NullInt64
		var item TrajectorySummary
		if err := rows.Scan(&trajectoryID, &raw, &item.Trajectory.Line, &item.Trajectory.ByteOffset, &item.Trajectory.ByteLength,
			&item.EventCount, &first, &last, &item.ErrorCount, &item.SignalCount, &item.ArtifactCount, &signalName, &signalRaw, &signalLine); err != nil {
			return page, err
		}
		readBytes += int64(len(raw) + len(signalRaw))
		if readBytes > MaxGroupSummaryRawBytes {
			return page, fmt.Errorf("%w: group summaries exceeded maximum %d raw bytes while reading", ErrResultTooLarge, MaxGroupSummaryRawBytes)
		}
		index, exists := byID[trajectoryID]
		if !exists {
			if first.Valid {
				value := first.Int64
				item.FirstSequence = &value
			}
			if last.Valid {
				value := last.Int64
				item.LastSequence = &value
			}
			if err := decodeRaw(raw, &item.Trajectory.Value, &item.Trajectory.Raw); err != nil {
				return page, err
			}
			item.Status = item.Trajectory.Value.Status
			item.Termination = item.Trajectory.Value.Termination
			item.Signals = make(map[string]json.RawMessage)
			item.signalUnits = make(map[string]string)
			result = append(result, item)
			index = len(result) - 1
			byID[trajectoryID] = index
		}
		if signalName.Valid {
			value, unit, valueErr := signalValue(signalRaw)
			if valueErr != nil {
				return page, fmt.Errorf("decode signal %q: %w", signalName.String, valueErr)
			}
			name := canonicalMetricName(signalName.String)
			result[index].Signals[name] = value
			result[index].signalUnits[name] = strings.ToLower(unit)
		}
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	for n := range result {
		normalizeSummary(&result[n])
	}
	page.Items = result
	page.RawBytes = readBytes
	return page, nil
}

func canonicalMetricName(name string) string {
	lower := strings.ToLower(name)
	switch lower {
	case "reward", "pass", "success", "token_count", "total_tokens", "tokens", "error_count", "latency_ms", "duration_ms", "latency_seconds", "duration_seconds", "latency", "duration":
		return lower
	default:
		return name
	}
}

func (i *Index) Records(ctx context.Context, sourceID string, afterOrdinal int64, limit int) ([]RawRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := i.db.QueryContext(ctx, `SELECT ordinal,record_type,record_id,byte_offset,byte_length,raw FROM records
    WHERE source_id=? AND ordinal>? ORDER BY ordinal LIMIT ?`, sourceID, afterOrdinal, limit)
	if err != nil {
		return nil, fmt.Errorf("query canonical records: %w", err)
	}
	defer rows.Close()
	var result []RawRecord
	for rows.Next() {
		var item RawRecord
		var kind string
		var raw []byte
		if err := rows.Scan(&item.Ordinal, &kind, &item.ID, &item.ByteOffset, &item.ByteLength, &raw); err != nil {
			return nil, err
		}
		item.Type = model.RecordType(kind)
		item.Raw = append(json.RawMessage(nil), raw...)
		result = append(result, item)
	}
	return result, rows.Err()
}

func decodeRaw[T any](raw []byte, value *T, retained *json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode indexed record: %w", err)
	}
	*retained = append(json.RawMessage(nil), raw...)
	return nil
}

func encodeTime(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixNano()
}
