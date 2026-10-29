package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context) (err error) {
	data := os.Getenv("PLUGIN_DATA")
	if !filepath.IsAbs(data) {
		return errors.New("a2a: PLUGIN_DATA must be absolute")
	}
	var specs []agentSpec
	if raw, configured := os.LookupEnv("A2A_AGENTS"); configured {
		if len(raw) > 1<<20 {
			return errors.New("a2a: A2A_AGENTS exceeds the configuration limit")
		}
		if err := json.Unmarshal([]byte(raw), &specs, json.RejectUnknownMembers(true)); err != nil || specs == nil {
			return errors.New("a2a: A2A_AGENTS must be a valid agent configuration array")
		}
	}
	lease, err := filesystem.AcquireDirectoryLease(ctx, data)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()
	connector, err := openConnector(ctx, filepath.Join(data, "a2a.db"), specs)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, connector.db.Close()) }()
	server, err := newServer(connector)
	if err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
