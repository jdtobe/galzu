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
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Goal               string `json:"goal"`
	Status             string `json:"status"`
	Concurrency        int    `json:"concurrency"`
	TargetIP           string `json:"target_ip,omitempty"`
	RateIntervalMS     int    `json:"rate_interval_ms,omitempty"`
	InferenceTimeoutMS int    `json:"inference_timeout_ms,omitempty"`
	RetryDelaysMS      []int  `json:"retry_delays_ms,omitempty"`
}
type Agent struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Model     string `json:"model"`
	Status    string `json:"status"`
	Error     string `json:"error"`
	Thinking  bool   `json:"thinking"`
}
type Message struct {
	ID          int64     `json:"id"`
	SessionID   string    `json:"session_id"`
	AgentID     string    `json:"agent_id"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"created_at"`
	Thinking    string    `json:"thinking,omitempty"`
	RecipientID string    `json:"recipient_id,omitempty"`
	Kind        string    `json:"kind,omitempty"`
}
type Event struct {
	ID        int64           `json:"id"`
	SessionID string          `json:"session_id"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}
type Store struct{ db *sql.DB }

const MaxAgentMemoryBytes = 2048

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
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS agent_settings(agent_id TEXT PRIMARY KEY REFERENCES agents(id), thinking INTEGER NOT NULL DEFAULT 0); CREATE TABLE IF NOT EXISTS message_thinking(message_id INTEGER PRIMARY KEY REFERENCES messages(id), content TEXT NOT NULL)`)
	}
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS message_routing(message_id INTEGER PRIMARY KEY REFERENCES messages(id), recipient_id TEXT NOT NULL, kind TEXT NOT NULL)`)
	}
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS agent_checkpoints(agent_id TEXT PRIMARY KEY REFERENCES agents(id), message_id INTEGER NOT NULL)`)
	}
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS agent_recovery(agent_id TEXT PRIMARY KEY REFERENCES agents(id), failures INTEGER NOT NULL, next_at TEXT NOT NULL, category TEXT NOT NULL, reason TEXT NOT NULL, excerpt TEXT NOT NULL); CREATE TABLE IF NOT EXISTS agent_failures(id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL REFERENCES sessions(id), agent_id TEXT NOT NULL REFERENCES agents(id), attempt INTEGER NOT NULL, category TEXT NOT NULL, reason TEXT NOT NULL, excerpt TEXT NOT NULL, created_at TEXT NOT NULL)`)
	}
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS storage_probe(id INTEGER PRIMARY KEY, checked_at TEXT NOT NULL)`)
	}
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS delivery_baselines(agent_id TEXT PRIMARY KEY REFERENCES agents(id), message_id INTEGER NOT NULL); INSERT INTO delivery_baselines(agent_id,message_id) SELECT a.id,COALESCE((SELECT MAX(id) FROM messages WHERE session_id=a.session_id),0) FROM agents a WHERE NOT EXISTS (SELECT 1 FROM delivery_baselines b WHERE b.agent_id=a.id)`)
	}
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS inference_policies(session_id TEXT PRIMARY KEY REFERENCES sessions(id), timeout_ms INTEGER NOT NULL, retry_delays TEXT NOT NULL)`)
	}
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS agent_inflight(agent_id TEXT PRIMARY KEY REFERENCES agents(id), started_at TEXT NOT NULL)`)
	}
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS agent_memory(agent_id TEXT PRIMARY KEY REFERENCES agents(id), content TEXT NOT NULL, fresh INTEGER NOT NULL)`)
	}
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS agent_prompt_state(agent_id TEXT PRIMARY KEY REFERENCES agents(id), cut_message_id INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL)`)
	}
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS session_names(session_id TEXT PRIMARY KEY REFERENCES sessions(id), name TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS events_session_activity ON events(session_id,created_at);
CREATE INDEX IF NOT EXISTS agents_session ON agents(session_id);`)
	}
	if err == nil {
		err = toolsSchema(db)
	}
	if err == nil {
		err = governanceSchema(db)
	}
	if err == nil {
		err = plansSchema(db)
	}
	if err == nil {
		err = workSchema(db)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db}, nil
}
func (s *Store) Close() error { return s.db.Close() }

// ProbeWrite commits a real SQLite write, not merely a read or rolled-back transaction.
func (s *Store) ProbeWrite(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO storage_probe(id,checked_at) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET checked_at=excluded.checked_at`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) BeginAttempt(ctx context.Context, sid, aid string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	when := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_inflight(agent_id,started_at) VALUES(?,?)`, aid, when); err != nil {
		return err
	}
	if err = event(ctx, tx, sid, "inference_started", map[string]string{"agent_id": aid, "started_at": when}); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) InterruptAttempts(ctx context.Context, sid string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT i.agent_id,i.started_at FROM agent_inflight i JOIN agents a ON a.id=i.agent_id WHERE a.session_id=?`, sid)
	if err != nil {
		return err
	}
	type attempt struct{ id, started string }
	var attempts []attempt
	for rows.Next() {
		var a attempt
		if err = rows.Scan(&a.id, &a.started); err != nil {
			rows.Close()
			return err
		}
		attempts = append(attempts, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, a := range attempts {
		if err = event(ctx, tx, sid, "inference_interrupted", map[string]string{"agent_id": a.id, "started_at": a.started}); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO agent_failures(session_id,agent_id,attempt,category,reason,excerpt,created_at) VALUES(?,?,0,'interrupted','Inference interrupted; no retry consumed','',?)`, sid, a.id, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM agent_inflight WHERE agent_id=?`, a.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type Recovery struct {
	Failures int       `json:"failures"`
	NextAt   time.Time `json:"next_at"`
	Category string    `json:"category"`
	Reason   string    `json:"reason"`
	Excerpt  string    `json:"excerpt,omitempty"`
}
type Failure struct {
	AgentID   string `json:"agent_id"`
	Attempt   int    `json:"attempt"`
	Category  string `json:"category"`
	Reason    string `json:"reason"`
	Excerpt   string `json:"excerpt,omitempty"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) Failures(ctx context.Context, sessionID string) ([]Failure, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT agent_id,attempt,category,reason,excerpt,created_at FROM agent_failures WHERE session_id=? ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Failure
	for rows.Next() {
		var f Failure
		if err = rows.Scan(&f.AgentID, &f.Attempt, &f.Category, &f.Reason, &f.Excerpt, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) Recovery(ctx context.Context, agentID string) (Recovery, error) {
	var v Recovery
	var next string
	err := s.db.QueryRowContext(ctx, `SELECT failures,next_at,category,reason,excerpt FROM agent_recovery WHERE agent_id=?`, agentID).Scan(&v.Failures, &next, &v.Category, &v.Reason, &v.Excerpt)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	if next != "" {
		v.NextAt, err = time.Parse(time.RFC3339Nano, next)
	}
	return v, err
}

func (s *Store) RecordFailure(ctx context.Context, sid, aid, category, reason, excerpt string, next time.Time, failures int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	nextText := ""
	if !next.IsZero() {
		nextText = next.UTC().Format(time.RFC3339Nano)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_recovery VALUES(?,?,?,?,?,?) ON CONFLICT(agent_id) DO UPDATE SET failures=excluded.failures,next_at=excluded.next_at,category=excluded.category,reason=excluded.reason,excerpt=excluded.excerpt`, aid, failures, nextText, category, reason, excerpt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_failures(session_id,agent_id,attempt,category,reason,excerpt,created_at) VALUES(?,?,?,?,?,?,?)`, sid, aid, failures, category, reason, excerpt, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM agent_inflight WHERE agent_id=?`, aid); err != nil {
		return err
	}
	if next.IsZero() {
		if _, err = tx.ExecContext(ctx, `UPDATE agents SET status='error',error=? WHERE id=? AND session_id=?`, reason, aid, sid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ClearRecovery(ctx context.Context, aid string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agent_recovery WHERE agent_id=?`, aid)
	return err
}
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
	if _, e = tx.ExecContext(ctx, `INSERT INTO session_names(session_id,name) VALUES(?,?)`, v.ID, v.Name); e != nil {
		return e
	}
	if v.TargetIP != "" {
		if _, e = tx.ExecContext(ctx, `INSERT INTO network_policies VALUES(?,?,?)`, v.ID, v.TargetIP, v.RateIntervalMS); e != nil {
			return e
		}
	}
	if v.InferenceTimeoutMS > 0 && v.RetryDelaysMS != nil {
		b, err := json.Marshal(v.RetryDelaysMS)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO inference_policies VALUES(?,?,?)`, v.ID, v.InferenceTimeoutMS, string(b)); err != nil {
			return err
		}
	}
	for _, a := range agents {
		if _, e = tx.ExecContext(ctx, `INSERT INTO agents VALUES(?,?,?,?,?,?,?)`, a.ID, a.SessionID, a.Name, a.Role, a.Model, a.Status, a.Error); e != nil {
			return e
		}
		if a.Thinking {
			if _, e = tx.ExecContext(ctx, `INSERT INTO agent_settings(agent_id,thinking) VALUES(?,1)`, a.ID); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO agent_checkpoints(agent_id,message_id) VALUES(?,0)`, a.ID); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO delivery_baselines(agent_id,message_id) VALUES(?,0)`, a.ID); e != nil {
			return e
		}
	}
	if e = event(ctx, tx, v.ID, "session_created", v); e != nil {
		return e
	}
	return tx.Commit()
}

type DeliveryProgress struct {
	Checkpoint *int64 `json:"checkpoint"`
	Baseline   int64  `json:"baseline"`
}

func (s *Store) DeliveryProgress(ctx context.Context, sessionID string) (map[string]DeliveryProgress, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,c.message_id,b.message_id FROM agents a LEFT JOIN agent_checkpoints c ON c.agent_id=a.id JOIN delivery_baselines b ON b.agent_id=a.id WHERE a.session_id=?`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]DeliveryProgress{}
	for rows.Next() {
		var id string
		var cp sql.NullInt64
		var p DeliveryProgress
		if err = rows.Scan(&id, &cp, &p.Baseline); err != nil {
			return nil, err
		}
		if cp.Valid {
			p.Checkpoint = &cp.Int64
		}
		out[id] = p
	}
	return out, rows.Err()
}

// Checkpoint initializes older agents at the upgrade boundary. Messages added
// after migration remain pending even if the agent has not started yet.
func (s *Store) Checkpoint(ctx context.Context, sessionID, agentID string) (int64, error) {
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_checkpoints(agent_id,message_id) SELECT id, COALESCE((SELECT message_id FROM delivery_baselines WHERE agent_id=agents.id),0) FROM agents WHERE id=? AND session_id=? ON CONFLICT(agent_id) DO NOTHING`, agentID, sessionID)
	if err != nil {
		return 0, err
	}
	var last int64
	err = s.db.QueryRowContext(ctx, `SELECT c.message_id FROM agent_checkpoints c JOIN agents a ON a.id=c.agent_id WHERE a.id=? AND a.session_id=?`, agentID, sessionID).Scan(&last)
	return last, err
}

func (s *Store) SetCheckpoint(ctx context.Context, sessionID, agentID string, last int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agent_checkpoints SET message_id=MAX(message_id,?) WHERE agent_id=? AND EXISTS (SELECT 1 FROM agents WHERE id=? AND session_id=?)`, last, agentID, agentID, sessionID)
	return err
}
func (s *Store) AgentMemory(ctx context.Context, sessionID, agentID string) (string, bool, error) {
	var memory string
	var fresh bool
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(m.content,''),COALESCE(m.fresh,0) FROM agents a LEFT JOIN agent_memory m ON m.agent_id=a.id WHERE a.session_id=? AND a.id=?`, sessionID, agentID).Scan(&memory, &fresh)
	return memory, fresh, err
}
func saveMemory(ctx context.Context, tx *sql.Tx, agentID string, memory *string) error {
	if memory == nil {
		_, err := tx.ExecContext(ctx, `UPDATE agent_memory SET fresh=0 WHERE agent_id=?`, agentID)
		return err
	}
	if len(*memory) > MaxAgentMemoryBytes {
		return errors.New("agent memory exceeds size limit")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO agent_memory(agent_id,content,fresh) VALUES(?,?,1) ON CONFLICT(agent_id) DO UPDATE SET content=excluded.content,fresh=1`, agentID, *memory)
	return err
}
func (s *Store) CompleteRest(ctx context.Context, sessionID, agentID string, last int64) error {
	return s.CompleteRestWithMemory(ctx, sessionID, agentID, last, nil)
}
func (s *Store) CompleteRestWithMemory(ctx context.Context, sessionID, agentID string, last int64, memory *string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE agent_checkpoints SET message_id=MAX(message_id,?) WHERE agent_id=? AND EXISTS (SELECT 1 FROM agents WHERE id=? AND session_id=?)`, last, agentID, agentID, sessionID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	if err = saveMemory(ctx, tx, agentID, memory); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM agent_recovery WHERE agent_id=?`, agentID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM agent_inflight WHERE agent_id=?`, agentID); err != nil {
		return err
	}
	if err = event(ctx, tx, sessionID, "agent_rest", map[string]any{"agent_id": agentID, "checkpoint": last}); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) GetSession(ctx context.Context, id string) (Session, []Agent, error) {
	var v Session
	var delays string
	e := s.db.QueryRowContext(ctx, `SELECT s.id,COALESCE(n.name,''),s.goal,s.status,s.concurrency,COALESCE(p.target_ip,''),COALESCE(p.rate_interval_ms,0),COALESCE(i.timeout_ms,600000),COALESCE(i.retry_delays,'[2000,10000,30000]') FROM sessions s LEFT JOIN session_names n ON n.session_id=s.id LEFT JOIN network_policies p ON p.session_id=s.id LEFT JOIN inference_policies i ON i.session_id=s.id WHERE s.id=?`, id).Scan(&v.ID, &v.Name, &v.Goal, &v.Status, &v.Concurrency, &v.TargetIP, &v.RateIntervalMS, &v.InferenceTimeoutMS, &delays)
	if e != nil {
		return v, nil, e
	}
	if e = json.Unmarshal([]byte(delays), &v.RetryDelaysMS); e != nil {
		return v, nil, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT a.id,a.session_id,a.name,a.role,a.model,a.status,a.error,COALESCE(p.thinking,0) FROM agents a LEFT JOIN agent_settings p ON p.agent_id=a.id WHERE a.session_id=? ORDER BY a.rowid`, id)
	if e != nil {
		return v, nil, e
	}
	defer rows.Close()
	var a []Agent
	for rows.Next() {
		var x Agent
		if e = rows.Scan(&x.ID, &x.SessionID, &x.Name, &x.Role, &x.Model, &x.Status, &x.Error, &x.Thinking); e != nil {
			return v, nil, e
		}
		a = append(a, x)
	}
	return v, a, rows.Err()
}
func (s *Store) SetThinking(ctx context.Context, sessionID, agentID string, enabled bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT 1 FROM agents WHERE id=? AND session_id=?`, agentID, sessionID).Scan(&exists); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_settings(agent_id,thinking) VALUES(?,?) ON CONFLICT(agent_id) DO UPDATE SET thinking=excluded.thinking`, agentID, enabled); err != nil {
		return err
	}
	if err = event(ctx, tx, sessionID, "agent_settings", map[string]any{"agent_id": agentID, "thinking": enabled}); err != nil {
		return err
	}
	return tx.Commit()
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
	return s.AppendMessageWithThinking(ctx, sid, aid, body, "")
}
func (s *Store) AppendMessageWithThinking(ctx context.Context, sid, aid, body, thinking string) (Message, error) {
	return s.AppendSocialMessage(ctx, sid, aid, "", "", body, thinking)
}
func (s *Store) AppendSocialMessage(ctx context.Context, sid, aid, recipient, kind, body, thinking string) (Message, error) {
	return s.appendSocialMessage(ctx, sid, aid, recipient, kind, body, thinking, nil, nil)
}

// AppendAgentReply commits the reply and its input checkpoint atomically.
func (s *Store) AppendAgentReply(ctx context.Context, sid, aid, recipient, kind, body, thinking string, checkpoint int64) (Message, error) {
	return s.appendSocialMessage(ctx, sid, aid, recipient, kind, body, thinking, &checkpoint, nil)
}

func (s *Store) AppendAgentReplyWithMemory(ctx context.Context, sid, aid, recipient, kind, body, thinking string, checkpoint int64, memory *string) (Message, error) {
	return s.appendSocialMessage(ctx, sid, aid, recipient, kind, body, thinking, &checkpoint, memory)
}

func (s *Store) appendSocialMessage(ctx context.Context, sid, aid, recipient, kind, body, thinking string, checkpoint *int64, memory *string) (Message, error) {
	if kind != "" && kind != "direct" && kind != "help" && kind != "available" {
		return Message{}, errors.New("invalid message kind")
	}
	if (kind == "direct" && recipient == "") || (kind == "" && recipient != "") {
		return Message{}, errors.New("invalid recipient")
	}
	m := Message{SessionID: sid, AgentID: aid, RecipientID: recipient, Kind: kind, Body: body, CreatedAt: time.Now().UTC()}
	m.Thinking = thinking
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return m, e
	}
	defer tx.Rollback()
	if recipient != "" {
		var exists int
		if e = tx.QueryRowContext(ctx, `SELECT 1 FROM agents WHERE id=? AND session_id=?`, recipient, sid).Scan(&exists); e != nil {
			return m, e
		}
	}
	r, e := tx.ExecContext(ctx, `INSERT INTO messages(session_id,agent_id,body,created_at) VALUES(?,?,?,?)`, sid, aid, body, m.CreatedAt.Format(time.RFC3339Nano))
	if e != nil {
		return m, e
	}
	m.ID, e = r.LastInsertId()
	if e != nil {
		return m, e
	}
	if thinking != "" {
		if _, e = tx.ExecContext(ctx, `INSERT INTO message_thinking(message_id,content) VALUES(?,?)`, m.ID, thinking); e != nil {
			return m, e
		}
	}
	if kind != "" {
		if _, e = tx.ExecContext(ctx, `INSERT INTO message_routing(message_id,recipient_id,kind) VALUES(?,?,?)`, m.ID, recipient, kind); e != nil {
			return m, e
		}
	}
	if e = event(ctx, tx, sid, "message", m); e != nil {
		return m, e
	}
	if checkpoint != nil {
		result, err := tx.ExecContext(ctx, `UPDATE agent_checkpoints SET message_id=MAX(message_id,?) WHERE agent_id=? AND EXISTS (SELECT 1 FROM agents WHERE id=? AND session_id=?)`, *checkpoint, aid, aid, sid)
		if err != nil {
			return m, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return m, err
		}
		if count != 1 {
			return m, sql.ErrNoRows
		}
		if err = saveMemory(ctx, tx, aid, memory); err != nil {
			return m, err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM agent_recovery WHERE agent_id=?`, aid); err != nil {
			return m, err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM agent_inflight WHERE agent_id=?`, aid); err != nil {
			return m, err
		}
	}
	return m, tx.Commit()
}
func (s *Store) Messages(ctx context.Context, id string, after int64) ([]Message, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT m.id,m.session_id,m.agent_id,m.body,m.created_at,COALESCE(t.content,''),COALESCE(r.recipient_id,''),COALESCE(r.kind,'') FROM messages m LEFT JOIN message_thinking t ON t.message_id=m.id LEFT JOIN message_routing r ON r.message_id=m.id WHERE m.session_id=? AND m.id>? ORDER BY m.id`, id, after)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var t string
		if e = rows.Scan(&m.ID, &m.SessionID, &m.AgentID, &m.Body, &t, &m.Thinking, &m.RecipientID, &m.Kind); e != nil {
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
