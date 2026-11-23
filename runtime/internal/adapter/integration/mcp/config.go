package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	"github.com/Tangerg/flame/packages/go-sdk/httpresponse"
	"github.com/Tangerg/flame/packages/go-sdk/procgroup"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/secret"
	"github.com/Tangerg/go-sdk/auth"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/samber/lo"
	"golang.org/x/oauth2"
)

type ServerConfig struct {
	mcpserver.Server
	OAuthHandler auth.OAuthHandler
}

func (s ServerConfig) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "ServerConfig{Server:%v, OAuthHandler:%s}", s.Server, secret.Presence(s.OAuthHandler != nil))
}

func (s ServerConfig) Clone() ServerConfig {
	s.Server = s.Server.Clone()
	return s
}

func (s ServerConfig) Validate() error {
	if err := s.Server.Validate(); err != nil {
		return err
	}
	if s.OAuthHandler != nil {
		if lo.IsNil(s.OAuthHandler) {
			return fmt.Errorf("mcp server %q: supplied OAuth handler is nil", s.ID())
		}
		http, ok := s.Connection.(mcpserver.HTTPConnection)
		if !ok {
			return fmt.Errorf("mcp server %q: OAuth applies to HTTP transport only", s.ID())
		}
		if http.Authorization != "" {
			return fmt.Errorf("mcp server %q: OAuth and static Authorization are mutually exclusive", s.ID())
		}
	}
	return nil
}

func processEnvironment(values map[string]string) []string {
	if values == nil {
		return nil
	}
	entries := make([]string, 0, len(values))
	for key, value := range values {
		entries = append(entries, key+"="+value)
	}
	slices.Sort(entries)
	return entries
}

func dial(
	ctx context.Context,
	lifetime context.Context,
	client *sdk.Client,
	input *launch,
) (_ *sdk.ClientSession, _ sessionCleanup, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, input.close())
		}
	}()
	if err := input.config.Validate(); err != nil {
		return nil, nil, err
	}
	connection := &dialedConnection{client: client, input: input}
	var timeout *time.Duration
	if duration, bounded := input.config.HandshakeTimeout.Duration(); bounded {
		timeout = &duration
	}
	session, cancelLifetime, err := connectSession(ctx, lifetime, timeout, connection.connect)
	retire := input.retire
	input.retire = nil
	cleanup := connection.cleanup(retire, cancelLifetime)
	if err != nil {
		return nil, nil, errors.Join(err, cleanup())
	}
	return session, cleanup, nil
}

type dialedConnection struct {
	client  *sdk.Client
	input   *launch
	command *exec.Cmd
}

func (d *dialedConnection) connect(ctx context.Context) (*sdk.ClientSession, error) {
	cfg := d.input.config
	switch connection := cfg.Connection.(type) {
	case mcpserver.HTTPConnection:
		httpClient, err := endpointHTTPClient(connection.URL, connection.Authorization, connection.Headers)
		if err != nil {
			return nil, fmt.Errorf("mcp server %q: build HTTP client: %w", cfg.ID(), err)
		}
		transport := &sdk.StreamableClientTransport{
			Endpoint:     connection.URL,
			HTTPClient:   httpClient,
			OAuthHandler: authorizationChallenge{oauth: cfg.OAuthHandler},
		}
		return d.client.Connect(ctx, transport, nil)
	case mcpserver.StdioConnection:
		stdio := d.input.stdio
		if stdio == nil {
			stdio = &stdioLaunch{Command: connection.Command, Args: connection.Args, Env: connection.Env, Dir: connection.Dir}
		}
		cmd := exec.CommandContext(ctx, stdio.Command, stdio.Args...)
		cmd.Env = processEnvironment(stdio.Env)
		cmd.Dir = stdio.Dir
		procgroup.Prepare(cmd)
		cmd.Cancel = func() error { return procgroup.Stop(cmd) }
		d.command = cmd
		return d.client.Connect(ctx, &sdk.CommandTransport{Command: cmd}, nil)
	default:
		return nil, fmt.Errorf("mcp: unknown transport %q", cfg.Transport())
	}
}

func (d *dialedConnection) cleanup(retire sessionCleanup, cancelLifetime context.CancelFunc) sessionCleanup {
	return func() (err error) {
		if retire != nil {
			defer func() {
				if retireErr := retire(); retireErr != nil {
					err = errors.Join(err, fmt.Errorf("mcp: retire launch resource: %w", retireErr))
				}
			}()
		}
		if cancelLifetime != nil {
			cancelLifetime()
		}
		if d.command == nil {
			return nil
		}
		if err := procgroup.Stop(d.command); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("mcp: stop stdio process group: %w", err)
		}
		return nil
	}
}

type sessionCleanup func() error

func connectSession(
	parent context.Context,
	lifetime context.Context,
	timeout *time.Duration,
	connect func(context.Context) (*sdk.ClientSession, error),
) (_ *sdk.ClientSession, release context.CancelFunc, _ error) {
	if parent == nil {
		return nil, nil, errors.New("mcp: handshake context is required")
	}
	if lifetime == nil {
		return nil, nil, errors.New("mcp: session lifetime is required")
	}
	lifetimeCtx, cancelLifetime := context.WithCancel(lifetime)
	defer func() {
		if release == nil {
			cancelLifetime()
		}
	}()
	handshakeCtx := parent
	cancelHandshake := func() {}
	if timeout != nil {
		handshakeCtx, cancelHandshake = context.WithTimeout(parent, *timeout)
	}
	defer cancelHandshake()
	stopHandshake := context.AfterFunc(handshakeCtx, cancelLifetime)
	defer stopHandshake()
	if err := errors.Join(errors.Join(handshakeCtx.Err(), context.Cause(handshakeCtx)), errors.Join(lifetime.Err(), context.Cause(lifetime))); err != nil {
		return nil, nil, err
	}

	session, err := connect(lifetimeCtx)
	stopHandshake()
	lifetimeErr := errors.Join(errors.Join(handshakeCtx.Err(), context.Cause(handshakeCtx)), errors.Join(lifetime.Err(), context.Cause(lifetime)))
	if err != nil {
		return nil, nil, errors.Join(err, lifetimeErr)
	}
	if lifetimeErr != nil {
		cancelLifetime()
		return nil, nil, errors.Join(lifetimeErr, session.Close())
	}
	return session, cancelLifetime, nil
}

const maxMCPResponseFrameBytes = 64 << 20

var (
	errCrossOrigin              = errors.New("mcp: cross-origin request blocked")
	errMCPResponseFrameTooLarge = errors.New("mcp: response frame too large")
)

func endpointHTTPClient(endpoint, authorization string, headers map[string]string) (*http.Client, error) {
	origin, err := httporigin.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	transport := &headerRoundTripper{
		origin:                origin,
		authorization:         authorization,
		headers:               maps.Clone(headers),
		base:                  http.DefaultTransport,
		maxResponseFrameBytes: maxMCPResponseFrameBytes,
	}
	return &http.Client{Transport: transport, CheckRedirect: checkRedirect}, nil
}

type headerRoundTripper struct {
	origin                httporigin.Origin
	authorization         string
	headers               map[string]string
	base                  http.RoundTripper
	maxResponseFrameBytes int64
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := h.validateTarget(req.URL); err != nil {
		return nil, err
	}
	r := req.Clone(req.Context())
	for k, v := range h.headers {
		r.Header.Set(k, v)
	}
	if h.authorization != "" {
		r.Header.Set("Authorization", h.authorization)
	}
	response, err := h.base.RoundTrip(r)
	if response != nil {
		httpresponse.LimitBody(response, httpresponse.DocumentOrEvents, h.maxResponseFrameBytes, errMCPResponseFrameTooLarge)
	}
	return response, err
}

// The SDK hands Authorize the exact refused request and response while its own
// status error carries only text, so a 401 marks only the request it answered.
// Without an OAuth handler the challenge fails the request instead of letting the
// SDK retry.
type authorizationChallenge struct {
	oauth auth.OAuthHandler
}

func (a authorizationChallenge) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	if a.oauth == nil {
		return nil, nil
	}
	return a.oauth.TokenSource(ctx)
}

func (a authorizationChallenge) Authorize(ctx context.Context, request *http.Request, response *http.Response) error {
	var err error
	if a.oauth == nil {
		err = errors.Join(fmt.Errorf("mcp: server responded %s", response.Status), response.Body.Close())
	} else {
		err = a.oauth.Authorize(ctx, request, response)
	}
	if err != nil && response.StatusCode == http.StatusUnauthorized {
		return errors.Join(mcpserver.ErrAuthorizationRequired, err)
	}
	return err
}

func (h *headerRoundTripper) validateTarget(target *url.URL) error {
	origin, err := httporigin.FromURL(target)
	if err != nil {
		return fmt.Errorf("%w: %w", errCrossOrigin, err)
	}
	if origin != h.origin {
		return fmt.Errorf("%w: target origin %s differs from configured origin %s",
			errCrossOrigin, origin, h.origin)
	}
	return nil
}
