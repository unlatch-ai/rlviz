package index

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

const DefaultProgressiveBatchRecords = 128

var ErrSourceChanged = errors.New("source changed while indexing")

type ProgressiveOptions struct {
	BatchRecords int
	PollInterval time.Duration
	Ready        func(SourceInfo)
}

// ProgressiveFile incrementally commits an initially uncached canonical file.
// It tails a valid incomplete stream until a complete record appears. Callers
// must not use this to replace a valid cache; Replace provides that atomic path.
func (i *Index) ProgressiveFile(ctx context.Context, source Source, options ProgressiveOptions) (result SourceInfo, err error) {
	if source.ID == "" || source.Path == "" {
		return result, errors.New("source id and path are required")
	}
	if source.Adapter != "" {
		return result, errors.New("progressive indexing only supports canonical files")
	}
	batchLimit := options.BatchRecords
	if batchLimit <= 0 {
		batchLimit = DefaultProgressiveBatchRecords
	}
	interval := options.PollInterval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	file, err := os.Open(source.Path)
	if err != nil {
		return result, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return result, err
	}
	prefixLength := openedInfo.Size()
	if prefixLength > 4096 {
		prefixLength = 4096
	}
	prefix, err := progressivePrefix(file, prefixLength)
	if err != nil {
		return result, err
	}
	tail, err := progressiveTail(file, openedInfo.Size())
	if err != nil {
		return result, err
	}
	source.Size, source.ModTime = openedInfo.Size(), openedInfo.ModTime()
	indexedAt := time.Now().UTC()
	if err := i.beginProgressive(ctx, source, indexedAt); err != nil {
		return result, err
	}
	readySent := false
	defer func() {
		if err != nil {
			message := err.Error()
			if errors.Is(err, context.Canceled) {
				message = "indexing canceled"
			}
			_ = i.SetIndexState(context.Background(), source.ID, IndexFailed, message)
		}
	}()

	decoder := model.NewDecoder(file)
	validator := model.NewValidator()
	batch := make([]*model.Record, 0, batchLimit)
	var ordinal, recordCount int64
	var complete *model.Complete
	var completeRaw []byte
	var runSeen, caseSeen, groupSeen, trajectorySeen bool
	var eventCount int64
	lastSize, lastModified := openedInfo.Size(), openedInfo.ModTime()
	publishReady := func(requireEvent bool) error {
		if readySent || !runSeen || !caseSeen || !groupSeen || !trajectorySeen || (requireEvent && eventCount == 0) {
			return nil
		}
		info, readErr := i.Source(ctx, source.ID)
		if readErr != nil {
			return readErr
		}
		readySent = true
		if options.Ready != nil {
			options.Ready(info)
		}
		return nil
	}

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := i.appendProgressive(ctx, source, batch, ordinal-int64(len(batch))+1, recordCount); err != nil {
			return err
		}
		batch = batch[:0]
		return publishReady(true)
	}

	for {
		record, decodeErr := decoder.NextContext(ctx)
		if errors.Is(decodeErr, io.EOF) {
			if err = flush(); err != nil {
				return result, err
			}
			if err = publishReady(false); err != nil {
				return result, err
			}
			if finishErr := validator.Finish(); finishErr == nil {
				if complete == nil {
					return result, errors.New("completed validator without complete record")
				}
				if err = i.finishProgressive(ctx, source.ID, complete, completeRaw, lastSize, lastModified); err != nil {
					return result, err
				}
				result, err = i.Source(ctx, source.ID)
				if err == nil && !readySent && options.Ready != nil {
					options.Ready(result)
				}
				return result, err
			}
			lastSize, lastModified, tail, err = waitForGrowth(ctx, file, source.Path, prefix, tail, prefixLength, lastSize, lastModified, interval)
			if err != nil {
				return result, err
			}
			if err = i.updateProgressiveMetadata(ctx, source.ID, lastSize, lastModified); err != nil {
				return result, err
			}
			continue
		}
		if decodeErr != nil {
			return result, fmt.Errorf("decode source: %w", decodeErr)
		}
		if err = validator.Add(record); err != nil {
			return result, fmt.Errorf("line %d: %w", record.Line, err)
		}
		ordinal++
		batch = append(batch, record)
		if record.Type != model.RecordComplete {
			recordCount++
		}
		switch value := record.Value.(type) {
		case *model.Run:
			runSeen = true
		case *model.Case:
			caseSeen = true
		case *model.Group:
			groupSeen = true
		case *model.Trajectory:
			trajectorySeen = true
		case *model.Event:
			eventCount++
		case *model.Complete:
			complete = value
			completeRaw = append([]byte(nil), record.Raw...)
		}
		if len(batch) >= batchLimit {
			if err = flush(); err != nil {
				return result, err
			}
		}
	}
}

func (i *Index) beginProgressive(ctx context.Context, source Source, indexedAt time.Time) error {
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sources WHERE id=?`, source.ID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sources
    (id,path,adapter,fingerprint,size,mod_time_ns,indexed_at_ns,records,warnings,complete_raw,index_state,index_error)
    VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, source.ID, source.Path, source.Adapter, source.Fingerprint, source.Size,
		encodeTime(source.ModTime), indexedAt.UnixNano(), 0, 0, []byte{}, Indexing, "")
	if err != nil {
		return fmt.Errorf("begin progressive source: %w", err)
	}
	return tx.Commit()
}

func (i *Index) appendProgressive(ctx context.Context, source Source, records []*model.Record, firstOrdinal, recordCount int64) error {
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for offset, record := range records {
		if err := insertRecord(ctx, tx, source, firstOrdinal+int64(offset), record.ByteOffset, record.ByteLength, record); err != nil {
			return fmt.Errorf("index line %d: %w", record.Line, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sources SET records=? WHERE id=? AND index_state=?`, recordCount, source.ID, Indexing); err != nil {
		return err
	}
	return tx.Commit()
}

func (i *Index) finishProgressive(ctx context.Context, sourceID string, complete *model.Complete, raw []byte, size int64, modified time.Time) error {
	_, err := i.db.ExecContext(ctx, `UPDATE sources SET size=?,mod_time_ns=?,records=?,warnings=?,complete_raw=?,index_state=?,index_error='' WHERE id=?`,
		size, encodeTime(modified), complete.Records, complete.Warnings, raw, IndexComplete, sourceID)
	return err
}

func (i *Index) updateProgressiveMetadata(ctx context.Context, sourceID string, size int64, modified time.Time) error {
	_, err := i.db.ExecContext(ctx, `UPDATE sources SET size=?,mod_time_ns=? WHERE id=? AND index_state=?`, size, encodeTime(modified), sourceID, Indexing)
	return err
}

func (i *Index) SetIndexState(ctx context.Context, sourceID string, state IndexState, message string) error {
	if state != Indexing && state != IndexComplete && state != IndexRefreshing && state != IndexFailed {
		return fmt.Errorf("unsupported index state %q", state)
	}
	_, err := i.db.ExecContext(ctx, `UPDATE sources SET index_state=?,index_error=? WHERE id=?`, state, message, sourceID)
	return err
}

func progressivePrefix(file *os.File, size int64) ([]byte, error) {
	length := size
	if length > 4096 {
		length = 4096
	}
	data := make([]byte, length)
	if length > 0 {
		if _, err := file.ReadAt(data, 0); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
	}
	sum := sha256.Sum256(data)
	return sum[:], nil
}

func progressiveTail(file *os.File, size int64) ([]byte, error) {
	start := size - 4096
	if start < 0 {
		start = 0
	}
	data := make([]byte, size-start)
	if len(data) > 0 {
		if _, err := file.ReadAt(data, start); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
	}
	sum := sha256.Sum256(data)
	return sum[:], nil
}

func waitForGrowth(ctx context.Context, opened *os.File, path string, prefix, tail []byte, prefixLength, previousSize int64, previousModified time.Time, interval time.Duration) (int64, time.Time, []byte, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return previousSize, previousModified, tail, ctx.Err()
		case <-ticker.C:
			pathInfo, err := os.Stat(path)
			if err != nil {
				return previousSize, previousModified, tail, fmt.Errorf("%w: %v", ErrSourceChanged, err)
			}
			openedInfo, err := opened.Stat()
			if err != nil {
				return previousSize, previousModified, tail, err
			}
			if !os.SameFile(pathInfo, openedInfo) || pathInfo.Size() < previousSize {
				return previousSize, previousModified, tail, ErrSourceChanged
			}
			currentPrefix, err := progressivePrefix(opened, prefixLength)
			if err != nil {
				return previousSize, previousModified, tail, err
			}
			if !equalBytes(prefix, currentPrefix) {
				return previousSize, previousModified, tail, ErrSourceChanged
			}
			currentTail, err := progressiveTail(opened, previousSize)
			if err != nil {
				return previousSize, previousModified, tail, err
			}
			if !equalBytes(tail, currentTail) {
				return previousSize, previousModified, tail, ErrSourceChanged
			}
			if pathInfo.Size() > previousSize {
				newTail, tailErr := progressiveTail(opened, pathInfo.Size())
				return pathInfo.Size(), pathInfo.ModTime(), newTail, tailErr
			}
			if !pathInfo.ModTime().Equal(previousModified) {
				return previousSize, previousModified, tail, ErrSourceChanged
			}
		}
	}
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var different byte
	for i := range left {
		different |= left[i] ^ right[i]
	}
	return different == 0
}
