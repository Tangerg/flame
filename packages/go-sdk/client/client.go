package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

const defaultMaximumMessageBytes = 128 << 20

var ErrClosed = errors.New("runtime client: closed")

// ErrAcknowledgementUnknown never authorizes an automatic retry or a new command
// identity: preserve the original input and idempotency identity.
var ErrAcknowledgementUnknown = errors.New("runtime: command acknowledgement is unknown")

// ErrDisconnected may justify reconnecting a read or subscription, never
// replaying an uncertain command.
var ErrDisconnected = errors.New("runtime: disconnected")

type RemoteConfig struct {
	Endpoint        string
	Token           string
	MaxMessageBytes int
}

// Closing a Client never stops the Runtime or cancels an accepted Run. Calls are
// safe concurrently; do not copy a Client.
type Client struct {
	endpoint        string
	token           string
	callbackCall    string
	http            *http.Client
	maxMessageBytes int
	mu              sync.Mutex
	closeDone       chan struct{}
	calls           map[*remoteCall]struct{}
}

func Connect(ctx context.Context, cfg RemoteConfig) (*Client, error) {
	transport, err := cloneDefaultTransport()
	if err != nil {
		return nil, err
	}
	return connect(ctx, cfg, transport)
}

// A private clone keeps this client's idle connections out of the process
// default transport while using the standard dialing and proxy policy.
func cloneDefaultTransport() (*http.Transport, error) {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("runtime: http.DefaultTransport is %T, not *http.Transport", http.DefaultTransport)
	}
	return transport.Clone(), nil
}

func connect(ctx context.Context, cfg RemoteConfig, transport *http.Transport) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	endpoint, err := httporigin.ParseEndpoint(cfg.Endpoint)
	if err != nil || endpoint.HasQuery() || endpoint.HasFragment() {
		return nil, errors.New("runtime: endpoint must be an absolute http or https base url without credentials, query, or fragment")
	}
	if cfg.Token != "" && !endpoint.Secure() && !endpoint.Loopback() {
		return nil, errors.New("runtime: bearer credentials require HTTPS outside loopback")
	}
	if err := validateHeaderValue(headerAuthorization, cfg.Token); err != nil {
		return nil, err
	}
	if cfg.MaxMessageBytes < 0 {
		return nil, errors.New("runtime: max message bytes must not be negative")
	}
	if cfg.MaxMessageBytes == 0 {
		cfg.MaxMessageBytes = defaultMaximumMessageBytes
	}
	client := &Client{
		endpoint: endpoint.CanonicalBase() + rpcPath, token: cfg.Token, calls: make(map[*remoteCall]struct{}),
		maxMessageBytes: cfg.MaxMessageBytes,
		http: &http.Client{
			Transport:     transport,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	return client, nil
}

func ConnectCallback(ctx context.Context, ref pluginmcp.CallbackRef) (*Client, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	limit := int(ref.MaxResponseBytes)
	if int64(limit) != ref.MaxResponseBytes {
		return nil, errors.New("runtime: callback message capacity is not representable")
	}
	transport, err := cloneDefaultTransport()
	if err != nil {
		return nil, err
	}
	transport.Proxy = nil
	client, err := connect(ctx, RemoteConfig{Endpoint: ref.Endpoint, Token: ref.Credential, MaxMessageBytes: limit}, transport)
	if err != nil {
		return nil, err
	}
	client.callbackCall = ref.Call
	return client, nil
}

func (r *Client) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if done := r.closeDone; done != nil {
		r.mu.Unlock()
		<-done
		return nil
	}
	done := make(chan struct{})
	r.closeDone = done
	calls := make([]*remoteCall, 0, len(r.calls))
	for call := range r.calls {
		calls = append(calls, call)
	}
	r.mu.Unlock()
	for _, call := range calls {
		call.finish(ErrClosed)
	}
	if r.http != nil {
		r.http.CloseIdleConnections()
	}
	close(done)
	return nil
}
