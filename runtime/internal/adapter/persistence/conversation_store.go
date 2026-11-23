package persistence

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	runsapp "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/frame/sqlite"
	"github.com/Tangerg/scope/core/chat"
)

type ConversationStore struct{ db *database }

func newConversationStore(db *database) *ConversationStore {
	return &ConversationStore{db: db}
}

// A malformed row fails the whole read: message count is a durable Run/compaction
// coordinate, so omitting a row would shift every later watermark.
func (c *ConversationStore) Read(ctx context.Context, sessionID identity.SessionID) ([]chat.Message, error) {
	if err := validateResource("read conversation", sessionID); err != nil {
		return nil, err
	}
	return scopedValue(ctx, c.db.read, func(ctx context.Context) ([]chat.Message, error) {
		rows, err := c.db.connection(ctx).QueryContext(ctx, `SELECT seq, message FROM messages WHERE session_id = ? ORDER BY seq`, sessionID)
		if err != nil {
			return nil, fmt.Errorf("sqlite: read messages: %w", err)
		}
		defer rows.Close()
		out := make([]chat.Message, 0)
		for rows.Next() {
			var seq int64
			var blob string
			if err := rows.Scan(&seq, &blob); err != nil {
				return nil, fmt.Errorf("sqlite: scan message: %w", err)
			}
			var msg chat.Message
			if err := json.Unmarshal([]byte(blob), &msg); err != nil {
				return nil, fmt.Errorf("sqlite: decode message row %d: %w", seq, err)
			}
			out = append(out, msg)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite: read messages: %w", err)
		}
		return out, nil
	})
}

func (c *ConversationStore) Write(ctx context.Context, sessionID identity.SessionID, messages ...chat.Message) error {
	err := c.append(ctx, sessionID, messages)
	if errors.Is(err, sqlite.ErrIndeterminate) {
		return errors.Join(runsapp.ErrConversationWriteUncertain, err)
	}
	return err
}

func (c *ConversationStore) append(ctx context.Context, sessionID identity.SessionID, messages []chat.Message) error {
	if err := validateResource("write conversation", sessionID); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	for index := range messages {
		if err := messages[index].Validate(); err != nil {
			return fmt.Errorf("sqlite: write message %d: %w", index+1, err)
		}
	}
	return c.db.write(ctx, func(ctx context.Context) error {
		q := c.db.connection(ctx)
		for _, msg := range messages {
			data, err := encodeStoredJSON(msg)
			if err != nil {
				return fmt.Errorf("sqlite: marshal message: %w", err)
			}
			if _, err := q.ExecContext(ctx, `INSERT INTO messages(session_id, message) VALUES (?, ?)`, sessionID, string(data)); err != nil {
				return fmt.Errorf("sqlite: append message: %w", err)
			}
		}
		return nil
	})
}

// Clear followed by Write would lose the conversation if Write failed after Clear
// committed, so the rewrite is one transaction.
func (c *ConversationStore) Replace(ctx context.Context, sessionID identity.SessionID, messages ...chat.Message) error {
	if err := validateResource("replace conversation", sessionID); err != nil {
		return err
	}
	return c.db.write(ctx, func(ctx context.Context) error {
		if err := c.Clear(ctx, sessionID); err != nil {
			return err
		}
		return c.append(ctx, sessionID, messages)
	})
}

func (c *ConversationStore) Truncate(ctx context.Context, sessionID identity.SessionID, keepN int) error {
	if err := validateResource("truncate conversation", sessionID); err != nil {
		return err
	}
	if keepN < 0 {
		return fmt.Errorf("sqlite: truncate messages: keep count %d is negative", keepN)
	}
	return c.db.write(ctx, func(ctx context.Context) error {
		if _, err := c.db.connection(ctx).ExecContext(ctx, `DELETE FROM messages
		  WHERE session_id = ?
		    AND seq IN (
		      SELECT seq FROM messages WHERE session_id = ? ORDER BY seq LIMIT -1 OFFSET ?
		    )`, sessionID, sessionID, keepN); err != nil {
			return fmt.Errorf("sqlite: truncate messages: %w", err)
		}
		return nil
	})
}

func (c *ConversationStore) Count(ctx context.Context, sessionID identity.SessionID) (int, error) {
	if err := validateResource("count conversation", sessionID); err != nil {
		return 0, err
	}
	return scopedValue(ctx, c.db.read, func(ctx context.Context) (int, error) {
		var n int
		if err := c.db.connection(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE session_id = ?`, sessionID).Scan(&n); err != nil {
			return 0, fmt.Errorf("sqlite: count messages: %w", err)
		}
		return n, nil
	})
}

func (c *ConversationStore) Clear(ctx context.Context, sessionID identity.SessionID) error {
	if err := validateResource("clear conversation", sessionID); err != nil {
		return err
	}
	return c.db.write(ctx, func(ctx context.Context) error {
		if _, err := c.db.connection(ctx).ExecContext(ctx, `DELETE FROM messages WHERE session_id = ?`, sessionID); err != nil {
			return fmt.Errorf("sqlite: clear messages: %w", err)
		}
		return nil
	})
}
