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

	agentmemory "github.com/Tangerg/flame/plugins/memory/internal/domain"
	"github.com/Tangerg/flame/qimu/sqlite"
)

func newMemoryItemID() agentmemory.ItemID {
	var entropy [agentmemory.ItemIDEntropyBytes]byte
	rand.Read(entropy[:])
	return agentmemory.NewItemID(entropy)
}

func encodeVec(vector []float32) []byte {
	encoded := make([]byte, 4*len(vector))
	for index, value := range vector {
		binary.LittleEndian.PutUint32(encoded[index*4:], math.Float32bits(value))
	}
	return encoded
}

func decodeVec(encoded []byte) ([]float32, error) {
	if len(encoded) == 0 || len(encoded)%4 != 0 {
		return nil, errors.New("sqlite: invalid agent memory embedding encoding")
	}
	vector := make([]float32, len(encoded)/4)
	for index := range vector {
		vector[index] = math.Float32frombits(binary.LittleEndian.Uint32(encoded[index*4:]))
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
		if _, execContextErr := sqlite.Connection(ctx, a.db).ExecContext(ctx, `DELETE FROM agent_memory_items WHERE id = ?`, id.String()); execContextErr != nil {
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
	if err := agentmemory.ValidateTarget(agentmemory.ScopeProject, project); err != nil {
		return nil, err
	}
	rows, err := sqlite.Connection(ctx, a.db).QueryContext(ctx,
		`SELECT `+agentMemoryItemColumns+` FROM agent_memory_items
		 WHERE scope = 'project' AND project = ?
		 LIMIT ?`, project, agentmemory.MaxVisiblePerTarget+agentmemory.MaxRejectedPerTarget+1)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list agent memory items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var items []agentmemory.Item
	for rows.Next() {
		item, scanErr := scanItem(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate agent memory items: %w", err)
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
		return nil, errors.New("sqlite: agent memory target exceeds its lifecycle bounds")
	}
	return items, nil
}

// Pinned and user-authored facts may already occupy the content identity; insertion must preserve them.
func (a *AgentMemoryStore) insertItem(ctx context.Context, item agentmemory.Item) (bool, error) {
	result, err := sqlite.Connection(ctx, a.db).ExecContext(ctx,
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

func scanItem(row scanRow) (agentmemory.Item, error) {
	var (
		snapshot                          agentmemory.ItemSnapshot
		rawID                             string
		scopeText, originText, statusText string
		pinned                            int
		createdAt, updatedAt              int64
	)
	if err := row.Scan(&rawID, &scopeText, &snapshot.Project, &snapshot.Content, &originText, &statusText,
		&pinned, &snapshot.SessionID, &snapshot.Day, &createdAt, &updatedAt, &snapshot.Revision); err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: scan agent memory item: %w", err)
	}
	if err := decodeItemSnapshot(&snapshot, rawID, scopeText, originText, statusText, pinned, createdAt, updatedAt); err != nil {
		return agentmemory.Item{}, err
	}
	return restoreItem(snapshot)
}

func decodeItemSnapshot(
	snapshot *agentmemory.ItemSnapshot,
	rawID, scopeText, originText, statusText string,
	pinned int,
	createdAt, updatedAt int64,
) error {
	var err error
	snapshot.ID, err = agentmemory.ParseItemID(rawID)
	if err != nil {
		return fmt.Errorf("sqlite: decode agent memory item identity %q: %w", rawID, err)
	}
	snapshot.Scope, err = agentmemory.ParseScope(scopeText)
	if err != nil {
		return fmt.Errorf("sqlite: decode agent memory item %q scope: %w", snapshot.ID, err)
	}
	snapshot.Origin, err = agentmemory.ParseOrigin(originText)
	if err != nil {
		return fmt.Errorf("sqlite: decode agent memory item %q origin: %w", snapshot.ID, err)
	}
	snapshot.Status, err = agentmemory.ParseStatus(statusText)
	if err != nil {
		return fmt.Errorf("sqlite: decode agent memory item %q status: %w", snapshot.ID, err)
	}
	snapshot.Pinned = pinned != 0
	snapshot.CreatedAt = time.Unix(0, createdAt).UTC()
	snapshot.UpdatedAt = time.Unix(0, updatedAt).UTC()
	return nil
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
	if _, err := memoryPartition(agentmemory.ScopeProject, project); err != nil {
		return nil, err
	}
	rows, err := sqlite.Connection(ctx, a.db).QueryContext(ctx,
		`SELECT `+agentMemoryItemColumns+`, embedding_space, embedding
		 FROM agent_memory_items
		 WHERE status = 'active' AND (
		       (scope = 'project' AND project = ?) OR
		       (scope = 'user' AND project = '')
		 )
		 LIMIT ?`, project, 2*agentmemory.MaxVisiblePerTarget+1)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list agent memory items for search: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var items []agentmemory.Item
	visibleByScope := make(map[agentmemory.Scope]int, 2)
	for rows.Next() {
		var (
			snapshot                          agentmemory.ItemSnapshot
			rawID                             string
			scopeText, originText, statusText string
			pinned                            int
			createdAt, updatedAt              int64
			blob                              []byte
		)
		if scanErr := rows.Scan(&rawID, &scopeText, &snapshot.Project, &snapshot.Content, &originText, &statusText,
			&pinned, &snapshot.SessionID, &snapshot.Day, &createdAt, &updatedAt, &snapshot.Revision, &snapshot.EmbeddingSpace, &blob); scanErr != nil {
			return nil, fmt.Errorf("sqlite: scan agent memory item: %w", scanErr)
		}
		if err := decodeItemSnapshot(&snapshot, rawID, scopeText, originText, statusText, pinned, createdAt, updatedAt); err != nil {
			return nil, err
		}
		if len(blob) != 0 {
			snapshot.Embedding, err = decodeVec(blob)
			if err != nil {
				return nil, err
			}
		}
		item, err := restoreItem(snapshot)
		if err != nil {
			return nil, err
		}
		visibleByScope[item.Scope()]++
		if visibleByScope[item.Scope()] > agentmemory.MaxVisiblePerTarget {
			return nil, fmt.Errorf("sqlite: agent memory %s search target exceeds its complete-list bound", item.Scope())
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate agent memory items: %w", err)
	}
	return items, nil
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
	args = append(args, maximum+1)
	rows, err := sqlite.Connection(ctx, a.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list %s: %w", operation, err)
	}
	defer func() { _ = rows.Close() }()
	var items []agentmemory.Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate %s: %w", operation, err)
	}
	if len(items) > maximum {
		return nil, fmt.Errorf("sqlite: %s target exceeds its complete-list bound of %d", operation, maximum)
	}
	return items, nil
}

func (a *AgentMemoryStore) Get(ctx context.Context, id agentmemory.ItemID) (agentmemory.Item, bool, error) {
	item, err := scanItem(sqlite.Connection(ctx, a.db).QueryRowContext(ctx,
		`SELECT `+agentMemoryItemColumns+` FROM agent_memory_items WHERE id = ?`, id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return agentmemory.Item{}, false, nil
	}
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	return item, true, nil
}

func (a *AgentMemoryStore) Update(ctx context.Context, id agentmemory.ItemID, expectedRevision uint64, content *string, pinned *bool, now time.Time) (agentmemory.Item, error) {
	var updated agentmemory.Item
	err := sqlite.RunInTx(ctx, a.db, func(ctx context.Context) error {
		current, found, err := a.Get(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return agentmemory.ErrNotFound
		}
		change, err := current.Edit(expectedRevision, content, pinned, now)
		if err != nil {
			return err
		}
		if !change.Changed() {
			updated = current
			return nil
		}
		item := change.Item()
		if item.Content() != current.Content() {
			existing, found, err := a.itemByDigest(ctx, item.Scope(), item.Project(), agentmemory.Digest(item.Content()))
			if err != nil {
				return err
			}
			if found && existing.ID() != item.ID() {
				return agentmemory.ErrDuplicateContent
			}
		}
		if err := a.replaceItem(ctx, change); err != nil {
			return err
		}
		updated = change.Item()
		return nil
	})
	if err != nil {
		return agentmemory.Item{}, err
	}
	return updated, nil
}

func (a *AgentMemoryStore) Review(ctx context.Context, id agentmemory.ItemID, expectedRevision uint64, decision agentmemory.ReviewDecision, now time.Time) error {
	return sqlite.RunInTx(ctx, a.db, func(ctx context.Context) error {
		current, found, err := a.Get(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return agentmemory.ErrNotFound
		}
		replacement, err := current.Review(expectedRevision, decision, now)
		if err != nil {
			return err
		}
		result, err := sqlite.Connection(ctx, a.db).ExecContext(ctx,
			`UPDATE agent_memory_items SET status = ?, updated_at = ?, revision = ? WHERE id = ? AND status = 'pending'`,
			replacement.Status().String(), replacement.UpdatedAt().UnixNano(), replacement.Revision(), id.String())
		if err != nil {
			return fmt.Errorf("sqlite: review agent memory item: %w", err)
		}
		matched, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect agent memory review: %w", err)
		}
		if matched != 1 {
			return agentmemory.ErrNotPending
		}
		if replacement.Status() == agentmemory.StatusRejected {
			return a.pruneRejectedItems(ctx, replacement)
		}
		return nil
	})
}

func (a *AgentMemoryStore) pruneRejectedItems(ctx context.Context, preserved agentmemory.Item) error {
	if _, err := sqlite.Connection(ctx, a.db).ExecContext(ctx, `
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

func (a *AgentMemoryStore) Delete(ctx context.Context, id agentmemory.ItemID, expectedRevision uint64) error {
	return sqlite.RunInTx(ctx, a.db, func(ctx context.Context) error {
		item, found, err := a.Get(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return agentmemory.ErrNotFound
		}
		if err := item.ValidateRemoval(expectedRevision); err != nil {
			return err
		}
		result, err := sqlite.Connection(ctx, a.db).ExecContext(ctx,
			`DELETE FROM agent_memory_items WHERE id = ?`, id.String())
		if err != nil {
			return fmt.Errorf("sqlite: delete agent memory: %w", err)
		}
		return affectedOne(result, "delete")
	})
}

func (a *AgentMemoryStore) Add(ctx context.Context, scope agentmemory.Scope, project, content string, now time.Time) (agentmemory.Item, bool, error) {
	content, err := agentmemory.NormalizeContent(content)
	if err != nil {
		return agentmemory.Item{}, false, fmt.Errorf("sqlite: add agent memory: %w", err)
	}
	addition := userMemoryAddition{store: a, scope: scope, project: project, content: content, now: now.UTC()}
	err = sqlite.RunInTx(ctx, a.db, addition.apply)
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	return addition.stored, addition.changed, nil
}

type userMemoryAddition struct {
	store   *AgentMemoryStore
	scope   agentmemory.Scope
	project string
	content string
	now     time.Time
	stored  agentmemory.Item
	changed bool
}

func (addition *userMemoryAddition) apply(ctx context.Context) error {
	existing, found, err := addition.store.itemByDigest(
		ctx,
		addition.scope,
		addition.project,
		agentmemory.Digest(addition.content),
	)
	if err != nil {
		return err
	}
	if found {
		return addition.reuseOrActivate(ctx, existing)
	}
	return addition.insert(ctx)
}

func (addition *userMemoryAddition) reuseOrActivate(
	ctx context.Context,
	existing agentmemory.Item,
) error {
	if existing.Status() == agentmemory.StatusActive {
		addition.stored = existing
		return nil
	}
	if existing.Status() == agentmemory.StatusRejected {
		if err := addition.ensureCapacity(ctx); err != nil {
			return err
		}
	}
	change, err := existing.ActivateFromUser(addition.content, addition.now)
	if err != nil {
		return err
	}
	if err := addition.store.replaceItem(ctx, change); err != nil {
		return err
	}
	addition.stored = change.Item()
	addition.changed = true
	return nil
}

func (addition *userMemoryAddition) insert(ctx context.Context) error {
	if err := addition.ensureCapacity(ctx); err != nil {
		return err
	}
	id := newMemoryItemID()
	item, err := agentmemory.NewUserItem(id, addition.scope, addition.project, addition.content, addition.now)
	if err != nil {
		return err
	}
	inserted, err := addition.store.insertItem(ctx, item)
	if err != nil {
		return err
	}
	if !inserted {
		return errors.New("sqlite: agent memory insert lost digest identity inside transaction")
	}
	addition.stored = item
	addition.changed = true
	return nil
}

func (addition *userMemoryAddition) ensureCapacity(ctx context.Context) error {
	visible, err := addition.store.countVisibleItems(
		ctx,
		addition.scope,
		addition.project,
	)
	if err != nil {
		return err
	}
	if visible >= agentmemory.MaxVisiblePerTarget {
		return agentmemory.ErrTargetFull
	}
	return nil
}

func (a *AgentMemoryStore) replaceItem(
	ctx context.Context,
	change agentmemory.ItemChange,
) error {
	item := change.Item()
	discardEmbedding := boolToInt(change.DiscardsEmbedding())
	result, err := sqlite.Connection(ctx, a.db).ExecContext(ctx, `
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

func (a *AgentMemoryStore) countVisibleItems(
	ctx context.Context,
	scope agentmemory.Scope,
	project string,
) (int, error) {
	token, err := memoryPartition(scope, project)
	if err != nil {
		return 0, err
	}
	var count int
	if err := sqlite.Connection(ctx, a.db).QueryRowContext(ctx, `
		SELECT count(*) FROM agent_memory_items
		WHERE scope = ? AND project = ? AND status IN ('active','pending')`,
		token, project).Scan(&count); err != nil {
		return 0, fmt.Errorf("sqlite: count visible agent memory items: %w", err)
	}
	if count > agentmemory.MaxVisiblePerTarget {
		return 0, fmt.Errorf(
			"sqlite: agent memory target exceeds its complete-list bound of %d",
			agentmemory.MaxVisiblePerTarget,
		)
	}
	return count, nil
}

func (a *AgentMemoryStore) itemByDigest(ctx context.Context, scope agentmemory.Scope, project, digest string) (agentmemory.Item, bool, error) {
	token, err := memoryPartition(scope, project)
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	item, err := scanItem(sqlite.Connection(ctx, a.db).QueryRowContext(ctx,
		`SELECT `+agentMemoryItemColumns+` FROM agent_memory_items WHERE scope = ? AND project = ? AND digest = ?`,
		token, project, digest))
	if errors.Is(err, sql.ErrNoRows) {
		return agentmemory.Item{}, false, nil
	}
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	return item, true, nil
}

func memoryPartition(scope agentmemory.Scope, project string) (string, error) {
	if err := agentmemory.ValidateTarget(scope, project); err != nil {
		return "", err
	}
	return scope.String(), nil
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
	return sqlite.RunInTx(ctx, a.db, func(ctx context.Context) error {
		for _, update := range updates {
			if _, err := sqlite.Connection(ctx, a.db).ExecContext(ctx,
				`UPDATE agent_memory_items SET embedding_space = ?, embedding = ?
				 WHERE id = ? AND digest = ? AND status = 'active'`,
				update.Space, encodeVec(update.Vector), update.ItemID.String(), update.ContentDigest); err != nil {
				return fmt.Errorf("sqlite: set agent memory embedding: %w", err)
			}
		}
		return nil
	})
}
