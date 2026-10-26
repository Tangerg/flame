package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/knowledgebase/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/contractgen --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/packages/go-sdk/cmd/contractgen --model-contract context > ../../package/io.github.tangerg.flame/contracts/context.generated.json"

import (
	"errors"
	"strconv"

	"github.com/invopop/jsonschema"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/exactjson"
	"github.com/Tangerg/flame/packages/go-sdk/idempotency"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/backend/domain"
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
		limit.Maximum = exactjson.Number(strconv.Itoa(knowledge.MaxResults))
	}
}

type Contracts struct {
	Capability *capability.Contract
	Import     *pluginsdk.Method[ImportInput, knowledge.Header]
	Delete     *pluginsdk.Method[DocumentInput, bool]
	List       *pluginsdk.Method[struct{}, []knowledge.Header]
	Search     *pluginsdk.Method[SearchInput, []knowledge.Match]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Import, err = pluginsdk.NewMethod[ImportInput, knowledge.Header](pluginsdk.MethodConfig{Name: "flame.knowledgebase/import.v1", Kind: capability.Mutation, Target: capability.ScopeInstallation, Description: "Import or replace a user document and its embedding index at the inspected revision; use zero for a new document."})
	if err != nil {
		return Contracts{}, err
	}
	c.Delete, err = pluginsdk.NewMethod[DocumentInput, bool](pluginsdk.MethodConfig{Name: "flame.knowledgebase/delete.v1", Kind: capability.Mutation, Target: capability.ScopeInstallation, Description: "Delete a document and its index at the inspected revision."})
	if err != nil {
		return Contracts{}, err
	}
	c.List, err = pluginsdk.NewMethod[struct{}, []knowledge.Header](pluginsdk.MethodConfig{Name: "flame.knowledgebase/list.v1", Kind: capability.Query, Target: capability.ScopeInstallation, Description: "List indexed user documents and their revisions."})
	if err != nil {
		return Contracts{}, err
	}
	c.Search, err = pluginsdk.NewMethod[SearchInput, []knowledge.Match](pluginsdk.MethodConfig{Name: "flame.knowledgebase/search.v1", Kind: capability.Query, Target: capability.ScopeInstallation, Description: "Retrieve cited chunks from indexed user documents through the configured embedding role."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = capability.New(capability.Config{Reference: "flame.knowledgebase.v1", Composition: capability.Collection, Scope: capability.ScopeInstallation}, c.Import.Descriptor(), c.Delete.Descriptor(), c.List.Descriptor(), c.Search.Descriptor())
	return c, err
}

func Classify(err error) string {
	for _, failure := range []struct {
		cause error
		code  string
	}{
		{knowledge.ErrInvalidDocument, "invalid_document"}, {knowledge.ErrRevisionConflict, "revision_conflict"},
		{knowledge.ErrCapacity, "capacity"}, {knowledge.ErrNotFound, "not_found"},
		{knowledge.ErrRetired, "retired_document"},
		{knowledge.ErrInvalidIndex, "invalid_index"}, {knowledge.ErrEmbeddingSpace, "embedding_space_changed"},
		{idempotency.ErrKeyConflict, "command_conflict"},
	} {
		if errors.Is(err, failure.cause) {
			return failure.code
		}
	}
	return ""
}
