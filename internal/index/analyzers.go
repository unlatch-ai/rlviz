package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/TheSnakeFang/rlviz/internal/analyzers"
	"github.com/TheSnakeFang/rlviz/internal/model"
)

// LoopRetryAnalysis returns a validated cached analysis when the analyzer and
// normalized canonical input digests match. Source replacement cascades cached
// rows, while the input digest also protects against partial or stale results.
func (i *Index) LoopRetryAnalysis(ctx context.Context, sourceID, trajectoryID string) (AnalysisResult, error) {
	input, err := i.analyzerInput(ctx, sourceID, trajectoryID)
	if err != nil {
		return AnalysisResult{}, err
	}
	inputDigest, err := analyzers.InputDigest(input)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("digest analyzer input: %w", err)
	}
	expected := analyzers.Provenance{Name: analyzers.LoopRetryName, Version: analyzers.LoopRetryVersion, Digest: analyzers.LoopRetryDigest, InputDigest: inputDigest}

	result, err := i.cachedAnalysis(ctx, sourceID, trajectoryID, input, expected)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return AnalysisResult{}, err
	}

	output, err := (analyzers.LoopRetry{}).Analyze(ctx, input)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("analyze loop/retry behavior: %w", err)
	}
	data, err := json.Marshal(output)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("encode analyzer result: %w", err)
	}
	analyzedAt := time.Now().UTC()
	_, err = i.db.ExecContext(ctx, `INSERT OR REPLACE INTO analyzer_results
    (source_id,trajectory_id,name,version,digest,input_digest,analyzed_at_ns,output_json)
    VALUES(?,?,?,?,?,?,?,?)`, sourceID, trajectoryID, expected.Name, expected.Version, expected.Digest,
		expected.InputDigest, analyzedAt.UnixNano(), data)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("cache analyzer result: %w", err)
	}
	return AnalysisResult{Output: output, AnalyzedAt: analyzedAt}, nil
}

func (i *Index) cachedAnalysis(ctx context.Context, sourceID, trajectoryID string, input analyzers.Input, expected analyzers.Provenance) (AnalysisResult, error) {
	var data []byte
	var analyzedNS int64
	err := i.db.QueryRowContext(ctx, `SELECT output_json,analyzed_at_ns FROM analyzer_results
    WHERE source_id=? AND trajectory_id=? AND name=? AND version=? AND digest=? AND input_digest=?`,
		sourceID, trajectoryID, expected.Name, expected.Version, expected.Digest, expected.InputDigest).Scan(&data, &analyzedNS)
	if errors.Is(err, sql.ErrNoRows) {
		return AnalysisResult{}, ErrNotFound
	}
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("read analyzer result: %w", err)
	}
	var output analyzers.Output
	if err := json.Unmarshal(data, &output); err != nil {
		return AnalysisResult{}, fmt.Errorf("decode cached analyzer result: %w", err)
	}
	if err := analyzers.ValidateOutput(output, input, expected); err != nil {
		return AnalysisResult{}, fmt.Errorf("validate cached analyzer result: %w", err)
	}
	return AnalysisResult{Output: output, Cached: true, AnalyzedAt: time.Unix(0, analyzedNS).UTC()}, nil
}

func (i *Index) analyzerInput(ctx context.Context, sourceID, trajectoryID string) (analyzers.Input, error) {
	var exists int
	if err := i.db.QueryRowContext(ctx, `SELECT 1 FROM trajectories WHERE source_id=? AND id=?`, sourceID, trajectoryID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return analyzers.Input{}, ErrNotFound
	} else if err != nil {
		return analyzers.Input{}, fmt.Errorf("find analysis trajectory: %w", err)
	}
	var eventCount, signalCount, rawBytes int64
	if err := i.db.QueryRowContext(ctx, `SELECT
	    (SELECT COUNT(*) FROM events WHERE source_id=? AND trajectory_id=?),
	    (SELECT COUNT(*) FROM signals WHERE source_id=? AND trajectory_id=?),
	    COALESCE((SELECT SUM(length(raw)) FROM events WHERE source_id=? AND trajectory_id=?),0) +
	      COALESCE((SELECT SUM(length(raw)) FROM signals WHERE source_id=? AND trajectory_id=?),0)`,
		sourceID, trajectoryID, sourceID, trajectoryID, sourceID, trajectoryID, sourceID, trajectoryID).
		Scan(&eventCount, &signalCount, &rawBytes); err != nil {
		return analyzers.Input{}, fmt.Errorf("size analyzer input: %w", err)
	}
	if eventCount > analyzers.MaxInputEvents {
		return analyzers.Input{}, fmt.Errorf("%w: analyzer events are %d; maximum is %d", ErrResultTooLarge, eventCount, analyzers.MaxInputEvents)
	}
	if signalCount > analyzers.MaxInputSignals {
		return analyzers.Input{}, fmt.Errorf("%w: analyzer signals are %d; maximum is %d", ErrResultTooLarge, signalCount, analyzers.MaxInputSignals)
	}
	if rawBytes > analyzers.MaxInputBytes {
		return analyzers.Input{}, fmt.Errorf("%w: analyzer raw input is %d bytes; maximum encoded input is %d", ErrResultTooLarge, rawBytes, analyzers.MaxInputBytes)
	}
	input := analyzers.Input{APIVersion: analyzers.APIVersion, Operation: analyzers.OperationAnalyze, TrajectoryID: trajectoryID}
	rows, err := i.db.QueryContext(ctx, `SELECT raw FROM events WHERE source_id=? AND trajectory_id=? ORDER BY sequence,id LIMIT ?`, sourceID, trajectoryID, analyzers.MaxInputEvents+1)
	if err != nil {
		return analyzers.Input{}, fmt.Errorf("query analyzer events: %w", err)
	}
	for rows.Next() {
		if len(input.Events) >= analyzers.MaxInputEvents {
			rows.Close()
			return analyzers.Input{}, fmt.Errorf("%w: analyzer events exceed maximum %d", ErrResultTooLarge, analyzers.MaxInputEvents)
		}
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return analyzers.Input{}, err
		}
		var event model.Event
		if err := json.Unmarshal(raw, &event); err != nil {
			rows.Close()
			return analyzers.Input{}, fmt.Errorf("decode analyzer event: %w", err)
		}
		input.Events = append(input.Events, event)
	}
	if err := rows.Close(); err != nil {
		return analyzers.Input{}, err
	}
	if err := rows.Err(); err != nil {
		return analyzers.Input{}, err
	}

	rows, err = i.db.QueryContext(ctx, `SELECT raw FROM signals WHERE source_id=? AND trajectory_id=? ORDER BY id LIMIT ?`, sourceID, trajectoryID, analyzers.MaxInputSignals+1)
	if err != nil {
		return analyzers.Input{}, fmt.Errorf("query analyzer signals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		if len(input.Signals) >= analyzers.MaxInputSignals {
			return analyzers.Input{}, fmt.Errorf("%w: analyzer signals exceed maximum %d", ErrResultTooLarge, analyzers.MaxInputSignals)
		}
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return analyzers.Input{}, err
		}
		var signal model.Signal
		if err := json.Unmarshal(raw, &signal); err != nil {
			return analyzers.Input{}, fmt.Errorf("decode analyzer signal: %w", err)
		}
		input.Signals = append(input.Signals, signal)
	}
	if err := rows.Err(); err != nil {
		return analyzers.Input{}, err
	}
	if err := analyzers.ValidateInput(input); err != nil {
		return analyzers.Input{}, fmt.Errorf("validate analyzer input: %w", err)
	}
	return input, nil
}
