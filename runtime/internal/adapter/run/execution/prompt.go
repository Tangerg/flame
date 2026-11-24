package execution

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/planpresentation"
	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	plandomain "github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	corechat "github.com/Tangerg/scope/core/chat"
)

const agentMemoryInjectBudget = 4096

const basePrompt = `You are Flame, a general-purpose AI coding agent.

You can read and modify files, run shell commands, search the
codebase, and (when configured) fetch web content. Use the
available tools to accomplish the user's task; explain your
reasoning briefly when it isn't obvious. Prefer concrete actions
over hypotheticals.

When you change files, show the change. When a tool returns an
error, read the message and adjust — don't blindly retry. If a
task is ambiguous, ask one focused question rather than guess.`

// MemoryTarget is the Memory project target of a Run: its Project when the
// Session has one, otherwise its workspace directory, so directory memories never
// reach a Project implicitly.
func MemoryTarget(project identity.ProjectID, workspaceCWD string) string {
	if !project.IsZero() {
		return project.String()
	}
	if strings.TrimSpace(workspaceCWD) == "" {
		return ""
	}
	return filepath.Clean(workspaceCWD)
}

func (w *WorkingContextComposer) composeSystemMessage(
	ctx context.Context,
	executionCWD, workspaceCWD string,
	projectID identity.ProjectID,
) (corechat.Message, error) {
	var prompt promptComposition
	prompt.append(
		basePrompt,
		contextSourceBasePrompt.source("builtin:flame"),
	)
	if !projectID.IsZero() {
		current, err := w.config.Projects.Get(ctx, projectID)
		if err != nil {
			return corechat.Message{}, fmt.Errorf("execution: load Project instructions: %w", err)
		}
		if instructions := current.Instructions(); instructions != "" {
			prompt.append(
				"## Project instructions: "+current.Name()+"\n\n"+instructions,
				contextSourceProjectInstructions.source(projectID.String()+"@"+strconv.FormatUint(current.Revision(), 10)),
			)
		}
	}

	var pinned []MemoryPromptItem
	if target := MemoryTarget(projectID, workspaceCWD); target != "" {
		items, err := w.config.AgentMemory.Items(ctx, memorycontract.ItemsInputScopeProject, target)
		if err != nil {
			return corechat.Message{}, fmt.Errorf("execution: load project pinned memory: %w", err)
		}
		pinned = append(pinned, items...)
	}
	userItems, err := w.config.AgentMemory.Items(ctx, memorycontract.ItemsInputScopeUser, "")
	if err != nil {
		return corechat.Message{}, fmt.Errorf("execution: load user pinned memory: %w", err)
	}
	pinned = append(pinned, userItems...)
	newPinnedMemoryPrompt(pinned, agentMemoryInjectBudget).appendTo(&prompt)

	if dir := strings.TrimSpace(executionCWD); dir != "" {
		files, err := promptsource.AgentDocs{}.Find(ctx, dir, w.config.UserHome)
		if err != nil {
			return corechat.Message{}, fmt.Errorf("execution: load agent documents: %w", err)
		}
		documents, err := newAgentDocumentsPrompt(files, agentDocPromptMaxBytes)
		if err != nil {
			return corechat.Message{}, fmt.Errorf("execution: render agent documents: %w", err)
		}
		documents.appendTo(&prompt)
	}

	return prompt.systemMessage()
}

func (w *WorkingContextComposer) CurrentSessionState(
	ctx context.Context,
	sessionID identity.SessionID,
) ([]corechat.Message, error) {
	messages := make([]corechat.Message, 0, 2)
	if !sessionID.IsZero() {
		current, found, err := w.config.Goal.Current(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("execution: load current Session Goal: %w", err)
		}
		if found {
			var prompt promptComposition
			prompt.append(
				"## Current Autonomous Goal\n\nObjective: "+strconv.Quote(current.Objective())+
					"\nStatus: "+string(current.Status())+
					"\n\nThis snapshot is current for this model call. Use get_goal for full usage and reason details.",
				contextSourceSessionGoal.source(sessionID.String()),
			)
			message, err := prompt.runtimeContextMessage(RuntimeContextSessionGoal)
			if err != nil {
				return nil, err
			}
			messages = append(messages, message)
		}
	}
	currentPlan, found, err := w.currentSessionPlan(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if found {
		messages = append(messages, currentPlan)
	}
	return messages, nil
}

func (w *WorkingContextComposer) currentSessionPlan(
	ctx context.Context,
	sessionID identity.SessionID,
) (corechat.Message, bool, error) {
	if sessionID.IsZero() {
		return corechat.Message{}, false, nil
	}
	steps, listErr := w.config.Plan.List(ctx, sessionID)
	if listErr != nil {
		return corechat.Message{}, false, fmt.Errorf("execution: load current Session Plan: %w", listErr)
	}
	if validationErr := plandomain.ValidateSteps(steps); validationErr != nil {
		return corechat.Message{}, false, fmt.Errorf("execution: current Session Plan is invalid: %w", validationErr)
	}
	if len(steps) == 0 {
		return corechat.Message{}, false, nil
	}
	var prompt promptComposition
	prompt.append(
		"## Current Plan\n\n"+planpresentation.Render(steps),
		contextSourceSessionPlan.source(sessionID.String()),
	)
	message, err := prompt.runtimeContextMessage(RuntimeContextSessionPlan)
	if err != nil {
		return corechat.Message{}, false, err
	}
	return message, true, nil
}
