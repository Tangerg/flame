package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/knowledgebase/backend/cmd/contracts | go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --model-contract context > ../../package/io.github.tangerg.flame/contracts/context.generated.json"

import (
	"strconv"

	"github.com/invopop/jsonschema"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

type ImportInput struct {
	Content          knowledge.Content `json:"content"`
	ExpectedRevision int64             `json:"expectedRevision" jsonschema:"minimum=0"`
}

type DocumentInput struct {
	ID               string `json:"id" jsonschema:"minLength=1"`
	ExpectedRevision int64  `json:"expectedRevision" jsonschema:"minimum=1"`
}

type ReadInput struct {
	ID string `json:"id" jsonschema:"minLength=1"`
}

func (ReadInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	if id, ok := schema.Properties.Get("id"); ok {
		id.MaxLength = new(uint64(knowledge.MaxIDRunes))
	}
}

type DocumentRead struct {
	Document knowledge.Header `json:"document"`
	Text     string           `json:"text"`
}

func (DocumentInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	if id, ok := schema.Properties.Get("id"); ok {
		id.MaxLength = new(uint64(knowledge.MaxIDRunes))
	}
}

type SearchInput struct {
	Query string `json:"query" jsonschema:"minLength=1"`
	Limit int    `json:"limit" jsonschema:"minimum=1"`
}

func (SearchInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	if query, ok := schema.Properties.Get("query"); ok {
		query.MaxLength = new(uint64(knowledge.MaxQueryRunes))
	}
	if limit, ok := schema.Properties.Get("limit"); ok {
		limit.Maximum = jsonvalue.Number(strconv.Itoa(knowledge.MaxResults))
	}
}

type Contracts struct {
	Capability *portable.Capability
	Import     *qimumcp.Method[ImportInput, knowledge.Header]
	Reindex    *qimumcp.Method[DocumentInput, knowledge.Header]
	Delete     *qimumcp.Method[DocumentInput, bool]
	List       *qimumcp.Method[struct{}, []knowledge.Header]
	Read       *qimumcp.Method[ReadInput, DocumentRead]
	Search     *qimumcp.Method[SearchInput, []knowledge.Match]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Import, err = qimumcp.NewMethod[ImportInput, knowledge.Header](qimumcp.MethodConfig{Name: "flame.knowledgebase/import.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Import or replace a user document and its embedding index at the inspected revision; use zero for a new document."})
	if err != nil {
		return Contracts{}, err
	}
	c.Reindex, err = qimumcp.NewMethod[DocumentInput, knowledge.Header](qimumcp.MethodConfig{Name: "flame.knowledgebase/reindex.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Rebuild an existing document's embedding index from its stored original text at the inspected revision through the configured embedding role."})
	if err != nil {
		return Contracts{}, err
	}
	c.Delete, err = qimumcp.NewMethod[DocumentInput, bool](qimumcp.MethodConfig{Name: "flame.knowledgebase/delete.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Delete a document and its index at the inspected revision."})
	if err != nil {
		return Contracts{}, err
	}
	c.List, err = qimumcp.NewMethod[struct{}, []knowledge.Header](qimumcp.MethodConfig{Name: "flame.knowledgebase/list.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "List indexed user documents and their revisions."})
	if err != nil {
		return Contracts{}, err
	}
	c.Search, err = qimumcp.NewMethod[SearchInput, []knowledge.Match](qimumcp.MethodConfig{Name: "flame.knowledgebase/search.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Retrieve cited chunks from indexed user documents through the configured embedding role."})
	if err != nil {
		return Contracts{}, err
	}
	c.Read, err = qimumcp.NewMethod[ReadInput, DocumentRead](qimumcp.MethodConfig{Name: "flame.knowledgebase/read.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read one complete stored document with its exact source, revision and index identity without embedding or provider I/O."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = portable.NewCapability(portable.CapabilityConfig{Reference: "flame.knowledgebase.v1", Composition: portable.Collection, Scope: extension.ScopeInstallation}, c.Import.Descriptor(), c.Reindex.Descriptor(), c.Delete.Descriptor(), c.List.Descriptor(), c.Read.Descriptor(), c.Search.Descriptor())
	return c, err
}
