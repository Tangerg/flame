package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/go-sdk/auth"
	"github.com/Tangerg/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

// OAuth for remote (HTTP) MCP servers. A server that
// needs OAuth surfaces as needsAuth (a 401 on dial maps there via failedStatus),
// which is the cue to sign in. [connections.Authorize] then runs the
// authorization-code flow: open the system browser to the authorization URL,
// catch the redirect on a loopback callback server, exchange the code — all
// driven by the go-sdk's AuthorizationCodeHandler (discovery + Dynamic Client
// Registration + PKCE + refresh). The resulting OAuth config and token are
// persisted through [OAuthSessionStore]; restarts restore a refreshing token
// source without opening a browser. Credentials are bound to the server name
// and normalized endpoint origin, and a rejected credential is deleted before
// the server returns to needsAuth.

const (
	// oauthCallbackPath is the loopback redirect path the authorization server sends
	// the code back to.
	oauthCallbackPath = "/callback"
	// oauthFlowTimeout bounds one interactive sign-in — discovery plus the human
	// completing the browser flow. Generous, because a person is in the loop.
	oauthFlowTimeout             = 5 * time.Minute
	oauthServerReadHeaderTimeout = 5 * time.Second
	oauthServerShutdownTimeout   = time.Second
)

// oauthCallback is the authorization-server redirect outcome.
type oauthCallback struct {
	response auth.AuthorizationResult
	err      error
}

// oauthFlow is one interactive authorization: a loopback HTTP server on an
// ephemeral port catching the redirect, plus the browser-open + wait the
// go-sdk's [auth.AuthorizationCodeFetcher] drives. The redirect URI (known only
// after binding) is registered via Dynamic Client Registration.
type oauthFlow struct {
	redirectURI string
	server      *http.Server
	mu          sync.Mutex
	state       string
	result      chan oauthCallback
	serveDone   chan error
}

// newOAuthFlow binds a loopback callback server on an ephemeral port and starts
// serving. The caller closes it once the flow settles.
func newOAuthFlow(ctx context.Context) (*oauthFlow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("mcp oauth: bind callback server: %w", err)
	}
	f := &oauthFlow{
		redirectURI: fmt.Sprintf("http://%s%s", ln.Addr().String(), oauthCallbackPath),
		result:      make(chan oauthCallback, 1),
		serveDone:   make(chan error, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(oauthCallbackPath, f.handleCallback)
	f.server = &http.Server{Handler: mux, ReadHeaderTimeout: oauthServerReadHeaderTimeout}
	go func() { f.serveDone <- f.server.Serve(ln) }()
	return f, nil
}

// handleCallback captures the authorization code (or error) from the redirect,
// shows the user a close-this-tab page, and hands the outcome to fetch.
func (o *oauthFlow) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	cb := oauthCallback{response: auth.AuthorizationResult{
		Code: q.Get("code"), State: q.Get("state"), Iss: q.Get("iss"),
	}}
	denied := q.Get("error")
	if len(q["state"]) != 1 || len(q["code"]) > 1 || len(q["error"]) > 1 || (denied == "") == (cb.response.Code == "") {
		http.Error(w, "invalid authorization response", http.StatusBadRequest)
		return
	}
	if denied != "" {
		cb.err = fmt.Errorf("authorization denied: %s", denied)
	}
	o.mu.Lock()
	if o.state == "" || cb.response.State != o.state {
		o.mu.Unlock()
		http.Error(w, "authorization attempt does not match", http.StatusBadRequest)
		return
	}
	o.state = ""
	o.result <- cb
	o.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if cb.err != nil {
		_, _ = w.Write([]byte(oauthResultHTML("Authorization failed — you can close this tab.")))
	} else {
		_, _ = w.Write([]byte(oauthResultHTML("Authorization response received — return to Flame to check the result.")))
	}
}

func (o *oauthFlow) fetch(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
	return o.waitForCallback(ctx, args.URL, openBrowser)
}

func (o *oauthFlow) waitForCallback(ctx context.Context, authorizationURL string, launch func(context.Context, string) error) (*auth.AuthorizationResult, error) {
	endpoint, err := url.Parse(authorizationURL)
	if err != nil {
		return nil, err
	}
	state := endpoint.Query().Get("state")
	if state == "" {
		return nil, errors.New("mcp oauth: authorization URL lacks state")
	}
	o.mu.Lock()
	o.state = state
	o.mu.Unlock()
	defer func() { o.mu.Lock(); o.state = ""; o.mu.Unlock() }()
	if err := launch(ctx, authorizationURL); err != nil {
		return nil, fmt.Errorf("mcp oauth: enable a system browser launcher and restart sign-in: %w", err)
	}
	select {
	case cb := <-o.result:
		if cb.err != nil {
			return nil, cb.err
		}
		return &cb.response, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// close owns callback-server settlement even after the authorization is
// canceled. Its independent deadline bounds graceful shutdown before a forced
// close, and its caller owns reporting any cleanup failure.
func (o *oauthFlow) close(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), oauthServerShutdownTimeout)
	defer cancel()
	shutdownErr := o.server.Shutdown(shutdownCtx)
	var forceErr error
	if shutdownErr != nil {
		forceErr = o.server.Close()
	}
	serveErr := <-o.serveDone
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	if err := errors.Join(shutdownErr, forceErr, serveErr); err != nil {
		return fmt.Errorf("mcp oauth: close callback server: %w", err)
	}
	return nil
}

// newOAuthHandler builds the interactive authorization-code handler for one
// sign-in, registering the flow's loopback redirect via Dynamic Client
// Registration so no client id need be preconfigured.
func newOAuthHandler(
	ctx context.Context,
	flow *oauthFlow,
	lifetime context.Context,
	store oauthSessionStore,
	target mcpserver.OAuthTarget,
) (auth.OAuthHandler, error) {
	if lifetime == nil {
		return nil, errors.New("mcp oauth: lifetime is required")
	}
	config := &auth.AuthorizationCodeHandlerConfig{
		Client: oauthHTTPClient(),
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				ClientName:              "Flame",
				RedirectURIs:            []string{flow.redirectURI},
				GrantTypes:              []string{"authorization_code", "refresh_token"},
				ResponseTypes:           []string{"code"},
				TokenEndpointAuthMethod: "none",
			},
		},
		RedirectURL:              flow.redirectURI,
		AuthorizationCodeFetcher: flow.fetch,
		RequestRefreshToken:      store != nil,
	}
	if store != nil {
		binding, err := store.BeginOAuthSession(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("mcp oauth: begin session for %q: %w", target.ID(), err)
		}
		session := &oauthSession{store: store, server: target.ID(), binding: binding}
		config.NewTokenSource = func(tokenCtx context.Context, cfg *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			if err := session.save(ctx, cfg, token); err != nil {
				return nil, err
			}
			refreshCtx := lifetime
			if client := tokenCtx.Value(oauth2.HTTPClient); client != nil {
				refreshCtx = context.WithValue(refreshCtx, oauth2.HTTPClient, client)
			}
			source := newSavingTokenSource(
				cfg.TokenSource(refreshCtx, token),
				cfg,
				token,
				func(updatedConfig *oauth2.Config, updatedToken *oauth2.Token) error {
					return session.save(lifetime, updatedConfig, updatedToken)
				},
			)
			return invalidateRejectedTokens(source, lifetime, session), nil
		}
	}
	return auth.NewAuthorizationCodeHandler(config)
}

// The issuer can differ from the MCP resource. Each OAuth request starts at
// its own discovered endpoint, but its form credentials cannot follow a
// redirect to another origin.
func oauthHTTPClient() *http.Client { return &http.Client{CheckRedirect: checkRedirect} }

// openBrowser opens url in the user's default browser, best-effort and
// platform-specific. This service is local and binds loopback, so this
// opens on the user's machine; launch failure terminates this authorization attempt.
func openBrowser(ctx context.Context, url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	}
	return cmd.Run()
}

func oauthResultHTML(msg string) string {
	return "<!doctype html><meta charset=utf-8><title>Flame</title>" +
		`<body style="font:14px system-ui;display:grid;place-items:center;height:100vh;margin:0">` +
		"<p>" + msg + "</p></body>"
}
