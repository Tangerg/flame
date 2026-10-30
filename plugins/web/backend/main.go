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
	"github.com/Tangerg/scope/tools/httpreq"
)

func configuredKey(name string) (apiKey, error) {
	value, present := os.LookupEnv(name)
	if !present {
		return newAPIKey(nil)
	}
	return newAPIKey(&value)
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
	lease, err := filesystem.AcquireDirectoryLease(ctx, data)
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
	fetch, err := jinaSource(ctx, jina, client)
	if err != nil {
		return err
	}
	search, err := tavilySource(ctx, tavily, client)
	if err != nil {
		return err
	}
	research, err := newResearch(store, fetch, search)
	if err != nil {
		return err
	}
	var allowedHosts []string
	if raw, present := os.LookupEnv("HTTP_ALLOWED_HOSTS"); present {
		if err := json.Unmarshal([]byte(raw), &allowedHosts); err != nil {
			return fmt.Errorf("web: HTTP_ALLOWED_HOSTS must be a JSON array of hosts: %w", err)
		}
		if allowedHosts == nil {
			return errors.New("web: HTTP_ALLOWED_HOSTS must be a JSON array of hosts")
		}
	}
	requests, err := newRequests(store.db, httpreq.ClientConfig{AllowedHosts: allowedHosts, HTTPClient: client})
	if err != nil {
		return err
	}
	server, err := newServer(research, requests)
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
