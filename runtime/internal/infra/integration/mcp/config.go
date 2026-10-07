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
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/httporigin"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/httpresponse"
	"github.com/Tangerg/flame/runtime/internal/infra/process/procgroup"
	"github.com/Tangerg/flame/runtime/internal/optional"
	"github.com/Tangerg/go-sdk/auth"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
	"golang.org/x/oauth2"
)

// Transport is the wire mode of an MCP server connection. The zero value is
// invalid so a misconfigured runtime entry fails validation instead of silently
// defaulting.
type Transport string

const (
	// TransportHTTP is Streamable HTTP. [ServerConfig.Endpoint] is the URL.
	TransportHTTP Transport = "http"
	// TransportStdio is a local subprocess over stdin/stdout.
	TransportStdio Transport = "stdio"
)

func (s ServerConfig) oauthTarget() mcpserver.OAuthTarget {
	return mcpserver.OAuthTarget{Source: s.Source, Name: s.Name, URL: s.Endpoint, Headers: maps.Clone(s.Headers)}
}

// Valid reports whether transport names one supported MCP connection mode.
func (t Transport) Valid() bool {
	return t == TransportHTTP || t == TransportStdio
}

func (t Transport) String() string {
	if !t.Valid() {
		return "invalid"
	}
	return string(t)
}

// ServerConfig declaratively describes one runtime MCP server connection. This
// is application configuration, not part of the reusable mcp package: the
// protocol package exposes transports and sessions, while the runtime owns how
// persisted descriptors become live sessions.
type ServerConfig struct {
	// SourceFingerprint is the authority this connection realizes; tool
	// policy compares it with the registry's current authority.
	SourceFingerprint fingerprint.Digest
	// Source owns the record and, for an installation, names the release
	// this connection realizes. Required.
	Source mcpserver.Source
	// Name namespaces the server's tools. Required.
	Name mcpserver.ServerName

	// Transport picks the connection mode. Required.
	Transport Transport

	// Endpoint is the Streamable HTTP URL. Used with [TransportHTTP].
	Endpoint string

	// Command is the executable to spawn. Used with [TransportStdio].
	Command string

	// Args are command arguments. Used with [TransportStdio].
	Args []string

	// Env, when non-nil, replaces the subprocess environment.
	Env []string

	// Dir sets the subprocess working directory.
	Dir string

	// Authorization is sent as the HTTP Authorization header.
	Authorization string

	// Headers carries extra static HTTP headers.
	Headers map[string]string

	// HandshakeTimeout bounds MCP initialization when present. nil is the
	// explicit unbounded policy and does not affect the live session.
	HandshakeTimeout *time.Duration

	// OAuthHandler authorizes an HTTP connection via OAuth 2.1. The handler is
	// live process state; its opaque session may be persisted separately through
	// [OAuthSessionStore].
	OAuthHandler auth.OAuthHandler
}

func (s ServerConfig) ID() mcpserver.ID { return s.Source.ID(s.Name) }

// Format keeps the credential-bearing fields behind a redaction boundary for
// every fmt verb, as mcpserver.Server does for the same fields upstream. This
// is the connection adapter the domain type's comment defers the raw values to,
// so it is the last place a diagnostic or a failing test could print them.
func (s ServerConfig) Format(state fmt.State, _ rune) {
	timeout := "unbounded"
	if s.HandshakeTimeout != nil {
		timeout = s.HandshakeTimeout.String()
	}
	_, _ = fmt.Fprintf(
		state,
		"ServerConfig{ID:%q, Transport:%q, Endpoint:%s, Command:%q, Args:%q, Env:%s, Dir:%q, "+
			"Authorization:%s, Headers:%s, HandshakeTimeout:%s, OAuthHandler:%s}",
		s.ID(),
		s.Transport,
		mcpserver.SecretPresence(s.Endpoint != ""),
		s.Command,
		s.Args,
		mcpserver.SecretPresence(len(s.Env) > 0),
		s.Dir,
		mcpserver.SecretPresence(s.Authorization != ""),
		mcpserver.SecretPresence(len(s.Headers) > 0),
		timeout,
		mcpserver.SecretPresence(s.OAuthHandler != nil),
	)
}

// Clone returns an independently owned configuration snapshot. Live handler
// values are intentionally shared; only mutable collection storage is copied.
func (s ServerConfig) Clone() ServerConfig {
	s.Args = slices.Clone(s.Args)
	s.Env = slices.Clone(s.Env)
	s.Headers = maps.Clone(s.Headers)
	s.HandshakeTimeout = optional.Clone(s.HandshakeTimeout)
	return s
}

// SameConnection compares the configuration realized by an executable. OAuth
// refresh belongs to the credential store; handshake limits govern a future
// attempt. Neither changes the configuration of an already connected session.
func (s ServerConfig) SameConnection(other ServerConfig) bool {
	if s.Source != other.Source || s.Name != other.Name || s.Transport != other.Transport ||
		s.SourceFingerprint != other.SourceFingerprint {
		return false
	}
	switch s.Transport {
	case TransportHTTP:
		return s.Endpoint == other.Endpoint && s.Authorization == other.Authorization && maps.Equal(s.Headers, other.Headers)
	case TransportStdio:
		return s.Command == other.Command && s.Dir == other.Dir && slices.Equal(s.Args, other.Args) &&
			(s.Env == nil) == (other.Env == nil) && slices.Equal(s.Env, other.Env)
	default:
		return false
	}
}

// Validate reports whether exactly one transport is fully specified and the
// other transport's fields are blank.
func (s ServerConfig) Validate() error {
	if err := s.Name.Validate(); err != nil {
		return fmt.Errorf("mcp: server name: %w", err)
	}
	if s.HandshakeTimeout != nil && *s.HandshakeTimeout <= 0 {
		return fmt.Errorf("mcp server %q: HandshakeTimeout must be positive when present", s.ID())
	}
	switch s.Transport {
	case TransportHTTP:
		if s.Endpoint == "" {
			return fmt.Errorf("mcp server %q: Endpoint is required for HTTP transport", s.ID())
		}
		if _, err := httporigin.Parse(s.Endpoint); err != nil {
			return fmt.Errorf("mcp server %q: invalid Endpoint: %w", s.ID(), err)
		}
		if s.Command != "" {
			return fmt.Errorf("mcp server %q: Command must be empty for HTTP transport", s.ID())
		}
		for name := range s.Headers {
			if strings.EqualFold(name, "Authorization") {
				return fmt.Errorf("mcp server %q: Headers must not duplicate Authorization", s.ID())
			}
		}
		if s.OAuthHandler != nil && s.Authorization != "" {
			return fmt.Errorf("mcp server %q: OAuth and static Authorization are mutually exclusive", s.ID())
		}
	case TransportStdio:
		if s.Command == "" {
			return fmt.Errorf("mcp server %q: Command is required for stdio transport", s.ID())
		}
		if s.Endpoint != "" {
			return fmt.Errorf("mcp server %q: Endpoint must be empty for stdio transport", s.ID())
		}
		if s.Authorization != "" {
			return fmt.Errorf("mcp server %q: Authorization applies to HTTP transport only", s.ID())
		}
		if len(s.Headers) > 0 {
			return fmt.Errorf("mcp server %q: Headers apply to HTTP transport only", s.ID())
		}
		if s.OAuthHandler != nil {
			return fmt.Errorf("mcp server %q: OAuth applies to HTTP transport only", s.ID())
		}
	default:
		return fmt.Errorf("mcp server %q: unknown transport %q", s.ID(), s.Transport)
	}
	return nil
}

func dial(
	ctx context.Context,
	lifetime context.Context,
	client *sdkmcp.Client,
	cfg ServerConfig,
) (*sdkmcp.ClientSession, sessionCleanup, error) {
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
		case TransportHTTP:
			httpClient, err := endpointHTTPClient(cfg.Endpoint, cfg.Authorization, cfg.Headers)
			if err != nil {
				return nil, fmt.Errorf("mcp server %q: build HTTP client: %w", cfg.ID(), err)
			}
			transport := &sdkmcp.StreamableClientTransport{
				Endpoint:     cfg.Endpoint,
				HTTPClient:   httpClient,
				OAuthHandler: authorizationChallenge{oauth: cfg.OAuthHandler},
			}
			return client.Connect(sessionCtx, transport, nil)
		case TransportStdio:
			cmd := exec.CommandContext(sessionCtx, cfg.Command, cfg.Args...)
			if cfg.Env != nil {
				cmd.Env = cfg.Env
			}
			if cfg.Dir != "" {
				cmd.Dir = cfg.Dir
			}
			procgroup.Prepare(cmd)
			cmd.Cancel = func() error { return procgroup.Stop(cmd) }
			command = cmd
			return client.Connect(sessionCtx, &sdkmcp.CommandTransport{Command: cmd}, nil)
		default:
			return nil, fmt.Errorf("mcp: unknown transport %q", cfg.Transport)
		}
	}
	session, cancelLifetime, err := connectSession(ctx, lifetime, cfg.HandshakeTimeout, connect)
	cleanup := sessionCleanup(func() error {
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
		return stopStdioProcessErr
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
// makes a successful dynamic Configure look connected while its transport is
// already being torn down.
func connectSession(
	parent context.Context,
	lifetime context.Context,
	timeout *time.Duration,
	connect func(context.Context) (*sdkmcp.ClientSession, error),
) (*sdkmcp.ClientSession, context.CancelFunc, error) {
	if parent == nil {
		return nil, nil, errors.New("mcp: handshake context is required")
	}
	if lifetime == nil {
		return nil, nil, errors.New("mcp: session lifetime is required")
	}
	lifetimeCtx, cancelLifetime := context.WithCancel(lifetime)
	handshakeCtx := parent
	cancelHandshake := func() {}
	if timeout != nil {
		handshakeCtx, cancelHandshake = context.WithTimeout(parent, *timeout)
	}
	stopHandshake := context.AfterFunc(handshakeCtx, cancelLifetime)

	session, err := connect(lifetimeCtx)
	if err != nil {
		stopHandshake()
		cancelHandshake()
		cancelLifetime()
		return nil, nil, err
	}
	if handshakeErr := handshakeCtx.Err(); handshakeErr != nil || !stopHandshake() {
		if handshakeErr == nil {
			handshakeErr = context.Canceled
		}
		cancelHandshake()
		cancelLifetime()
		return nil, nil, errors.Join(handshakeErr, session.Close())
	}
	cancelHandshake()
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
	return &http.Client{Transport: transport, CheckRedirect: httporigin.CheckRedirect}, nil
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
