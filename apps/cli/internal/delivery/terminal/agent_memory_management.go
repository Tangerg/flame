package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/memory"
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

func agentMemoryDocument(target conversation.MemoryTarget, items []memory.Item) readerDocument {
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
	if a.workbench != nil {
		input, found, err := a.workbench.PendingMemoryAddition()
		if err != nil {
			return err
		}
		if found {
			a.message("reconciling original " + input.Scope + " memory addition")
			return a.addAgentMemory(nil, input, nil)
		}
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
			return a.addAgentMemory(&target, memory.AddV1Input{Scope: string(target.Scope), CWD: target.Workspace, Content: content}, complete)
		},
	})
	return nil
}

func (a *app) EditAgentMemory(argument string) error {
	if resumed, err := a.resumeMemoryUpdate(argument); resumed || err != nil {
		return err
	}
	return a.loadAgentMemoryItem(argument, "loading agent memory to edit", func(target conversation.MemoryTarget, item memory.Item) {
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
				pending := false
				if a.workbench != nil {
					var err error
					_, pending, err = a.workbench.PendingMemoryUpdate(item.ID)
					if err != nil {
						return err
					}
				}
				if content == item.Content && !pending {
					a.message("agent memory unchanged · " + item.ID)
					complete(nil)
					return nil
				}
				return a.updateAgentMemory(&target, memory.UpdateV1Input{ID: item.ID, Content: &content}, "updating agent memory "+item.ID, complete)
			},
		})
	})
}

func (a *app) SetAgentMemoryPinned(argument string, pinned bool) error {
	if resumed, err := a.resumeMemoryUpdate(argument); resumed || err != nil {
		return err
	}
	verb := "pinning"
	if !pinned {
		verb = "unpinning"
	}
	return a.loadAgentMemoryItem(argument, verb+" agent memory", func(target conversation.MemoryTarget, item memory.Item) {
		if item.Pinned == pinned {
			state := "unpinned"
			if pinned {
				state = "pinned"
			}
			a.message("agent memory is already " + state + " · " + item.ID)
			return
		}
		if err := a.updateAgentMemory(&target, memory.UpdateV1Input{ID: item.ID, Pinned: &pinned}, verb+" agent memory "+item.ID, nil); err != nil {
			a.message(err.Error())
		}
	})
}

func (a *app) resumeMemoryUpdate(argument string) (bool, error) {
	if a.agentMemory == nil {
		return false, errors.New("this runtime composition has no agent memory service")
	}
	_, identity, err := parseAgentMemoryReference(argument)
	if err != nil {
		return false, err
	}
	if a.workbench == nil {
		return false, nil
	}
	input, found, err := a.workbench.PendingMemoryUpdate(identity)
	if err != nil || !found {
		return false, err
	}

	a.message("reconciling original agent memory update · " + input.ID)
	return true, a.updateAgentMemory(nil, input, "reconciling original agent memory update "+input.ID, nil)
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
	if a.workbench != nil {
		input, found, err := a.workbench.PendingMemoryReview(identity, decision)
		if err != nil {
			if found && input.Decision != "" {
				return fmt.Errorf("%w · retry /memory-%s %s to reconcile the original command", err, input.Decision, input.ID)
			}
			return err
		}
		if found {
			a.reviewAgentMemory(nil, input.ID, decision)
			return nil
		}
	}
	return a.loadAgentMemoryItem(argument, verb+" agent memory", func(target conversation.MemoryTarget, item memory.Item) {
		if item.Status != memory.StatusPending {
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
	if a.workbench != nil {
		input, found, err := a.workbench.PendingMemoryDeletion(identity)
		if err != nil {
			return err
		}
		if found {
			a.deleteAgentMemory(nil, input.ID)
			return nil
		}
	}
	return a.loadAgentMemoryItem(argument, "loading agent memory to delete", func(target conversation.MemoryTarget, item memory.Item) {
		a.confirmAction("Delete agent memory", "Delete item "+item.ID+" permanently?", "Delete permanently", func() {
			a.deleteAgentMemory(&target, item.ID)
		})
	})
}

func (a *app) loadAgentMemoryItem(argument, label string, apply func(conversation.MemoryTarget, memory.Item)) error {
	if a.agentMemory == nil {
		return errors.New("this runtime composition has no agent memory service")
	}
	target, identity, err := parseAgentMemoryIdentity(argument, a.session.current.Workspace.Path)
	if err != nil {
		return err
	}
	a.status.note(label)
	started := a.runOperation(agentMemoryOperation, false,
		func(ctx context.Context) (memory.Item, error) {
			items, err := a.agentMemory.Items(ctx, target)
			if err != nil {
				return memory.Item{}, err
			}
			return resolveAgentMemory(items, identity)
		},
		func(item memory.Item, err error) {
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

func resolveAgentMemory(items []memory.Item, identity string) (memory.Item, error) {
	return resolveByID(items, identity, "agent memory", func(item memory.Item) string { return item.ID })
}

func parseAgentMemoryTarget(argument, workspace string) (conversation.MemoryTarget, error) {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		argument = string(memory.ScopeProject)
	}
	scope, err := conversation.ParseMemoryScope(argument)
	if err != nil {
		return conversation.MemoryTarget{}, err
	}
	if scope == memory.ScopeUser {
		workspace = ""
	}
	return conversation.NewMemoryTarget(scope, workspace)
}

func parseAgentMemoryIdentity(argument, workspace string) (conversation.MemoryTarget, string, error) {
	scope, identity, err := parseAgentMemoryReference(argument)
	if err != nil {
		return conversation.MemoryTarget{}, "", err
	}
	if scope == memory.ScopeUser {
		workspace = ""
	}
	target, err := conversation.NewMemoryTarget(scope, workspace)
	return target, identity, err
}

func parseAgentMemoryReference(argument string) (memory.Scope, string, error) {
	fields := strings.Fields(argument)
	scope, identity := memory.ScopeProject, ""
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

func (a *app) addAgentMemory(target *conversation.MemoryTarget, request memory.AddV1Input, complete func(error) bool) error {
	presentation := a.session.context
	a.status.note("adding agent memory")
	if !a.runApplicationOperation(agentMemoryOperation, false,
		func(ctx context.Context) (memory.AddV1Output, error) {
			return workbench.ExecuteMemoryAddition(ctx, a.agentMemory, a.workbench, a.replayPolicy, request)
		},
		func(result memory.AddV1Output, err error) {
			if err != nil {
				message := "add agent memory failed: " + err.Error()
				if a.workbench != nil {
					if _, pending, _ := a.workbench.PendingMemoryAddition(); pending {
						message += " · retry /memory-add to reconcile the original command"
					}
				}
				a.message(message)
				if complete != nil {
					complete(err)
				}
				return
			}
			closed := true
			if complete != nil {
				closed = complete(nil)
			}
			outcome := "added"
			if !result.Changed {
				outcome = "already present"
			}
			a.message("agent memory " + outcome + " · " + string(result.Item.ID))
			if closed && target != nil && a.session.context.current(presentation) {
				a.showAgentMemory(*target)
			}
		},
	) {
		return errors.New("another agent memory operation is running")
	}
	return nil
}

func (a *app) updateAgentMemory(target *conversation.MemoryTarget, request memory.UpdateV1Input, label string, complete func(error) bool) error {
	if request.Content != nil {
		request.Content = new(*request.Content)
	}
	if request.Pinned != nil {
		request.Pinned = new(*request.Pinned)
	}
	presentation := a.session.context
	a.status.note(label)
	if !a.runApplicationOperation(agentMemoryOperation, false,
		func(ctx context.Context) (memory.Item, error) {
			return workbench.ExecuteMemoryUpdate(ctx, a.agentMemory, a.workbench, a.replayPolicy, request)
		},
		func(item memory.Item, err error) {
			if err != nil {
				message := label + " failed: " + err.Error()
				if a.workbench != nil {
					if _, pending, _ := a.workbench.PendingMemoryUpdate(request.ID); pending {
						message += " · retry /memory-edit " + request.ID + " to reconcile the original command"
					}
				}
				a.message(message)
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
			if closed && target != nil && a.session.context.current(presentation) {
				a.showAgentMemory(*target)
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
			err := workbench.ExecuteMemoryReview(ctx, a.agentMemory, a.workbench, a.replayPolicy, id, decision)
			return id, err
		},
		func(reviewed string, err error) {
			if err != nil {
				message := label + " failed: " + err.Error()
				if a.workbench != nil {
					if original, found, _ := a.workbench.PendingMemoryReview(id, decision); found {
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
			err := workbench.ExecuteMemoryDeletion(ctx, a.agentMemory, a.workbench, a.replayPolicy, id)
			return id, err
		},
		func(deleted string, err error) {
			if err != nil {
				message := "delete agent memory failed: " + err.Error()
				if a.workbench != nil {
					if _, pending, _ := a.workbench.PendingMemoryDeletion(id); pending {
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
