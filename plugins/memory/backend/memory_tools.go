package main

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	app "github.com/Tangerg/flame/plugins/memory/internal/application"
	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
	"github.com/Tangerg/scope/core/chat"
	tool "github.com/Tangerg/scope/core/tool"
	"github.com/invopop/jsonschema"
)

const (
	addMemoryTool    = "add_memory"
	updateMemoryTool = "update_memory"
	deleteMemoryTool = "delete_memory"
	reviewMemoryTool = "review_memory"
)

type memoryView struct {
	ID        string        `json:"id"`
	Revision  uint64        `json:"revision"`
	Scope     memory.Scope  `json:"scope"`
	Project   string        `json:"project,omitzero"`
	Content   string        `json:"content"`
	Origin    memory.Origin `json:"origin"`
	Status    memory.Status `json:"status"`
	Pinned    bool          `json:"pinned"`
	SessionID string        `json:"session_id,omitzero"`
	UpdatedAt time.Time     `json:"updated_at"`
}

func viewMemory(item memory.Item) memoryView {
	return memoryView{ID: item.ID().String(), Revision: item.Revision(), Scope: item.Scope(), Project: item.Project(),
		Content: item.Content(), Origin: item.Origin(), Status: item.Status(), Pinned: item.Pinned(),
		SessionID: item.SessionID(), UpdatedAt: item.UpdatedAt()}
}

type memoryResults struct {
	Memories []memoryView `json:"memories"`
}

func memorySearchResults(items []memory.Item) memoryResults {
	views := make([]memoryView, len(items))
	for index, item := range items {
		views[index] = viewMemory(item)
	}
	return memoryResults{Memories: views}
}

type listMemoryArgs struct {
	Scope  memory.Scope `json:"scope" jsonschema_description:"Project memory visible in the current workspace, or user memory shared across workspaces."`
	Cursor string       `json:"cursor,omitzero" jsonschema_description:"Opaque next_cursor returned by list_memories; omit for the first page."`
	Limit  *int         `json:"limit,omitzero" jsonschema:"minimum=1"`
}

func (listMemoryArgs) JSONSchemaExtend(schema *jsonschema.Schema) {
	if limit, ok := schema.Properties.Get("limit"); ok {
		limit.Maximum = jsonvalue.Number(strconv.Itoa(memory.MaxManagementPageItems))
	}
}

type memoryPage struct {
	Memories   []memoryView `json:"memories"`
	NextCursor string       `json:"next_cursor,omitzero"`
	Capacity   int          `json:"capacity"`
}

type addMemoryArgs struct {
	Scope   memory.Scope `json:"scope" jsonschema_description:"Use project for a fact about the current workspace, or user for a user preference shared across workspaces."`
	Content string       `json:"content" jsonschema:"minLength=1"`
}

type memoryItemArgs struct {
	Scope            memory.Scope `json:"scope"`
	ID               string       `json:"id" jsonschema:"minLength=1"`
	ExpectedRevision uint64       `json:"expected_revision" jsonschema:"minimum=1" jsonschema_description:"Exact revision returned by search_memory or list_memories. Reread after a revision conflict; never guess the current revision."`
}

func (memoryItemArgs) JSONSchemaExtend(schema *jsonschema.Schema) {
	if revision, ok := schema.Properties.Get("expected_revision"); ok {
		revision.Maximum = jsonvalue.Number(strconv.FormatUint(exactint.Maximum, 10))
	}
}

type updateMemoryArgs struct {
	memoryItemArgs
	Content *string `json:"content,omitzero" jsonschema:"minLength=1"`
	Pinned  *bool   `json:"pinned,omitzero"`
}

type reviewMemoryArgs struct {
	memoryItemArgs
	Decision memory.ReviewDecision `json:"decision"`
}

type memoryTools struct {
	coordinator *app.Coordinator
	db          *sql.DB
}

func buildMemoryTools(coordinator *app.Coordinator, db *sql.DB) ([]tool.Tool, error) {
	if coordinator == nil || db == nil {
		return nil, errors.New("memory: coordinator and database are required")
	}
	t := memoryTools{coordinator: coordinator, db: db}
	list, err := tool.NewFunc[listMemoryArgs, memoryPage](tool.FuncConfig{Name: "list_memories", Description: "List active and pending memories with their exact ids, scopes and revisions. Choose project for the current workspace or user for shared user memory. Follow next_cursor to continue."}, t.list)
	if err != nil {
		return nil, err
	}
	add, err := tool.NewFunc[addMemoryArgs, memoryView](tool.FuncConfig{Name: addMemoryTool, Description: "Store one durable fact the user explicitly asks to remember. Use project for this workspace or user for a cross-project user preference. Do not store secrets or speculative inferences. Explicitly authored memory becomes active immediately."}, t.add)
	if err != nil {
		return nil, err
	}
	update, err := tool.NewFunc[updateMemoryArgs, memoryView](tool.FuncConfig{Name: updateMemoryTool, Description: "Correct or pin an existing visible memory when the user asks. Supply its exact id, scope and observed revision from search_memory or list_memories. Omit fields to retain them. A revision conflict requires rereading the item before deciding on a new change."}, t.update)
	if err != nil {
		return nil, err
	}
	remove, err := tool.NewFunc[memoryItemArgs, rpc.Ack](tool.FuncConfig{Name: deleteMemoryTool, Description: "Permanently delete a visible memory the user asks to remove, using its exact id, scope and observed revision. Automatic extraction may propose the fact again for review; deletion does not erase conversation history."}, t.remove)
	if err != nil {
		return nil, err
	}
	review, err := tool.NewFunc[reviewMemoryArgs, rpc.Ack](tool.FuncConfig{Name: reviewMemoryTool, Description: "Approve or reject a pending automatically extracted memory when the user makes that review decision. Use its exact id, scope and observed revision from list_memories. Rejection suppresses the same proposal while it remains in recent rejection history."}, t.review)
	if err != nil {
		return nil, err
	}
	return []tool.Tool{list, add, update, remove, review}, nil
}

func memoryToolCWD(ctx context.Context) (string, error) {
	scope, err := pluginsdk.ToolContextFromContext(ctx)
	if err != nil {
		return "", errors.Join(app.ErrWorkspaceUnavailable, err)
	}
	return scope.CWD, nil
}

func (t memoryTools) list(ctx context.Context, input listMemoryArgs) (memoryPage, error) {
	cwd, err := memoryToolCWD(ctx)
	if err != nil {
		return memoryPage{}, pluginsdk.DefiniteToolFailure(err)
	}
	limit := pagination.DefaultLimit()
	if input.Limit != nil {
		limit, err = pagination.NewLimit(*input.Limit)
		if err != nil {
			return memoryPage{}, pluginsdk.DefiniteToolFailure(err)
		}
	}
	page, err := t.coordinator.List(ctx, app.ListInput{Scope: input.Scope, CWD: cwd, Cursor: input.Cursor, Limit: limit})
	if err != nil {
		return memoryPage{}, pluginsdk.DefiniteToolFailure(err)
	}
	return memoryPage{Memories: memorySearchResults(page.Rows).Memories, NextCursor: page.NextCursor, Capacity: memory.MaxVisiblePerTarget}, nil
}

func (t memoryTools) add(ctx context.Context, input addMemoryArgs) (memoryView, error) {
	return memoryToolCommand(ctx, t.db, addMemoryTool, input, func(ctx context.Context, input addMemoryArgs) (memoryView, error) {
		cwd, err := memoryToolCWD(ctx)
		if err != nil {
			return memoryView{}, err
		}
		item, err := t.coordinator.Add(ctx, input.Scope, cwd, input.Content)
		return viewMemory(item), err
	})
}

func (t memoryTools) authorize(ctx context.Context, input memoryItemArgs) error {
	cwd, err := memoryToolCWD(ctx)
	if err != nil {
		return err
	}
	_, err = t.coordinator.Item(ctx, input.Scope, cwd, input.ID)
	return err
}

func (t memoryTools) update(ctx context.Context, input updateMemoryArgs) (memoryView, error) {
	return memoryToolCommand(ctx, t.db, updateMemoryTool, input, func(ctx context.Context, input updateMemoryArgs) (memoryView, error) {
		if err := t.authorize(ctx, input.memoryItemArgs); err != nil {
			return memoryView{}, err
		}
		item, err := t.coordinator.Update(ctx, input.ID, input.ExpectedRevision, input.Content, input.Pinned)
		return viewMemory(item), err
	})
}

func (t memoryTools) remove(ctx context.Context, input memoryItemArgs) (rpc.Ack, error) {
	return memoryToolCommand(ctx, t.db, deleteMemoryTool, input, func(ctx context.Context, input memoryItemArgs) (rpc.Ack, error) {
		if err := t.authorize(ctx, input); err != nil {
			return rpc.Ack{}, err
		}
		return rpc.Ack{}, t.coordinator.Delete(ctx, input.ID, input.ExpectedRevision)
	})
}

func (t memoryTools) review(ctx context.Context, input reviewMemoryArgs) (rpc.Ack, error) {
	return memoryToolCommand(ctx, t.db, reviewMemoryTool, input, func(ctx context.Context, input reviewMemoryArgs) (rpc.Ack, error) {
		if err := t.authorize(ctx, input.memoryItemArgs); err != nil {
			return rpc.Ack{}, err
		}
		return rpc.Ack{}, t.coordinator.Review(ctx, input.ID, input.ExpectedRevision, input.Decision)
	})
}

func memoryToolCommand[P, R any](ctx context.Context, db *sql.DB, name string, input P, apply func(context.Context, P) (R, error)) (R, error) {
	var zero R
	id, err := pluginsdk.ToolCommandID(ctx)
	if err != nil {
		return zero, pluginsdk.DefiniteToolFailure(err)
	}
	admitted, err := pluginsdk.RequestInput(ctx)
	if err != nil {
		return zero, pluginsdk.DefiniteToolFailure(err)
	}
	value, err := pluginstorage.Command(ctx, db, name, id, input, admitted, classify, apply)
	return value, memoryToolFailure(name, err)
}

func memoryToolFailure(name string, err error) error {
	if err == nil {
		return nil
	}
	wrapped := fmt.Errorf("%s: %w", name, err)
	if _, known := errors.AsType[*extension.Refusal](err); known {
		return pluginsdk.DefiniteToolFailure(wrapped)
	}
	return wrapped
}

func memoryToolReceipt(ctx context.Context, db *sql.DB, input pluginsdk.ReceiptRequest) (pluginsdk.ToolReceipt, error) {
	switch input.Method {
	case addMemoryTool, updateMemoryTool, deleteMemoryTool, reviewMemoryTool:
	default:
		return pluginsdk.ToolReceipt{}, nil
	}
	receipt, err := pluginstorage.ReadReceipt(ctx, db, input)
	if err != nil || !receipt.Found {
		return pluginsdk.ToolReceipt{}, err
	}
	value, err := pluginsdk.DecodeReply[jsontext.Value](receipt.Reply)
	if err != nil {
		failure, known := errors.AsType[*tool.Failure](memoryToolFailure(input.Method, err))
		if !known {
			return pluginsdk.ToolReceipt{}, err
		}
		return pluginsdk.NewToolReceipt(failure.Output(), true)
	}
	output, err := chat.NewJSONToolOutput(value)
	if err != nil {
		return pluginsdk.ToolReceipt{}, err
	}
	return pluginsdk.NewToolReceipt(output, false)
}
