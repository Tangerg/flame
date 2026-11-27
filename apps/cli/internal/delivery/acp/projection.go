package acp

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/Tangerg/acp"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type projection struct {
	ctx     context.Context
	session *acp.AgentSession
	root    string
	text    map[string]*sentText
	tools   map[acp.ToolCallID]acp.ToolCallStatus
}

type sentText struct {
	body     strings.Builder
	finished bool
}

func newProjection(ctx context.Context, session *acp.AgentSession) *projection {
	return &projection{
		ctx: ctx, session: session, text: make(map[string]*sentText),
		tools: make(map[acp.ToolCallID]acp.ToolCallStatus),
	}
}

func (p *projection) Begin(run conversation.Run, _ authoring.RunOptions) error {
	p.root = run.ID
	return nil
}

func (*projection) Close() error { return nil }

func (p *projection) Render(envelope conversation.RunEvent) error {
	switch event := envelope.Event.(type) {
	case conversation.BlockStarted:
		return p.block(event.Block, envelope.RunID == p.root, false)
	case conversation.BlockCompleted:
		return p.block(event.Block, envelope.RunID == p.root, false)
	case conversation.BlockDelta:
		key := conversation.BlockKey(envelope.RunID, event.BlockID)
		if text := p.text[key]; text != nil && !text.finished {
			return p.appendText(key, text, event.Text)
		}
		return p.toolOutputStarted(acp.ToolCallID(key))
	case conversation.PlanChanged:
		return p.plan(&event.Plan)
	default:
		return nil
	}
}

func (p *projection) Reconcile(snapshot conversation.SessionSnapshot) error {
	runIDs := make(map[string]bool)
	for _, run := range snapshot.Runs {
		if run.ID == p.root || run.Lineage.RootRunID() == p.root {
			runIDs[run.ID] = true
		}
	}
	for _, block := range snapshot.Transcript {
		if runIDs[block.RunID] {
			if err := p.block(block, block.RunID == p.root, false); err != nil {
				return err
			}
		}
	}
	return p.plan(snapshot.Plan)
}

func (p *projection) replay(snapshot conversation.SessionSnapshot) error {
	root := make(map[string]bool)
	for _, run := range snapshot.Runs {
		root[run.ID] = run.Lineage.IsRoot()
	}
	for _, block := range snapshot.Transcript {
		if block.Status == conversation.BlockStatusRunning {
			continue
		}
		if err := p.block(block, block.RunID == "" || root[block.RunID], true); err != nil {
			return err
		}
	}
	return p.plan(snapshot.Plan)
}

func (p *projection) block(block conversation.Block, root, history bool) error {
	key := conversation.BlockKey(block.RunID, block.ID)
	if block.Tool != nil {
		return p.publishTool(acp.ToolCallID(key), block.Tool, toolStatus(block.Tool))
	}
	if !root {
		return nil
	}
	switch block.Kind {
	case conversation.BlockAssistant:
		return p.assistant(key, block)
	case conversation.BlockReasoning:
		if !block.Redacted && block.Status != conversation.BlockStatusRunning && block.Text != "" {
			return p.completedText(key, block)
		}
	case conversation.BlockUser:
		if history {
			return p.completedText(key, block)
		}
	}
	return nil
}

func (p *projection) assistant(key string, block conversation.Block) error {
	text := p.text[key]
	if text == nil {
		text = &sentText{}
		p.text[key] = text
	}
	if text.finished {
		return nil
	}
	sent := text.body.String()
	switch {
	case strings.HasPrefix(block.Text, sent):
		if err := p.appendText(key, text, block.Text[len(sent):]); err != nil {
			return err
		}
	case block.Status == conversation.BlockStatusRunning && strings.HasPrefix(sent, block.Text):
		// A recovered snapshot can omit previews already emitted to the editor.
	default:
		return fmt.Errorf("runtime item %s changed text already sent to the ACP client", block.ID)
	}
	if block.Status != conversation.BlockStatusRunning {
		if err := p.images(key, block); err != nil {
			return err
		}
		text.finished = true
		text.body.Reset()
	}
	return nil
}

func (p *projection) completedText(key string, block conversation.Block) error {
	if _, sent := p.text[key]; sent {
		return nil
	}
	if block.Text != "" {
		if err := p.chunk(block.Kind, key, &acp.TextContent{Text: block.Text}); err != nil {
			return err
		}
	}
	if err := p.images(key, block); err != nil {
		return err
	}
	p.text[key] = &sentText{finished: true}
	return nil
}

func (p *projection) images(key string, block conversation.Block) error {
	for _, image := range block.Images {
		content := &acp.ImageContent{Data: base64.StdEncoding.EncodeToString(image.Data), MimeType: image.MIMEType}
		if err := p.chunk(block.Kind, key, content); err != nil {
			return err
		}
	}
	return nil
}

func (p *projection) appendText(key string, text *sentText, delta string) error {
	if delta == "" {
		return nil
	}
	if err := p.chunk(conversation.BlockAssistant, key, &acp.TextContent{Text: delta}); err != nil {
		return err
	}
	text.body.WriteString(delta)
	return nil
}

func (p *projection) chunk(kind conversation.BlockKind, key string, content acp.ContentBlock) error {
	chunk := acp.ContentChunk{MessageID: acp.OptValue(acp.MessageID(key)), Content: content}
	switch kind {
	case conversation.BlockUser:
		return p.update(&acp.UserMessageChunk{ContentChunk: chunk})
	case conversation.BlockReasoning:
		return p.update(&acp.AgentThoughtChunk{ContentChunk: chunk})
	default:
		return p.update(&acp.AgentMessageChunk{ContentChunk: chunk})
	}
}

func (p *projection) plan(plan *protocol.Plan) error {
	if plan == nil || plan.State == nil {
		return nil
	}
	entries := make([]acp.PlanEntry, 0, len(plan.State.Steps))
	for _, step := range plan.State.Steps {
		status := acp.PlanEntryStatusPending
		switch step.Status {
		case protocol.PlanStatusInProgress:
			status = acp.PlanEntryStatusInProgress
		case protocol.PlanStatusCompleted:
			status = acp.PlanEntryStatusCompleted
		}
		entries = append(entries, acp.PlanEntry{Content: step.Description, Priority: acp.PlanEntryPriorityMedium, Status: status})
	}
	return p.update(&acp.Plan{Entries: entries})
}

func (p *projection) update(update acp.SessionUpdate) error {
	return p.session.Update(p.ctx, &acp.SessionUpdateParams{Update: update})
}
