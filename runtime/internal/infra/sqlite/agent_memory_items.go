package sqlite

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

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

// reconcileItems applies the domain fold ([agentmemory.Fold]) to the project's
// auto-origin items: prune the stale pending proposals it flags, insert the new
// curated facts as pending proposals. The review invariants (tombstone,
// active-sticky, pending-default, digest identity) live in the domain; this is
// the persistence that carries the plan out.
func (a *AgentMemoryStore) reconcileItems(ctx context.Context, project string, contents []string, now time.Time) error {
	existing, err := a.autoItems(ctx, project)
	if err != nil {
		return err
	}
	plan, err := agentmemory.Fold(existing, contents)
	if err != nil {
		return fmt.Errorf("sqlite: plan agent memory fold: %w", err)
	}
	for _, id := range plan.PruneIDs {
		if _, execContextErr := conn(ctx, a.db).ExecContext(ctx, `DELETE FROM agent_memory_items WHERE id = ?`, id.String()); execContextErr != nil {
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
		id, err := agentmemory.NewItemID()
		if err != nil {
			return err
		}
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

// autoItems fetches the project's auto-origin fold set: unpinned visible items
// plus every retained rejected tombstone (id + content + status suffice).
func (a *AgentMemoryStore) autoItems(ctx context.Context, project string) ([]agentmemory.Item, error) {
	rows, err := conn(ctx, a.db).QueryContext(ctx,
		`SELECT id, content, status FROM agent_memory_items
		 WHERE scope = 'project' AND project = ? AND origin = 'auto'
		   AND (pinned = 0 OR status = 'rejected')
		 LIMIT ?`, project, agentmemory.MaxVisiblePerTarget+agentmemory.MaxRejectedPerTarget+1)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list agent memory items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var items []agentmemory.Item
	for rows.Next() {
		var (
			item       agentmemory.Item
			rawID      string
			statusText string
		)
		if scanErr := rows.Scan(&rawID, &item.Content, &statusText); scanErr != nil {
			return nil, fmt.Errorf("sqlite: scan agent memory item: %w", scanErr)
		}
		item.ID, err = agentmemory.ParseItemID(rawID)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode agent memory item identity %q: %w", rawID, err)
		}
		item.Status, err = agentmemory.ParseStatus(statusText)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode agent memory item %q status: %w", item.ID, err)
		}
		content, err := agentmemory.NormalizeContent(item.Content)
		if err != nil || content != item.Content {
			if err == nil {
				err = errors.New("content is not canonical")
			}
			return nil, fmt.Errorf("sqlite: decode invalid agent memory item %q: %w", item.ID, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate agent memory items: %w", err)
	}
	visible, rejected := 0, 0
	for _, item := range items {
		if item.Status == agentmemory.StatusRejected {
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

// insertItem writes a constructed item. OR IGNORE: a pinned or user item may
// already hold this content under the unique (scope, project, digest) index —
// keep it, don't duplicate.
func (a *AgentMemoryStore) insertItem(ctx context.Context, item agentmemory.Item) (bool, error) {
	if err := item.Validate(); err != nil {
		return false, fmt.Errorf("sqlite: insert invalid agent memory item: %w", err)
	}
	result, err := conn(ctx, a.db).ExecContext(ctx,
		`INSERT OR IGNORE INTO agent_memory_items(
			id, scope, project, content, digest, origin, status, pinned, session_id, day, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID.String(), item.Scope.String(), item.Project, item.Content, agentmemory.Digest(item.Content),
		item.Origin.String(), item.Status.String(), boolToInt(item.Pinned), item.SessionID, item.Day,
		item.CreatedAt.UTC().UnixNano(), item.UpdatedAt.UTC().UnixNano())
	if err != nil {
		return false, fmt.Errorf("sqlite: insert agent memory item: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect agent memory insert: %w", err)
	}
	return inserted == 1, nil
}

const agentMemoryItemColumns = `id, scope, project, content, origin, status, pinned, session_id, day, created_at, updated_at`

// scanItem decodes one item's base columns (embedding excluded — see
// [AgentMemoryStore.SearchCorpus] for the search path that reads it).
func scanItem(row scanRow) (agentmemory.Item, error) {
	var (
		item                              agentmemory.Item
		rawID                             string
		scopeText, originText, statusText string
		pinned                            int
		createdAt, updatedAt              int64
	)
	if err := row.Scan(&rawID, &scopeText, &item.Project, &item.Content, &originText, &statusText,
		&pinned, &item.SessionID, &item.Day, &createdAt, &updatedAt); err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: scan agent memory item: %w", err)
	}
	var err error
	item.ID, err = agentmemory.ParseItemID(rawID)
	if err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: decode agent memory item identity %q: %w", rawID, err)
	}
	return decodeItem(item, scopeText, originText, statusText, pinned, createdAt, updatedAt)
}

// decodeItem is the single persistence boundary for a fully loaded memory
// item. Both ordinary reads and search reads pass through the same closed
// vocabulary and domain-invariant checks.
func decodeItem(
	item agentmemory.Item,
	scopeText, originText, statusText string,
	pinned int,
	createdAt, updatedAt int64,
) (agentmemory.Item, error) {
	var err error
	item.Scope, err = agentmemory.ParseScope(scopeText)
	if err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: decode agent memory item %q scope: %w", item.ID, err)
	}
	item.Origin, err = agentmemory.ParseOrigin(originText)
	if err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: decode agent memory item %q origin: %w", item.ID, err)
	}
	item.Status, err = agentmemory.ParseStatus(statusText)
	if err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: decode agent memory item %q status: %w", item.ID, err)
	}
	item.Pinned = pinned != 0
	item.CreatedAt = time.Unix(0, createdAt).UTC()
	item.UpdatedAt = time.Unix(0, updatedAt).UTC()
	if err := item.Validate(); err != nil {
		return agentmemory.Item{}, fmt.Errorf("sqlite: decode invalid agent memory item %q: %w", item.ID, err)
	}
	return item, nil
}

// Items lists the active items for (scope, project): pinned first, then most
// recently updated. Pending and rejected items are excluded — only approved
// memory is injected into the prompt.
func (a *AgentMemoryStore) Items(ctx context.Context, scope agentmemory.Scope, project string) ([]agentmemory.Item, error) {
	token, err := memoryPartition(scope, project)
	if err != nil {
		return nil, err
	}
	return a.listItems(ctx,
		`SELECT `+agentMemoryItemColumns+`
		 FROM agent_memory_items
		 WHERE scope = ? AND project = ? AND status = 'active'
		 ORDER BY pinned DESC, updated_at DESC
		 LIMIT ?`, "agent memory items", agentmemory.MaxVisiblePerTarget, token, project)
}

// SearchCorpus lists the active exact-project and user-scoped items visible
// from one project context, with their embedding cache decoded. Fetching both
// partitions in one snapshot lets the application rank one combined corpus.
func (a *AgentMemoryStore) SearchCorpus(ctx context.Context, project string) ([]agentmemory.Item, error) {
	if _, err := memoryPartition(agentmemory.ScopeProject, project); err != nil {
		return nil, err
	}
	rows, err := conn(ctx, a.db).QueryContext(ctx,
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
			item                              agentmemory.Item
			rawID                             string
			scopeText, originText, statusText string
			pinned                            int
			createdAt, updatedAt              int64
			space                             string
			blob                              []byte
		)
		if scanErr := rows.Scan(&rawID, &scopeText, &item.Project, &item.Content, &originText, &statusText,
			&pinned, &item.SessionID, &item.Day, &createdAt, &updatedAt, &space, &blob); scanErr != nil {
			return nil, fmt.Errorf("sqlite: scan agent memory item: %w", scanErr)
		}
		item.ID, err = agentmemory.ParseItemID(rawID)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode agent memory search item identity %q: %w", rawID, err)
		}
		item, err = decodeItem(item, scopeText, originText, statusText, pinned, createdAt, updatedAt)
		if err != nil {
			return nil, err
		}
		if space != "" || len(blob) != 0 {
			vector, decodeErr := decodeVec(blob)
			if decodeErr != nil {
				return nil, decodeErr
			}
			item.EmbeddingSpace = space
			item.Embedding = vector
			if validateErr := item.Validate(); validateErr != nil {
				return nil, fmt.Errorf("sqlite: decode invalid agent memory search item %q: %w", item.ID, validateErr)
			}
		}
		visibleByScope[item.Scope]++
		if visibleByScope[item.Scope] > agentmemory.MaxVisiblePerTarget {
			return nil, fmt.Errorf("sqlite: agent memory %s search target exceeds its complete-list bound", item.Scope)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate agent memory items: %w", err)
	}
	return items, nil
}

// List returns the complete bounded (scope, project) item set consumed by the
// review use case. Rejected tombstones are hidden; Application owns the
// management order.
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
	rows, err := conn(ctx, a.db).QueryContext(ctx, query, args...)
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

// Get returns one item by id.
func (a *AgentMemoryStore) Get(ctx context.Context, id agentmemory.ItemID) (agentmemory.Item, bool, error) {
	item, err := scanItem(conn(ctx, a.db).QueryRowContext(ctx,
		`SELECT `+agentMemoryItemColumns+` FROM agent_memory_items WHERE id = ?`, id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return agentmemory.Item{}, false, nil
	}
	if err != nil {
		return agentmemory.Item{}, false, err
	}
	return item, true, nil
}

// Update applies the review surface's content/pin patch atomically. Content
// edits clear stale embeddings; either validation or persistence failure rolls
// back every requested field, so callers never observe a half-applied update.
func (a *AgentMemoryStore) Update(ctx context.Context, id agentmemory.ItemID, content *string, pinned *bool, now time.Time) (agentmemory.Item, error) {
	var updated agentmemory.Item
	err := RunInTx(ctx, a.db, func(ctx context.Context) error {
		if content != nil {
			if err := a.UpdateContent(ctx, id, *content, now); err != nil {
				return err
			}
		}
		if pinned != nil {
			if err := a.SetPinned(ctx, id, *pinned, now); err != nil {
				return err
			}
		}
		item, found, err := a.Get(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return agentmemory.ErrNotFound
		}
		updated = item
		return nil
	})
	if err != nil {
		return agentmemory.Item{}, err
	}
	return updated, nil
}

// Review atomically resolves one pending proposal. A user-authored, already
// reviewed, or rejected item cannot be rewritten through the review command.
func (a *AgentMemoryStore) Review(ctx context.Context, id agentmemory.ItemID, decision agentmemory.ReviewDecision, now time.Time) error {
	status, err := decision.Result()
	if err != nil {
		return err
	}
	return RunInTx(ctx, a.db, func(ctx context.Context) error {
		result, err := conn(ctx, a.db).ExecContext(ctx,
			`UPDATE agent_memory_items SET status = ?, updated_at = ? WHERE id = ? AND status = 'pending'`,
			status.String(), now.UTC().UnixNano(), id.String())
		if err != nil {
			return fmt.Errorf("sqlite: review agent memory item: %w", err)
		}
		matched, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("sqlite: inspect agent memory review: %w", err)
		}
		if matched == 1 {
			if status == agentmemory.StatusRejected {
				if pruneRejectedItemsErr := a.pruneRejectedItems(ctx, id); pruneRejectedItemsErr != nil {
					return pruneRejectedItemsErr
				}
			}
			return nil
		}
		var stored string
		if scanErr := conn(ctx, a.db).QueryRowContext(ctx,
			`SELECT status FROM agent_memory_items WHERE id = ?`, id.String()).Scan(&stored); errors.Is(scanErr, sql.ErrNoRows) {
			return agentmemory.ErrNotFound
		} else if scanErr != nil {
			return fmt.Errorf("sqlite: inspect agent memory review target: %w", scanErr)
		}
		current, err := agentmemory.ParseStatus(stored)
		if err != nil {
			return fmt.Errorf("sqlite: decode agent memory review target %q: %w", id, err)
		}
		return fmt.Errorf("%w: item %q is %s", agentmemory.ErrNotPending, id, current)
	})
}

func (a *AgentMemoryStore) pruneRejectedItems(ctx context.Context, preservedID agentmemory.ItemID) error {
	var scope, project string
	if err := conn(ctx, a.db).QueryRowContext(ctx,
		`SELECT scope, project FROM agent_memory_items WHERE id = ?`, preservedID.String(),
	).Scan(&scope, &project); err != nil {
		return fmt.Errorf("sqlite: locate rejected agent memory item: %w", err)
	}
	if _, err := conn(ctx, a.db).ExecContext(ctx, `
		DELETE FROM agent_memory_items
		WHERE id IN (
			SELECT id FROM agent_memory_items
			WHERE scope = ? AND project = ? AND status = 'rejected' AND id != ?
			ORDER BY updated_at DESC, id DESC
			LIMIT -1 OFFSET ?
		)`, scope, project, preservedID.String(), agentmemory.MaxRejectedPerTarget-1); err != nil {
		return fmt.Errorf("sqlite: prune rejected agent memory items: %w", err)
	}
	return nil
}

// SetPinned pins or unpins an item; pinned items are always injected and never
// auto-pruned.
func (a *AgentMemoryStore) SetPinned(ctx context.Context, id agentmemory.ItemID, pinned bool, now time.Time) error {
	result, err := conn(ctx, a.db).ExecContext(ctx,
		`UPDATE agent_memory_items SET pinned = ?, updated_at = ? WHERE id = ?`,
		boolToInt(pinned), now.UTC().UnixNano(), id.String())
	if err != nil {
		return fmt.Errorf("sqlite: pin agent memory: %w", err)
	}
	return affectedOne(result, "pin")
}

// UpdateContent edits an item's content, recomputes its digest, and clears the
// now-stale embedding so a later fold re-embeds it.
func (a *AgentMemoryStore) UpdateContent(ctx context.Context, id agentmemory.ItemID, content string, now time.Time) error {
	content, err := agentmemory.NormalizeContent(content)
	if err != nil {
		return fmt.Errorf("sqlite: edit agent memory: %w", err)
	}
	result, err := conn(ctx, a.db).ExecContext(ctx,
		`UPDATE agent_memory_items SET content = ?, digest = ?, embedding_space = '', embedding = x'', updated_at = ? WHERE id = ?`,
		content, agentmemory.Digest(content), now.UTC().UnixNano(), id.String())
	if err != nil {
		return fmt.Errorf("sqlite: edit agent memory: %w", err)
	}
	return affectedOne(result, "edit")
}

// Delete removes an item outright.
func (a *AgentMemoryStore) Delete(ctx context.Context, id agentmemory.ItemID) error {
	result, err := conn(ctx, a.db).ExecContext(ctx,
		`DELETE FROM agent_memory_items WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("sqlite: delete agent memory: %w", err)
	}
	return affectedOne(result, "delete")
}

// Add stores a user-authored active item. An existing active digest is
// idempotent; an explicit user addition promotes a matching pending or rejected
// proposal while preserving its stable id.
func (a *AgentMemoryStore) Add(ctx context.Context, scope agentmemory.Scope, project, content string, now time.Time) (agentmemory.Item, bool, error) {
	content, err := agentmemory.NormalizeContent(content)
	if err != nil {
		return agentmemory.Item{}, false, fmt.Errorf("sqlite: add agent memory: %w", err)
	}
	addition := userMemoryAddition{store: a, scope: scope, project: project, content: content, now: now.UTC()}
	err = RunInTx(ctx, a.db, addition.apply)
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
	if existing.Status == agentmemory.StatusActive {
		addition.stored = existing
		return nil
	}
	if existing.Status == agentmemory.StatusRejected {
		if err := addition.ensureCapacity(ctx); err != nil {
			return err
		}
	}
	activated, err := existing.ActivateFromUser(addition.content, addition.now)
	if err != nil {
		return err
	}
	if err := addition.persistActivation(ctx, activated); err != nil {
		return err
	}
	addition.stored = activated
	addition.changed = true
	return nil
}

func (addition *userMemoryAddition) insert(ctx context.Context) error {
	if err := addition.ensureCapacity(ctx); err != nil {
		return err
	}
	id, err := agentmemory.NewItemID()
	if err != nil {
		return err
	}
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

func (addition *userMemoryAddition) persistActivation(
	ctx context.Context,
	item agentmemory.Item,
) error {
	result, err := conn(ctx, addition.store.db).ExecContext(ctx, `
		UPDATE agent_memory_items
		SET content = ?, digest = ?, origin = ?, status = ?, pinned = ?,
			session_id = ?, day = ?, embedding_space = ?, embedding = ?, updated_at = ?
		WHERE id = ?`,
		item.Content,
		agentmemory.Digest(item.Content),
		item.Origin.String(),
		item.Status.String(),
		boolToInt(item.Pinned),
		item.SessionID,
		item.Day,
		item.EmbeddingSpace,
		[]byte{},
		item.UpdatedAt.UTC().UnixNano(),
		item.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: activate agent memory item: %w", err)
	}
	return affectedOne(result, "activate")
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
	if err := conn(ctx, a.db).QueryRowContext(ctx, `
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
	item, err := scanItem(conn(ctx, a.db).QueryRowContext(ctx,
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

// affectedOne maps a zero-row update/delete to [agentmemory.ErrNotFound].
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

// SetEmbeddings caches vectors only while the exact item content remains
// active. A concurrent edit, review, or reconcile therefore makes the update a
// no-op instead of attaching a late vector to different content.
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
	return RunInTx(ctx, a.db, func(ctx context.Context) error {
		for _, update := range updates {
			if _, err := conn(ctx, a.db).ExecContext(ctx,
				`UPDATE agent_memory_items SET embedding_space = ?, embedding = ?
				 WHERE id = ? AND digest = ? AND status = 'active'`,
				update.Space, encodeVec(update.Vector), update.ItemID.String(), update.ContentDigest); err != nil {
				return fmt.Errorf("sqlite: set agent memory embedding: %w", err)
			}
		}
		return nil
	})
}
