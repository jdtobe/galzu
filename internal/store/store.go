package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

type Session struct {
	ID             string `json:"id"`
	Goal           string `json:"goal"`
	Status         string `json:"status"`
	Concurrency    int    `json:"concurrency"`
	TargetIP       string `json:"target_ip,omitempty"`
	RateIntervalMS int    `json:"rate_interval_ms,omitempty"`
}
type Agent struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Model     string `json:"model"`
	Status    string `json:"status"`
	Error     string `json:"error"`
}
type Message struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	AgentID   string    `json:"agent_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
type Event struct {
	ID        int64           `json:"id"`
	SessionID string          `json:"session_id"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}
type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA foreign_keys=ON;
 CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY, goal TEXT NOT NULL, status TEXT NOT NULL, concurrency INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS agents(id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES sessions(id), name TEXT NOT NULL, role TEXT NOT NULL, model TEXT NOT NULL, status TEXT NOT NULL, error TEXT NOT NULL DEFAULT '');
 CREATE TABLE IF NOT EXISTS messages(id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL REFERENCES sessions(id), agent_id TEXT NOT NULL, body TEXT NOT NULL, created_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS events(id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL REFERENCES sessions(id), kind TEXT NOT NULL, payload TEXT NOT NULL, created_at TEXT NOT NULL);`)
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS network_policies(session_id TEXT PRIMARY KEY REFERENCES sessions(id), target_ip TEXT NOT NULL, rate_interval_ms INTEGER NOT NULL)`)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func event(ctx context.Context, tx *sql.Tx, id, kind string, payload any) error {
	b, e := json.Marshal(payload)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO events(session_id,kind,payload,created_at) VALUES(?,?,?,?)`, id, kind, string(b), time.Now().UTC().Format(time.RFC3339Nano))
	return e
}
func (s *Store) CreateSession(ctx context.Context, v Session, agents []Agent) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `INSERT INTO sessions VALUES(?,?,?,?)`, v.ID, v.Goal, v.Status, v.Concurrency); e != nil {
		return e
	}
	if v.TargetIP != "" {
		if _, e = tx.ExecContext(ctx, `INSERT INTO network_policies VALUES(?,?,?)`, v.ID, v.TargetIP, v.RateIntervalMS); e != nil {
			return e
		}
	}
	for _, a := range agents {
		if _, e = tx.ExecContext(ctx, `INSERT INTO agents VALUES(?,?,?,?,?,?,?)`, a.ID, a.SessionID, a.Name, a.Role, a.Model, a.Status, a.Error); e != nil {
			return e
		}
	}
	if e = event(ctx, tx, v.ID, "session_created", v); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) GetSession(ctx context.Context, id string) (Session, []Agent, error) {
	var v Session
	e := s.db.QueryRowContext(ctx, `SELECT s.id,s.goal,s.status,s.concurrency,COALESCE(p.target_ip,''),COALESCE(p.rate_interval_ms,0) FROM sessions s LEFT JOIN network_policies p ON p.session_id=s.id WHERE s.id=?`, id).Scan(&v.ID, &v.Goal, &v.Status, &v.Concurrency, &v.TargetIP, &v.RateIntervalMS)
	if e != nil {
		return v, nil, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT id,session_id,name,role,model,status,error FROM agents WHERE session_id=? ORDER BY rowid`, id)
	if e != nil {
		return v, nil, e
	}
	defer rows.Close()
	var a []Agent
	for rows.Next() {
		var x Agent
		if e = rows.Scan(&x.ID, &x.SessionID, &x.Name, &x.Role, &x.Model, &x.Status, &x.Error); e != nil {
			return v, nil, e
		}
		a = append(a, x)
	}
	return v, a, rows.Err()
}
func (s *Store) SetSessionStatus(ctx context.Context, id, status string) error {
	if status != "running" && status != "paused" {
		return errors.New("invalid status")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `UPDATE sessions SET status=? WHERE id=?`, status, id)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	if e = event(ctx, tx, id, "session_status", map[string]string{"status": status}); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) SetAgentStatus(ctx context.Context, id, status, detail string) error {
	if status != "ready" && status != "error" {
		return errors.New("invalid status")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var sid string
	if e = tx.QueryRowContext(ctx, `SELECT session_id FROM agents WHERE id=?`, id).Scan(&sid); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE agents SET status=?,error=? WHERE id=?`, status, detail, id); e != nil {
		return e
	}
	if e = event(ctx, tx, sid, "agent_status", map[string]string{"id": id, "status": status, "error": detail}); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) AppendMessage(ctx context.Context, sid, aid, body string) (Message, error) {
	m := Message{SessionID: sid, AgentID: aid, Body: body, CreatedAt: time.Now().UTC()}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return m, e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `INSERT INTO messages(session_id,agent_id,body,created_at) VALUES(?,?,?,?)`, sid, aid, body, m.CreatedAt.Format(time.RFC3339Nano))
	if e != nil {
		return m, e
	}
	m.ID, e = r.LastInsertId()
	if e != nil {
		return m, e
	}
	if e = event(ctx, tx, sid, "message", m); e != nil {
		return m, e
	}
	return m, tx.Commit()
}
func (s *Store) Messages(ctx context.Context, id string, after int64) ([]Message, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,session_id,agent_id,body,created_at FROM messages WHERE session_id=? AND id>? ORDER BY id`, id, after)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var t string
		if e = rows.Scan(&m.ID, &m.SessionID, &m.AgentID, &m.Body, &t); e != nil {
			return nil, e
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, t)
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Store) Events(ctx context.Context, id string, after int64) ([]Event, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,session_id,kind,payload,created_at FROM events WHERE session_id=? AND id>? ORDER BY id`, id, after)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var x Event
		var t, p string
		if e = rows.Scan(&x.ID, &x.SessionID, &x.Kind, &p, &t); e != nil {
			return nil, e
		}
		x.Payload = json.RawMessage(p)
		x.CreatedAt, _ = time.Parse(time.RFC3339Nano, t)
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) PauseAll(ctx context.Context) error {
	_, e := s.db.ExecContext(ctx, `UPDATE sessions SET status='paused' WHERE status='running'`)
	return e
}
