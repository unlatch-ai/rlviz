package index

const schema = `
CREATE TABLE IF NOT EXISTS sources (
  id TEXT PRIMARY KEY, path TEXT NOT NULL, adapter TEXT NOT NULL,
  fingerprint TEXT NOT NULL, size INTEGER NOT NULL, mod_time_ns INTEGER NOT NULL,
  indexed_at_ns INTEGER NOT NULL, records INTEGER NOT NULL, warnings INTEGER NOT NULL,
  complete_raw BLOB NOT NULL, index_state TEXT NOT NULL DEFAULT 'complete', index_error TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS runs (
  source_id TEXT NOT NULL, id TEXT NOT NULL, name TEXT NOT NULL, started_at TEXT NOT NULL,
  line INTEGER NOT NULL, byte_offset INTEGER NOT NULL, byte_length INTEGER NOT NULL, raw BLOB NOT NULL, PRIMARY KEY(source_id,id),
  FOREIGN KEY(source_id) REFERENCES sources(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS cases (
  source_id TEXT NOT NULL, id TEXT NOT NULL, run_id TEXT NOT NULL, name TEXT NOT NULL,
  line INTEGER NOT NULL, byte_offset INTEGER NOT NULL, byte_length INTEGER NOT NULL, raw BLOB NOT NULL, PRIMARY KEY(source_id,id),
  FOREIGN KEY(source_id) REFERENCES sources(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS groups (
  source_id TEXT NOT NULL, id TEXT NOT NULL, case_id TEXT NOT NULL, name TEXT NOT NULL,
  line INTEGER NOT NULL, byte_offset INTEGER NOT NULL, byte_length INTEGER NOT NULL, raw BLOB NOT NULL, PRIMARY KEY(source_id,id),
  FOREIGN KEY(source_id) REFERENCES sources(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS trajectories (
  source_id TEXT NOT NULL, id TEXT NOT NULL, group_id TEXT NOT NULL, parent_id TEXT NOT NULL,
  branch_id TEXT NOT NULL, status TEXT NOT NULL, termination TEXT NOT NULL,
  line INTEGER NOT NULL, byte_offset INTEGER NOT NULL, byte_length INTEGER NOT NULL, raw BLOB NOT NULL, PRIMARY KEY(source_id,id),
  FOREIGN KEY(source_id) REFERENCES sources(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS events (
  source_id TEXT NOT NULL, id TEXT NOT NULL, trajectory_id TEXT NOT NULL, sequence INTEGER NOT NULL,
  kind TEXT NOT NULL, timestamp TEXT NOT NULL, parent_id TEXT NOT NULL, branch_id TEXT NOT NULL,
  alignment_key TEXT NOT NULL, state_hash TEXT NOT NULL, search_text TEXT NOT NULL,
  source_path TEXT, source_line INTEGER, byte_offset INTEGER, byte_length INTEGER,
  line INTEGER NOT NULL, record_byte_offset INTEGER NOT NULL, record_byte_length INTEGER NOT NULL, raw BLOB NOT NULL,
  context_present INTEGER NOT NULL DEFAULT 0, context_operation TEXT, context_input_tokens INTEGER,
  context_input_tokens_before INTEGER, context_capacity INTEGER, context_provenance TEXT,
  PRIMARY KEY(source_id,id),
  UNIQUE(source_id,trajectory_id,sequence),
  FOREIGN KEY(source_id) REFERENCES sources(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS events_page ON events(source_id,trajectory_id,sequence);
CREATE INDEX IF NOT EXISTS events_kind ON events(source_id,trajectory_id,kind,sequence);
CREATE TABLE IF NOT EXISTS signals (
  source_id TEXT NOT NULL, id TEXT NOT NULL, trajectory_id TEXT NOT NULL, event_id TEXT NOT NULL,
  name TEXT NOT NULL, unit TEXT NOT NULL, line INTEGER NOT NULL, byte_offset INTEGER NOT NULL, byte_length INTEGER NOT NULL, raw BLOB NOT NULL,
  PRIMARY KEY(source_id,id), FOREIGN KEY(source_id) REFERENCES sources(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS signals_trajectory ON signals(source_id,trajectory_id);
CREATE TABLE IF NOT EXISTS artifacts (
  source_id TEXT NOT NULL, id TEXT NOT NULL, trajectory_id TEXT NOT NULL, event_id TEXT NOT NULL,
  name TEXT NOT NULL, media_type TEXT NOT NULL, path TEXT NOT NULL, sha256 TEXT NOT NULL,
  line INTEGER NOT NULL, byte_offset INTEGER NOT NULL, byte_length INTEGER NOT NULL, raw BLOB NOT NULL, PRIMARY KEY(source_id,id),
  FOREIGN KEY(source_id) REFERENCES sources(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS artifacts_trajectory ON artifacts(source_id,trajectory_id);
CREATE TABLE IF NOT EXISTS records (
  source_id TEXT NOT NULL, ordinal INTEGER NOT NULL, record_type TEXT NOT NULL,
  record_id TEXT NOT NULL, byte_offset INTEGER NOT NULL, byte_length INTEGER NOT NULL, raw BLOB NOT NULL, PRIMARY KEY(source_id,ordinal),
  FOREIGN KEY(source_id) REFERENCES sources(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS analyzer_results (
  source_id TEXT NOT NULL, trajectory_id TEXT NOT NULL,
  name TEXT NOT NULL, version TEXT NOT NULL, digest TEXT NOT NULL, input_digest TEXT NOT NULL,
  analyzed_at_ns INTEGER NOT NULL, output_json BLOB NOT NULL,
  PRIMARY KEY(source_id,trajectory_id,name,version,digest,input_digest),
  FOREIGN KEY(source_id,trajectory_id) REFERENCES trajectories(source_id,id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS analyzer_results_lookup ON analyzer_results(source_id,trajectory_id,name);
-- Presentation is intentionally separate from source identity and data
-- fingerprinting. Source refreshes replace canonical rows without changing a
-- user's validated display configuration.
CREATE TABLE IF NOT EXISTS source_presentations (
  source_id TEXT PRIMARY KEY, config_json BLOB NOT NULL, updated_at_ns INTEGER NOT NULL
);
`
