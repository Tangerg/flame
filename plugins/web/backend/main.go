package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	capture "github.com/Tangerg/flame/plugins/web/backend/domain"
	"github.com/Tangerg/flame/runtime/storage/advisorylock"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func configuredKey(name string) (capture.APIKey, error) {
	value, present := os.LookupEnv(name)
	if !present {
		return capture.NewAPIKey(nil)
	}
	return capture.NewAPIKey(&value)
}

func run(ctx context.Context) (err error) {
	data := os.Getenv("PLUGIN_DATA")
	if !filepath.IsAbs(data) {
		return errors.New("web: PLUGIN_DATA must be absolute")
	}
	jina, err := configuredKey("JINA_API_KEY")
	if err != nil {
		return err
	}
	tavily, err := configuredKey("TAVILY_API_KEY")
	if err != nil {
		return err
	}
	lease, err := advisorylock.AcquireDirectory(ctx, data)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()
	store, err := openStore(ctx, filepath.Join(data, "web.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.db.Close()) }()
	client := newProviderHTTPClient()
	defer client.CloseIdleConnections()
	fetch, err := jinaSource(jina, client)
	if err != nil {
		return err
	}
	search, err := tavilySource(tavily, client)
	if err != nil {
		return err
	}
	research, err := newResearch(store, fetch, search)
	if err != nil {
		return err
	}
	server, err := newServer(research)
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
