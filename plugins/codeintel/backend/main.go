package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tangerg/flame/plugins/codeintel/internal/analyzer"
	"github.com/Tangerg/flame/plugins/codeintel/internal/lsp"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context) (err error) {
	var servers []lsp.ServerSpec
	if raw, configured := os.LookupEnv("LSP_SERVERS"); configured {
		if err := json.Unmarshal([]byte(raw), &servers, json.RejectUnknownMembers(true)); err != nil {
			return errors.New("code intelligence: LSP_SERVERS must be a valid server array")
		}
		if servers == nil {
			return errors.New("code intelligence: LSP_SERVERS must be an array")
		}
	}
	analysis, err := analyzer.New(ctx, servers)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, analysis.Close()) }()
	server, err := newServer(analysis)
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
