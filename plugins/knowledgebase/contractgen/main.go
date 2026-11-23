package main

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/knowledge"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/wire"

	"github.com/Tangerg/frame"
)

func main() { plugincontract.Publish(define) }

func define() (*plugincontract.Capability, error) {
	importMethod, err := pluginmcp.NewMethod[wire.ImportInput, knowledge.Header](frame.MethodConfig{Name: "flame.knowledgebase/import.v1", Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Import or replace a user document and its embedding index at the inspected revision; use zero for a new document."})
	if err != nil {
		return nil, err
	}
	reindexMethod, err := pluginmcp.NewMethod[wire.DocumentInput, knowledge.Header](frame.MethodConfig{Name: "flame.knowledgebase/reindex.v1", Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Rebuild an existing document's embedding index from its stored original text at the inspected revision through the configured embedding role."})
	if err != nil {
		return nil, err
	}
	deleteMethod, err := pluginmcp.NewMethod[wire.DocumentInput, bool](frame.MethodConfig{Name: "flame.knowledgebase/delete.v1", Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Delete a document and its index at the inspected revision."})
	if err != nil {
		return nil, err
	}
	listMethod, err := pluginmcp.NewMethod[struct{}, []knowledge.Header](frame.MethodConfig{Name: "flame.knowledgebase/list.v1", Kind: frame.Query, Target: extension.ScopeInstallation, Description: "List indexed user documents and their revisions."})
	if err != nil {
		return nil, err
	}
	searchMethod, err := pluginmcp.NewMethod[wire.SearchInput, []knowledge.Match](frame.MethodConfig{Name: "flame.knowledgebase/search.v1", Kind: frame.Query, Target: extension.ScopeInstallation, Description: "Retrieve cited chunks from indexed user documents through the configured embedding role."})
	if err != nil {
		return nil, err
	}
	readMethod, err := pluginmcp.NewMethod[wire.ReadInput, wire.DocumentRead](frame.MethodConfig{Name: "flame.knowledgebase/read.v1", Kind: frame.Query, Target: extension.ScopeInstallation, Description: "Read one complete stored document with its exact source, revision and index identity without embedding or provider I/O."})
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.knowledgebase.v1", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation}, importMethod.Descriptor(), reindexMethod.Descriptor(), deleteMethod.Descriptor(), listMethod.Descriptor(), readMethod.Descriptor(), searchMethod.Descriptor())
}
