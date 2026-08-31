package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func (a *app) ShowAgentMemory(argument string) error {
	if a.agentMemory == nil {
		return errors.New("this runtime composition has no agent memory service")
	}
	target, err := parseAgentMemoryTarget(argument, a.session.Workspace.Path)
	if err != nil {
		return err
	}
	a.showAgentMemory(target)
	return nil
}

func (a *app) showAgentMemory(target agent.MemoryTarget) {
	a.executeRuntimeReaderQuery(a.agentMemoryReaderQuery(target))
}

func (a *app) agentMemoryReaderQuery(target agent.MemoryTarget) runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading " + string(target.Scope) + " agent memory", mode: runtimeReaderAgentMemory,
		selection: runtimeReaderSelection{agentMemoryTarget: target},
		read: func(ctx context.Context) (readerDocument, error) {
			items, err := a.agentMemory.Items(ctx, target)
			if err != nil {
				return readerDocument{}, err
			}
			return agentMemoryDocument(target, items), nil
		},
	}
}

func agentMemoryDocument(target agent.MemoryTarget, items []agent.MemoryItem) readerDocument {
	title := "Agent memory · " + string(target.Scope)
	detail := fmt.Sprintf("%d items", len(items))
	if target.Workspace != "" {
		detail += " · " + target.Workspace
	}
	if len(items) == 0 {
		return paragraphDocument(title, detail, []string{"No active or pending memory is stored in this scope."})
	}
	sections := make([]ToolSection, 0, len(items)*2)
	for _, item := range items {
		state := string(item.Status)
		if item.Pinned {
			state += " · pinned"
		}
		metadata := []string{
			"id       " + item.ID,
			"scope    " + string(item.Scope),
			"origin   " + string(item.Origin),
			"status   " + state,
			"created  " + item.CreatedAt.Format(time.RFC3339),
			"updated  " + item.UpdatedAt.Format(time.RFC3339),
		}
		if item.SessionID != "" {
			metadata = append(metadata, "session  "+item.SessionID)
		}
		if item.Day != "" {
			metadata = append(metadata, "day      "+item.Day)
		}
		sections = append(sections,
			ToolSection{Title: state, Style: toolSectionParagraph, Text: item.Content, Links: true},
			ToolSection{Title: "Provenance", Style: toolSectionCode, Language: "text", Text: strings.Join(metadata, "\n")},
		)
	}
	return readerDocument{Title: title, Detail: detail, Sections: sections}
}

func (a *app) AddAgentMemory(argument string) error {
	if a.agentMemory == nil {
		return errors.New("this runtime composition has no agent memory service")
	}
	target, err := parseAgentMemoryTarget(argument, a.session.Workspace.Path)
	if err != nil {
		return err
	}
	a.openContextEditor(contextEditorRequest{
		Title:       "Add " + string(target.Scope) + " memory",
		Description: "User-authored memory becomes active immediately.",
		Placeholder: "Write one durable fact. Enter inserts a newline; Ctrl+S saves.",
		Save: func(content string, complete func(error) bool) error {
			content = strings.TrimSpace(content)
			if content == "" {
				return errors.New("memory content is empty")
			}
			return a.addAgentMemory(target, content, complete)
		},
	})
	return nil
}

func (a *app) EditAgentMemory(argument string) error {
	return a.loadAgentMemoryItem(argument, "loading agent memory to edit", func(target agent.MemoryTarget, item agent.MemoryItem) {
		a.openContextEditor(contextEditorRequest{
			Title:       "Edit agent memory · " + item.ID,
			Description: "The item identity and provenance are preserved.",
			Content:     item.Content,
			Placeholder: "Memory content",
			Save: func(content string, complete func(error) bool) error {
				content = strings.TrimSpace(content)
				if content == "" {
					return errors.New("memory content is empty")
				}
				if content == item.Content {
					a.message("agent memory unchanged · " + item.ID)
					complete(nil)
					return nil
				}
				return a.updateAgentMemory(target, agent.MemoryPatch{ID: item.ID, Content: &content}, "updating agent memory "+item.ID, complete)
			},
		})
	})
}

func (a *app) SetAgentMemoryPinned(argument string, pinned bool) error {
	verb := "pinning"
	if !pinned {
		verb = "unpinning"
	}
	return a.loadAgentMemoryItem(argument, verb+" agent memory", func(target agent.MemoryTarget, item agent.MemoryItem) {
		if item.Pinned == pinned {
			state := "unpinned"
			if pinned {
				state = "pinned"
			}
			a.message("agent memory is already " + state + " · " + item.ID)
			return
		}
		if err := a.updateAgentMemory(target, agent.MemoryPatch{ID: item.ID, Pinned: &pinned}, verb+" agent memory "+item.ID, nil); err != nil {
			a.message(err.Error())
		}
	})
}

func (a *app) PrepareAgentMemoryReview(argument string, approve bool) error {
	action, verb := "Reject", "rejecting"
	decision := agent.MemoryReject
	if approve {
		action, verb, decision = "Approve", "approving", agent.MemoryApprove
	}
	return a.loadAgentMemoryItem(argument, verb+" agent memory", func(target agent.MemoryTarget, item agent.MemoryItem) {
		if item.Status != agent.MemoryPending {
			a.message("only pending agent memory can be reviewed · " + item.ID)
			return
		}
		a.confirmAction(
			action+" agent memory",
			action+" pending item "+item.ID+"?",
			action,
			func() { a.reviewAgentMemory(target, item.ID, decision) },
		)
	})
}

func (a *app) PrepareDeleteAgentMemory(argument string) error {
	return a.loadAgentMemoryItem(argument, "loading agent memory to delete", func(target agent.MemoryTarget, item agent.MemoryItem) {
		a.confirmAction("Delete agent memory", "Delete item "+item.ID+" permanently?", "Delete permanently", func() {
			a.deleteAgentMemory(target, item.ID)
		})
	})
}

func (a *app) loadAgentMemoryItem(argument, label string, apply func(agent.MemoryTarget, agent.MemoryItem)) error {
	if a.agentMemory == nil {
		return errors.New("this runtime composition has no agent memory service")
	}
	target, identity, err := parseAgentMemoryIdentity(argument, a.session.Workspace.Path)
	if err != nil {
		return err
	}
	a.status.note(label)
	started := a.runOperation(agentMemoryOperation, false,
		func(ctx context.Context) (agent.MemoryItem, error) {
			items, err := a.agentMemory.Items(ctx, target)
			if err != nil {
				return agent.MemoryItem{}, err
			}
			return resolveAgentMemory(items, identity)
		},
		func(item agent.MemoryItem, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			apply(target, item)
		},
	)
	if !started {
		return errors.New("another agent memory operation is running")
	}
	return nil
}

func resolveAgentMemory(items []agent.MemoryItem, identity string) (agent.MemoryItem, error) {
	for _, item := range items {
		if item.ID == identity {
			return item, nil
		}
	}
	var matches []agent.MemoryItem
	for _, item := range items {
		if strings.HasPrefix(item.ID, identity) {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return agent.MemoryItem{}, errors.New("agent memory not found: " + identity)
	case 1:
		return matches[0], nil
	default:
		return agent.MemoryItem{}, errors.New("agent memory identity is ambiguous; use the full id")
	}
}

func parseAgentMemoryTarget(argument, workspace string) (agent.MemoryTarget, error) {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		argument = string(agent.MemoryProject)
	}
	scope, err := agent.ParseMemoryScope(argument)
	if err != nil {
		return agent.MemoryTarget{}, err
	}
	if scope == agent.MemoryUser {
		workspace = ""
	}
	return agent.NewMemoryTarget(scope, workspace)
}

func parseAgentMemoryIdentity(argument, workspace string) (agent.MemoryTarget, string, error) {
	fields := strings.Fields(argument)
	scope, identity := agent.MemoryProject, ""
	switch len(fields) {
	case 1:
		identity = fields[0]
	case 2:
		parsed, err := agent.ParseMemoryScope(fields[0])
		if err != nil {
			return agent.MemoryTarget{}, "", errors.New("usage: [project|user] <memory-id>")
		}
		scope, identity = parsed, fields[1]
	default:
		return agent.MemoryTarget{}, "", errors.New("usage: [project|user] <memory-id>")
	}
	if scope == agent.MemoryUser {
		workspace = ""
	}
	target, err := agent.NewMemoryTarget(scope, workspace)
	return target, identity, err
}

func (a *app) addAgentMemory(target agent.MemoryTarget, content string, complete func(error) bool) error {
	presentation := a.sessionContext
	a.status.note("adding agent memory")
	if !a.runAdmissionMutation(agentMemoryOperation, false,
		func(ctx context.Context) (agent.MemoryItem, error) { return a.agentMemory.Add(ctx, target, content) },
		func(item agent.MemoryItem, err error) {
			if err != nil {
				a.message("add agent memory failed: " + err.Error())
				if complete != nil {
					complete(err)
				}
				return
			}
			closed := true
			if complete != nil {
				closed = complete(nil)
			}
			a.message("agent memory added · " + item.ID)
			if closed && a.sessionContext.current(presentation) {
				a.showAgentMemory(target)
			}
		},
	) {
		return errors.New("another agent memory operation is running")
	}
	return nil
}

func (a *app) updateAgentMemory(target agent.MemoryTarget, patch agent.MemoryPatch, label string, complete func(error) bool) error {
	presentation := a.sessionContext
	a.status.note(label)
	if !a.runAdmissionMutation(agentMemoryOperation, false,
		func(ctx context.Context) (agent.MemoryItem, error) { return a.agentMemory.Update(ctx, patch) },
		func(item agent.MemoryItem, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				if complete != nil {
					complete(err)
				}
				return
			}
			closed := true
			if complete != nil {
				closed = complete(nil)
			}
			a.message("agent memory updated · " + item.ID)
			if closed && a.sessionContext.current(presentation) {
				a.showAgentMemory(target)
			}
		},
	) {
		return errors.New("another agent memory operation is running")
	}
	return nil
}

func (a *app) reviewAgentMemory(target agent.MemoryTarget, id string, decision agent.MemoryReviewDecision) {
	presentation := a.sessionContext
	label := string(decision) + " agent memory " + id
	a.status.note(label)
	if !a.runAdmissionMutation(agentMemoryOperation, false,
		func(ctx context.Context) (string, error) { return id, a.agentMemory.Review(ctx, id, decision) },
		func(reviewed string, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			outcome := "rejected"
			if decision == agent.MemoryApprove {
				outcome = "approved"
			}
			a.message("agent memory " + outcome + " · " + reviewed)
			if a.sessionContext.current(presentation) {
				a.showAgentMemory(target)
			}
		},
	) {
		a.message("another agent memory operation is running")
	}
}

func (a *app) deleteAgentMemory(target agent.MemoryTarget, id string) {
	presentation := a.sessionContext
	a.status.note("deleting agent memory " + id)
	if !a.runAdmissionMutation(agentMemoryOperation, false,
		func(ctx context.Context) (string, error) { return id, a.agentMemory.Delete(ctx, id) },
		func(deleted string, err error) {
			if err != nil {
				a.message("delete agent memory failed: " + err.Error())
				return
			}
			a.message("agent memory deleted · " + deleted)
			if a.sessionContext.current(presentation) {
				a.showAgentMemory(target)
			}
		},
	) {
		a.message("another agent memory operation is running")
	}
}
