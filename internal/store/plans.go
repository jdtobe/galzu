package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Plan is a versioned statement of intent for a session: the steps, the
// definition of done, and the acceptance criteria. The human approves a whole
// version; the harness never parses model prose into authority.
//
// Spec: docs/superpowers/specs/2026-11-03-plan-artifact.md
type Plan struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Version   int       `json:"version"`
	Body      string    `json:"body"`
	AuthorID  string    `json:"author_id"`
	Status    string    `json:"status"` // pending, approved, rejected, superseded
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	DecidedAt time.Time `json:"decided_at,omitempty"`
}

var ErrPlanNotPending = errors.New("plan is not pending review")

const MaxPlanBytes = 8 << 10

func plansSchema(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS plans(
  id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, version INTEGER NOT NULL,
  body TEXT NOT NULL, author_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
  note TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, decided_at TEXT NOT NULL DEFAULT '',
  decided_by TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS plans_session ON plans(session_id, version);`)
	return err
}

// ProposePlan records a new version and supersedes any plan still awaiting
// review. Only one plan can be pending: a queue of competing plans is how a
// herd stalls.
func (s *Store) ProposePlan(ctx context.Context, sessionID, authorID, body string) (Plan, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Plan{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE plans SET status='superseded', decided_at=? WHERE session_id=? AND status='pending'`,
		time.Now().UTC().Format(time.RFC3339Nano), sessionID); err != nil {
		return Plan{}, err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM plans WHERE session_id=?`, sessionID).Scan(&version); err != nil {
		return Plan{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO plans(session_id,version,body,author_id,created_at) VALUES(?,?,?,?,?)`,
		sessionID, version, body, authorID, now.Format(time.RFC3339Nano)); err != nil {
		return Plan{}, err
	}
	if err := tx.Commit(); err != nil {
		return Plan{}, err
	}
	return Plan{SessionID: sessionID, Version: version, Body: body, AuthorID: authorID, Status: "pending", CreatedAt: now}, nil
}

// DecidePlan approves or rejects a pending version, with the human's note.
func (s *Store) DecidePlan(ctx context.Context, sessionID string, version int, approve bool, note, decidedBy string) error {
	status := "rejected"
	if approve {
		status = "approved"
	}
	row, err := s.db.ExecContext(ctx, `UPDATE plans SET status=?,note=?,decided_at=?,decided_by=? WHERE session_id=? AND version=? AND status='pending'`,
		status, note, time.Now().UTC().Format(time.RFC3339Nano), decidedBy, sessionID, version)
	if err != nil {
		return err
	}
	n, err := row.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrPlanNotPending
	}
	return nil
}

// CurrentPlan returns the approved plan, or the pending one when nothing has
// been approved yet. ok is false when the session has no plan at all.
func (s *Store) CurrentPlan(ctx context.Context, sessionID string) (Plan, bool, error) {
	plan, err := s.planWithStatus(ctx, sessionID, "approved")
	if err == nil {
		return plan, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Plan{}, false, err
	}
	plan, err = s.planWithStatus(ctx, sessionID, "pending")
	if errors.Is(err, sql.ErrNoRows) {
		return Plan{}, false, nil
	}
	return plan, err == nil, err
}

func (s *Store) planWithStatus(ctx context.Context, sessionID, status string) (Plan, error) {
	var p Plan
	var created, decided string
	err := s.db.QueryRowContext(ctx, `SELECT id,session_id,version,body,author_id,status,note,created_at,decided_at
FROM plans WHERE session_id=? AND status=? ORDER BY version DESC LIMIT 1`, sessionID, status).
		Scan(&p.ID, &p.SessionID, &p.Version, &p.Body, &p.AuthorID, &p.Status, &p.Note, &created, &decided)
	if err != nil {
		return Plan{}, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if decided != "" {
		p.DecidedAt, _ = time.Parse(time.RFC3339Nano, decided)
	}
	return p, nil
}

// Plans lists every version, oldest first.
func (s *Store) Plans(ctx context.Context, sessionID string) ([]Plan, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,session_id,version,body,author_id,status,note,created_at,decided_at
FROM plans WHERE session_id=? ORDER BY version`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		var p Plan
		var created, decided string
		if err := rows.Scan(&p.ID, &p.SessionID, &p.Version, &p.Body, &p.AuthorID, &p.Status, &p.Note, &created, &decided); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if decided != "" {
			p.DecidedAt, _ = time.Parse(time.RFC3339Nano, decided)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
