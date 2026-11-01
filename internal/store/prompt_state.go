package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// PromptCut is the newest message ID excluded from an agent's append-only
// prompt. Everything after it is rendered in order. The cut only moves forward,
// so the prompt prefix stays stable between compactions.
func (s *Store) PromptCut(ctx context.Context, agentID string) (int64, error) {
	var cut int64
	err := s.db.QueryRowContext(ctx, `SELECT cut_message_id FROM agent_prompt_state WHERE agent_id=?`, agentID).Scan(&cut)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return cut, err
}

func (s *Store) SetPromptCut(ctx context.Context, agentID string, cut int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_prompt_state(agent_id,cut_message_id,updated_at) VALUES(?,?,?)
ON CONFLICT(agent_id) DO UPDATE SET cut_message_id=excluded.cut_message_id,updated_at=excluded.updated_at`, agentID, cut, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
