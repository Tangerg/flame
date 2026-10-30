package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func newServer(db *sql.DB) (*sdk.Server, error) {
	if db == nil {
		return nil, errors.New("hooks: private command store is required")
	}
	contract, err := hookprovider.Contract()
	if err != nil {
		return nil, err
	}
	read, err := hookprovider.DiscoverMethod()
	if err != nil {
		return nil, err
	}
	execute, err := hookprovider.ExecuteMethod()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.hooks", Version: "1.0.0"}, nil)
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, db),
		read.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[hookprovider.Discovery]) ([]hook.Hook, error) {
			return discover(ctx, command.Input)
		}),
		execute.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[hookprovider.Command]) (hookprovider.Result, error) {
			return executeCommand(ctx, db, execute.Name(), command)
		}),
		backend.BindCapability("hook-provider", contract),
	); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}

func executeCommand(ctx context.Context, db *sql.DB, method string, command qimumcp.Command[hookprovider.Command]) (hookprovider.Result, error) {
	input, err := command.RequestInput()
	if err != nil {
		return hookprovider.Result{}, err
	}
	var request hookprovider.Command
	prepared, reply, err := pluginstorage.AdmitCommand(ctx, db, method, command.ID, input, func(context.Context) error {
		if err := json.Unmarshal(input.JSON(), &request, json.RejectUnknownMembers(true)); err != nil {
			return err
		}
		if err := request.Validate(); err != nil {
			return &portable.Refusal{Code: "invalid_input", Message: err.Error()}
		}
		return nil
	})
	if err != nil {
		return hookprovider.Result{}, err
	}
	if len(reply) != 0 {
		return qimumcp.DecodeReply[hookprovider.Result](reply)
	}
	if !prepared.NewlyAdmitted() {
		return hookprovider.Result{}, pluginstorage.ErrCommandUnconfirmed
	}
	result := (Shell{}).RunHookCommand(ctx, commandRequest{Command: request.Command, CWD: request.CWD, Input: request.Input, Timeout: time.Duration(request.TimeoutMillis) * time.Millisecond}).project()
	if err := result.Validate(); err != nil {
		return hookprovider.Result{}, err
	}
	encoded, err := qimumcp.EncodeReply(result, nil, nil)
	if err != nil {
		return hookprovider.Result{}, err
	}
	if err := prepared.Complete(context.WithoutCancel(ctx), encoded.JSON()); err != nil {
		return hookprovider.Result{}, err
	}
	return result, nil
}
