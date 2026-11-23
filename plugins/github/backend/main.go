package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context, data string) (err error) {
	var configured *string
	if value, present := os.LookupEnv("GITHUB_TOKEN"); present {
		configured = &value
	}
	credential, err := newCredential(configured)
	if err != nil {
		return err
	}
	store, err := openStore(ctx, filepath.Join(data, "github.db"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.db.Close()) }()
	transport := newGitHubTransport()
	defer transport.CloseIdleConnections()
	connector, err := newConnector(store, transport, credential)
	if err != nil {
		return err
	}
	server, err := newServer(connector)
	if err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

func main() { backend.Main("github", run) }

// One small pool serves a single user's GitHub API traffic.
const (
	gitHubDialTimeout       = 10 * time.Second
	gitHubKeepAlive         = 30 * time.Second
	gitHubIdleConnections   = 4
	gitHubIdlePerHost       = 2
	gitHubConnectionsLimit  = 4
	gitHubIdleTimeout       = 90 * time.Second
	gitHubTLSHandshake      = 10 * time.Second
	gitHubMaxResponseHeader = 64 << 10
)

func newGitHubTransport() *http.Transport {
	return &http.Transport{
		DialContext:            (&net.Dialer{Timeout: gitHubDialTimeout, KeepAlive: gitHubKeepAlive}).DialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           gitHubIdleConnections,
		MaxIdleConnsPerHost:    gitHubIdlePerHost,
		MaxConnsPerHost:        gitHubConnectionsLimit,
		IdleConnTimeout:        gitHubIdleTimeout,
		TLSHandshakeTimeout:    gitHubTLSHandshake,
		MaxResponseHeaderBytes: gitHubMaxResponseHeader,
	}
}
