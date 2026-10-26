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

	"github.com/Tangerg/flame/packages/go-sdk/cancellation"
	"github.com/Tangerg/flame/packages/go-sdk/httpresponse"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/httporigin"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/infra/process/procgroup"
	"github.com/Tangerg/go-sdk/auth"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	"golang.org/x/oauth2"
)

// ServerConfig combines an owned domain descriptor with live OAuth state.
// The descriptor alone owns connection invariants, identity and authority;
// OAuth refresh cannot create another representation of those facts.
type ServerConfig struct {
	mcpserver.Server
	OAuthHandler auth.OAuthHandler
}

func (s ServerConfig) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "ServerConfig{Server:%v, OAuthHandler:%s}", s.Server, mcpserver.SecretPresence(s.OAuthHandler != nil))
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
		if s.Transport != mcpserver.TransportStreamableHTTP {
			return fmt.Errorf("mcp server %q: OAuth applies to HTTP transport only", s.ID())
		}
		if s.Authorization != "" {
			return fmt.Errorf("mcp server %q: OAuth and static Authorization are mutually exclusive", s.ID())
		}
	}
	return nil
}

// Process environment encoding belongs at the exec boundary. Preserve nil:
// exec.Cmd inherits the parent only when Env is absent, not when it is empty.
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
	client *sdkmcp.Client,
	input *launch,
) (_ *sdkmcp.ClientSession, _ sessionCleanup, err error) {
	if input == nil {
		return nil, nil, errors.New("mcp: launch is required")
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, input.close())
		}
	}()
	cfg := input.config
	if ctx == nil {
		return nil, nil, errors.New("mcp: dial context is required")
	}
	if lifetime == nil {
		return nil, nil, errors.New("mcp: session lifetime is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	if client == nil {
		return nil, nil, errors.New("mcp: client must not be nil")
	}
	var command *exec.Cmd
	connect := func(sessionCtx context.Context) (*sdkmcp.ClientSession, error) {
		switch cfg.Transport {
		case mcpserver.TransportStreamableHTTP:
			httpClient, err := endpointHTTPClient(cfg.URL, cfg.Authorization, cfg.Headers)
			if err != nil {
				return nil, fmt.Errorf("mcp server %q: build HTTP client: %w", cfg.ID(), err)
			}
			transport := &sdkmcp.StreamableClientTransport{
				Endpoint:     cfg.URL,
				HTTPClient:   httpClient,
				OAuthHandler: authorizationChallenge{oauth: cfg.OAuthHandler},
			}
			return client.Connect(sessionCtx, transport, nil)
		case mcpserver.TransportStdio:
			stdio := input.stdio
			if stdio == nil {
				stdio = &stdioLaunch{Command: cfg.Command, Args: cfg.Args, Env: cfg.Env, Dir: cfg.Dir}
			}
			cmd := exec.CommandContext(sessionCtx, stdio.Command, stdio.Args...)
			cmd.Env = processEnvironment(stdio.Env)
			cmd.Dir = stdio.Dir
			procgroup.Prepare(cmd)
			cmd.Cancel = func() error { return procgroup.Stop(cmd) }
			command = cmd
			return client.Connect(sessionCtx, &sdkmcp.CommandTransport{Command: cmd}, nil)
		default:
			return nil, fmt.Errorf("mcp: unknown transport %q", cfg.Transport)
		}
	}
	var timeout *time.Duration
	if duration, bounded := cfg.HandshakeTimeout.Duration(); bounded {
		timeout = &duration
	}
	session, cancelLifetime, err := connectSession(ctx, lifetime, timeout, connect)
	retire := input.retire
	input.retire = nil
	cleanup := sessionCleanup(func() (err error) {
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
		if command == nil {
			return nil
		}
		stopStdioProcessErr := procgroup.Stop(command)
		if errors.Is(stopStdioProcessErr, os.ErrProcessDone) {
			return nil
		}
		if stopStdioProcessErr != nil {
			return fmt.Errorf("mcp: stop stdio process group: %w", stopStdioProcessErr)
		}
		return nil
	})
	if err != nil {
		return nil, nil, errors.Join(err, cleanup())
	}
	return session, cleanup, nil
}

type sessionCleanup func() error

// connectSession gives an MCP session a lifecycle distinct from the operation
// that establishes it. Parent cancellation and the configured timeout still
// abort the handshake, but after Connect succeeds the session remains alive
// until its owner explicitly cancels it during detach, replacement, or
// shutdown. Binding the session directly to a short-lived command context
// makes a successful connection look connected while its transport is
// already being torn down.
func connectSession(
	parent context.Context,
	lifetime context.Context,
	timeout *time.Duration,
	connect func(context.Context) (*sdkmcp.ClientSession, error),
) (_ *sdkmcp.ClientSession, release context.CancelFunc, _ error) {
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
	if err := errors.Join(cancellation.Err(handshakeCtx), cancellation.Err(lifetime)); err != nil {
		return nil, nil, err
	}

	session, err := connect(lifetimeCtx)
	stopHandshake()
	lifetimeErr := errors.Join(cancellation.Err(handshakeCtx), cancellation.Err(lifetime))
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
	// The transport pins every request, including the first, to the configured
	// origin, so the shared same-origin redirect policy keeps the chain there.
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
		httpresponse.LimitBody(response, h.maxResponseFrameBytes, errMCPResponseFrameTooLarge)
	}
	return response, err
}

// authorizationChallenge is installed on every Streamable HTTP transport
// because the SDK hands Authorize the exact refused request and response; its
// own status error carries only text. A 401 therefore marks the failure of the
// request it answered, never a concurrent or later request's outcome. Without
// an OAuth handler the challenge cannot be answered here, so it fails the
// request instead of letting the SDK retry.
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
		return fmt.Errorf("%w: %v", errCrossOrigin, err)
	}
	if origin != h.origin {
		return fmt.Errorf("%w: target origin %s differs from configured origin %s",
			errCrossOrigin, origin, h.origin)
	}
	return nil
}
