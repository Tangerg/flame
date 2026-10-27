package contract

import (
	"strconv"

	"github.com/invopop/jsonschema"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
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

func (ListInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	limit, ok := schema.Properties.Get("Limit")
	if ok {
		limit.Minimum = jsonvalue.Number("1")
		limit.Maximum = jsonvalue.Number(strconv.Itoa(memory.MaxManagementPageItems))
	}
}

type Page struct {
	Rows       []memory.ItemSnapshot
	NextCursor string
	Capacity   int
}

func (Page) JSONSchemaExtend(schema *jsonschema.Schema) {
	rows, ok := schema.Properties.Get("Rows")
	if ok {
		rows.MaxItems = new(uint64(memory.MaxManagementPageItems))
	}
	capacity, ok := schema.Properties.Get("Capacity")
	if ok {
		capacity.Const = memory.MaxVisiblePerTarget
	}
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

func (UpdateInput) JSONSchemaExtend(schema *jsonschema.Schema) { boundRevision(schema) }
func (ItemInput) JSONSchemaExtend(schema *jsonschema.Schema)   { boundRevision(schema) }
func (ReviewInput) JSONSchemaExtend(schema *jsonschema.Schema) { boundRevision(schema) }

func boundRevision(schema *jsonschema.Schema) {
	if revision, ok := schema.Properties.Get("ExpectedRevision"); ok {
		revision.Maximum = jsonvalue.Number(strconv.FormatUint(exactint.Maximum, 10))
	}
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
