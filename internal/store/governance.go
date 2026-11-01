package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Proposal is a yes/no question put to the herd. Spec:
// docs/superpowers/specs/2026-11-02-governance.md
type Proposal struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Body      string    `json:"body"`
	AuthorID  string    `json:"author_id"`
	Status    string    `json:"status"` // open, passed, failed
	Result    string    `json:"result"`
	CreatedAt time.Time `json:"created_at"`
	ClosedAt  time.Time `json:"closed_at,omitempty"`
}

// Ballot is one agent's current position. A later ballot replaces an earlier
// one, but the proposal keeps its own record of when it opened and closed.
type Ballot struct {
	ProposalID int64     `json:"proposal_id"`
	AgentID    string    `json:"agent_id"`
	Choice     string    `json:"choice"`
	Text       string    `json:"text"`
	At         time.Time `json:"at"`
}

var ErrProposalClosed = errors.New("proposal is not open")

func governanceSchema(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS proposals(
  id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, body TEXT NOT NULL,
  author_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'open', result TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL, closed_at TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS ballots(
  proposal_id INTEGER NOT NULL REFERENCES proposals(id), agent_id TEXT NOT NULL,
  choice TEXT NOT NULL, text TEXT NOT NULL DEFAULT '', at TEXT NOT NULL,
  PRIMARY KEY(proposal_id, agent_id));`)
	return err
}

// OpenProposal records a new question and returns its id.
func (s *Store) OpenProposal(ctx context.Context, sessionID, authorID, body string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO proposals(session_id,body,author_id,created_at) VALUES(?,?,?,?)`,
		sessionID, body, authorID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// CastBallot records one agent's position. A second ballot from the same agent
// replaces the first; voting on a closed proposal is refused.
func (s *Store) CastBallot(ctx context.Context, proposalID int64, agentID, choice, text string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM proposals WHERE id=?`, proposalID).Scan(&status); err != nil {
		return err
	}
	if status != "open" {
		return ErrProposalClosed
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ballots(proposal_id,agent_id,choice,text,at) VALUES(?,?,?,?,?)
ON CONFLICT(proposal_id,agent_id) DO UPDATE SET choice=excluded.choice,text=excluded.text,at=excluded.at`,
		proposalID, agentID, choice, text, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

// OpenProposals lists the session's open questions, oldest first.
func (s *Store) OpenProposals(ctx context.Context, sessionID string) ([]Proposal, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,session_id,body,author_id,status,result,created_at,closed_at FROM proposals WHERE session_id=? AND status='open' ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProposals(rows)
}

// Proposals lists every question in a session, newest last.
func (s *Store) Proposals(ctx context.Context, sessionID string) ([]Proposal, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,session_id,body,author_id,status,result,created_at,closed_at FROM proposals WHERE session_id=? ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProposals(rows)
}

func scanProposals(rows *sql.Rows) ([]Proposal, error) {
	out := []Proposal{}
	for rows.Next() {
		var p Proposal
		var created, closed string
		if err := rows.Scan(&p.ID, &p.SessionID, &p.Body, &p.AuthorID, &p.Status, &p.Result, &created, &closed); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		if closed != "" {
			p.ClosedAt, _ = time.Parse(time.RFC3339Nano, closed)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Ballots returns the current ballot per agent for one proposal.
func (s *Store) Ballots(ctx context.Context, proposalID int64) ([]Ballot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT proposal_id,agent_id,choice,text,at FROM ballots WHERE proposal_id=? ORDER BY agent_id`, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ballot{}
	for rows.Next() {
		var b Ballot
		var at string
		if err := rows.Scan(&b.ProposalID, &b.AgentID, &b.Choice, &b.Text, &at); err != nil {
			return nil, err
		}
		b.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, b)
	}
	return out, rows.Err()
}

// CloseProposal settles a question once and refuses to settle it twice.
func (s *Store) CloseProposal(ctx context.Context, proposalID int64, status, result string) error {
	row, err := s.db.ExecContext(ctx, `UPDATE proposals SET status=?,result=?,closed_at=? WHERE id=? AND status='open'`,
		status, result, time.Now().UTC().Format(time.RFC3339Nano), proposalID)
	if err != nil {
		return err
	}
	n, err := row.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrProposalClosed
	}
	return nil
}
