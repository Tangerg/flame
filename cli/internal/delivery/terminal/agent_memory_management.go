package terminal

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/application/installedcommand"
	"github.com/Tangerg/flame/cli/internal/application/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/domain/memory"
	"github.com/Tangerg/flame/runtime/protocol"
)

func (a *app) ShowAgentMemory(argument string) error {
	if a.agentMemory == nil {
		return errors.New("this runtime composition has no agent memory service")
	}
	target, err := parseAgentMemoryTarget(argument, a.session.current.Workspace.Path)
	if err != nil {
		return err
	}
	a.showAgentMemory(target)
	return nil
}

func (a *app) showAgentMemory(target conversation.MemoryTarget) {
	a.executeRuntimeReaderQuery(a.agentMemoryReaderQuery(target))
}

func (a *app) agentMemoryReaderQuery(target conversation.MemoryTarget) runtimeReaderQuery {
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

func agentMemoryDocument(target conversation.MemoryTarget, items []protocol.AgentMemoryItem) readerDocument {
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
			ToolSection{Title: "Provenance", Style: toolSectionCode, Text: strings.Join(metadata, "\n")},
		)
	}
	return readerDocument{Title: title, Detail: detail, Sections: sections}
}

func (a *app) AddAgentMemory(argument string) error {
	if a.agentMemory == nil {
		return errors.New("this runtime composition has no agent memory service")
	}
	target, err := parseAgentMemoryTarget(argument, a.session.current.Workspace.Path)
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
	return a.loadAgentMemoryItem(argument, "loading agent memory to edit", func(target conversation.MemoryTarget, item protocol.AgentMemoryItem) {
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
				return a.updateAgentMemory(target, protocol.AgentMemoryUpdateRequest{ID: item.ID, Content: &content}, "updating agent memory "+item.ID, complete)
			},
		})
	})
}

func (a *app) SetAgentMemoryPinned(argument string, pinned bool) error {
	verb := "pinning"
	if !pinned {
		verb = "unpinning"
	}
	return a.loadAgentMemoryItem(argument, verb+" agent memory", func(target conversation.MemoryTarget, item protocol.AgentMemoryItem) {
		if item.Pinned == pinned {
			state := "unpinned"
			if pinned {
				state = "pinned"
			}
			a.message("agent memory is already " + state + " · " + item.ID)
			return
		}
		if err := a.updateAgentMemory(target, protocol.AgentMemoryUpdateRequest{ID: item.ID, Pinned: &pinned}, verb+" agent memory "+item.ID, nil); err != nil {
			a.message(err.Error())
		}
	})
}

func (a *app) PrepareAgentMemoryReview(argument string, approve bool) error {
	if a.agentMemory == nil {
		return errors.New("this runtime composition has no agent memory service")
	}
	action, verb := "Reject", "rejecting"
	decision := memory.ReviewReject
	if approve {
		action, verb, decision = "Approve", "approving", memory.ReviewApprove
	}
	_, identity, err := parseAgentMemoryReference(argument)
	if err != nil {
		return err
	}
	if pending, found, err := a.pendingMemoryItemCommand(memory.ReviewV1Method, memoryReviewIntent(""), identity); err != nil {
		return err
	} else if found {
		id := strings.TrimPrefix(pending.Intent, memoryReviewIntent(""))
		if _, err := retainedMemoryReview(pending.Request, id, decision); err != nil {
			return err
		}
		a.reviewAgentMemory(nil, id, decision)
		return nil
	}
	return a.loadAgentMemoryItem(argument, verb+" agent memory", func(target conversation.MemoryTarget, item protocol.AgentMemoryItem) {
		if item.Status != protocol.AgentMemoryStatusPending {
			a.message("only pending agent memory can be reviewed · " + item.ID)
			return
		}
		a.confirmAction(
			action+" agent memory",
			action+" pending item "+item.ID+"?",
			action,
			func() { a.reviewAgentMemory(&target, item.ID, decision) },
		)
	})
}

func (a *app) PrepareDeleteAgentMemory(argument string) error {
	if a.agentMemory == nil {
		return errors.New("this runtime composition has no agent memory service")
	}
	_, identity, err := parseAgentMemoryReference(argument)
	if err != nil {
		return err
	}
	if pending, found, err := a.pendingMemoryItemCommand(memory.DeleteV1Method, memoryDeletionIntent(""), identity); err != nil {
		return err
	} else if found {
		a.deleteAgentMemory(nil, strings.TrimPrefix(pending.Intent, memoryDeletionIntent("")))
		return nil
	}
	return a.loadAgentMemoryItem(argument, "loading agent memory to delete", func(target conversation.MemoryTarget, item protocol.AgentMemoryItem) {
		a.confirmAction("Delete agent memory", "Delete item "+item.ID+" permanently?", "Delete permanently", func() {
			a.deleteAgentMemory(&target, item.ID)
		})
	})
}

func (a *app) pendingMemoryItemCommand(method, prefix, identity string) (workbench.PendingInstalledCommand, bool, error) {
	if a.workbench == nil {
		return workbench.PendingInstalledCommand{}, false, nil
	}
	var matches []workbench.PendingInstalledCommand
	for _, pending := range a.workbench.PendingInstalledCommands() {
		id, found := strings.CutPrefix(pending.Intent, prefix)
		if found && pending.Request.Method == method && strings.HasPrefix(id, identity) {
			matches = append(matches, pending)
		}
	}
	if len(matches) == 0 {
		return workbench.PendingInstalledCommand{}, false, nil
	}
	pending, err := resolveByID(matches, identity, "pending agent memory command", func(pending workbench.PendingInstalledCommand) string {
		return strings.TrimPrefix(pending.Intent, prefix)
	})
	return pending, err == nil, err
}

func retainedMemoryReview(request protocol.CallPluginRequest, id string, decision memory.ReviewDecision) (memory.ReviewV1Input, error) {
	var input memory.ReviewV1Input
	if err := json.Unmarshal(request.Input, &input, json.RejectUnknownMembers(true)); err != nil {
		return memory.ReviewV1Input{}, fmt.Errorf("pending agent memory review input is invalid: %w", err)
	}
	if input.ID != id || (input.Decision != string(memory.ReviewApprove) && input.Decision != string(memory.ReviewReject)) {
		return memory.ReviewV1Input{}, errors.New("pending agent memory review identity or decision is invalid")
	}
	if input.Decision != string(decision) {
		return input, fmt.Errorf("agent memory %s has an unconfirmed %s review; retry /memory-%s %s to reconcile the original command", id, input.Decision, input.Decision, id)
	}
	return input, nil
}

func (a *app) loadAgentMemoryItem(argument, label string, apply func(conversation.MemoryTarget, protocol.AgentMemoryItem)) error {
	if a.agentMemory == nil {
		return errors.New("this runtime composition has no agent memory service")
	}
	target, identity, err := parseAgentMemoryIdentity(argument, a.session.current.Workspace.Path)
	if err != nil {
		return err
	}
	a.status.note(label)
	started := a.runOperation(agentMemoryOperation, false,
		func(ctx context.Context) (protocol.AgentMemoryItem, error) {
			items, err := a.agentMemory.Items(ctx, target)
			if err != nil {
				return protocol.AgentMemoryItem{}, err
			}
			return resolveAgentMemory(items, identity)
		},
		func(item protocol.AgentMemoryItem, err error) {
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

func resolveAgentMemory(items []protocol.AgentMemoryItem, identity string) (protocol.AgentMemoryItem, error) {
	return resolveByID(items, identity, "agent memory", func(item protocol.AgentMemoryItem) string { return item.ID })
}

func parseAgentMemoryTarget(argument, workspace string) (conversation.MemoryTarget, error) {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		argument = string(protocol.AgentMemoryScopeProject)
	}
	scope, err := conversation.ParseMemoryScope(argument)
	if err != nil {
		return conversation.MemoryTarget{}, err
	}
	if scope == protocol.AgentMemoryScopeUser {
		workspace = ""
	}
	return conversation.NewMemoryTarget(scope, workspace)
}

func parseAgentMemoryIdentity(argument, workspace string) (conversation.MemoryTarget, string, error) {
	scope, identity, err := parseAgentMemoryReference(argument)
	if err != nil {
		return conversation.MemoryTarget{}, "", err
	}
	if scope == protocol.AgentMemoryScopeUser {
		workspace = ""
	}
	target, err := conversation.NewMemoryTarget(scope, workspace)
	return target, identity, err
}

func parseAgentMemoryReference(argument string) (protocol.AgentMemoryScope, string, error) {
	fields := strings.Fields(argument)
	scope, identity := protocol.AgentMemoryScopeProject, ""
	switch len(fields) {
	case 1:
		identity = fields[0]
	case 2:
		parsed, err := conversation.ParseMemoryScope(fields[0])
		if err != nil {
			return "", "", errors.New("usage: [project|user] <memory-id>")
		}
		scope, identity = parsed, fields[1]
	default:
		return "", "", errors.New("usage: [project|user] <memory-id>")
	}
	return scope, identity, nil
}

func (a *app) addAgentMemory(target conversation.MemoryTarget, content string, complete func(error) bool) error {
	presentation := a.session.context
	a.status.note("adding agent memory")
	if !a.runAdmissionMutation(agentMemoryOperation, false,
		func(ctx context.Context) (protocol.AgentMemoryItem, error) {
			return a.agentMemory.Add(ctx, target, content)
		},
		func(item protocol.AgentMemoryItem, err error) {
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
			if closed && a.session.context.current(presentation) {
				a.showAgentMemory(target)
			}
		},
	) {
		return errors.New("another agent memory operation is running")
	}
	return nil
}

func (a *app) updateAgentMemory(target conversation.MemoryTarget, request protocol.AgentMemoryUpdateRequest, label string, complete func(error) bool) error {
	presentation := a.session.context
	a.status.note(label)
	if !a.runAdmissionMutation(agentMemoryOperation, false,
		func(ctx context.Context) (protocol.AgentMemoryItem, error) { return a.agentMemory.Update(ctx, request) },
		func(item protocol.AgentMemoryItem, err error) {
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
			if closed && a.session.context.current(presentation) {
				a.showAgentMemory(target)
			}
		},
	) {
		return errors.New("another agent memory operation is running")
	}
	return nil
}

func (a *app) reviewAgentMemory(target *conversation.MemoryTarget, id string, decision memory.ReviewDecision) {
	presentation := a.session.context
	label := string(decision) + " agent memory " + id
	a.status.note(label)
	if !a.runApplicationOperation(agentMemoryOperation, false,
		func(ctx context.Context) (string, error) {
			if a.workbench != nil {
				if pending, found := a.workbench.PendingInstalledCommand(memoryReviewIntent(id)); found {
					if _, err := retainedMemoryReview(pending.Request, id, decision); err != nil {
						return "", err
					}
				}
			}
			_, err := installedcommand.Execute(ctx, a.agentMemory, a.workbench, memoryReviewIntent(id), a.replayPolicy, func(ctx context.Context) (protocol.CallPluginRequest, error) {
				return a.agentMemory.PrepareReview(ctx, id, decision)
			}, func(*protocol.CallPluginResponse) (memory.ReviewV1Output, error) { return memory.ReviewV1Output{}, nil })
			return id, err
		},
		func(reviewed string, err error) {
			if err != nil {
				message := label + " failed: " + err.Error()
				if a.workbench != nil {
					if pending, found := a.workbench.PendingInstalledCommand(memoryReviewIntent(id)); found {
						original, _ := retainedMemoryReview(pending.Request, id, decision)
						if original.Decision != "" {
							message += " · retry /memory-" + original.Decision + " " + id + " to reconcile the original command"
						}
					}
				}
				a.message(message)
				return
			}
			outcome := "rejected"
			if decision == memory.ReviewApprove {
				outcome = "approved"
			}
			a.message("agent memory " + outcome + " · " + reviewed)
			if target != nil && a.session.context.current(presentation) {
				a.showAgentMemory(*target)
			}
		},
	) {
		a.message("another agent memory operation is running")
	}
}

func (a *app) deleteAgentMemory(target *conversation.MemoryTarget, id string) {
	presentation := a.session.context
	a.status.note("deleting agent memory " + id)
	if !a.runApplicationOperation(agentMemoryOperation, false,
		func(ctx context.Context) (string, error) {
			_, err := installedcommand.Execute(ctx, a.agentMemory, a.workbench, memoryDeletionIntent(id), a.replayPolicy, func(ctx context.Context) (protocol.CallPluginRequest, error) {
				return a.agentMemory.PrepareDelete(ctx, id)
			}, func(*protocol.CallPluginResponse) (memory.DeleteV1Output, error) { return memory.DeleteV1Output{}, nil })
			return id, err
		},
		func(deleted string, err error) {
			if err != nil {
				message := "delete agent memory failed: " + err.Error()
				if a.workbench != nil {
					if _, pending := a.workbench.PendingInstalledCommand(memoryDeletionIntent(id)); pending {
						message += " · retry /memory-delete " + id + " to reconcile the original command"
					}
				}
				a.message(message)
				return
			}
			a.message("agent memory deleted · " + deleted)
			if target != nil && a.session.context.current(presentation) {
				a.showAgentMemory(*target)
			}
		},
	) {
		a.message("another agent memory operation is running")
	}
}

func memoryDeletionIntent(id string) string { return "memory-delete:" + id }
func memoryReviewIntent(id string) string   { return "memory-review:" + id }
