package store

import (
	"context"
	"database/sql"
	"errors"
)

// SessionEntry excludes conversation, private memory and agent output.
type SessionEntry struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Goal         string `json:"goal"`
	Status       string `json:"status"`
	AgentCount   int    `json:"agent_count"`
	LastActivity string `json:"last_activity"`
}

var ErrSessionNotPaused = errors.New("pause the session before deleting it")

// DeleteSession removes a paused run and all of its owned data in one transaction.
// It is not secure erasure: SQLite free pages and external backups may retain data.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM sessions WHERE id=?`, id).Scan(&status); err != nil {
		return err
	}
	if status != "paused" {
		return ErrSessionNotPaused
	}
	for _, statement := range []string{
		`DELETE FROM message_thinking WHERE message_id IN (SELECT id FROM messages WHERE session_id=?)`,
		`DELETE FROM message_routing WHERE message_id IN (SELECT id FROM messages WHERE session_id=?)`,
		`DELETE FROM agent_recovery WHERE agent_id IN (SELECT id FROM agents WHERE session_id=?)`,
		`DELETE FROM agent_inflight WHERE agent_id IN (SELECT id FROM agents WHERE session_id=?)`,
		`DELETE FROM agent_memory WHERE agent_id IN (SELECT id FROM agents WHERE session_id=?)`,
		`DELETE FROM agent_checkpoints WHERE agent_id IN (SELECT id FROM agents WHERE session_id=?)`,
		`DELETE FROM agent_settings WHERE agent_id IN (SELECT id FROM agents WHERE session_id=?)`,
		`DELETE FROM delivery_baselines WHERE agent_id IN (SELECT id FROM agents WHERE session_id=?)`,
		`DELETE FROM agent_failures WHERE session_id=?`,
		`DELETE FROM network_policies WHERE session_id=?`,
		`DELETE FROM inference_policies WHERE session_id=?`,
		`DELETE FROM session_names WHERE session_id=?`,
		`DELETE FROM events WHERE session_id=?`,
		`DELETE FROM messages WHERE session_id=?`,
		`DELETE FROM agents WHERE session_id=?`,
		`DELETE FROM sessions WHERE id=?`,
	} {
		if _, err := tx.ExecContext(ctx, statement, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type SessionQuery struct {
	Search string
	Status string
	Limit  int
	Offset int
}

func (s *Store) ListSessions(ctx context.Context, query SessionQuery) ([]SessionEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,COALESCE(n.name,''),s.goal,s.status,
(SELECT COUNT(*) FROM agents a WHERE a.session_id=s.id),
COALESCE((SELECT MAX(created_at) FROM events e WHERE e.session_id=s.id),'') AS last_activity
FROM sessions s LEFT JOIN session_names n ON n.session_id=s.id
WHERE (?='' OR s.status=?) AND (?='' OR
instr(lower(COALESCE(n.name,'')),lower(?))>0 OR
instr(lower(s.goal),lower(?))>0 OR instr(lower(s.id),lower(?))>0)
ORDER BY last_activity DESC,s.id LIMIT ? OFFSET ?`, query.Status, query.Status, query.Search, query.Search, query.Search, query.Search, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionEntry{}
	for rows.Next() {
		var entry SessionEntry
		if err := rows.Scan(&entry.ID, &entry.Name, &entry.Goal, &entry.Status, &entry.AgentCount, &entry.LastActivity); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (s *Store) RenameSession(ctx context.Context, id, name string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO session_names(session_id,name) SELECT id,? FROM sessions WHERE id=? ON CONFLICT(session_id) DO UPDATE SET name=excluded.name`, name, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	if err := event(ctx, tx, id, "session_renamed", map[string]string{"name": name}); err != nil {
		return err
	}
	return tx.Commit()
}
