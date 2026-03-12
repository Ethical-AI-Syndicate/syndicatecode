package audit

import (
	"database/sql"
	"fmt"
	"time"
)

const latestSchemaVersion = 1

func applyMigrations(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	tableStmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, repo_path TEXT NOT NULL, trust_tier TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS turns (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS approvals (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, tool_name TEXT NOT NULL, state TEXT NOT NULL, arguments_hash TEXT, call_payload TEXT, execution_context TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE, FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS tool_invocations (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT, approval_id TEXT, tool_name TEXT NOT NULL, success INTEGER NOT NULL, duration_ms INTEGER NOT NULL, output_ref TEXT, created_at TEXT NOT NULL, FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE, FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE SET NULL, FOREIGN KEY(approval_id) REFERENCES approvals(id) ON DELETE SET NULL)`,
		`CREATE TABLE IF NOT EXISTS model_invocations (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT, provider TEXT NOT NULL, model TEXT NOT NULL, routing_policy TEXT, prompt_ref TEXT, response_ref TEXT, created_at TEXT NOT NULL, FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE, FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE SET NULL)`,
		`CREATE TABLE IF NOT EXISTS context_fragments (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, source_type TEXT NOT NULL, token_count INTEGER NOT NULL, content_ref TEXT NOT NULL, rank REAL, created_at TEXT NOT NULL, FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE, FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS patch_proposals (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, proposal_ref TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE, FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS file_mutations (id TEXT PRIMARY KEY, session_id TEXT, turn_id TEXT, patch_id TEXT, path TEXT NOT NULL, mutation_type TEXT NOT NULL, before_hash TEXT, after_hash TEXT, applied_at TEXT NOT NULL, FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE SET NULL, FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE SET NULL, FOREIGN KEY(patch_id) REFERENCES patch_proposals(id) ON DELETE SET NULL)`,
		`CREATE TABLE IF NOT EXISTS artifacts (id TEXT PRIMARY KEY, session_id TEXT, turn_id TEXT, kind TEXT NOT NULL, storage_path TEXT NOT NULL, sha256 TEXT, redaction_state TEXT, expires_at TEXT, created_at TEXT NOT NULL, FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE SET NULL, FOREIGN KEY(turn_id) REFERENCES turns(id) ON DELETE SET NULL)`,
		`CREATE TABLE IF NOT EXISTS events (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT, timestamp TEXT NOT NULL, event_type TEXT NOT NULL, actor TEXT NOT NULL, policy_version TEXT, trust_tier TEXT, payload TEXT)`,
	}

	for _, stmt := range tableStmts {
		if _, err = tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec migration statement: %w", err)
		}
	}

	if err = ensureEventColumn(tx, "turn_id", "TEXT"); err != nil {
		return err
	}
	if err = ensureEventColumn(tx, "policy_version", "TEXT"); err != nil {
		return err
	}
	if err = ensureEventColumn(tx, "trust_tier", "TEXT"); err != nil {
		return err
	}

	indexStmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status)`,
		`CREATE INDEX IF NOT EXISTS idx_turns_session ON turns(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_approvals_session ON approvals(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_invocations_session ON tool_invocations(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_model_invocations_session ON model_invocations(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_context_fragments_turn ON context_fragments(turn_id)`,
		`CREATE INDEX IF NOT EXISTS idx_patch_proposals_turn ON patch_proposals(turn_id)`,
		`CREATE INDEX IF NOT EXISTS idx_file_mutations_turn ON file_mutations(turn_id)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_session ON artifacts(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type)`,
		`CREATE INDEX IF NOT EXISTS idx_events_turn ON events(turn_id)`,
	}

	for _, stmt := range indexStmts {
		if _, err = tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec migration statement: %w", err)
		}
	}

	if _, err = tx.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (?, ?)`, latestSchemaVersion, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("record schema migration: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit migration tx: %w", err)
	}
	return nil
}

func currentSchemaVersion(db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func ensureEventColumn(tx *sql.Tx, name, dataType string) error {
	rows, err := tx.Query(`PRAGMA table_info(events)`)
	if err != nil {
		return fmt.Errorf("query events table info: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			cid      int
			colName  string
			colType  string
			notNull  int
			defaultV interface{}
			pk       int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultV, &pk); err != nil {
			return fmt.Errorf("scan events table info: %w", err)
		}
		if colName == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate events table info: %w", err)
	}

	if _, err := tx.Exec(fmt.Sprintf("ALTER TABLE events ADD COLUMN %s %s", name, dataType)); err != nil {
		return fmt.Errorf("add events column %s: %w", name, err)
	}
	return nil
}
