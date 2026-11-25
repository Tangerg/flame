package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/plugins/memory/internal/agentmemory"
	"github.com/Tangerg/flame/plugins/memory/internal/management"
)

func (a *AgentMemoryStore) PrepareUpdate(ctx context.Context, id agentmemory.ItemID, expectedRevision uint64, content *agentmemory.Content, pinned *bool, now time.Time) (management.Mutation, error) {
	current, found, err := a.Get(ctx, id)
	if err != nil {
		return management.Mutation{}, err
	}
	if !found {
		return management.Mutation{}, agentmemory.ErrNotFound
	}
	change, err := current.Edit(expectedRevision, content, pinned, now)
	if err != nil {
		return management.Mutation{}, err
	}
	if !change.Changed() {
		return management.New(current, nil), nil
	}
	item := change.Item()
	if item.Content() != current.Content() {
		existing, found, err := a.itemByDigest(ctx, item.Scope(), item.Project(), agentmemory.Digest(item.Content()))
		if err != nil {
			return management.Mutation{}, err
		}
		if found && existing.ID() != item.ID() {
			return management.Mutation{}, agentmemory.ErrDuplicateContent
		}
	}
	return management.New(item, func(ctx context.Context) error {
		return a.replaceItem(ctx, change)
	}), nil
}

func (a *AgentMemoryStore) PrepareReview(ctx context.Context, id agentmemory.ItemID, expectedRevision uint64, decision agentmemory.ReviewDecision, now time.Time) (management.Mutation, error) {
	current, found, err := a.Get(ctx, id)
	if err != nil {
		return management.Mutation{}, err
	}
	if !found {
		return management.Mutation{}, agentmemory.ErrNotFound
	}
	replacement, err := current.Review(expectedRevision, decision, now)
	if err != nil {
		return management.Mutation{}, err
	}
	return management.New(replacement, func(ctx context.Context) error {
		result, err := a.db.Executor(ctx).ExecContext(ctx,
			`UPDATE agent_memory_items SET status = ?, updated_at = ?, revision = ? WHERE id = ? AND revision = ? AND status = 'pending'`,
			replacement.Status().String(), replacement.UpdatedAt().UnixNano(), replacement.Revision(), id.String(), expectedRevision)
		if err != nil {
			return fmt.Errorf("sqlite: review agent memory item: %w", err)
		}
		if err := affectedOne(result, "review"); err != nil {
			return err
		}
		if replacement.Status() == agentmemory.StatusRejected {
			return a.pruneRejectedItems(ctx, replacement)
		}
		return nil
	}), nil
}

func (a *AgentMemoryStore) pruneRejectedItems(ctx context.Context, preserved agentmemory.Item) error {
	if _, err := a.db.Executor(ctx).ExecContext(ctx, `
		DELETE FROM agent_memory_items
		WHERE id IN (
			SELECT id FROM agent_memory_items
			WHERE scope = ? AND project = ? AND status = 'rejected' AND id != ?
			ORDER BY updated_at DESC, id DESC
			LIMIT -1 OFFSET ?
		)`, preserved.Scope().String(), preserved.Project(), preserved.ID().String(), agentmemory.MaxRejectedPerTarget-1); err != nil {
		return fmt.Errorf("sqlite: prune rejected agent memory items: %w", err)
	}
	return nil
}

func (a *AgentMemoryStore) PrepareDelete(ctx context.Context, id agentmemory.ItemID, expectedRevision uint64) (management.Mutation, error) {
	item, found, err := a.Get(ctx, id)
	if err != nil {
		return management.Mutation{}, err
	}
	if !found {
		return management.Mutation{}, agentmemory.ErrNotFound
	}
	if err := item.ValidateRemoval(expectedRevision); err != nil {
		return management.Mutation{}, err
	}
	return management.New(item, func(ctx context.Context) error {
		result, err := a.db.Executor(ctx).ExecContext(ctx,
			`DELETE FROM agent_memory_items WHERE id = ? AND revision = ?`, id.String(), expectedRevision)
		if err != nil {
			return fmt.Errorf("sqlite: delete agent memory: %w", err)
		}
		return affectedOne(result, "delete")
	}), nil
}

func (a *AgentMemoryStore) PrepareAdd(ctx context.Context, scope agentmemory.Scope, project string, content agentmemory.Content, now time.Time) (management.Mutation, error) {
	existing, found, err := a.itemByDigest(ctx, scope, project, content.Digest())
	if err != nil {
		return management.Mutation{}, err
	}
	if found && existing.Status() == agentmemory.StatusActive {
		return management.New(existing, nil), nil
	}
	if !found || existing.Status() == agentmemory.StatusRejected {
		visible, err := a.countVisibleItems(ctx, scope, project)
		if err != nil {
			return management.Mutation{}, err
		}
		if visible >= agentmemory.MaxVisiblePerTarget {
			return management.Mutation{}, agentmemory.ErrTargetFull
		}
	}
	if found {
		change, err := existing.ActivateFromUser(content, now)
		if err != nil {
			return management.Mutation{}, err
		}
		return management.New(change.Item(), func(ctx context.Context) error {
			return a.replaceItem(ctx, change)
		}), nil
	}
	item, err := agentmemory.NewUserItem(newMemoryItemID(), scope, project, content, now)
	if err != nil {
		return management.Mutation{}, err
	}
	return management.New(item, func(ctx context.Context) error {
		inserted, err := a.insertItem(ctx, item)
		if err != nil {
			return err
		}
		if !inserted {
			return errors.New("sqlite: agent memory insert lost digest identity inside transaction")
		}
		return nil
	}), nil
}

func (a *AgentMemoryStore) replaceItem(
	ctx context.Context,
	change agentmemory.ItemChange,
) error {
	item := change.Item()
	discardEmbedding := boolToInt(change.DiscardsEmbedding())
	result, err := a.db.Executor(ctx).ExecContext(ctx, `
		UPDATE agent_memory_items
		SET content = ?, digest = ?, origin = ?, status = ?, pinned = ?,
			session_id = ?, day = ?,
			embedding_space = CASE WHEN ? THEN '' ELSE embedding_space END,
			embedding = CASE WHEN ? THEN x'' ELSE embedding END, updated_at = ?, revision = ?
		WHERE id = ?`,
		item.Content(),
		agentmemory.Digest(item.Content()),
		item.Origin().String(),
		item.Status().String(),
		boolToInt(item.Pinned()),
		item.SessionID(),
		item.Day(),
		discardEmbedding,
		discardEmbedding,
		item.UpdatedAt().UTC().UnixNano(),
		item.Revision(),
		item.ID().String(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: replace agent memory item: %w", err)
	}
	return affectedOne(result, "replace")
}

func (a *AgentMemoryStore) itemByDigest(ctx context.Context, scope agentmemory.Scope, project, digest string) (agentmemory.Item, bool, error) {
	var storedItem agentmemory.Item
	var foundItem bool
	err := a.db.RunReadSnapshot(ctx, func(ctx context.Context) error {
		token, err := memoryPartition(scope, project)
		if err != nil {
			return err
		}
		item, err := scanItem(a.db.Executor(ctx).QueryRowContext(ctx,
			`SELECT `+agentMemoryItemColumns+` FROM agent_memory_items WHERE scope = ? AND project = ? AND digest = ?`,
			token, project, digest))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		storedItem, foundItem = item, true
		return nil
	})
	return storedItem, foundItem, err
}

func affectedOne(result sql.Result, op string) error {
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect agent memory %s: %w", op, err)
	}
	if n == 0 {
		return agentmemory.ErrNotFound
	}
	return nil
}

// A late vector must not attach to edited or inactive content, so cache writes match the exact digest.
func (a *AgentMemoryStore) SetEmbeddings(ctx context.Context, updates []agentmemory.EmbeddingUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	seen := make(map[agentmemory.ItemID]struct{}, len(updates))
	for _, update := range updates {
		if err := update.Validate(); err != nil {
			return fmt.Errorf("sqlite: invalid agent memory embedding update: %w", err)
		}
		if _, duplicate := seen[update.ItemID]; duplicate {
			return fmt.Errorf("sqlite: duplicate agent memory embedding update %q", update.ItemID)
		}
		seen[update.ItemID] = struct{}{}
	}
	return a.db.RunInTx(ctx, func(ctx context.Context) error {
		for _, update := range updates {
			if _, err := a.db.Executor(ctx).ExecContext(ctx,
				`UPDATE agent_memory_items SET embedding_space = ?, embedding = ?
				 WHERE id = ? AND digest = ? AND status = 'active'`,
				update.Space, encodeVec(update.Vector), update.ItemID.String(), update.ContentDigest); err != nil {
				return fmt.Errorf("sqlite: set agent memory embedding: %w", err)
			}
		}
		return nil
	})
}
