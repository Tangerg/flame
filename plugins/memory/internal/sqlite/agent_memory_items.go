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

	framesqlite "github.com/Tangerg/frame/sqlite"

	agentmemory "github.com/Tangerg/flame/plugins/memory/internal/domain"
	"github.com/Tangerg/flame/plugins/memory/internal/management"
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
		if _, execContextErr := framesqlite.Connection(ctx, a.db).ExecContext(ctx, `DELETE FROM agent_memory_items WHERE id = ?`, id.String()); execContextErr != nil {
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
	err := framesqlite.RunReadSnapshot(ctx, a.db, func(ctx context.Context) error {
		if err := agentmemory.ValidateTarget(agentmemory.ScopeProject, project); err != nil {

			return err
		}
		rows, err := framesqlite.Connection(ctx, a.db).QueryContext(ctx,
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

// Pinned and user-authored facts may already occupy the content identity; insertion must preserve them.
func (a *AgentMemoryStore) insertItem(ctx context.Context, item agentmemory.Item) (bool, error) {
	result, err := framesqlite.Connection(ctx, a.db).ExecContext(ctx,
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
	var result []agentmemory.Item
	err := framesqlite.RunReadSnapshot(ctx, a.db, func(ctx context.Context) error {
		if _, err := memoryPartition(agentmemory.ScopeProject, project); err != nil {

			return err
		}
		rows, err := framesqlite.Connection(ctx, a.db).QueryContext(ctx,
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

				return fmt.Errorf("sqlite: scan agent memory item: %w", scanErr)
			}
			if err := decodeItemSnapshot(&snapshot, rawID, scopeText, originText, statusText, pinned, createdAt, updatedAt); err != nil {

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
	err := framesqlite.RunReadSnapshot(ctx, a.db, func(ctx context.Context) error {
		args = append(args, maximum+1)
		rows, err := framesqlite.Connection(ctx, a.db).QueryContext(ctx, query, args...)
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
	err := framesqlite.RunReadSnapshot(ctx, a.db, func(ctx context.Context) error {
		item, err := scanItem(framesqlite.Connection(ctx, a.db).QueryRowContext(ctx,
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

func (a *AgentMemoryStore) PrepareUpdate(ctx context.Context, id agentmemory.ItemID, expectedRevision uint64, content *string, pinned *bool, now time.Time) (management.Mutation, error) {
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
		result, err := framesqlite.Connection(ctx, a.db).ExecContext(ctx,
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
	if _, err := framesqlite.Connection(ctx, a.db).ExecContext(ctx, `
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
		result, err := framesqlite.Connection(ctx, a.db).ExecContext(ctx,
			`DELETE FROM agent_memory_items WHERE id = ? AND revision = ?`, id.String(), expectedRevision)
		if err != nil {
			return fmt.Errorf("sqlite: delete agent memory: %w", err)
		}
		return affectedOne(result, "delete")
	}), nil
}

func (a *AgentMemoryStore) PrepareAdd(ctx context.Context, scope agentmemory.Scope, project, content string, now time.Time) (management.Mutation, error) {
	content, err := agentmemory.NormalizeContent(content)
	if err != nil {
		return management.Mutation{}, fmt.Errorf("sqlite: add agent memory: %w", err)
	}
	existing, found, err := a.itemByDigest(ctx, scope, project, agentmemory.Digest(content))
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
	result, err := framesqlite.Connection(ctx, a.db).ExecContext(ctx, `
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
	var result int
	err := framesqlite.RunReadSnapshot(ctx, a.db, func(ctx context.Context) error {
		token, err := memoryPartition(scope, project)
		if err != nil {

			return err
		}
		var count int
		if err := framesqlite.Connection(ctx, a.db).QueryRowContext(ctx, `
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

func (a *AgentMemoryStore) itemByDigest(ctx context.Context, scope agentmemory.Scope, project, digest string) (agentmemory.Item, bool, error) {
	var storedItem agentmemory.Item
	var foundItem bool
	err := framesqlite.RunReadSnapshot(ctx, a.db, func(ctx context.Context) error {
		token, err := memoryPartition(scope, project)
		if err != nil {

			return err
		}
		item, err := scanItem(framesqlite.Connection(ctx, a.db).QueryRowContext(ctx,
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
	return framesqlite.RunInTx(ctx, a.db, func(ctx context.Context) error {
		for _, update := range updates {
			if _, err := framesqlite.Connection(ctx, a.db).ExecContext(ctx,
				`UPDATE agent_memory_items SET embedding_space = ?, embedding = ?
				 WHERE id = ? AND digest = ? AND status = 'active'`,
				update.Space, encodeVec(update.Vector), update.ItemID.String(), update.ContentDigest); err != nil {
				return fmt.Errorf("sqlite: set agent memory embedding: %w", err)
			}
		}
		return nil
	})
}
