package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Finding is a claim the herd can support or challenge. Agreement is recorded
// but never treated as verification: nothing in the harness closes a finding or
// calls it true.
//
// Spec: docs/superpowers/specs/2026-11-03-findings.md
type Finding struct {
	ID        int64             `json:"id"`
	SessionID string            `json:"session_id"`
	AuthorID  string            `json:"author_id"`
	Claim     string            `json:"claim"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Evidence  []FindingEvidence `json:"evidence,omitempty"`
	Positions []FindingPosition `json:"positions,omitempty"`
	Revisions []FindingRevision `json:"revisions,omitempty"`
}

type FindingRevision struct {
	ID        int64     `json:"id"`
	FindingID int64     `json:"finding_id"`
	AuthorID  string    `json:"author_id"`
	Claim     string    `json:"claim"`
	CreatedAt time.Time `json:"created_at"`
}

type FindingEvidence struct {
	ID        int64     `json:"id"`
	FindingID int64     `json:"finding_id"`
	AuthorID  string    `json:"author_id"`
	Stance    string    `json:"stance"` // support, oppose
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type FindingPosition struct {
	FindingID int64     `json:"finding_id"`
	AgentID   string    `json:"agent_id"`
	Choice    string    `json:"choice"` // agree, disagree
	Text      string    `json:"text"`
	At        time.Time `json:"at"`
}

var ErrFindingGone = errors.New("finding does not exist in this session")
var ErrFindingWithdrawn = errors.New("finding is already withdrawn")

func findingsSchema(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS findings(
  id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, author_id TEXT NOT NULL,
  claim TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'open',
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS finding_revisions(
  id INTEGER PRIMARY KEY AUTOINCREMENT, finding_id INTEGER NOT NULL REFERENCES findings(id),
  author_id TEXT NOT NULL, claim TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS finding_evidence(
  id INTEGER PRIMARY KEY AUTOINCREMENT, finding_id INTEGER NOT NULL REFERENCES findings(id),
  author_id TEXT NOT NULL, stance TEXT NOT NULL, body TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS finding_positions(
  finding_id INTEGER NOT NULL REFERENCES findings(id), agent_id TEXT NOT NULL,
  choice TEXT NOT NULL, text TEXT NOT NULL DEFAULT '', at TEXT NOT NULL,
  PRIMARY KEY(finding_id, agent_id));`)
	return err
}

// CreateFinding records a claim and its first evidence, if any. The first
// revision is the original claim, so revisions always contain the whole history.
func (s *Store) CreateFinding(ctx context.Context, sessionID, authorID, claim, evidence, stance string) (Finding, error) {
	claim = strings.TrimSpace(claim)
	if claim == "" {
		return Finding{}, errors.New("a finding needs a claim")
	}
	stance = normaliseStance(stance)
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Finding{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO findings(session_id,author_id,claim,created_at,updated_at) VALUES(?,?,?,?,?)`,
		sessionID, authorID, claim, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return Finding{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Finding{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO finding_revisions(finding_id,author_id,claim,created_at) VALUES(?,?,?,?)`,
		id, authorID, claim, now.Format(time.RFC3339Nano)); err != nil {
		return Finding{}, err
	}
	if evidence = strings.TrimSpace(evidence); evidence != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO finding_evidence(finding_id,author_id,stance,body,created_at) VALUES(?,?,?,?,?)`,
			id, authorID, stance, evidence, now.Format(time.RFC3339Nano)); err != nil {
			return Finding{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Finding{}, err
	}
	return s.finding(ctx, sessionID, id)
}

// ReviseFinding keeps the previous claim and adds the new one. An empty claim
// means evidence only. A finding from another session is not reachable.
func (s *Store) ReviseFinding(ctx context.Context, sessionID string, findingID int64, authorID, claim, evidence, stance string) (Finding, error) {
	claim = strings.TrimSpace(claim)
	evidence = strings.TrimSpace(evidence)
	if claim == "" && evidence == "" {
		return Finding{}, errors.New("a revision needs a claim or evidence")
	}
	stance = normaliseStance(stance)
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Finding{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE findings SET updated_at=? WHERE id=? AND session_id=?`,
		now.Format(time.RFC3339Nano), findingID, sessionID)
	if err != nil {
		return Finding{}, err
	}
	if n, err := result.RowsAffected(); err != nil {
		return Finding{}, err
	} else if n != 1 {
		return Finding{}, ErrFindingGone
	}
	if claim != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE findings SET claim=? WHERE id=?`, claim, findingID); err != nil {
			return Finding{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO finding_revisions(finding_id,author_id,claim,created_at) VALUES(?,?,?,?)`,
			findingID, authorID, claim, now.Format(time.RFC3339Nano)); err != nil {
			return Finding{}, err
		}
	}
	if evidence != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO finding_evidence(finding_id,author_id,stance,body,created_at) VALUES(?,?,?,?,?)`,
			findingID, authorID, stance, evidence, now.Format(time.RFC3339Nano)); err != nil {
			return Finding{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Finding{}, err
	}
	return s.finding(ctx, sessionID, findingID)
}

// RetractFinding withdraws a claim without erasing it. Evidence and positions
// stay in the record, because a withdrawn claim is history worth keeping.
func (s *Store) RetractFinding(ctx context.Context, sessionID string, findingID int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE findings SET status='withdrawn',updated_at=? WHERE id=? AND session_id=? AND status='open'`,
		time.Now().UTC().Format(time.RFC3339Nano), findingID, sessionID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		return nil
	}
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM findings WHERE id=? AND session_id=?`, findingID, sessionID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFindingGone
		}
		return err
	}
	if status == "withdrawn" {
		return ErrFindingWithdrawn
	}
	return ErrFindingGone
}

// EndorseFinding records or replaces one agent's position. It is a position, not
// a truth claim, and the latest position is the one the herd sees.
func (s *Store) EndorseFinding(ctx context.Context, sessionID string, findingID int64, agentID, choice, text string) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM findings WHERE id=? AND session_id=?`, findingID, sessionID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFindingGone
		}
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO finding_positions(finding_id,agent_id,choice,text,at) VALUES(?,?,?,?,?)
ON CONFLICT(finding_id,agent_id) DO UPDATE SET choice=excluded.choice,text=excluded.text,at=excluded.at`,
		findingID, agentID, choice, text, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// Finding returns one finding in a session.
func (s *Store) Finding(ctx context.Context, sessionID string, id int64) (Finding, error) {
	return s.finding(ctx, sessionID, id)
}

// Findings returns findings oldest-first. A positive limit returns the most
// recent ones, still oldest-first, so a prompt window cannot starve new claims.
func (s *Store) Findings(ctx context.Context, sessionID string, limit int) ([]Finding, error) {
	query := `SELECT id FROM findings WHERE session_id=? ORDER BY id`
	if limit > 0 {
		query = `SELECT id FROM (SELECT id FROM findings WHERE session_id=? ORDER BY id DESC LIMIT ?) ORDER BY id`
	}
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.QueryContext(ctx, query, sessionID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, query, sessionID)
	}
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := make([]Finding, 0, len(ids))
	for _, id := range ids {
		finding, err := s.finding(ctx, sessionID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, finding)
	}
	return out, nil
}

func (s *Store) finding(ctx context.Context, sessionID string, id int64) (Finding, error) {
	var f Finding
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,session_id,author_id,claim,status,created_at,updated_at
FROM findings WHERE id=? AND session_id=?`, id, sessionID).
		Scan(&f.ID, &f.SessionID, &f.AuthorID, &f.Claim, &f.Status, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Finding{}, ErrFindingGone
	}
	if err != nil {
		return Finding{}, err
	}
	f.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	f.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if f.Evidence, err = s.findingEvidence(ctx, id); err != nil {
		return Finding{}, err
	}
	if f.Positions, err = s.findingPositions(ctx, id); err != nil {
		return Finding{}, err
	}
	if f.Revisions, err = s.findingRevisions(ctx, id); err != nil {
		return Finding{}, err
	}
	return f, nil
}

func (s *Store) findingEvidence(ctx context.Context, id int64) ([]FindingEvidence, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,finding_id,author_id,stance,body,created_at
FROM finding_evidence WHERE finding_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FindingEvidence{}
	for rows.Next() {
		var e FindingEvidence
		var created string
		if err := rows.Scan(&e.ID, &e.FindingID, &e.AuthorID, &e.Stance, &e.Body, &created); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) findingPositions(ctx context.Context, id int64) ([]FindingPosition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT finding_id,agent_id,choice,text,at
FROM finding_positions WHERE finding_id=? ORDER BY agent_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FindingPosition{}
	for rows.Next() {
		var p FindingPosition
		var at string
		if err := rows.Scan(&p.FindingID, &p.AgentID, &p.Choice, &p.Text, &at); err != nil {
			return nil, err
		}
		p.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) findingRevisions(ctx context.Context, id int64) ([]FindingRevision, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,finding_id,author_id,claim,created_at
FROM finding_revisions WHERE finding_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FindingRevision{}
	for rows.Next() {
		var r FindingRevision
		var created string
		if err := rows.Scan(&r.ID, &r.FindingID, &r.AuthorID, &r.Claim, &created); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, r)
	}
	return out, rows.Err()
}

func normaliseStance(stance string) string {
	if strings.EqualFold(strings.TrimSpace(stance), "oppose") {
		return "oppose"
	}
	return "support"
}

// SharedFinding is a finding outside its session, for the cross-session
// knowledge view. Session and author names are copied so the caller need not
// resolve them.
type SharedFinding struct {
	Finding
	SessionName string `json:"session_name"`
	SessionGoal string `json:"session_goal"`
	AuthorName  string `json:"author_name"`
}

// AllFindings lists findings across sessions, newest update first. Search matches
// the claim, its evidence, the session name, or the author name.
func (s *Store) AllFindings(ctx context.Context, search string, limit, offset int) ([]SharedFinding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT f.id,f.session_id,f.author_id,f.claim,f.status,f.created_at,f.updated_at,
COALESCE(n.name,''),s.goal,COALESCE(a.name,'')
FROM findings f
JOIN sessions s ON s.id=f.session_id
LEFT JOIN session_names n ON n.session_id=s.id
LEFT JOIN agents a ON a.id=f.author_id
WHERE (@q='' OR
  instr(lower(f.claim),lower(@q))>0 OR
  instr(lower(COALESCE(n.name,'')),lower(@q))>0 OR
  instr(lower(COALESCE(a.name,'')),lower(@q))>0 OR
  EXISTS (SELECT 1 FROM finding_evidence e WHERE e.finding_id=f.id AND instr(lower(e.body),lower(@q))>0))
ORDER BY f.updated_at DESC,f.id DESC LIMIT @limit OFFSET @offset`,
		sql.Named("q", search), sql.Named("limit", limit), sql.Named("offset", offset))
	if err != nil {
		return nil, err
	}
	out := []SharedFinding{}
	for rows.Next() {
		var f SharedFinding
		var created, updated string
		if err := rows.Scan(&f.ID, &f.SessionID, &f.AuthorID, &f.Claim, &f.Status, &created, &updated,
			&f.SessionName, &f.SessionGoal, &f.AuthorName); err != nil {
			rows.Close()
			return nil, err
		}
		f.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		f.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, f)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	// Load children only after the row set is closed: the store uses one
	// connection, so a nested query while rows are open would block.
	for i := range out {
		if out[i].Evidence, err = s.findingEvidence(ctx, out[i].ID); err != nil {
			return nil, err
		}
		if out[i].Positions, err = s.findingPositions(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}
