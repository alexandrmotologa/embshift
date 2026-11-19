package queue

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/alexandrmotologa/embshift/internal/models"
	_ "modernc.org/sqlite"
)

// SQLiteQueue provides a durable, transactional replication event buffer in WAL mode.
type SQLiteQueue struct {
	db *sql.DB
	mu sync.Mutex
}

// NewSQLiteQueue creates or opens a pure-Go SQLite queue database.
func NewSQLiteQueue(dbPath string) (*SQLiteQueue, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// Configure connection pool for SQLite WAL mode concurrency
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	schema := `
	PRAGMA journal_mode=WAL;
	PRAGMA synchronous=NORMAL;

	CREATE TABLE IF NOT EXISTS replication_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		operation TEXT NOT NULL,
		record_id TEXT NOT NULL,
		payload BLOB,
		text TEXT,
		created_at DATETIME NOT NULL,
		attempts INTEGER DEFAULT 0,
		last_error TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_rep_events_created ON replication_events(created_at);

	CREATE TABLE IF NOT EXISTS backfill_checkpoints (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		migrated_id TEXT UNIQUE NOT NULL,
		migrated_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_bf_migrated_id ON backfill_checkpoints(migrated_id);

	CREATE TABLE IF NOT EXISTS shadow_queries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		query_id TEXT NOT NULL,
		query_text TEXT NOT NULL,
		jaccard_similarity REAL NOT NULL,
		legacy_latency_ms REAL NOT NULL,
		target_latency_ms REAL NOT NULL,
		created_at DATETIME NOT NULL
	);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize sqlite schema: %w", err)
	}

	return &SQLiteQueue{db: db}, nil
}

// Enqueue inserts a new replication task into the durable queue.
func (q *SQLiteQueue) Enqueue(operation, recordID string, payload []byte, text string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	query := `INSERT INTO replication_events (operation, record_id, payload, text, created_at, attempts)
	          VALUES (?, ?, ?, ?, ?, 0)`
	_, err := q.db.Exec(query, operation, recordID, payload, text, time.Now().UTC())
	return err
}

// DequeueBatch retrieves up to limit pending events for asynchronous processing.
func (q *SQLiteQueue) DequeueBatch(limit int) ([]models.ReplicationEvent, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	query := `SELECT id, operation, record_id, payload, text, created_at, attempts, COALESCE(last_error, '')
	          FROM replication_events
	          ORDER BY id ASC
	          LIMIT ?`

	rows, err := q.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.ReplicationEvent
	for rows.Next() {
		var ev models.ReplicationEvent
		var payload []byte
		var createdAt string
		if err := rows.Scan(&ev.ID, &ev.Operation, &ev.RecordID, &payload, &ev.Text, &createdAt, &ev.Attempts, &ev.LastError); err != nil {
			return nil, err
		}
		ev.Payload = payload
		ev.CreatedAt, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt)
		if ev.CreatedAt.IsZero() {
			ev.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		}
		events = append(events, ev)
	}

	return events, rows.Err()
}

// Ack removes successfully replicated events from the queue.
func (q *SQLiteQueue) Ack(eventIDs []int64) error {
	if len(eventIDs) == 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	tx, err := q.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("DELETE FROM replication_events WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range eventIDs {
		if _, err := stmt.Exec(id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Nack increments the attempt counter and records the error description.
func (q *SQLiteQueue) Nack(eventID int64, errReason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	query := `UPDATE replication_events 
	          SET attempts = attempts + 1, last_error = ? 
	          WHERE id = ?`
	_, err := q.db.Exec(query, errReason, eventID)
	return err
}

// GetQueueSize returns the count of outstanding events in the queue.
func (q *SQLiteQueue) GetQueueSize() (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var count int64
	err := q.db.QueryRow("SELECT COUNT(*) FROM replication_events").Scan(&count)
	return count, err
}

// GetReplicationLagMs calculates the age in milliseconds of the oldest uncommitted event.
func (q *SQLiteQueue) GetReplicationLagMs() (float64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var createdAtStr sql.NullString
	err := q.db.QueryRow("SELECT created_at FROM replication_events ORDER BY id ASC LIMIT 1").Scan(&createdAtStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0.0, nil
		}
		return 0.0, err
	}

	if !createdAtStr.Valid {
		return 0.0, nil
	}

	t, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAtStr.String)
	if err != nil {
		t, err = time.Parse(time.RFC3339, createdAtStr.String)
	}
	if err != nil || t.IsZero() {
		return 0.0, nil
	}

	lag := time.Since(t).Seconds() * 1000.0
	if lag < 0 {
		lag = 0
	}
	return lag, nil
}

// MarkDocumentMigrated stores a document ID in the backfill checkpoint store.
func (q *SQLiteQueue) MarkDocumentMigrated(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, err := q.db.Exec("INSERT OR IGNORE INTO backfill_checkpoints (migrated_id, migrated_at) VALUES (?, ?)", id, time.Now().UTC())
	return err
}

// IsDocumentMigrated checks if a document ID has already been migrated.
func (q *SQLiteQueue) IsDocumentMigrated(id string) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var exists int
	err := q.db.QueryRow("SELECT 1 FROM backfill_checkpoints WHERE migrated_id = ?", id).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return exists == 1, nil
}

// CountMigratedDocuments returns the total count of successfully migrated document IDs.
func (q *SQLiteQueue) CountMigratedDocuments() (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var count int64
	err := q.db.QueryRow("SELECT COUNT(*) FROM backfill_checkpoints").Scan(&count)
	return count, err
}

// RecordShadowQuery persists a shadow query comparison for auditing.
func (q *SQLiteQueue) RecordShadowQuery(queryID, queryText string, jaccard float64, legacyLatency, targetLatency float64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	query := `INSERT INTO shadow_queries (query_id, query_text, jaccard_similarity, legacy_latency_ms, target_latency_ms, created_at)
	          VALUES (?, ?, ?, ?, ?, ?)`
	_, err := q.db.Exec(query, queryID, queryText, jaccard, legacyLatency, targetLatency, time.Now().UTC())
	return err
}

// GetAverageJaccard computes the rolling average semantic overlap across all shadow queries.
func (q *SQLiteQueue) GetAverageJaccard() (float64, int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var avg sql.NullFloat64
	var count int64
	err := q.db.QueryRow("SELECT AVG(jaccard_similarity), COUNT(*) FROM shadow_queries").Scan(&avg, &count)
	if err != nil {
		return 0.0, 0, err
	}
	if !avg.Valid {
		return 0.0, 0, nil
	}
	return avg.Float64, count, nil
}

// Close closes the SQLite connection.
func (q *SQLiteQueue) Close() error {
	return q.db.Close()
}
