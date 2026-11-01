package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Envelope is the scope a human grants to one session, and the budgets that
// bound it. Spec: docs/superpowers/specs/2026-11-01-tool-policy.md
//
// Every grant, tightening or revocation writes a new version rather than
// editing the old one, so a past decision can be read against the scope that
// was in force when it happened.
type Envelope struct {
	SessionID    string          `json:"session_id"`
	Version      int             `json:"version"`
	Armed        bool            `json:"armed"`
	DamageLine   string          `json:"damage_line"`
	TotalBudget  int             `json:"total_budget"`
	SafeBudget   int             `json:"safe_budget"`
	UnsafeBudget int             `json:"unsafe_budget"`
	IntervalMS   int             `json:"rate_interval_ms"`
	MaxBodyBytes int             `json:"max_body_bytes"`
	Entries      []EnvelopeEntry `json:"entries"`
}

// EnvelopeEntry is one host an agent may name. IPs is the set the name may
// resolve to; an empty set means the IPs recorded when the entry was granted.
type EnvelopeEntry struct {
	Name       string   `json:"name"`
	Ports      []int    `json:"ports"`
	Methods    []string `json:"methods"`
	PathPrefix string   `json:"path_prefix,omitempty"`
	IPs        []string `json:"ips"`
}

// RequestRecord is one attempt, kept whether it was allowed or refused.
// Records are append-only and carry no foreign key: they outlive the session.
type RequestRecord struct {
	ID         int64
	SessionID  string
	AgentID    string
	Host       string
	Port       int
	Method     string
	Path       string
	Purpose    string
	Expect     string
	Tier       string
	Status     string
	PinnedIP   string
	Reason     string
	BodyBytes  int
	BodyDigest string
	CreatedAt  time.Time
	DecidedAt  time.Time
	Decider    string
}

// AuditRecord is the human-readable trail: decisions, refusals, armings and
// revocations. It is not the request log.
type AuditRecord struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Kind      string    `json:"kind"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

var ErrNoEnvelope = errors.New("session has no tool envelope")

func toolsSchema(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS tool_envelopes(
  session_id TEXT NOT NULL, version INTEGER NOT NULL, armed INTEGER NOT NULL DEFAULT 0,
  damage_line TEXT NOT NULL DEFAULT '', total_budget INTEGER NOT NULL, safe_budget INTEGER NOT NULL,
  unsafe_budget INTEGER NOT NULL, rate_interval_ms INTEGER NOT NULL, max_body_bytes INTEGER NOT NULL,
  created_at TEXT NOT NULL, PRIMARY KEY(session_id, version));
CREATE TABLE IF NOT EXISTS tool_envelope_entries(
  session_id TEXT NOT NULL, version INTEGER NOT NULL, name TEXT NOT NULL,
  ports TEXT NOT NULL, methods TEXT NOT NULL, path_prefix TEXT NOT NULL DEFAULT '', ips TEXT NOT NULL,
  PRIMARY KEY(session_id, version, name));
CREATE TABLE IF NOT EXISTS tool_requests(
  id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, agent_id TEXT NOT NULL DEFAULT '',
  host TEXT NOT NULL, port INTEGER NOT NULL, method TEXT NOT NULL, path TEXT NOT NULL,
  purpose TEXT NOT NULL, expectation TEXT NOT NULL, tier TEXT NOT NULL, status TEXT NOT NULL,
  pinned_ip TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '', body_bytes INTEGER NOT NULL DEFAULT 0,
  body_digest TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, decided_at TEXT NOT NULL DEFAULT '',
  decider TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS tool_approvals(
  session_id TEXT NOT NULL, host TEXT NOT NULL, approver TEXT NOT NULL, created_at TEXT NOT NULL,
  PRIMARY KEY(session_id, host));
CREATE TABLE IF NOT EXISTS tool_audit(
  id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, kind TEXT NOT NULL,
  detail TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS tool_requests_session ON tool_requests(session_id, tier);
CREATE INDEX IF NOT EXISTS tool_audit_session ON tool_audit(session_id, id);`)
	return err
}

// SaveEnvelope records a new envelope version. The previous version stays.
func (s *Store) SaveEnvelope(ctx context.Context, e Envelope) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id=?`, e.SessionID).Scan(&exists); err != nil {
		return 0, err
	}
	if exists != 1 {
		return 0, sql.ErrNoRows
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM tool_envelopes WHERE session_id=?`, e.SessionID).Scan(&version); err != nil {
		return 0, err
	}
	// A new version always starts disarmed: scope changes never re-arm silently.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO tool_envelopes(session_id,version,armed,damage_line,total_budget,safe_budget,unsafe_budget,rate_interval_ms,max_body_bytes,created_at) VALUES(?,?,0,?,?,?,?,?,?,?)`,
		e.SessionID, version, e.DamageLine, e.TotalBudget, e.SafeBudget, e.UnsafeBudget, e.IntervalMS, e.MaxBodyBytes, now); err != nil {
		return 0, err
	}
	for _, entry := range e.Entries {
		ports, _ := json.Marshal(entry.Ports)
		methods, _ := json.Marshal(entry.Methods)
		ips, _ := json.Marshal(entry.IPs)
		if _, err := tx.ExecContext(ctx, `INSERT INTO tool_envelope_entries(session_id,version,name,ports,methods,path_prefix,ips) VALUES(?,?,?,?,?,?,?)`,
			e.SessionID, version, entry.Name, string(ports), string(methods), entry.PathPrefix, string(ips)); err != nil {
			return 0, err
		}
	}
	if err := audit(ctx, tx, e.SessionID, "envelope_granted", map[string]any{"version": version, "entries": len(e.Entries)}); err != nil {
		return 0, err
	}
	return version, tx.Commit()
}

// Envelope returns the latest granted scope, or ErrNoEnvelope.
func (s *Store) Envelope(ctx context.Context, sessionID string) (*Envelope, error) {
	e := &Envelope{SessionID: sessionID}
	var armed int
	var rateMS int
	err := s.db.QueryRowContext(ctx, `SELECT version,armed,damage_line,total_budget,safe_budget,unsafe_budget,rate_interval_ms,max_body_bytes FROM tool_envelopes WHERE session_id=? ORDER BY version DESC LIMIT 1`, sessionID).
		Scan(&e.Version, &armed, &e.DamageLine, &e.TotalBudget, &e.SafeBudget, &e.UnsafeBudget, &rateMS, &e.MaxBodyBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoEnvelope
	}
	if err != nil {
		return nil, err
	}
	e.Armed, e.IntervalMS = armed == 1, rateMS
	rows, err := s.db.QueryContext(ctx, `SELECT name,ports,methods,path_prefix,ips FROM tool_envelope_entries WHERE session_id=? AND version=? ORDER BY name`, sessionID, e.Version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry EnvelopeEntry
		var ports, methods, ips string
		if err := rows.Scan(&entry.Name, &ports, &methods, &entry.PathPrefix, &ips); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(ports), &entry.Ports)
		_ = json.Unmarshal([]byte(methods), &entry.Methods)
		_ = json.Unmarshal([]byte(ips), &entry.IPs)
		e.Entries = append(e.Entries, entry)
	}
	return e, rows.Err()
}

// SetArmed arms or disarms the current envelope version. Arming never survives
// a restart: startup pauses every session, and a paused session is unarmed.
func (s *Store) SetArmed(ctx context.Context, sessionID string, armed bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	value := 0
	if armed {
		value = 1
	}
	result, err := tx.ExecContext(ctx, `UPDATE tool_envelopes SET armed=? WHERE session_id=? AND version=(SELECT MAX(version) FROM tool_envelopes WHERE session_id=?)`, value, sessionID, sessionID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNoEnvelope
	}
	kind := "disarmed"
	if armed {
		kind = "armed"
	}
	if err := audit(ctx, tx, sessionID, kind, map[string]any{}); err != nil {
		return err
	}
	return tx.Commit()
}

// HostApproved reports whether a human has cleared this host for DELETE.
func (s *Store) HostApproved(ctx context.Context, sessionID, host string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_approvals WHERE session_id=? AND host=?`, sessionID, host).Scan(&n)
	return n > 0, err
}

// ApproveHost records the one-time human clearance for DELETE against a host.
func (s *Store) ApproveHost(ctx context.Context, sessionID, host, approver string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO tool_approvals(session_id,host,approver,created_at) VALUES(?,?,?,?) ON CONFLICT(session_id,host) DO NOTHING`,
		sessionID, host, approver, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if err := audit(ctx, tx, sessionID, "host_approved", map[string]any{"host": host, "approver": approver}); err != nil {
		return err
	}
	return tx.Commit()
}

// CountRequests counts attempts that consumed budget for one tier. Refusals do
// not consume budget; they are recorded, not counted.
func (s *Store) CountRequests(ctx context.Context, sessionID, tier string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_requests WHERE session_id=? AND (?='' OR tier=?) AND status IN ('allowed','approved','unknown')`, sessionID, tier, tier).Scan(&n)
	return n, err
}

// RecordRequest appends one attempt, allowed or refused.
func (s *Store) RecordRequest(ctx context.Context, r RequestRecord) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	created := r.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	decided := ""
	if !r.DecidedAt.IsZero() {
		decided = r.DecidedAt.UTC().Format(time.RFC3339Nano)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO tool_requests(session_id,agent_id,host,port,method,path,purpose,expectation,tier,status,pinned_ip,reason,body_bytes,body_digest,created_at,decided_at,decider) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.SessionID, r.AgentID, r.Host, r.Port, r.Method, r.Path, r.Purpose, r.Expect, r.Tier, r.Status, r.PinnedIP, r.Reason, r.BodyBytes, r.BodyDigest, created.UTC().Format(time.RFC3339Nano), decided, r.Decider)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := audit(ctx, tx, r.SessionID, "request_"+r.Status, map[string]any{
		"host": r.Host, "method": r.Method, "path": r.Path, "tier": r.Tier, "reason": r.Reason,
	}); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// Audit returns the trail for a session, newest last. It survives session
// deletion on purpose: deleting a session must not erase what its agents did.
func (s *Store) Audit(ctx context.Context, sessionID string) ([]AuditRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,session_id,kind,detail,created_at FROM tool_audit WHERE session_id=? ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditRecord{}
	for rows.Next() {
		var a AuditRecord
		var created string
		if err := rows.Scan(&a.ID, &a.SessionID, &a.Kind, &a.Detail, &created); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, a)
	}
	return out, rows.Err()
}

// audit is the in-transaction form, so a decision and its evidence commit together.
func audit(ctx context.Context, tx *sql.Tx, sessionID, kind string, detail map[string]any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO tool_audit(session_id,kind,detail,created_at) VALUES(?,?,?,?)`,
		sessionID, kind, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
