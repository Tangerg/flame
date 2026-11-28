package main

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/acp"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/opencode/internal/wire"
	"github.com/google/uuid"
)

type turnHandle struct {
	id   string
	sent chan struct{}
	done chan struct{}
}

type sessionAction[P any] func(context.Context, P, string) (wire.Session, func(), error)

func localCommand[P any](c *connector, ctx context.Context, method string, command pluginmcp.Command[P], prepare sessionAction[P]) (wire.Session, error) {
	body, err := command.RequestInput()
	if err != nil {
		return wire.Session{}, err
	}
	c.actions.Lock()
	defer c.actions.Unlock()
	var after func()
	result, err := pluginstorage.Command(ctx, c.db, method, string(command.ID), body, classify, func(tx context.Context, input P) (pluginstorage.CommandPlan[wire.Session], error) {
		value, action, err := prepare(tx, input, string(command.ID))
		if err != nil {
			return pluginstorage.CommandPlan[wire.Session]{}, err
		}
		after = action
		return pluginstorage.NewCommandPlan(value, func(tx context.Context) error { return c.save(tx, value) })
	})
	if err == nil && after != nil {
		after()
	}
	return result, err
}

func (c *connector) prompt(ctx context.Context, in wire.PromptRequest, command string) (wire.Session, func(), error) {
	value, err := c.get(ctx, wire.SessionRequest{ID: in.ID})
	if err != nil {
		return value, nil, err
	}
	if strings.TrimSpace(in.Text) == "" || len(in.Text) > 64*1024 {
		return value, nil, refuse("enter a prompt up to 64 KiB")
	}
	prior := ""
	if value.Turn != nil {
		prior = value.Turn.ID
		if value.Turn.Status == "running" {
			return value, nil, refuse("the original prompt is still running")
		}
		if value.Turn.Status == "unconfirmed" && !in.Continue {
			return value, nil, refuse("acknowledge the unsettled turn before starting a new one")
		}
	}
	if in.PreviousTurn != prior {
		return value, nil, refuse("the native turn changed after it was read")
	}
	c.mu.Lock()
	_, bound := c.live(value)
	c.mu.Unlock()
	if bound == nil {
		return value, nil, refuse("reopen the original native session before prompting")
	}
	value.Turn = &wire.Turn{ID: command, Status: "running"}
	return value, func() {
		turn := &turnHandle{id: command, sent: make(chan struct{}), done: make(chan struct{})}
		c.mu.Lock()
		bound.turn = turn
		c.mu.Unlock()
		c.goLive(func() {
			response, err := bound.handle.Prompt(c.ctx, &acp.PromptParams{Prompt: []acp.ContentBlock{&acp.TextContent{Text: in.Text}}})
			c.reconcilePrompt(in.ID, turn.id, response, err)
			close(turn.done)
			c.mu.Lock()
			if bound.turn == turn {
				bound.turn = nil
			}
			c.mu.Unlock()
		})
	}, nil
}

func (c *connector) reconcilePrompt(id, turn string, response *acp.PromptResponse, cause error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.ctx), 5*time.Second)
	defer cancel()
	err := c.change(ctx, id, func(value *wire.Session) {
		if value.Turn == nil || value.Turn.ID != turn {
			return
		}
		if response == nil || cause != nil {
			value.Turn.Status = "unconfirmed"
			if cause != nil {
				value.Events = appendEvent(value.Events, wire.Event{Kind: "connection", Text: bounded(cause.Error(), 2048)})
			}
			return
		}
		value.Turn.Status, value.Turn.StopReason = "finished", string(response.StopReason)
		for i := range value.Permissions {
			item := &value.Permissions[i]
			if item.TurnID == turn && (item.Status == "requested" || item.Status == "submitted") {
				item.Status = "closed"
			}
		}
	})
	c.report(ctx, err)
}

func (c *connector) cancel(ctx context.Context, in wire.TurnRequest, _ string) (wire.Session, func(), error) {
	value, err := c.get(ctx, wire.SessionRequest{ID: in.ID})
	if err != nil {
		return value, nil, err
	}
	if value.Turn == nil || value.Turn.ID != in.TurnID || in.TurnID == "" {
		return value, nil, refuse("the native turn changed after it was read")
	}
	if value.Turn.CancelRequested || value.Turn.Status == "finished" {
		return value, nil, nil
	}
	value.Turn.CancelRequested = true
	c.mu.Lock()
	_, bound := c.live(value)
	var original *turnHandle
	if bound != nil && bound.turn != nil && bound.turn.id == in.TurnID {
		original = bound.turn
	}
	c.mu.Unlock()
	if original == nil {
		return value, nil, nil
	}
	return value, func() {
		deadline, stop := context.WithTimeout(c.ctx, 10*time.Second)
		defer stop()
		select {
		case <-original.sent:
			select {
			case <-original.done:
				return
			default:
			}
			c.report(deadline, bound.handle.Cancel(deadline, nil))
		case <-original.done:
		case <-deadline.Done():
		}
	}, nil
}

func (c *connector) answer(ctx context.Context, in wire.AnswerRequest, _ string) (wire.Session, func(), error) {
	value, err := c.get(ctx, wire.SessionRequest{ID: in.ID})
	if err != nil {
		return value, nil, err
	}
	index := slices.IndexFunc(value.Permissions, func(item wire.PermissionRequest) bool { return item.ID == in.PermissionID })
	if index < 0 || value.Turn == nil || value.Turn.CancelRequested || value.Permissions[index].Status != "requested" ||
		value.Turn.ID != value.Permissions[index].TurnID {
		return value, nil, refuse("the original permission is no longer answerable")
	}
	permission := &value.Permissions[index]
	if !slices.ContainsFunc(permission.Options, func(option wire.Option) bool { return option.ID == in.OptionID }) {
		return value, nil, refuse("choose an option offered by OpenCode")
	}
	c.mu.Lock()
	_, bound := c.live(value)
	var destination chan acp.RequestPermissionOutcome
	if bound != nil {
		destination = bound.permissions[in.PermissionID]
	}
	c.mu.Unlock()
	if destination == nil {
		return value, nil, refuse("the original permission connection is unavailable")
	}
	permission.Status, permission.Selected = "submitted", in.OptionID
	return value, func() {
		destination <- &acp.SelectedPermissionOutcome{OptionID: acp.PermissionOptionID(in.OptionID)}
	}, nil
}

func (c *connector) onPermission(ctx context.Context, connection *acp.ClientConn, request *acp.RequestPermissionRequest) (*acp.RequestPermissionResponse, error) {
	c.mu.Lock()
	bound := c.nativeBinding(connection, request.SessionID)
	if bound == nil || bound.turn == nil {
		c.mu.Unlock()
		return nil, errors.New("acp: permission arrived outside the attached prompt")
	}
	turn := bound.turn.id
	id := uuid.NewString()
	decision := make(chan acp.RequestPermissionOutcome, 1)
	bound.permissions[id] = decision
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(bound.permissions, id)
		c.mu.Unlock()
	}()
	title, _ := request.ToolCall.Title.Get()
	title = bounded(title, 2048)
	if title == "" {
		title = string(request.ToolCall.ToolCallID)
	}
	preview := ""
	if body, present := request.ToolCall.RawInput.Get(); present {
		preview = bounded(string(body), 8192)
	}
	options := make([]wire.Option, 0, len(request.Options))
	for _, option := range request.Options {
		options = append(options, wire.Option{ID: string(option.OptionID), Name: option.Name, Kind: string(option.Kind)})
	}
	req := wire.PermissionRequest{
		ID: id, TurnID: turn, ToolID: string(request.ToolCall.ToolCallID),
		Title: title, Preview: preview, Options: options, Status: "requested",
	}
	if err := c.change(ctx, bound.id, func(value *wire.Session) {
		value.Permissions = append(value.Permissions, req)
		if len(value.Permissions) > 64 {
			value.Permissions = slices.DeleteFunc(value.Permissions, func(item wire.PermissionRequest) bool {
				return item.TurnID != turn && item.Status == "closed"
			})
		}
	}); err != nil {
		return nil, err
	}
	select {
	case outcome := <-decision:
		return &acp.RequestPermissionResponse{Outcome: outcome}, nil
	case <-ctx.Done():
		return &acp.RequestPermissionResponse{Outcome: &acp.RequestPermissionOutcomeCancelled{}}, nil
	}
}

func (c *connector) onUpdate(ctx context.Context, connection *acp.ClientConn, notification *acp.SessionNotification) {
	event := wire.Event{}
	switch update := notification.Update.(type) {
	case *acp.UserMessageChunk:
		event.Kind = "user"
		event.Text = text(update.Content)
	case *acp.AgentMessageChunk:
		event.Kind = "agent"
		event.Text = text(update.Content)
	case *acp.ToolCall:
		event.Kind, event.ToolID, event.Text = "tool", string(update.ToolCallID), update.Title
		status, _ := update.Status.Get()
		event.Status = string(status)
	case *acp.ToolCallUpdate:
		event.Kind, event.ToolID = "tool", string(update.ToolCallID)
		event.Text, _ = update.Title.Get()
		status, _ := update.Status.Get()
		event.Status = string(status)
	default:
		return
	}
	event.Text = bounded(event.Text, 8192)
	if event.Text == "" && event.ToolID == "" {
		return
	}
	c.mu.Lock()
	bound := c.nativeBinding(connection, notification.SessionID)
	if bound == nil {
		c.mu.Unlock()
		return
	}
	if bound.handle == nil {
		bound.replay = appendEvent(bound.replay, event)
		c.mu.Unlock()
		return
	}
	turn := ""
	if bound.turn != nil {
		turn = bound.turn.id
	}
	c.mu.Unlock()
	c.report(ctx, c.change(ctx, bound.id, func(value *wire.Session) {
		value.Events = appendEvent(value.Events, event)
		if turn == "" || value.Turn == nil || value.Turn.ID != turn ||
			event.ToolID == "" || event.Status == "" || event.Status == "pending" {
			return
		}
		for i := range value.Permissions {
			permission := &value.Permissions[i]
			if permission.TurnID == turn && permission.ToolID == event.ToolID && permission.Status == "submitted" {
				permission.Status = "closed"
			}
		}
	}))
}

func (c *connector) report(ctx context.Context, err error) {
	if err != nil && c.ctx.Err() == nil {
		slog.WarnContext(ctx, "ACP observation could not be saved", "error", err)
	}
}

func appendEvent(events []wire.Event, event wire.Event) []wire.Event {
	if len(events) == 160 {
		events = events[1:]
	}
	return append(events, event)
}

func bounded(s string, size int) string {
	return strings.ToValidUTF8(s[:min(len(s), size)], "")
}

func text(block acp.ContentBlock) string {
	if value, ok := block.(*acp.TextContent); ok {
		return value.Text
	}
	return ""
}
