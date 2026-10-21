package agentmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/invopop/jsonschema"

	"github.com/Tangerg/flame/runtime/instant"
	"github.com/Tangerg/flame/runtime/resourceid"
)

const (
	// Unicode character admission keeps a whole item within the prompt's conservative token budget.
	MaxContentCharacters = 4096

	// Admission precedes deduplication so repeated output cannot evade the extraction envelope.
	MaxFactsPerBatch = 32

	// Approved facts survive curation, so a proposal limit cannot cap total stored memory.
	MaxCurationProposals = 32

	// The prompt envelope may shrink a page but must not authorize an unbounded preceding database read.
	MaxLedgerFoldFacts = 128

	MaxVisiblePerTarget    = 512
	MaxManagementPageItems = 100

	// Recent rejections suppress repeated proposals without retaining unbounded history.
	MaxRejectedPerTarget = 2048
)

func NormalizeContent(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", errors.New("agentmemory: item content is required")
	}
	if !utf8.ValidString(content) {
		return "", errors.New("agentmemory: item content must be valid UTF-8")
	}
	if utf8.RuneCountInString(content) > MaxContentCharacters {
		return "", fmt.Errorf(
			"agentmemory: item content exceeds %d Unicode characters",
			MaxContentCharacters,
		)
	}
	return content, nil
}

// Content identity survives review so folds suppress rejected facts without changing item handles.
func Digest(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
}

var ErrNotFound = errors.New("agentmemory: item not found")

var ErrNotPending = errors.New("agentmemory: item is not pending review")

var ErrNotVisible = errors.New("agentmemory: item is not visible")

var ErrTargetFull = errors.New("agentmemory: target is full")

type Scope string

const (
	ScopeProject Scope = "project"
	ScopeUser    Scope = "user"
)

func (s Scope) Valid() bool {
	return s == ScopeProject || s == ScopeUser
}

func (s Scope) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("agentmemory: invalid scope %q", s)
	}
	return nil
}

func (s Scope) String() string { return string(s) }

func (Scope) JSONSchemaEnum() []string { return []string{string(ScopeProject), string(ScopeUser)} }

func ParseScope(value string) (Scope, error) {
	scope := Scope(value)
	return scope, scope.Validate()
}

func ValidateTarget(scope Scope, project string) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	switch scope {
	case ScopeProject:
		if strings.TrimSpace(project) == "" {
			return errors.New("agentmemory: project scope requires a project")
		}
	case ScopeUser:
		if project != "" {
			return errors.New("agentmemory: user scope forbids a project")
		}
	}
	return nil
}

type Status string

const (
	StatusActive   Status = "active"
	StatusPending  Status = "pending"
	StatusRejected Status = "rejected"
)

func (s Status) Valid() bool {
	return s == StatusActive || s == StatusPending || s == StatusRejected
}

func (s Status) Validate() error {
	if !s.Valid() {
		return fmt.Errorf("agentmemory: invalid status %q", s)
	}
	return nil
}

func (s Status) String() string { return string(s) }

func (Status) JSONSchemaEnum() []string {
	return []string{string(StatusActive), string(StatusPending), string(StatusRejected)}
}

func ParseStatus(value string) (Status, error) {
	status := Status(value)
	return status, status.Validate()
}

type Origin string

const (
	OriginAuto Origin = "auto"
	OriginUser Origin = "user"
)

func (o Origin) Valid() bool {
	return o == OriginAuto || o == OriginUser
}

func (o Origin) Validate() error {
	if !o.Valid() {
		return fmt.Errorf("agentmemory: invalid origin %q", o)
	}
	return nil
}

func (o Origin) String() string { return string(o) }

func (Origin) JSONSchemaEnum() []string { return []string{string(OriginAuto), string(OriginUser)} }

func ParseOrigin(value string) (Origin, error) {
	origin := Origin(value)
	return origin, origin.Validate()
}

type ReviewDecision string

const (
	ReviewApprove ReviewDecision = "approve"
	ReviewReject  ReviewDecision = "reject"
)

func (ReviewDecision) JSONSchemaEnum() []string {
	return []string{string(ReviewApprove), string(ReviewReject)}
}

func (r ReviewDecision) Result() (Status, error) {
	switch r {
	case ReviewApprove:
		return StatusActive, nil
	case ReviewReject:
		return StatusRejected, nil
	default:
		return "", fmt.Errorf("agentmemory: invalid review decision %q", r)
	}
}

// The handle survives content edits; curation cannot rewrite user-authored or approved content.
type Item struct {
	id        ItemID
	scope     Scope
	project   string
	content   string
	origin    Origin
	status    Status
	pinned    bool
	sessionID string
	day       string
	createdAt time.Time
	updatedAt time.Time

	// Vectors from different model configurations cannot share a coordinate system.
	embeddingSpace string
	embedding      []float32
}

type ItemSnapshot struct {
	ID             ItemID
	Scope          Scope
	Project        string
	Content        string
	Origin         Origin
	Status         Status
	Pinned         bool
	SessionID      string
	Day            string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	EmbeddingSpace string
	Embedding      []float32
}

func (ItemSnapshot) JSONSchemaExtend(schema *jsonschema.Schema) {
	content, ok := schema.Properties.Get("Content")
	if ok {
		content.Pattern = `\S`
		content.MaxLength = new(uint64(MaxContentCharacters))
	}
}

// Ordinary reads omit vectors, so only content authoring can authorize discarding the persisted cache.
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
	item := Item{
		id: snapshot.ID, scope: snapshot.Scope, project: snapshot.Project, content: snapshot.Content,
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

func (i Item) Snapshot() ItemSnapshot {
	return ItemSnapshot{
		ID: i.id, Scope: i.scope, Project: i.project, Content: i.content,
		Origin: i.origin, Status: i.status, Pinned: i.pinned, SessionID: i.sessionID, Day: i.day,
		CreatedAt: i.createdAt, UpdatedAt: i.updatedAt,
		EmbeddingSpace: i.embeddingSpace, Embedding: slices.Clone(i.embedding),
	}
}

func (i Item) ID() ItemID           { return i.id }
func (i Item) Scope() Scope         { return i.scope }
func (i Item) Project() string      { return i.project }
func (i Item) Content() string      { return i.content }
func (i Item) Origin() Origin       { return i.origin }
func (i Item) Status() Status       { return i.status }
func (i Item) Pinned() bool         { return i.pinned }
func (i Item) SessionID() string    { return i.sessionID }
func (i Item) Day() string          { return i.day }
func (i Item) CreatedAt() time.Time { return i.createdAt }
func (i Item) UpdatedAt() time.Time { return i.updatedAt }

func (i Item) Embedding() (string, []float32, bool) {
	if i.embeddingSpace == "" {
		return "", nil, false
	}
	return i.embeddingSpace, slices.Clone(i.embedding), true
}

// Digest and space prevent late vectors from being compared with different content or models.
type EmbeddingUpdate struct {
	ItemID        ItemID
	ContentDigest string
	Space         string
	Vector        []float32
}

func (e EmbeddingUpdate) Clone() EmbeddingUpdate {
	e.Vector = slices.Clone(e.Vector)
	return e
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
	if err := e.ItemID.Validate(); err != nil {
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
	i = i.withUserContent(content, now)
	i.pinned = false
	i.updatedAt = now
	return ItemChange{item: i, kind: itemContentAuthored}, nil
}

// Content edits transfer authorship to the user so curation cannot undo the correction.
func (i Item) Edit(content *string, pinned *bool, now time.Time) (ItemChange, error) {
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
	i.sessionID = ""
	i.day = now.Format(time.DateOnly)
	i.embeddingSpace = ""
	i.embedding = nil
	return i
}

func (i Item) Review(decision ReviewDecision, now time.Time) (Item, error) {
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

func newItem(id ItemID, scope Scope, project, content string, origin Origin, status Status, now time.Time) (Item, error) {
	content, err := NormalizeContent(content)
	if err != nil {
		return Item{}, err
	}
	now = now.UTC()
	item := Item{
		id: id, scope: scope, project: project, content: content, origin: origin, status: status,
		day: now.Format(time.DateOnly), createdAt: now, updatedAt: now,
	}
	if err := item.validate(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (i Item) validate() error {
	if err := i.id.Validate(); err != nil {
		return err
	}
	if i.sessionID != "" {
		if err := resourceid.ValidateSession(i.sessionID); err != nil {
			return fmt.Errorf("agentmemory: item provenance: %w", err)
		}
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
	if err := i.origin.Validate(); err != nil {
		return err
	}
	if err := i.status.Validate(); err != nil {
		return err
	}
	if i.origin == OriginUser && i.status != StatusActive {
		return errors.New("agentmemory: user-authored item must be active")
	}
	if i.createdAt.IsZero() || i.updatedAt.IsZero() {
		return errors.New("agentmemory: item timestamps are required")
	}
	if err := instant.Validate(i.createdAt, i.updatedAt); err != nil {
		return fmt.Errorf("agentmemory: item timestamps: %w", err)
	}
	if i.updatedAt.Before(i.createdAt) {
		return errors.New("agentmemory: item update precedes creation")
	}
	if i.embeddingSpace == "" && len(i.embedding) == 0 {
		return nil
	}
	if err := validateEmbedding(i.embeddingSpace, i.embedding); err != nil {
		return fmt.Errorf("agentmemory: item embedding: %w", err)
	}
	return nil
}

type FactBatch struct {
	Project    string
	SessionID  string
	Day        string
	Facts      []string
	CapturedAt time.Time
}

func (f FactBatch) Normalize() (FactBatch, error) {
	f.Project = strings.TrimSpace(f.Project)
	if f.Project == "" {
		return FactBatch{}, errors.New("agentmemory: fact batch project is required")
	}
	if err := resourceid.ValidateSession(f.SessionID); err != nil {
		return FactBatch{}, fmt.Errorf("agentmemory: fact batch: %w", err)
	}
	day, err := time.Parse(time.DateOnly, f.Day)
	if err != nil || day.Format(time.DateOnly) != f.Day {
		return FactBatch{}, fmt.Errorf("agentmemory: invalid ledger day %q", f.Day)
	}
	if f.CapturedAt.IsZero() {
		return FactBatch{}, errors.New("agentmemory: fact batch capture time is required")
	}
	if err := instant.Validate(f.CapturedAt); err != nil {
		return FactBatch{}, fmt.Errorf("agentmemory: fact batch capture time: %w", err)
	}
	f.Facts, err = normalizeFactList(f.Facts, MaxFactsPerBatch, "fact batch")
	if err != nil {
		return FactBatch{}, err
	}
	return f, nil
}

type LedgerFact struct {
	Sequence   int64
	Day        string
	Content    string
	CapturedAt time.Time
}

func (l LedgerFact) Validate() error {
	if l.Sequence <= 0 {
		return errors.New("agentmemory: ledger fact sequence must be positive")
	}
	day, err := time.Parse(time.DateOnly, l.Day)
	if err != nil || day.Format(time.DateOnly) != l.Day {
		return fmt.Errorf("agentmemory: invalid ledger day %q", l.Day)
	}
	content, err := NormalizeContent(l.Content)
	if err != nil {
		return fmt.Errorf("agentmemory: ledger fact content: %w", err)
	}
	if content != l.Content {
		return errors.New("agentmemory: ledger fact content is not canonical")
	}
	if l.CapturedAt.IsZero() {
		return errors.New("agentmemory: ledger fact capture time is required")
	}
	return instant.Validate(l.CapturedAt)
}

// A zero watermark has no timestamp; an advanced watermark may legitimately use the Unix epoch.
type State struct {
	Watermark int64
	UpdatedAt time.Time
}

func (s State) Validate() error {
	if s.Watermark < 0 {
		return errors.New("agentmemory: curation watermark must not be negative")
	}
	if s.Watermark == 0 && !s.UpdatedAt.IsZero() {
		return errors.New("agentmemory: empty curation state carries an update time")
	}
	if s.Watermark > 0 && s.UpdatedAt.IsZero() {
		return errors.New("agentmemory: advanced curation state has no update time")
	}
	return instant.Validate(s.UpdatedAt)
}

func normalizeFactList(input []string, maximum int, collection string) ([]string, error) {
	if len(input) > maximum {
		return nil, fmt.Errorf("agentmemory: %s exceeds %d items", collection, maximum)
	}
	var normalized []string
	seen := make(map[string]struct{})
	for _, fact := range input {
		if strings.TrimSpace(fact) == "" {
			continue
		}
		var err error
		fact, err = NormalizeContent(fact)
		if err != nil {
			return nil, fmt.Errorf("agentmemory: invalid %s item: %w", collection, err)
		}
		if _, duplicate := seen[fact]; duplicate {
			continue
		}
		seen[fact] = struct{}{}
		normalized = append(normalized, fact)
	}
	return normalized, nil
}
