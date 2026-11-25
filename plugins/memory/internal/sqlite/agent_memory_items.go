package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/flame/plugins/memory/internal/agentmemory"
)

func newMemoryItemID() agentmemory.ItemID {
	var entropy [agentmemory.ItemIDEntropyBytes]byte
	rand.Read(entropy[:])
	return agentmemory.NewItemID(entropy)
}

const float32Bytes = 4

func encodeVec(vector []float32) []byte {
	encoded := make([]byte, 0, float32Bytes*len(vector))
	for _, value := range vector {
		encoded = binary.LittleEndian.AppendUint32(encoded, math.Float32bits(value))
	}
	return encoded
}

func decodeVec(encoded []byte) ([]float32, error) {
	if len(encoded) == 0 || len(encoded)%float32Bytes != 0 {
		return nil, errors.New("sqlite: invalid agent memory embedding encoding")
	}
	vector := make([]float32, len(encoded)/float32Bytes)
	for index := range vector {
		vector[index] = math.Float32frombits(binary.LittleEndian.Uint32(encoded[index*float32Bytes:]))
	}
	if err := agentmemory.ValidateEmbeddingVector(vector); err != nil {
		return nil, fmt.Errorf("sqlite: decode agent memory embedding: %w", err)
	}
	return vector, nil
}

func (a *AgentMemoryStore) reconcileItems(ctx context.Context, publication agentmemory.Publication) error {
	project, now := publication.Project(), publication.State().UpdatedAt
	existing, err := a.GenerationItems(ctx, project)
	if err != nil {
		return err
	}
	plan, err := publication.Fold(existing)
	if err != nil {
		return fmt.Errorf("sqlite: plan agent memory fold: %w", err)
	}
	for _, id := range plan.PruneIDs {
		if _, execContextErr := a.db.Executor(ctx).ExecContext(ctx, `DELETE FROM agent_memory_items WHERE id = ?`, id.String()); execContextErr != nil {
			return fmt.Errorf("sqlite: prune agent memory item: %w", execContextErr)
		}
	}
	visible, err := a.countVisibleItems(ctx, agentmemory.ScopeProject, project)
	if err != nil {
		return err
	}
	for _, content := range plan.InsertContents {
		if visible >= agentmemory.MaxVisiblePerTarget {
			break
		}
		id := newMemoryItemID()
		item, err := agentmemory.NewProposal(id, project, content, now)
		if err != nil {
			return err
		}
		inserted, err := a.insertItem(ctx, item)
		if err != nil {
			return err
		}
		if inserted {
			visible++
		}
	}
	return nil
}

func (a *AgentMemoryStore) GenerationItems(ctx context.Context, project string) ([]agentmemory.Item, error) {
	var result []agentmemory.Item
	err := a.db.RunReadSnapshot(ctx, func(ctx context.Context) error {
		if err := agentmemory.ValidateTarget(agentmemory.ScopeProject, project); err != nil {
			return err
		}
		rows, err := a.db.Executor(ctx).QueryContext(ctx,
			`SELECT `+agentMemoryItemColumns+` FROM agent_memory_items
		 WHERE scope = 'project' AND project = ?
		 LIMIT ?`, project, agentmemory.MaxVisiblePerTarget+agentmemory.MaxRejectedPerTarget+1)
		if err != nil {
			return fmt.Errorf("sqlite: list agent memory items: %w", err)
		}
		defer func() { _ = rows.Close() }()
		var items []agentmemory.Item
		for rows.Next() {
			item, scanErr := scanItem(rows)
			if scanErr != nil {
				return scanErr
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("sqlite: iterate agent memory items: %w", err)
		}
		visible, rejected := 0, 0
		for _, item := range items {
			if item.Status() == agentmemory.StatusRejected {
				rejected++
			} else {
				visible++
			}
		}
		if visible > agentmemory.MaxVisiblePerTarget || rejected > agentmemory.MaxRejectedPerTarget {
			return errors.New("sqlite: agent memory target exceeds its lifecycle bounds")
		}
		result = items
		return nil
	})
	return result, err
}

func (a *AgentMemoryStore) insertItem(ctx context.Context, item agentmemory.Item) (bool, error) {
	result, err := a.db.Executor(ctx).ExecContext(ctx,
		`INSERT OR IGNORE INTO agent_memory_items(
			id, scope, project, content, digest, origin, status, pinned, session_id, day, created_at, updated_at, revision
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID().String(), item.Scope().String(), item.Project(), item.Content(), agentmemory.Digest(item.Content()),
		item.Origin().String(), item.Status().String(), boolToInt(item.Pinned()), item.SessionID(), item.Day(),
		item.CreatedAt().UnixNano(), item.UpdatedAt().UnixNano(), item.Revision())
	if err != nil {
		return false, fmt.Errorf("sqlite: insert agent memory item: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect agent memory insert: %w", err)
	}
	return inserted == 1, nil
}

const agentMemoryItemColumns = `id, scope, project, content, origin, status, pinned, session_id, day, created_at, updated_at, revision`

type itemColumns struct {
	snapshot                 agentmemory.ItemSnapshot
	id, scope, origin, state string
	pinned                   int
	createdAt, updatedAt     int64
}

// The destinations follow agentMemoryItemColumns; a query may append its own columns after them.
func (c *itemColumns) targets() []any {
	return []any{&c.id, &c.scope, &c.snapshot.Project, &c.snapshot.Content, &c.origin, &c.state,
		&c.pinned, &c.snapshot.SessionID, &c.snapshot.Day, &c.createdAt, &c.updatedAt, &c.snapshot.Revision}
}

func (c *itemColumns) decode() (agentmemory.ItemSnapshot, error) {
	snapshot := c.snapshot
	var err error
	snapshot.ID, err = agentmemory.ParseItemID(c.id)
	if err != nil {
		return agentmemory.ItemSnapshot{}, fmt.Errorf("sqlite: decode agent memory item identity %q: %w", c.id, err)
	}
	snapshot.Scope, err = agentmemory.ParseScope(c.scope)
	if err != nil {
		return agentmemory.ItemSnapshot{}, fmt.Errorf("sqlite: decode agent memory item %q scope: %w", snapshot.ID, err)
	}
	snapshot.Origin, err = agentmemory.ParseOrigin(c.origin)
	if err != nil {
		return agentmemory.ItemSnapshot{}, fmt.Errorf("sqlite: decode agent memory item %q origin: %w", snapshot.ID, err)
	}
	snapshot.Status, err = agentmemory.ParseStatus(c.state)
	if err != nil {
		return agentmemory.ItemSnapshot{}, fmt.Errorf("sqlite: decode agent memory item %q status: %w", snapshot.ID, err)
	}
	snapshot.Pinned = c.pinned != 0
	snapshot.CreatedAt = time.Unix(0, c.createdAt).UTC()
	snapshot.UpdatedAt = time.Unix(0, c.updatedAt).UTC()
	return snapshot, nil
}

func scanItem(row scanRow) (agentmemory.Item, error) {
	var columns itemColumns
	if err := row.Scan(columns.targets()...); err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: scan agent memory item: %w", err)
	}
	snapshot, err := columns.decode()
	if err != nil {
		return agentmemory.Item{}, err
	}
	return restoreItem(snapshot)
}

func restoreItem(snapshot agentmemory.ItemSnapshot) (agentmemory.Item, error) {
	item, err := agentmemory.RestoreItem(snapshot)
	if err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: decode invalid agent memory item %q: %w", snapshot.ID, err)
	}
	return item, nil
}

func (a *AgentMemoryStore) Items(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	token, err := memoryPartition(scope, project)
	if err != nil {
		return nil, err
	}
	return a.listItems(ctx,
		`SELECT `+agentMemoryItemColumns+`
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ? AND status = 'active'
		 LIMIT ?`, "agent memory items", agentmemory.MaxVisiblePerTarget, token, project)
}

func (a *AgentMemoryStore) SearchCorpus(ctx context.Context, project string) ([]agentmemory.Item, error) {
	var result []agentmemory.Item
	err := a.db.RunReadSnapshot(ctx, func(ctx context.Context) error {
		if _, err := memoryPartition(agentmemory.ScopeProject, project); err != nil {
			return err
		}
		rows, err := a.db.Executor(ctx).QueryContext(ctx,
			`SELECT `+agentMemoryItemColumns+`, embedding_space, embedding
		 FROM agent_memory_items
		 WHERE status = 'active' AND (
		       (scope = 'project' AND project = ?) OR
		       (scope = 'user' AND project = '')
		 )
		 LIMIT ?`, project, 2*agentmemory.MaxVisiblePerTarget+1)
		if err != nil {
			return fmt.Errorf("sqlite: list agent memory items for search: %w", err)
		}
		defer func() { _ = rows.Close() }()
		var items []agentmemory.Item
		visibleByScope := make(map[agentmemory.Scope]int, 2)
		for rows.Next() {
			var columns itemColumns
			var blob []byte
			if scanErr := rows.Scan(append(columns.targets(), &columns.snapshot.EmbeddingSpace, &blob)...); scanErr != nil {
				return fmt.Errorf("sqlite: scan agent memory item: %w", scanErr)
			}
			snapshot, err := columns.decode()
			if err != nil {
				return err
			}
			if len(blob) != 0 {
				snapshot.Embedding, err = decodeVec(blob)
				if err != nil {
					return err
				}
			}
			item, err := restoreItem(snapshot)
			if err != nil {
				return err
			}
			visibleByScope[item.Scope()]++
			if visibleByScope[item.Scope()] > agentmemory.MaxVisiblePerTarget {
				return fmt.Errorf("sqlite: agent memory %s search target exceeds its complete-list bound", item.Scope())
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("sqlite: iterate agent memory items: %w", err)
		}
		result = items
		return nil
	})
	return result, err
}

func (a *AgentMemoryStore) List(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	token, err := memoryPartition(scope, project)
	if err != nil {
		return nil, err
	}
	return a.listItems(ctx,
		`SELECT `+agentMemoryItemColumns+`
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ? AND status IN ('active','pending')
		 LIMIT ?`,
		"agent memory", agentmemory.MaxVisiblePerTarget, token, project)
}

func (a *AgentMemoryStore) listItems(
	ctx context.Context,
	query string,
	operation string,
	maximum int,
	args ...any,
) ([]agentmemory.Item, error) {
	var result []agentmemory.Item
	err := a.db.RunReadSnapshot(ctx, func(ctx context.Context) error {
		args = append(args, maximum+1)
		rows, err := a.db.Executor(ctx).QueryContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("sqlite: list %s: %w", operation, err)
		}
		defer func() { _ = rows.Close() }()
		var items []agentmemory.Item
		for rows.Next() {
			item, err := scanItem(rows)
			if err != nil {
				return err
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("sqlite: iterate %s: %w", operation, err)
		}
		if len(items) > maximum {
			return fmt.Errorf("sqlite: %s target exceeds its complete-list bound of %d", operation, maximum)
		}
		result = items
		return nil
	})
	return result, err
}

func (a *AgentMemoryStore) Get(ctx context.Context, id agentmemory.ItemID) (agentmemory.Item, bool, error) {
	var storedItem agentmemory.Item
	var foundItem bool
	err := a.db.RunReadSnapshot(ctx, func(ctx context.Context) error {
		item, err := scanItem(a.db.Executor(ctx).QueryRowContext(ctx,
			`SELECT `+agentMemoryItemColumns+` FROM agent_memory_items WHERE id = ?`, id.String()))
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

func (a *AgentMemoryStore) countVisibleItems(
	ctx context.Context,
	scope agentmemory.Scope,
	project string,
) (int, error) {
	var result int
	err := a.db.RunReadSnapshot(ctx, func(ctx context.Context) error {
		token, err := memoryPartition(scope, project)
		if err != nil {
			return err
		}
		var count int
		if err := a.db.Executor(ctx).QueryRowContext(ctx, `
		SELECT count(*) FROM agent_memory_items
		WHERE scope = ? AND project = ? AND status IN ('active','pending')`,
			token, project).Scan(&count); err != nil {
			return fmt.Errorf("sqlite: count visible agent memory items: %w", err)
		}
		if count > agentmemory.MaxVisiblePerTarget {
			return fmt.Errorf(
				"sqlite: agent memory target exceeds its complete-list bound of %d",
				agentmemory.MaxVisiblePerTarget,
			)
		}
		result = count
		return nil
	})
	return result, err
}

func memoryPartition(scope agentmemory.Scope, project string) (string, error) {
	if err := agentmemory.ValidateTarget(scope, project); err != nil {
		return "", err
	}
	return scope.String(), nil
}
