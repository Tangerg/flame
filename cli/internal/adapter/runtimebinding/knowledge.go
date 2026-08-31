package runtimebinding

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type knowledgeBinding interface {
	ListKnowledge(context.Context, protocol.WorkspaceQuery, flameruntime.CallOptions) (*protocol.Page[protocol.KnowledgeEntry], error)
	GetKnowledge(context.Context, protocol.GetKnowledgeRequest, flameruntime.CallOptions) (*protocol.KnowledgeEntry, error)
	UpdateKnowledge(context.Context, protocol.UpdateKnowledgeRequest, flameruntime.CommandOptions) (*protocol.KnowledgeEntry, error)
}

type Knowledge struct{ runtime *Connection }

func (k *Knowledge) Entries(ctx context.Context, workspacePath string) ([]workspace.KnowledgeEntry, error) {
	r := k.runtime
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return nil, errors.New("list knowledge: workspace is empty")
	}
	page, err := r.knowledge.ListKnowledge(ctx, protocol.WorkspaceQuery{
		Workspace: protocol.WorkspaceRef{Path: workspacePath},
	}, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list knowledge", page)
	if err != nil {
		return nil, err
	}
	entries := make([]workspace.KnowledgeEntry, 0, len(values))
	seen := make(map[workspace.KnowledgeScope]struct{}, len(values))
	for index, value := range values {
		entry := projectKnowledgeEntry(value)
		if err := entry.Validate(); err != nil {
			return nil, runtimeContractViolation("list knowledge item %d is invalid: %v", index+1, err)
		}
		if _, duplicate := seen[entry.Scope]; duplicate {
			return nil, runtimeContractViolation("list knowledge repeats %s scope", entry.Scope)
		}
		seen[entry.Scope] = struct{}{}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (k *Knowledge) Document(ctx context.Context, target workspace.KnowledgeTarget) (workspace.KnowledgeEntry, error) {
	r := k.runtime
	if err := target.Validate(); err != nil {
		return workspace.KnowledgeEntry{}, err
	}
	request := protocol.GetKnowledgeRequest{Scope: protocol.KnowledgeScope(target.Scope)}
	if target.Scope != workspace.KnowledgeHome {
		request.Workspace = &protocol.WorkspaceRef{Path: target.Workspace}
	}
	result, err := r.knowledge.GetKnowledge(ctx, request, r.callOptions())
	if err != nil {
		return workspace.KnowledgeEntry{}, classifyError(err)
	}
	if result == nil {
		return workspace.KnowledgeEntry{}, runtimeContractViolation("get knowledge returned nil")
	}
	entry := projectKnowledgeEntry(*result)
	if err := entry.Validate(); err != nil {
		return workspace.KnowledgeEntry{}, runtimeContractViolation("get knowledge returned an invalid entry: %v", err)
	}
	if entry.Scope != target.Scope {
		return workspace.KnowledgeEntry{}, runtimeContractViolation("get knowledge returned %s scope, want %s", entry.Scope, target.Scope)
	}
	return entry, nil
}

func (k *Knowledge) Save(ctx context.Context, update workspace.KnowledgeUpdate) (workspace.KnowledgeEntry, error) {
	r := k.runtime
	if err := update.Validate(); err != nil {
		return workspace.KnowledgeEntry{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return workspace.KnowledgeEntry{}, err
	}
	target := update.Target
	request := protocol.UpdateKnowledgeRequest{
		Scope: protocol.KnowledgeScope(target.Scope), ExpectedRevision: update.ExpectedRevision, Content: update.Content,
	}
	if target.Scope != workspace.KnowledgeHome {
		request.Workspace = &protocol.WorkspaceRef{Path: target.Workspace}
	}
	updated, err := r.knowledge.UpdateKnowledge(ctx, request, options)
	if err != nil {
		return workspace.KnowledgeEntry{}, classifyError(err)
	}
	if updated == nil {
		return workspace.KnowledgeEntry{}, runtimeContractViolation("update knowledge returned nil")
	}
	entry := projectKnowledgeEntry(*updated)
	if validateErr := entry.Validate(); validateErr != nil {
		return workspace.KnowledgeEntry{}, runtimeContractViolation("update knowledge returned an invalid entry: %v", validateErr)
	}
	if entry.Scope != target.Scope || entry.Content != update.Content {
		return workspace.KnowledgeEntry{}, runtimeContractViolation("update knowledge returned a mismatched entry")
	}
	authoritative, err := k.Document(ctx, target)
	if err != nil {
		return workspace.KnowledgeEntry{}, err
	}
	if authoritative.Content != update.Content {
		return workspace.KnowledgeEntry{}, errors.New("verify knowledge update: authoritative document did not converge")
	}
	return authoritative, nil
}

func projectKnowledgeEntry(value protocol.KnowledgeEntry) workspace.KnowledgeEntry {
	entry := workspace.KnowledgeEntry{Scope: workspace.KnowledgeScope(value.Scope), Content: value.Content, Revision: value.Revision}
	if !value.UpdatedAt.IsZero() {
		entry.UpdatedAt = new(value.UpdatedAt)
	}
	return entry
}
