package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/workspace"
)

func (a *app) ShowKnowledge() {
	if a.knowledge == nil {
		a.message("this runtime composition has no knowledge service")
		return
	}
	a.executeRuntimeReaderQuery(a.knowledgeEntriesReaderQuery())
}

func (a *app) knowledgeEntriesReaderQuery() runtimeReaderQuery {
	workspace := a.session.Workspace.Path
	return runtimeReaderQuery{
		status: "loading FLAME.md knowledge", mode: runtimeReaderKnowledge,
		read: func(ctx context.Context) (readerDocument, error) {
			entries, err := a.knowledge.Entries(ctx, workspace)
			if err != nil {
				return readerDocument{}, err
			}
			return knowledgeEntriesDocument(workspace, entries), nil
		},
	}
}

func knowledgeEntriesDocument(workspace string, entries []workspace.KnowledgeEntry) readerDocument {
	if len(entries) == 0 {
		return paragraphDocument("FLAME.md knowledge", workspace, []string{"No knowledge documents exist in the cascade."})
	}
	sections := make([]ToolSection, 0, len(entries))
	for _, entry := range entries {
		detail := string(entry.Scope)
		if entry.UpdatedAt != nil {
			detail += " · updated " + entry.UpdatedAt.Format(time.RFC3339)
		}
		content := entry.Content
		if content == "" {
			content = "(empty document)"
		}
		sections = append(sections, ToolSection{Title: detail, Style: toolSectionParagraph, Text: content, Links: true})
	}
	return readerDocument{
		Title: "FLAME.md knowledge", Detail: fmt.Sprintf("%d cascade entries · %s", len(entries), workspace), Sections: sections,
	}
}

func (a *app) ReadKnowledge(argument string) error {
	if a.knowledge == nil {
		return errors.New("this runtime composition has no knowledge service")
	}
	target, err := parseKnowledgeTarget(argument, a.session.Workspace.Path)
	if err != nil {
		return err
	}
	a.readKnowledge(target)
	return nil
}

func (a *app) readKnowledge(target workspace.KnowledgeTarget) {
	a.executeRuntimeReaderQuery(a.knowledgeDocumentReaderQuery(target))
}

func (a *app) knowledgeDocumentReaderQuery(target workspace.KnowledgeTarget) runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading " + string(target.Scope) + " FLAME.md", mode: runtimeReaderKnowledge,
		selection: runtimeReaderSelection{knowledgeTarget: target, knowledgeEntry: true},
		read: func(ctx context.Context) (readerDocument, error) {
			entry, err := a.knowledge.Document(ctx, target)
			if err != nil {
				return readerDocument{}, err
			}
			return knowledgeDocument(target, entry), nil
		},
	}
}

func knowledgeDocument(target workspace.KnowledgeTarget, entry workspace.KnowledgeEntry) readerDocument {
	detail := string(target.Scope)
	if target.Workspace != "" {
		detail += " · " + target.Workspace
	}
	if entry.UpdatedAt != nil {
		detail += " · updated " + entry.UpdatedAt.Format(time.RFC3339)
	}
	content := entry.Content
	if content == "" {
		content = "(empty document)"
	}
	return readerDocument{
		Title: "FLAME.md · " + string(target.Scope), Detail: detail,
		Sections: []ToolSection{{Title: "Content", Style: toolSectionParagraph, Text: content, Links: true}},
	}
}

func (a *app) EditKnowledge(argument string) error {
	if a.knowledge == nil {
		return errors.New("this runtime composition has no knowledge service")
	}
	target, err := parseKnowledgeTarget(argument, a.session.Workspace.Path)
	if err != nil {
		return err
	}
	a.status.note("loading " + string(target.Scope) + " FLAME.md to edit")
	if !a.runOperation(knowledgeOperation, false,
		func(ctx context.Context) (workspace.KnowledgeEntry, error) { return a.knowledge.Document(ctx, target) },
		func(entry workspace.KnowledgeEntry, err error) {
			if err != nil {
				a.message("load FLAME.md to edit failed: " + err.Error())
				return
			}
			current := entry
			a.openContextEditor(contextEditorRequest{
				Title:       "Edit FLAME.md · " + string(target.Scope),
				Description: "Enter inserts a newline. Ctrl+S saves; an empty document clears this scope.",
				Content:     entry.Content,
				Placeholder: "Human-authored instructions and project knowledge",
				Save: func(content string, complete func(error) bool) error {
					if content == entry.Content {
						a.message("FLAME.md unchanged · " + string(target.Scope))
						complete(nil)
						return nil
					}
					return a.saveKnowledge(&current, target, content, complete)
				},
			})
		},
	) {
		return errors.New("another knowledge operation is running")
	}
	return nil
}

func (a *app) saveKnowledge(current *workspace.KnowledgeEntry, target workspace.KnowledgeTarget, content string, complete func(error) bool) error {
	if current == nil {
		return errors.New("knowledge editor has no revision owner")
	}
	update, err := current.Revise(target, content)
	if err != nil {
		return err
	}
	a.status.note("saving " + string(target.Scope) + " FLAME.md")
	if !a.runAdmissionMutation(knowledgeOperation, false,
		func(ctx context.Context) (workspace.KnowledgeEntry, error) { return a.knowledge.Save(ctx, update) },
		func(saved workspace.KnowledgeEntry, err error) {
			if err != nil {
				a.message("save FLAME.md failed: " + err.Error())
				if complete != nil {
					complete(err)
				}
				return
			}
			*current = saved
			closed := true
			if complete != nil {
				closed = complete(nil)
			}
			a.message("FLAME.md saved · " + string(target.Scope))
			if closed {
				a.openReaderDocument(knowledgeDocument(target, saved))
			}
		},
	) {
		return errors.New("another knowledge operation is running")
	}
	return nil
}

func parseKnowledgeTarget(argument, workspacePath string) (workspace.KnowledgeTarget, error) {
	scope, err := workspace.ParseKnowledgeScope(strings.TrimSpace(argument))
	if err != nil {
		return workspace.KnowledgeTarget{}, errors.New("usage: <cwd|projectRoot|home>")
	}
	if scope == workspace.KnowledgeHome {
		workspacePath = ""
	}
	return workspace.NewKnowledgeTarget(scope, workspacePath)
}
