package store

import (
	"context"
	"database/sql"
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

func (s *Store) ListSessions(ctx context.Context, limit, offset int) ([]SessionEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,COALESCE(n.name,''),s.goal,s.status,
(SELECT COUNT(*) FROM agents a WHERE a.session_id=s.id),
COALESCE((SELECT MAX(created_at) FROM events e WHERE e.session_id=s.id),'') AS last_activity
FROM sessions s LEFT JOIN session_names n ON n.session_id=s.id
ORDER BY last_activity DESC,s.id LIMIT ? OFFSET ?`, limit, offset)
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
