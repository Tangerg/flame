package agentmemory

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
)

type Item struct {
	id        ItemID
	revision  exactint.Counter
	scope     Scope
	project   string
	content   string
	origin    Origin
	status    Status
	pinned    bool
	sessionID identity.SessionID
	day       string
	createdAt time.Time
	updatedAt time.Time

	// Vectors from different model configurations cannot share a coordinate system.
	embeddingSpace string
	embedding      []float32
}

type ItemSnapshot struct {
	ID             ItemID
	Revision       uint64
	Scope          Scope
	Project        string
	Content        string
	Origin         Origin
	Status         Status
	Pinned         bool
	SessionID      identity.SessionID
	Day            string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	EmbeddingSpace string
	Embedding      []float32
}

type ItemChange struct {
	item Item
	kind itemChangeKind
}

type itemChangeKind uint8

const (
	itemUnchanged itemChangeKind = iota
	itemMetadataChanged
	itemContentAuthored
)

func (c ItemChange) Item() Item              { return c.item }
func (c ItemChange) Changed() bool           { return c.kind != itemUnchanged }
func (c ItemChange) DiscardsEmbedding() bool { return c.kind == itemContentAuthored }

func RestoreItem(snapshot ItemSnapshot) (Item, error) {
	revision, err := exactint.Restore(snapshot.Revision)
	if err != nil || revision.IsZero() {
		return Item{}, ErrInvalidRevision
	}
	item := Item{
		id: snapshot.ID, revision: revision, scope: snapshot.Scope, project: snapshot.Project, content: snapshot.Content,
		origin: snapshot.Origin, status: snapshot.Status, pinned: snapshot.Pinned,
		sessionID: snapshot.SessionID, day: snapshot.Day,
		createdAt: snapshot.CreatedAt.UTC(), updatedAt: snapshot.UpdatedAt.UTC(),
		embeddingSpace: snapshot.EmbeddingSpace, embedding: slices.Clone(snapshot.Embedding),
	}
	if err := item.validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (i Item) ID() ItemID                    { return i.id }
func (i Item) Revision() uint64              { return i.revision.Value() }
func (i Item) Scope() Scope                  { return i.scope }
func (i Item) Project() string               { return i.project }
func (i Item) Content() string               { return i.content }
func (i Item) Origin() Origin                { return i.origin }
func (i Item) Status() Status                { return i.status }
func (i Item) Pinned() bool                  { return i.pinned }
func (i Item) SessionID() identity.SessionID { return i.sessionID }
func (i Item) Day() string                   { return i.day }
func (i Item) CreatedAt() time.Time          { return i.createdAt }
func (i Item) UpdatedAt() time.Time          { return i.updatedAt }

func (i Item) Embedding() (string, []float32, bool) {
	if i.embeddingSpace == "" {
		return "", nil, false
	}
	return i.embeddingSpace, slices.Clone(i.embedding), true
}

type EmbeddingUpdate struct {
	ItemID        ItemID
	ContentDigest string
	Space         string
	Vector        []float32
}

func NewEmbeddingUpdate(item Item, space string, vector []float32) (EmbeddingUpdate, error) {
	if err := validateEmbedding(space, vector); err != nil {
		return EmbeddingUpdate{}, err
	}
	return EmbeddingUpdate{
		ItemID:        item.id,
		ContentDigest: Digest(item.content),
		Space:         space,
		Vector:        slices.Clone(vector),
	}, nil
}

func (i Item) WithEmbedding(update EmbeddingUpdate) (Item, error) {
	if update.ItemID != i.id || update.ContentDigest != Digest(i.content) {
		return Item{}, errors.New("agentmemory: embedding update belongs to other item content")
	}
	if err := validateEmbedding(update.Space, update.Vector); err != nil {
		return Item{}, err
	}
	i.embeddingSpace, i.embedding = update.Space, slices.Clone(update.Vector)
	return i, nil
}

func (i Item) WithoutEmbedding() Item {
	i.embeddingSpace, i.embedding = "", nil
	return i
}

func (e EmbeddingUpdate) Validate() error {
	if err := e.ItemID.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(e.ContentDigest) == "" {
		return errors.New("agentmemory: embedding content digest is required")
	}
	return validateEmbedding(e.Space, e.Vector)
}

func validateEmbedding(space string, vector []float32) error {
	if space == "" || strings.TrimSpace(space) != space {
		return errors.New("agentmemory: embedding space is required without surrounding whitespace")
	}
	return ValidateEmbeddingVector(vector)
}

func ValidateEmbeddingVector(vector []float32) error {
	if len(vector) == 0 {
		return errors.New("agentmemory: embedding vector is required")
	}
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return errors.New("agentmemory: embedding vector must be finite")
		}
	}
	return nil
}

func NewProposal(id ItemID, project, content string, now time.Time) (Item, error) {
	return newItem(id, ScopeProject, project, content, OriginAuto, StatusPending, now)
}

func NewUserItem(id ItemID, scope Scope, project, content string, now time.Time) (Item, error) {
	return newItem(id, scope, project, content, OriginUser, StatusActive, now)
}

func (i Item) ActivateFromUser(content string, now time.Time) (ItemChange, error) {
	if i.status == StatusActive {
		return ItemChange{}, errors.New("agentmemory: active item cannot be activated again")
	}
	content, err := NormalizeContent(content)
	if err != nil {
		return ItemChange{}, err
	}
	now = now.UTC()
	if now.IsZero() || now.Before(i.updatedAt) {
		return ItemChange{}, errors.New("agentmemory: activation time precedes current item")
	}
	if err := instant.Validate(now); err != nil {
		return ItemChange{}, fmt.Errorf("agentmemory: activation time: %w", err)
	}
	i.revision, err = i.revision.Next()
	if err != nil {
		return ItemChange{}, err
	}
	i = i.withUserContent(content, now)
	i.pinned = false
	i.updatedAt = now
	return ItemChange{item: i, kind: itemContentAuthored}, nil
}

func (i Item) Edit(expectedRevision uint64, content *string, pinned *bool, now time.Time) (ItemChange, error) {
	if err := i.CheckRevision(expectedRevision); err != nil {
		return ItemChange{}, err
	}
	if err := i.ValidateVisible(); err != nil {
		return ItemChange{}, fmt.Errorf("agentmemory: edit existing item: %w", err)
	}
	canonicalContent := i.content
	if content != nil {
		canonical, err := NormalizeContent(*content)
		if err != nil {
			return ItemChange{}, err
		}
		canonicalContent = canonical
	}
	contentChanged := canonicalContent != i.content
	pinChanged := pinned != nil && i.pinned != *pinned
	if !contentChanged && !pinChanged {
		return ItemChange{item: i}, nil
	}
	now = now.UTC()
	if now.IsZero() || now.Before(i.updatedAt) {
		return ItemChange{}, errors.New("agentmemory: edit time precedes current item")
	}
	if err := instant.Validate(now); err != nil {
		return ItemChange{}, fmt.Errorf("agentmemory: edit time: %w", err)
	}
	revision, err := i.revision.Next()
	if err != nil {
		return ItemChange{}, err
	}
	i.revision = revision
	kind := itemMetadataChanged
	if contentChanged {
		i = i.withUserContent(canonicalContent, now)
		kind = itemContentAuthored
	}
	if pinChanged {
		i.pinned = *pinned
	}
	i.updatedAt = now
	return ItemChange{item: i, kind: kind}, nil
}

func (i Item) withUserContent(content string, now time.Time) Item {
	i.content = content
	i.origin = OriginUser
	i.status = StatusActive
	i.sessionID = identity.SessionID{}
	i.day = now.Format(time.DateOnly)
	i.embeddingSpace = ""
	i.embedding = nil
	return i
}

func (i Item) Review(expectedRevision uint64, decision ReviewDecision, now time.Time) (Item, error) {
	if err := i.CheckRevision(expectedRevision); err != nil {
		return Item{}, err
	}
	status, err := decision.Result()
	if err != nil {
		return Item{}, err
	}
	if i.status != StatusPending {
		return Item{}, fmt.Errorf("%w: item %q is %s", ErrNotPending, i.id, i.status)
	}
	now = now.UTC()
	if now.IsZero() || now.Before(i.updatedAt) {
		return Item{}, errors.New("agentmemory: review time precedes current item")
	}
	if err := instant.Validate(now); err != nil {
		return Item{}, fmt.Errorf("agentmemory: review time: %w", err)
	}
	i.revision, err = i.revision.Next()
	if err != nil {
		return Item{}, err
	}
	i.status = status
	i.updatedAt = now
	return i, nil
}

func (i Item) ValidateVisible() error {
	if i.status == StatusRejected {
		return ErrNotVisible
	}
	return nil
}

func (i Item) CheckRevision(expected uint64) error {
	if expected == 0 || expected > exactint.Maximum {
		return ErrInvalidRevision
	}
	if expected != i.revision.Value() {
		return ErrRevisionConflict
	}
	return nil
}

func (i Item) ValidateRemoval(expectedRevision uint64) error {
	if err := i.CheckRevision(expectedRevision); err != nil {
		return err
	}
	return i.ValidateVisible()
}

func newItem(id ItemID, scope Scope, project, content string, origin Origin, status Status, now time.Time) (Item, error) {
	content, err := NormalizeContent(content)
	if err != nil {
		return Item{}, err
	}
	now = now.UTC()
	item := Item{
		id: id, revision: exactint.First(), scope: scope, project: project, content: content, origin: origin, status: status,
		day: now.Format(time.DateOnly), createdAt: now, updatedAt: now,
	}
	if err := item.validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (i Item) validate() error {
	if err := i.id.validate(); err != nil {
		return err
	}
	if err := ValidateTarget(i.scope, i.project); err != nil {
		return err
	}
	content, err := NormalizeContent(i.content)
	if err != nil {
		return err
	}
	if content != i.content {
		return errors.New("agentmemory: item content is not canonical")
	}
	if err := errors.Join(i.origin.validate(), i.status.validate()); err != nil {
		return err
	}
	if i.origin == OriginUser && i.status != StatusActive {
		return errors.New("agentmemory: user-authored item must be active")
	}
	if err := i.validateTimestamps(); err != nil {
		return err
	}
	if i.embeddingSpace == "" && len(i.embedding) == 0 {
		return nil
	}
	if err := validateEmbedding(i.embeddingSpace, i.embedding); err != nil {
		return fmt.Errorf("agentmemory: item embedding: %w", err)
	}
	return nil
}

func (i Item) validateTimestamps() error {
	if i.createdAt.IsZero() || i.updatedAt.IsZero() {
		return errors.New("agentmemory: item timestamps are required")
	}
	if err := instant.Validate(i.createdAt, i.updatedAt); err != nil {
		return fmt.Errorf("agentmemory: item timestamps: %w", err)
	}
	if i.updatedAt.Before(i.createdAt) {
		return errors.New("agentmemory: item update precedes creation")
	}
	return nil
}
