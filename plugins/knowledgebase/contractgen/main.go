package main

import (
	json "encoding/json/v2"
	"log"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

func main() {
	contract, err := define()
	if err != nil {
		log.Fatal(err)
	}
	data, err := json.Marshal(contract.Descriptor(), json.Deterministic(true))
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		log.Fatal(err)
	}
}

func define() (*portable.Capability, error) {
	importMethod, err := qimumcp.NewMethod[wire.ImportInput, knowledge.Header](qimumcp.MethodConfig{Name: "flame.knowledgebase/import.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Import or replace a user document and its embedding index at the inspected revision; use zero for a new document."})
	if err != nil {
		return nil, err
	}
	reindexMethod, err := qimumcp.NewMethod[wire.DocumentInput, knowledge.Header](qimumcp.MethodConfig{Name: "flame.knowledgebase/reindex.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Rebuild an existing document's embedding index from its stored original text at the inspected revision through the configured embedding role."})
	if err != nil {
		return nil, err
	}
	deleteMethod, err := qimumcp.NewMethod[wire.DocumentInput, bool](qimumcp.MethodConfig{Name: "flame.knowledgebase/delete.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Delete a document and its index at the inspected revision."})
	if err != nil {
		return nil, err
	}
	listMethod, err := qimumcp.NewMethod[struct{}, []knowledge.Header](qimumcp.MethodConfig{Name: "flame.knowledgebase/list.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "List indexed user documents and their revisions."})
	if err != nil {
		return nil, err
	}
	searchMethod, err := qimumcp.NewMethod[wire.SearchInput, []knowledge.Match](qimumcp.MethodConfig{Name: "flame.knowledgebase/search.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Retrieve cited chunks from indexed user documents through the configured embedding role."})
	if err != nil {
		return nil, err
	}
	readMethod, err := qimumcp.NewMethod[wire.ReadInput, wire.DocumentRead](qimumcp.MethodConfig{Name: "flame.knowledgebase/read.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read one complete stored document with its exact source, revision and index identity without embedding or provider I/O."})
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: "flame.knowledgebase.v1", Composition: portable.Collection, Scope: extension.ScopeInstallation}, importMethod.Descriptor(), reindexMethod.Descriptor(), deleteMethod.Descriptor(), listMethod.Descriptor(), readMethod.Descriptor(), searchMethod.Descriptor())
}
