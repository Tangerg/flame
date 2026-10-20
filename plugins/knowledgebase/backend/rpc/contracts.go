package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/knowledgebase/backend/cmd/contracts | go run github.com/Tangerg/flame/runtime/cmd/contractgen --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"
//go:generate sh -c "go run github.com/Tangerg/flame/runtime/cmd/modelcontextcontracts > ../../package/io.github.tangerg.flame/contracts/context.generated.json"

import (
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/plugins/knowledgebase/backend/domain"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/idempotency"
)

type ImportInput struct {
	Content          domain.Content `json:"content"`
	ExpectedRevision int64          `json:"expectedRevision" jsonschema:"minimum=0"`
}

type DocumentInput struct {
	ID               string `json:"id" jsonschema:"minLength=1,maxLength=128"`
	ExpectedRevision int64  `json:"expectedRevision" jsonschema:"minimum=1"`
}

type SearchInput struct {
	Query string `json:"query" jsonschema:"minLength=1,maxLength=4096"`
	Limit int    `json:"limit" jsonschema:"minimum=1,maximum=8"`
}

type Contracts struct {
	Capability *capability.Contract
	Import     *pluginsdk.Method[ImportInput, domain.Header]
	Delete     *pluginsdk.Method[DocumentInput, bool]
	List       *pluginsdk.Method[struct{}, []domain.Header]
	Search     *pluginsdk.Method[SearchInput, []domain.Match]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Import, err = pluginsdk.NewMethod[ImportInput, domain.Header](pluginsdk.MethodConfig{Name: "flame.knowledgebase/import.v1", Kind: capability.Mutation, Target: "installation", Description: "Import or replace a user document and its embedding index at the inspected revision; use zero for a new document."})
	if err != nil {
		return Contracts{}, err
	}
	c.Delete, err = pluginsdk.NewMethod[DocumentInput, bool](pluginsdk.MethodConfig{Name: "flame.knowledgebase/delete.v1", Kind: capability.Mutation, Target: "installation", Description: "Delete a document and its index at the inspected revision."})
	if err != nil {
		return Contracts{}, err
	}
	c.List, err = pluginsdk.NewMethod[struct{}, []domain.Header](pluginsdk.MethodConfig{Name: "flame.knowledgebase/list.v1", Kind: capability.Query, Target: "installation", Description: "List indexed user documents and their revisions."})
	if err != nil {
		return Contracts{}, err
	}
	c.Search, err = pluginsdk.NewMethod[SearchInput, []domain.Match](pluginsdk.MethodConfig{Name: "flame.knowledgebase/search.v1", Kind: capability.Query, Target: "installation", Description: "Retrieve cited chunks from indexed user documents through the configured embedding role."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = capability.New(capability.Config{Reference: "flame.knowledgebase.v1", Composition: capability.Collection, Scope: "installation"}, c.Import.Descriptor(), c.Delete.Descriptor(), c.List.Descriptor(), c.Search.Descriptor())
	return c, err
}

func Classify(err error) string {
	for _, failure := range []struct {
		cause error
		code  string
	}{
		{domain.ErrInvalidDocument, "invalid_document"}, {domain.ErrRevisionConflict, "revision_conflict"},
		{domain.ErrCapacity, "capacity"}, {domain.ErrNotFound, "not_found"},
		{domain.ErrRetired, "retired_document"},
		{domain.ErrInvalidIndex, "invalid_index"}, {domain.ErrEmbeddingSpace, "embedding_space_changed"},
		{idempotency.ErrKeyConflict, "command_conflict"},
	} {
		if errors.Is(err, failure.cause) {
			return failure.code
		}
	}
	return ""
}
