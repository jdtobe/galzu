package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// WorkRequest is an agent saying it has capacity. The fabric is the first place
// to look for work and the leader is the fallback, so the harness tracks whether
// anyone answered and escalates when nobody did.
//
// Spec: docs/superpowers/specs/2026-11-03-work-requests.md
type WorkRequest struct {
	ID          int64
	SessionID   string
	AgentID     string
	MessageID   int64
	CreatedAt   time.Time
	EscalatedAt time.Time
	ResolvedAt  time.Time
	Status      string // open, escalated, resolved
}

var ErrWorkRequestGone = errors.New("work request is no longer open")

func workSchema(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS work_requests(
  id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, agent_id TEXT NOT NULL,
  message_id INTEGER NOT NULL, status TEXT NOT NULL DEFAULT 'open',
  created_at TEXT NOT NULL, escalated_at TEXT NOT NULL DEFAULT '', resolved_at TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS work_requests_open ON work_requests(session_id, status);`)
	return err
}

// OpenWorkRequest records that an agent announced capacity.
func (s *Store) OpenWorkRequest(ctx context.Context, sessionID, agentID string, messageID int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO work_requests(session_id,agent_id,message_id,created_at) VALUES(?,?,?,?)`,
		sessionID, agentID, messageID, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// OpenWorkRequests lists the requests still waiting for an answer.
func (s *Store) OpenWorkRequests(ctx context.Context, sessionID string) ([]WorkRequest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,session_id,agent_id,message_id,status,created_at,escalated_at,resolved_at
FROM work_requests WHERE session_id=? AND status IN ('open','escalated') ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkRequest{}
	for rows.Next() {
		var w WorkRequest
		var created, escalated, resolved string
		if err := rows.Scan(&w.ID, &w.SessionID, &w.AgentID, &w.MessageID, &w.Status, &created, &escalated, &resolved); err != nil {
			return nil, err
		}
		w.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if escalated != "" {
			w.EscalatedAt, _ = time.Parse(time.RFC3339Nano, escalated)
		}
		if resolved != "" {
			w.ResolvedAt, _ = time.Parse(time.RFC3339Nano, resolved)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ResolveWorkRequest marks a request answered. It is idempotent enough: a second
// call on a closed request reports that it was already gone.
func (s *Store) ResolveWorkRequest(ctx context.Context, id int64, status string) error {
	row, err := s.db.ExecContext(ctx, `UPDATE work_requests SET status=?,resolved_at=? WHERE id=? AND status IN ('open','escalated')`,
		status, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	n, err := row.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrWorkRequestGone
	}
	return nil
}

// EscalateWorkRequest records that the leader has been asked directly.
func (s *Store) EscalateWorkRequest(ctx context.Context, id int64) error {
	row, err := s.db.ExecContext(ctx, `UPDATE work_requests SET status='escalated',escalated_at=? WHERE id=? AND status='open'`,
		time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	n, err := row.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrWorkRequestGone
	}
	return nil
}

// AnsweredAfter reports whether another agent has spoken since a message. It is
// the fabric's proxy for "someone picked this up". Host messages are excluded on
// purpose: the escalation itself is a host message, and counting it would make
// every escalation resolve the request it was raised about.
func (s *Store) AnsweredAfter(ctx context.Context, sessionID string, messageID int64, agentID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE session_id=? AND id>? AND agent_id<>'' AND agent_id<>?`,
		sessionID, messageID, agentID).Scan(&n)
	return n > 0, err
}
