package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	resource "github.com/Tangerg/flame/plugins/github/backend/domain"
	"github.com/Tangerg/flame/runtime/storage/advisorylock"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context) (err error) {
	data := os.Getenv("PLUGIN_DATA")
	if !filepath.IsAbs(data) {
		return errors.New("github: PLUGIN_DATA must be absolute")
	}
	var configured *string
	if value, present := os.LookupEnv("GITHUB_TOKEN"); present {
		configured = &value
	}
	credential, err := resource.NewCredential(configured)
	if err != nil {
		return err
	}
	lease, err := advisorylock.AcquireDirectory(ctx, data)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()
	cache, err := openCache(ctx, filepath.Join(data, "github.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, cache.db.Close()) }()
	transport := &http.Transport{DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext, ForceAttemptHTTP2: true, MaxIdleConns: 4, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 4, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, MaxResponseHeaderBytes: 64 << 10}
	defer transport.CloseIdleConnections()
	connector, err := newConnector(cache, transport, credential)
	if err != nil {
		return err
	}
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
