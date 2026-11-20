package main

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/wire"

	framesqlite "github.com/Tangerg/frame/sqlite"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func toolReceipt(ctx context.Context, db framesqlite.Database, input pluginmcp.ReceiptRequest) (pluginsdk.ToolReceipt, error) {
	remote := input.Method
	method, owned := toolCommandMethod(remote)
	if !owned {
		return pluginsdk.ToolReceipt{}, nil
	}
	input.Method = method
	receipt, err := pluginstorage.ReadReceipt(ctx, db, input)
	if err != nil || !receipt.Found {
		return pluginsdk.ToolReceipt{}, err
	}
	if _, err := pluginmcp.DecodeReply[jsontext.Value](receipt.Reply); err != nil {
		failure, known := toolcontract.ClassifyFailure(commandFailure(remote, decodeError(err)))
		if !known {
			return pluginsdk.ToolReceipt{}, err
		}
		return pluginsdk.NewFailedToolReceipt(failure)
	}
	projection, err := toolReceiptProjection(ctx, db, remote, input.ID, receipt.Reply)
	if err != nil {
		return pluginsdk.ToolReceipt{}, err
	}
	return pluginsdk.NewJSONToolReceipt(projection)
}

// toolCommandMethod names the private command each Tool records its receipt
// under; other Tools keep no receipt here.
func toolCommandMethod(tool string) (string, bool) {
	switch tool {
	case createScheduleTool:
		return wire.Create, true
	case updateScheduleTool:
		return wire.Update, true
	case deleteScheduleTool:
		return wire.Delete, true
	case runScheduleTool:
		return wire.RunNow, true
	default:
		return "", false
	}
}

func toolReceiptProjection(ctx context.Context, db framesqlite.Database, tool string, commandID pluginmcp.CommandID, reply jsontext.Value) (any, error) {
	switch tool {
	case createScheduleTool, updateScheduleTool:
		snapshot, err := pluginmcp.DecodeReply[schedule.Snapshot](reply)
		if err != nil {
			return nil, err
		}
		state, err := schedule.Restore(snapshot)
		if err != nil {
			return nil, err
		}
		return scheduleResponse{Schedule: viewSchedule(state)}, nil
	case deleteScheduleTool:
		if _, err := pluginmcp.DecodeReply[wire.Ack](reply); err != nil {
			return nil, err
		}
		return deletedScheduleProjection(ctx, db, commandID)
	case runScheduleTool:
		reference, err := pluginmcp.DecodeReply[app.RunReference](reply)
		if err != nil {
			return nil, err
		}
		if err := errors.Join(identity.ValidateSession(reference.SessionID), identity.ValidateRun(reference.RunID)); err != nil {
			return nil, err
		}
		return reference, nil
	default:
		return nil, fmt.Errorf("schedules: Tool %q keeps no receipt", tool)
	}
}

// deletedScheduleProjection answers from the original arguments, since a
// deleted Schedule has nothing left to read.
func deletedScheduleProjection(ctx context.Context, db framesqlite.Database, commandID pluginmcp.CommandID) (any, error) {
	var original string
	if err := db.Read(ctx, func(reader framesqlite.Reader) error {
		return reader.QueryRowContext(ctx, `SELECT business_input_json FROM commands WHERE id=?`, commandID).Scan(&original)
	}); err != nil {
		return nil, err
	}
	var arguments deleteScheduleArgs
	if err := json.Unmarshal([]byte(original), &arguments, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	if err := schedule.ValidateID(arguments.ScheduleID); err != nil {
		return nil, err
	}
	return scheduleDeleteResponse(arguments), nil
}
