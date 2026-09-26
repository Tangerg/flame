package runtime

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/transport/http"
)

// ErrInvalidResponse identifies an incompatible or malformed Runtime reply.
// A failed mutation acknowledgement never proves that the mutation did not run.
var ErrInvalidResponse = errors.New("runtime: invalid response")

// ErrAcknowledgementUnknown marks a dispatched command whose acknowledgement
// could not be established. Preserve its original input and idempotency identity;
// this marker never authorizes an automatic retry or a new command identity.
var ErrAcknowledgementUnknown = errors.New("runtime: command acknowledgement is unknown")

// ErrDisconnected identifies lost transport observation. It may justify
// reconnecting a read or subscription, never replaying an uncertain command.
var ErrDisconnected = errors.New("runtime: disconnected")

// RemoteConfig identifies an existing Runtime. Endpoint is an absolute HTTP(S)
// base URL, including any reverse-proxy prefix, without credentials or a query.
type RemoteConfig struct {
	Endpoint string
	Token    string
	// MaxMessageBytes optionally bounds one buffered reply or SSE frame. Zero
	// adds no client limit; the Runtime protocol imposes no export-size limit.
	MaxMessageBytes int
}

// Client consumes an existing Runtime through HTTP and SSE. It owns only its
// connections; closing it never stops the Runtime or cancels an accepted Run.
// Calls are safe concurrently. Construct with Connect and do not copy a Client.
type Client struct {
	binding
	endpoint        string
	token           string
	http            *http.Client
	maxMessageBytes int
	mu              sync.Mutex
	closed          bool
	calls           map[*remoteCall]struct{}
}

// Connect constructs a remote binding without spawning a Runtime or performing
// discovery. The caller negotiates once through Discover with its capabilities.
// ctx governs construction; each operation supplies its own lifetime.
func Connect(ctx context.Context, cfg RemoteConfig) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	base, err := url.Parse(cfg.Endpoint)
	if err != nil || base == nil || base.Host == "" || base.Hostname() == "" ||
		(base.Scheme != "http" && base.Scheme != "https") || base.User != nil ||
		base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || base.Opaque != "" {
		return nil, errors.New("runtime: endpoint must be an absolute http or https base url without credentials, query, or fragment")
	}
	if strings.ContainsAny(cfg.Token, "\r\n") {
		return nil, errors.New("runtime: token must not contain line breaks")
	}
	if cfg.MaxMessageBytes < 0 {
		return nil, errors.New("runtime: max message bytes must not be negative")
	}
	for _, endpoint := range flamehttp.Contract().Endpoints {
		if endpoint.Kind == flamehttp.EndpointKindRPC {
			base.Path = strings.TrimRight(base.Path, "/") + endpoint.Path
			base.RawPath = ""
			break
		}
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second, KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	client := &Client{
		endpoint: base.String(), token: cfg.Token, calls: make(map[*remoteCall]struct{}),
		maxMessageBytes: cfg.MaxMessageBytes,
		http: &http.Client{
			Transport:     transport,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	client.binding.caller = client
	return client, nil
}

// Close rejects new calls and detaches every outstanding request or stream.
// Run cancellation remains the explicit CancelRun operation.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	c.closed = true
	calls := make([]*remoteCall, 0, len(c.calls))
	for call := range c.calls {
		calls = append(calls, call)
	}
	c.mu.Unlock()
	for _, call := range calls {
		call.finish(ErrClosed)
	}
	if c.http != nil {
		c.http.CloseIdleConnections()
	}
	return nil
}
