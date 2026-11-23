package contract

import (
	"github.com/Tangerg/scope/core/jsonschema"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/plugins/memory/internal/agentmemory"
)

const (
	List     = "flame.memory/list.v3"
	Add      = "flame.memory/add.v3"
	Update   = "flame.memory/update.v3"
	Delete   = "flame.memory/delete.v3"
	Review   = "flame.memory/review.v3"
	Items    = "flame.memory/items.v3"
	Search   = "flame.memory/search.v3"
	Maintain = "flame.memory/maintain.v2"
)

type ListInput struct {
	Scope  agentmemory.Scope `json:"scope"`
	CWD    string            `json:"cwd"`
	Cursor string            `json:"cursor"`
	Limit  *int              `json:"limit,omitzero"`
}

func (ListInput) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"limit": {Minimum: new(float64(1)), Maximum: new(float64(agentmemory.MaxManagementPageItems))},
	}}
}

// Embeddings stay private to ranking: no caller reads them, and each vector would
// multiply every payload.
type Item struct {
	ID        agentmemory.ItemID `json:"id"`
	Revision  uint64             `json:"revision" jsonschema:"minimum=1"`
	Scope     agentmemory.Scope  `json:"scope"`
	Project   string             `json:"project"`
	Content   string             `json:"content" jsonschema:"pattern=\\S"`
	Origin    agentmemory.Origin `json:"origin"`
	Status    agentmemory.Status `json:"status"`
	Pinned    bool               `json:"pinned"`
	SessionID string             `json:"sessionId"`
	Day       string             `json:"day"`
	CreatedAt instant.Timestamp  `json:"createdAt" jsonschema:"format=date-time"`
	UpdatedAt instant.Timestamp  `json:"updatedAt" jsonschema:"format=date-time"`
}

func NewItem(item agentmemory.Item) Item {
	return Item{
		ID: item.ID(), Revision: item.Revision(), Scope: item.Scope(), Project: item.Project(), Content: item.Content(),
		Origin: item.Origin(), Status: item.Status(), Pinned: item.Pinned(), SessionID: item.SessionID().String(), Day: item.Day(),
		CreatedAt: instant.Timestamp{Time: item.CreatedAt()}, UpdatedAt: instant.Timestamp{Time: item.UpdatedAt()},
	}
}

func NewItems(items []agentmemory.Item) []Item {
	result := make([]Item, len(items))
	for index, item := range items {
		result[index] = NewItem(item)
	}
	return result
}

func (Item) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"revision": {Maximum: new(float64(exactint.Maximum))},
		"content":  {MaxLength: new(uint64(agentmemory.MaxContentCharacters))},
	}}
}

type Page struct {
	Rows       []Item `json:"rows"`
	NextCursor string `json:"nextCursor"`
	Capacity   int    `json:"capacity"`
}

func (Page) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"rows":     {MaxItems: new(uint64(agentmemory.MaxManagementPageItems))},
		"capacity": {Minimum: new(float64(agentmemory.MaxVisiblePerTarget)), Maximum: new(float64(agentmemory.MaxVisiblePerTarget))},
	}}
}

type AddInput struct {
	Scope   agentmemory.Scope `json:"scope"`
	CWD     string            `json:"cwd"`
	Content string            `json:"content"`
}
type AddResult struct {
	Item    Item `json:"item"`
	Changed bool `json:"changed"`
}
type UpdateInput struct {
	ID               string  `json:"id"`
	ExpectedRevision uint64  `json:"expectedRevision" jsonschema:"minimum=1"`
	Content          *string `json:"content,omitzero"`
	Pinned           *bool   `json:"pinned,omitzero"`
}
type ItemInput struct {
	ID               string `json:"id"`
	ExpectedRevision uint64 `json:"expectedRevision" jsonschema:"minimum=1"`
}
type ReviewInput struct {
	ID               string                     `json:"id"`
	ExpectedRevision uint64                     `json:"expectedRevision" jsonschema:"minimum=1"`
	Decision         agentmemory.ReviewDecision `json:"decision"`
}
type ItemsInput struct {
	Scope   agentmemory.Scope `json:"scope"`
	Project string            `json:"project"`
}

func (UpdateInput) JSONSchemaConstraints() jsonschema.Constraints { return revisionBounds() }
func (ItemInput) JSONSchemaConstraints() jsonschema.Constraints   { return revisionBounds() }
func (ReviewInput) JSONSchemaConstraints() jsonschema.Constraints { return revisionBounds() }

func revisionBounds() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"expectedRevision": {Maximum: new(float64(exactint.Maximum))},
	}}
}

type SearchInput struct {
	Project        string `json:"project"`
	Query          string `json:"query"`
	TopK           int    `json:"topK"`
	EmbeddingSpace string `json:"embeddingSpace"`
}
type Conversation struct {
	MessageCount int    `json:"messageCount"`
	Transcript   string `json:"transcript"`
}
type MaintenanceInput struct {
	SessionID    string       `json:"sessionId"`
	Project      string       `json:"project"`
	Compacted    bool         `json:"compacted"`
	Conversation Conversation `json:"conversation"`
}
type MaintenanceResult struct {
	Published bool `json:"published"`
}
type Ack struct{}
