package audit

import (
	"database/sql"
	"fmt"
	"time"
)

type migration struct {
	version int
	name    string
	upSQL   string
}

var schemaMigrations = []migration{
	{
		version: 1,
		name:    "create_core_tables",
		upSQL: `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	repo_path TEXT NOT NULL,
	trust_tier TEXT NOT NULL,
	status TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS turns (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	user_message TEXT,
	status TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS approvals (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	turn_id TEXT,
	tool_invocation_id TEXT,
	decision TEXT NOT NULL,
	actor TEXT NOT NULL,
	reason TEXT,
	created_at TEXT NOT NULL,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
	FOREIGN KEY (turn_id) REFERENCES turns(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS tool_invocations (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	turn_id TEXT,
	tool_name TEXT NOT NULL,
	arguments TEXT NOT NULL,
	status TEXT NOT NULL,
	started_at TEXT NOT NULL,
	completed_at TEXT,
	result_summary TEXT,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
	FOREIGN KEY (turn_id) REFERENCES turns(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS model_invocations (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	turn_id TEXT,
	provider TEXT NOT NULL,
	model TEXT NOT NULL,
	request_payload TEXT,
	response_payload TEXT,
	input_tokens INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL,
	started_at TEXT NOT NULL,
	completed_at TEXT,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
	FOREIGN KEY (turn_id) REFERENCES turns(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS context_fragments (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	turn_id TEXT,
	source_type TEXT NOT NULL,
	source_location TEXT NOT NULL,
	retrieved_at TEXT NOT NULL,
	inclusion_reason TEXT NOT NULL,
	token_count INTEGER NOT NULL DEFAULT 0,
	truncated INTEGER NOT NULL DEFAULT 0,
	redaction_state TEXT NOT NULL DEFAULT 'none',
	content_hash TEXT,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
	FOREIGN KEY (turn_id) REFERENCES turns(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS patch_proposals (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	turn_id TEXT,
	tool_invocation_id TEXT,
	target_path TEXT NOT NULL,
	before_hash TEXT,
	after_hash TEXT,
	patch_text TEXT NOT NULL,
	status TEXT NOT NULL,
	created_at TEXT NOT NULL,
	applied_at TEXT,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
	FOREIGN KEY (turn_id) REFERENCES turns(id) ON DELETE SET NULL,
	FOREIGN KEY (tool_invocation_id) REFERENCES tool_invocations(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS file_mutations (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	turn_id TEXT,
	patch_proposal_id TEXT,
	path TEXT NOT NULL,
	before_hash TEXT,
	after_hash TEXT NOT NULL,
	mutation_type TEXT NOT NULL,
	created_at TEXT NOT NULL,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
	FOREIGN KEY (turn_id) REFERENCES turns(id) ON DELETE SET NULL,
	FOREIGN KEY (patch_proposal_id) REFERENCES patch_proposals(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS artifacts (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	turn_id TEXT,
	artifact_type TEXT NOT NULL,
	uri TEXT NOT NULL,
	content_type TEXT,
	size_bytes INTEGER NOT NULL DEFAULT 0,
	metadata TEXT,
	created_at TEXT NOT NULL,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
	FOREIGN KEY (turn_id) REFERENCES turns(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS events (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	turn_id TEXT,
	timestamp TEXT NOT NULL,
	event_type TEXT NOT NULL,
	actor TEXT NOT NULL,
	policy_version TEXT,
	trust_tier TEXT,
	payload TEXT
);

CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type, timestamp);
`,
	},
}

func applyMigrations(db *sql.DB) error {
	if _, err := db.Exec(`
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
`); err != nil {
		return fmt.Errorf("failed to configure sqlite pragmas: %w", err)
	}

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
);
`); err != nil {
		return fmt.Errorf("failed to create schema migrations table: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}

	for _, m := range schemaMigrations {
		applied, err := migrationApplied(tx, m.version)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if applied {
			continue
		}

		if _, err := tx.Exec(m.upSQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to run migration %d (%s): %w", m.version, m.name, err)
		}

		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			m.version,
			m.name,
			time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to record migration %d (%s): %w", m.version, m.name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migrations: %w", err)
	}

	return nil
}

func migrationApplied(tx *sql.Tx, version int) (bool, error) {
	var count int
	if err := tx.QueryRow(`SELECT count(1) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
		return false, fmt.Errorf("failed checking migration %d: %w", version, err)
	}

	return count > 0, nil
}
