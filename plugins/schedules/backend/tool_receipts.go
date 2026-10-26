package main

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/schedules/internal/contract"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func toolReceipt(ctx context.Context, db *sql.DB, input pluginsdk.ReceiptRequest) (pluginsdk.ToolReceipt, error) {
	remote := input.Method
	if remote != createScheduleTool {
		return pluginsdk.ToolReceipt{}, nil
	}
	input.Method = rpc.Create
	receipt, err := pluginstorage.ReadReceipt(ctx, db, input)
	if err != nil || !receipt.Found {
		return pluginsdk.ToolReceipt{}, err
	}
	value, err := pluginsdk.DecodeReply[jsontext.Value](receipt.Reply)
	if err != nil {
		if classify(err) == "" {
			return pluginsdk.ToolReceipt{}, err
		}
		failure, known := errors.AsType[*toolcontract.Failure](commandFailure(remote, decodeError(err)))
		if !known {
			return pluginsdk.ToolReceipt{}, err
		}
		return pluginsdk.NewToolReceipt(failure.Output(), true)
	}
	var snapshot schedule.Snapshot
	if err := json.Unmarshal(value, &snapshot, json.RejectUnknownMembers(true)); err != nil {
		return pluginsdk.ToolReceipt{}, err
	}
	created, err := schedule.Restore(snapshot)
	if err != nil {
		return pluginsdk.ToolReceipt{}, err
	}
	output, err := createOutput(created)
	if err != nil {
		return pluginsdk.ToolReceipt{}, err
	}
	return pluginsdk.NewToolReceipt(output, false)
}
