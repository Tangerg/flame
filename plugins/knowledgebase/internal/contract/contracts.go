package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/knowledgebase/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --model-contract context > ../../package/io.github.tangerg.flame/contracts/context.generated.json"

import (
	"strconv"

	"github.com/invopop/jsonschema"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
)

type ImportInput struct {
	Content          knowledge.Content `json:"content"`
	ExpectedRevision int64             `json:"expectedRevision" jsonschema:"minimum=0"`
}

type DocumentInput struct {
	ID               string `json:"id" jsonschema:"minLength=1"`
	ExpectedRevision int64  `json:"expectedRevision" jsonschema:"minimum=1"`
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
	Capability *extension.Capability
	Import     *pluginsdk.Method[ImportInput, knowledge.Header]
	Delete     *pluginsdk.Method[DocumentInput, bool]
	List       *pluginsdk.Method[struct{}, []knowledge.Header]
	Search     *pluginsdk.Method[SearchInput, []knowledge.Match]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Import, err = pluginsdk.NewMethod[ImportInput, knowledge.Header](pluginsdk.MethodConfig{Name: "flame.knowledgebase/import.v1", Kind: extension.Mutation, Target: extension.ScopeInstallation, Description: "Import or replace a user document and its embedding index at the inspected revision; use zero for a new document."})
	if err != nil {
		return Contracts{}, err
	}
	c.Delete, err = pluginsdk.NewMethod[DocumentInput, bool](pluginsdk.MethodConfig{Name: "flame.knowledgebase/delete.v1", Kind: extension.Mutation, Target: extension.ScopeInstallation, Description: "Delete a document and its index at the inspected revision."})
	if err != nil {
		return Contracts{}, err
	}
	c.List, err = pluginsdk.NewMethod[struct{}, []knowledge.Header](pluginsdk.MethodConfig{Name: "flame.knowledgebase/list.v1", Kind: extension.Query, Target: extension.ScopeInstallation, Description: "List indexed user documents and their revisions."})
	if err != nil {
		return Contracts{}, err
	}
	c.Search, err = pluginsdk.NewMethod[SearchInput, []knowledge.Match](pluginsdk.MethodConfig{Name: "flame.knowledgebase/search.v1", Kind: extension.Query, Target: extension.ScopeInstallation, Description: "Retrieve cited chunks from indexed user documents through the configured embedding role."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = extension.NewCapability(extension.CapabilityConfig{Reference: "flame.knowledgebase.v1", Composition: extension.Collection, Scope: extension.ScopeInstallation}, c.Import.Descriptor(), c.Delete.Descriptor(), c.List.Descriptor(), c.Search.Descriptor())
	return c, err
}
