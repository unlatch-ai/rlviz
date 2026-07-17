// Package index provides the persistent, local SQLite index for canonical
// RLViz streams. It deliberately has no dependency on the HTTP server.
package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type Index struct {
	db *sql.DB
}

func Open(path string) (*Index, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("index path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve index path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return nil, fmt.Errorf("create index directory: %w", err)
	}
	dsn := (&url.URL{Scheme: "file", Path: abs}).String() + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite index: %w", err)
	}
	db.SetMaxOpenConns(4)
	idx := &Index{db: db}
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite index: %w", err)
	}
	if err := os.Chmod(abs, 0o600); err != nil {
		db.Close()
		return nil, fmt.Errorf("secure sqlite index: %w", err)
	}
	if err := idx.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return idx, nil
}

func (i *Index) Close() error { return i.db.Close() }

func (i *Index) migrate(ctx context.Context) error {
	if _, err := i.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate sqlite index: %w", err)
	}
	// CREATE TABLE IF NOT EXISTS does not add columns for existing v1/v2
	// databases. These idempotent checks keep local indexes upgradeable.
	for name, definition := range map[string]string{
		"index_state": "TEXT NOT NULL DEFAULT 'complete'",
		"index_error": "TEXT NOT NULL DEFAULT ''",
	} {
		present, err := i.sourceColumn(ctx, name)
		if err != nil {
			return err
		}
		if !present {
			if _, err := i.db.ExecContext(ctx, `ALTER TABLE sources ADD COLUMN `+name+` `+definition); err != nil {
				return fmt.Errorf("add sources.%s: %w", name, err)
			}
		}
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{"context_present", "INTEGER NOT NULL DEFAULT 0"},
		{"context_operation", "TEXT"},
		{"context_input_tokens", "INTEGER"},
		{"context_input_tokens_before", "INTEGER"},
		{"context_capacity", "INTEGER"},
		{"context_provenance", "TEXT"},
	} {
		present, err := i.tableColumn(ctx, "events", column.name)
		if err != nil {
			return err
		}
		if !present {
			if _, err := i.db.ExecContext(ctx, `ALTER TABLE events ADD COLUMN `+column.name+` `+column.definition); err != nil {
				return fmt.Errorf("add events.%s: %w", column.name, err)
			}
		}
	}
	if _, err := i.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS events_context ON events(source_id,trajectory_id,context_present,sequence)`); err != nil {
		return fmt.Errorf("create context event index: %w", err)
	}
	if _, err := i.db.ExecContext(ctx, `PRAGMA user_version=5`); err != nil {
		return err
	}
	return nil
}

func (i *Index) sourceColumn(ctx context.Context, name string) (bool, error) {
	return i.tableColumn(ctx, "sources", name)
}

func (i *Index) tableColumn(ctx context.Context, table, name string) (bool, error) {
	if table != "sources" && table != "events" {
		return false, fmt.Errorf("unsupported table %q", table)
	}
	rows, err := i.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var column, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &column, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if column == name {
			return true, nil
		}
	}
	return false, rows.Err()
}
