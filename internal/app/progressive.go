package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/plugins"
	"github.com/TheSnakeFang/rlviz/internal/server"
)

type SourceIndexer struct {
	ctx   context.Context
	store *rolloutindex.Index

	mu   sync.Mutex
	jobs map[string]*sourceIndexJob
}

type sourceIndexJob struct {
	ready chan struct{}
	once  sync.Once
	mu    sync.Mutex
	value IndexedSource
	err   error
}

func NewSourceIndexer(ctx context.Context, store *rolloutindex.Index) *SourceIndexer {
	if ctx == nil {
		ctx = context.Background()
	}
	return &SourceIndexer{ctx: ctx, store: store, jobs: make(map[string]*sourceIndexJob)}
}

// Index returns once an initial canonical source has a committed header and
// event batch. The per-source background job continues indexing/tailing. A
// valid stale cache is returned immediately while an atomic refresh runs.
func (indexer *SourceIndexer) Index(ctx context.Context, path, adapterPath string) (IndexedSource, error) {
	if indexer == nil || indexer.store == nil {
		return IndexedSource{}, errors.New("source indexer requires a rollout index")
	}
	if adapterPath != "" {
		return IndexSource(ctx, indexer.store, path, adapterPath)
	}
	source, err := canonicalSource(path)
	if err != nil {
		return IndexedSource{}, err
	}
	status, err := indexer.store.Status(ctx, source)
	if err != nil {
		return IndexedSource{}, err
	}
	if status.State == rolloutindex.CacheFresh && status.Cached != nil {
		return IndexedSource{Info: *status.Cached}, nil
	}

	if status.Cached != nil && len(status.Cached.CompleteRaw) != 0 {
		if err := indexer.startRefresh(source); err != nil {
			return IndexedSource{}, err
		}
		cached, readErr := indexer.store.Source(ctx, source.ID)
		if readErr != nil {
			return IndexedSource{}, readErr
		}
		return IndexedSource{Info: cached}, nil
	}

	job := indexer.startProgressive(source)
	select {
	case <-ctx.Done():
		return IndexedSource{}, ctx.Err()
	case <-job.ready:
		job.mu.Lock()
		defer job.mu.Unlock()
		return job.value, job.err
	}
}

func canonicalSource(path string) (rolloutindex.Source, error) {
	resolved, err := ValidateSource(path)
	if err != nil {
		return rolloutindex.Source{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return rolloutindex.Source{}, err
	}
	return rolloutindex.Source{
		ID: server.SourceID(resolved + "\x00builtin:canonical"), Path: resolved,
		Fingerprint: "canonical:" + plugins.APIVersion, Size: info.Size(), ModTime: info.ModTime(),
	}, nil
}

func (indexer *SourceIndexer) startProgressive(source rolloutindex.Source) *sourceIndexJob {
	indexer.mu.Lock()
	if current := indexer.jobs[source.ID]; current != nil {
		indexer.mu.Unlock()
		return current
	}
	// A duplicate caller may have observed a miss immediately before the first
	// job committed and removed itself. Recheck while holding the job lock so a
	// completed cache is never deleted by a late duplicate.
	if status, err := indexer.store.Status(context.Background(), source); err == nil && status.State == rolloutindex.CacheFresh && status.Cached != nil {
		job := &sourceIndexJob{ready: make(chan struct{})}
		job.publish(IndexedSource{Info: *status.Cached}, nil)
		indexer.mu.Unlock()
		return job
	}
	job := &sourceIndexJob{ready: make(chan struct{})}
	indexer.jobs[source.ID] = job
	indexer.mu.Unlock()
	go indexer.runProgressive(source, job)
	return job
}

func (indexer *SourceIndexer) runProgressive(source rolloutindex.Source, job *sourceIndexJob) {
	defer indexer.finishJob(source.ID, job)
	for {
		result, err := indexer.store.ProgressiveFile(indexer.ctx, source, rolloutindex.ProgressiveOptions{
			Ready: func(info rolloutindex.SourceInfo) { job.publish(IndexedSource{Info: info, Refreshed: true}, nil) },
		})
		if errors.Is(err, rolloutindex.ErrSourceChanged) && indexer.ctx.Err() == nil {
			updated, sourceErr := canonicalSource(source.Path)
			if sourceErr != nil {
				job.publish(IndexedSource{}, sourceErr)
				return
			}
			source = updated
			continue
		}
		if err != nil {
			job.publish(IndexedSource{}, err)
			return
		}
		job.publish(IndexedSource{Info: result, Refreshed: true}, nil)
		return
	}
}

func (indexer *SourceIndexer) startRefresh(source rolloutindex.Source) error {
	indexer.mu.Lock()
	if indexer.jobs[source.ID] != nil {
		indexer.mu.Unlock()
		return nil
	}
	job := &sourceIndexJob{ready: make(chan struct{})}
	indexer.jobs[source.ID] = job
	indexer.mu.Unlock()
	if err := indexer.store.SetIndexState(context.Background(), source.ID, rolloutindex.IndexRefreshing, ""); err != nil {
		indexer.finishJob(source.ID, job)
		return err
	}
	go func() {
		defer indexer.finishJob(source.ID, job)
		current := source
		var err error
		for {
			file, openErr := os.Open(current.Path)
			if openErr != nil {
				err = openErr
				break
			}
			_, err = indexer.store.Replace(indexer.ctx, current, file)
			_ = file.Close()
			if err != nil {
				break
			}
			latest, latestErr := canonicalSource(current.Path)
			if latestErr != nil {
				err = latestErr
				break
			}
			if latest.Size == current.Size && latest.ModTime.Equal(current.ModTime) {
				break
			}
			current = latest
			if stateErr := indexer.store.SetIndexState(context.Background(), source.ID, rolloutindex.IndexRefreshing, ""); stateErr != nil {
				err = stateErr
				break
			}
		}
		if err != nil {
			_ = indexer.store.SetIndexState(context.Background(), source.ID, rolloutindex.IndexFailed, err.Error())
		}
		job.publish(IndexedSource{}, err)
	}()
	return nil
}

func (indexer *SourceIndexer) finishJob(sourceID string, job *sourceIndexJob) {
	indexer.mu.Lock()
	if indexer.jobs[sourceID] == job {
		delete(indexer.jobs, sourceID)
	}
	indexer.mu.Unlock()
}

func (job *sourceIndexJob) publish(value IndexedSource, err error) {
	job.once.Do(func() {
		job.mu.Lock()
		job.value, job.err = value, err
		job.mu.Unlock()
		close(job.ready)
	})
}

func (indexer *SourceIndexer) waitIdle(ctx context.Context, sourceID string) error {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		indexer.mu.Lock()
		active := indexer.jobs[sourceID] != nil
		indexer.mu.Unlock()
		if !active {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for source indexing: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
