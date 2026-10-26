package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (err error) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := openStore(ctx, os.Getenv("PLUGIN_DATA"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	server, err := newServer(store)
	if err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}
