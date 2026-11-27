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
	importMethod, err := pluginmcp.NewMethod[wire.ImportInput, knowledge.Header](frame.MethodConfig{Name: "flame.knowledgebase/import.v1", Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Import or replace a user document and its embedding index at the inspected revision; use zero for a new document. A document with a project is visible only within that Project."})
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
	listMethod, err := pluginmcp.NewMethod[wire.ListInput, []knowledge.Header](frame.MethodConfig{Name: "flame.knowledgebase/list.v1", Kind: frame.Query, Target: extension.ScopeInstallation, Description: "List the indexed user documents visible to the caller, shared documents and its Project's own, with their revisions."})
	if err != nil {
		return nil, err
	}
	searchMethod, err := pluginmcp.NewMethod[wire.SearchInput, []knowledge.Match](frame.MethodConfig{Name: "flame.knowledgebase/search.v1", Kind: frame.Query, Target: extension.ScopeInstallation, Description: "Retrieve cited chunks from the indexed user documents visible to the caller, shared documents and its Project's own, through the configured embedding role."})
	if err != nil {
		return nil, err
	}
	readMethod, err := pluginmcp.NewMethod[wire.ReadInput, wire.DocumentRead](frame.MethodConfig{Name: "flame.knowledgebase/read.v1", Kind: frame.Query, Target: extension.ScopeInstallation, Description: "Read a stored document or one numbered page at the current or retained revision. Current Project access is checked before returning source metadata or text."})
	if err != nil {
		return nil, err
	}
	moveMethod, err := pluginmcp.NewMethod[wire.MoveInput, knowledge.Header](frame.MethodConfig{
		Name: "flame.knowledgebase/move.v1", Kind: frame.Command, Target: extension.ScopeInstallation,
		Description: "Move an inspected material to another Project without re-embedding its content. This withdraws access from the original Project.",
	})
	if err != nil {
		return nil, err
	}
	claimAccept, err := pluginmcp.NewMethod[wire.ClaimAcceptInput, wire.ClaimReceipt](frame.MethodConfig{
		Name: "flame.knowledgebase/accept-claim.v1", Kind: frame.Command, Target: extension.ScopeInstallation,
		Description: "Accept an exact research statement and reason with validated citations in one Project.",
	})
	if err != nil {
		return nil, err
	}
	claimRead, err := pluginmcp.NewMethod[wire.ClaimReadInput, wire.ClaimRead](frame.MethodConfig{
		Name: "flame.knowledgebase/read-claim.v1", Kind: frame.Query, Target: extension.ScopeInstallation,
		Description: "Read a retained research decision with exact page and line citations, checking current Project access and changes to each cited passage.",
	})
	if err != nil {
		return nil, err
	}
	claimList, err := pluginmcp.NewMethod[wire.ListInput, []wire.ClaimSummary](frame.MethodConfig{
		Name: "flame.knowledgebase/list-claims.v1", Kind: frame.Query, Target: extension.ScopeInstallation,
		Description: "List a Project's latest research decisions and current evidence status.",
	})
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.knowledgebase.v1", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation}, importMethod.Descriptor(), reindexMethod.Descriptor(), deleteMethod.Descriptor(), listMethod.Descriptor(), readMethod.Descriptor(), searchMethod.Descriptor(), claimAccept.Descriptor(), claimRead.Descriptor(), claimList.Descriptor(), moveMethod.Descriptor())
}
