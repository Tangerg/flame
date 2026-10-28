package main

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func toolReceipt(ctx context.Context, db *sql.DB, input qimumcp.ReceiptRequest) (pluginsdk.ToolReceipt, error) {
	remote := input.Method
	switch remote {
	case createScheduleTool:
		input.Method = wire.Create
	case updateScheduleTool:
		input.Method = wire.Update
	case deleteScheduleTool:
		input.Method = wire.Delete
	case runScheduleTool:
		input.Method = wire.RunNow
	default:
		return pluginsdk.ToolReceipt{}, nil
	}
	receipt, err := pluginstorage.ReadReceipt(ctx, db, input)
	if err != nil || !receipt.Found {
		return pluginsdk.ToolReceipt{}, err
	}
	_, err = qimumcp.DecodeReply[jsontext.Value](receipt.Reply)
	if err != nil {
		if classify(err) == "" {
			return pluginsdk.ToolReceipt{}, err
		}
		failure, known := toolcontract.ClassifyFailure(commandFailure(remote, decodeError(err)))
		if !known {
			return pluginsdk.ToolReceipt{}, err
		}
		return pluginsdk.NewToolReceipt(failure.Output(), true)
	}
	var projection any
	switch remote {
	case createScheduleTool, updateScheduleTool:
		snapshot, err := qimumcp.DecodeReply[schedule.Snapshot](receipt.Reply)
		if err != nil {
			return pluginsdk.ToolReceipt{}, err
		}
		state, err := schedule.Restore(snapshot)
		if err != nil {
			return pluginsdk.ToolReceipt{}, err
		}
		projection = scheduleResponse{Schedule: viewSchedule(state)}
	case deleteScheduleTool:
		if _, err := qimumcp.DecodeReply[wire.Ack](receipt.Reply); err != nil {
			return pluginsdk.ToolReceipt{}, err
		}
		var original string
		if err := sqlite.Connection(ctx, db).QueryRowContext(ctx, `SELECT business_input_json FROM commands WHERE id=?`, input.ID).Scan(&original); err != nil {
			return pluginsdk.ToolReceipt{}, err
		}
		var arguments deleteScheduleArgs
		if err := json.Unmarshal([]byte(original), &arguments, json.RejectUnknownMembers(true)); err != nil {
			return pluginsdk.ToolReceipt{}, err
		}
		if err := schedule.ValidateID(arguments.ScheduleID); err != nil {
			return pluginsdk.ToolReceipt{}, err
		}
		projection = scheduleDeleteResponse(arguments)
	case runScheduleTool:
		reference, err := qimumcp.DecodeReply[app.RunReference](receipt.Reply)
		if err != nil {
			return pluginsdk.ToolReceipt{}, err
		}
		if err := errors.Join(identity.ValidateSession(reference.SessionID), identity.ValidateRun(reference.RunID)); err != nil {
			return pluginsdk.ToolReceipt{}, err
		}
		projection = reference
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return pluginsdk.ToolReceipt{}, err
	}
	output, err := chat.NewJSONToolOutput(encoded)
	if err != nil {
		return pluginsdk.ToolReceipt{}, err
	}
	return pluginsdk.NewToolReceipt(output, false)
}
