package main

import (
	"context"
	"database/sql"
	"errors"
	connectormcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcpconnector/mcp"
	"path/filepath"
	"time"

	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	receipts "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
	qimumcp "github.com/Tangerg/qimu/mcp"
)

func run(ctx context.Context, data string) (returned error) {
	db, err := sqlite.OpenSchema(ctx, filepath.Join(data, "external-servers.db"), []string{entriesSchema, attemptsSchema, receipts.CommandsSchema})
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, db.Close()) }()
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.mcp", Version: "2.0.0"}, nil)
	bound, err := qimumcp.NewBackend(server)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, bound.Close()) }()
	if err := register(bound, db); err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}
func register(bound *qimumcp.Backend, db *sql.DB) error {
	m, err := connectormcp.NewMethods()
	if err != nil {
		return err
	}
	c := catalog{db: db, now: time.Now}
	if err := m.List.RegisterServer(bound, nil, func(ctx context.Context, _ *sdk.ServerSession, cmd qimumcp.Command[wire.Selector]) (wire.Directory, error) {
		return c.list(ctx, cmd.Input)
	}); err != nil {
		return err
	}
	if err := receipts.RegisterCommand(bound, db, m.Recover, nil, c.recover); err != nil {
		return err
	}
	if err := receipts.RegisterCommand(bound, db, m.Change, nil, c.change); err != nil {
		return err
	}
	if err := receipts.RegisterCommand(bound, db, m.Start, nil, c.start); err != nil {
		return err
	}
	if err := receipts.RegisterCommand(bound, db, m.Finish, nil, c.finish); err != nil {
		return err
	}
	if err := m.Get.RegisterServer(bound, nil, func(ctx context.Context, _ *sdk.ServerSession, cmd qimumcp.Command[wire.Request]) (wire.AttemptResult, error) {
		return c.get(ctx, cmd.Input)
	}); err != nil {
		return err
	}
	if err := bound.BindCapability(wire.Export, m.Contract); err != nil {
		return err
	}
	return bound.Publish()
}
func main() { backend.Main("external MCP connector", run) }
