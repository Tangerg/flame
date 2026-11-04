package contract

import (
	"github.com/Tangerg/scope/core/jsonschema"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
)

const (
	List     = "flame.memory/list.v2"
	Add      = "flame.memory/add.v2"
	Update   = "flame.memory/update.v2"
	Delete   = "flame.memory/delete.v2"
	Review   = "flame.memory/review.v2"
	Items    = "flame.memory/items.v2"
	Search   = "flame.memory/search.v2"
	Maintain = "flame.memory/maintain.v1"
)

type ListInput struct {
	Scope       memory.Scope
	CWD, Cursor string
	Limit       *int `json:"Limit,omitzero"`
}

func (ListInput) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"Limit": {Minimum: new(float64(1)), Maximum: new(float64(memory.MaxManagementPageItems))},
	}}
}

type Page struct {
	Rows       []memory.ItemSnapshot
	NextCursor string
	Capacity   int
}

func (Page) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"Rows":     {MaxItems: new(uint64(memory.MaxManagementPageItems))},
		"Capacity": {Minimum: new(float64(memory.MaxVisiblePerTarget)), Maximum: new(float64(memory.MaxVisiblePerTarget))},
	}}
}

type AddInput struct {
	Scope        memory.Scope
	CWD, Content string
}
type AddResult struct {
	Item    memory.ItemSnapshot
	Changed bool
}
type UpdateInput struct {
	ID               string
	ExpectedRevision uint64  `jsonschema:"minimum=1"`
	Content          *string `json:"Content,omitzero"`
	Pinned           *bool   `json:"Pinned,omitzero"`
}
type ItemInput struct {
	ID               string
	ExpectedRevision uint64 `jsonschema:"minimum=1"`
}
type ReviewInput struct {
	ID               string
	ExpectedRevision uint64 `jsonschema:"minimum=1"`
	Decision         memory.ReviewDecision
}
type ItemsInput struct {
	Scope   memory.Scope
	Project string
}

func (UpdateInput) JSONSchemaConstraints() jsonschema.Constraints { return revisionBounds() }
func (ItemInput) JSONSchemaConstraints() jsonschema.Constraints   { return revisionBounds() }
func (ReviewInput) JSONSchemaConstraints() jsonschema.Constraints { return revisionBounds() }

func revisionBounds() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"ExpectedRevision": {Maximum: new(float64(exactint.Maximum))},
	}}
}

type SearchInput struct {
	Project, Query string
	TopK           int
	EmbeddingSpace string
}
type Conversation struct {
	MessageCount int
	Transcript   string
}
type MaintenanceInput struct {
	SessionID, Project string
	Compacted          bool
	Conversation       Conversation
}
type MaintenanceResult struct{ Published bool }
type Ack struct{}
