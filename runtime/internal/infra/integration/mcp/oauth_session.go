package mcp

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/runtime/internal/optional"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/Tangerg/go-sdk/auth"
	"golang.org/x/oauth2"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// OAuthSessionStore binds each credential to the target that requested it and
// fences credential writes by their authorization grant. Load finds only a
// credential of the same target; Save and Remove must refuse a superseded
// binding without changing its replacement.
type OAuthSessionStore interface {
	BeginOAuthSession(ctx context.Context, target mcpserver.OAuthTarget) (binding string, err error)
	LoadOAuthSession(ctx context.Context, target mcpserver.OAuthTarget) (payload []byte, binding string, found bool, err error)
	SaveOAuthSession(ctx context.Context, server mcpserver.ID, binding string, payload []byte) error
	RemoveOAuthSession(ctx context.Context, server mcpserver.ID, binding string) error
}

const oauthSessionVersion = 1

var errStoredOAuthRejected = errors.New("mcp oauth: stored session was rejected; sign in again")

type storedOAuthSession struct {
	Version int               `json:"version"`
	Config  storedOAuthConfig `json:"config"`
	Token   storedOAuthToken  `json:"token"`
}

type storedOAuthConfig struct {
	ClientID     string           `json:"clientId"`
	ClientSecret string           `json:"clientSecret,omitempty"`
	AuthURL      string           `json:"authUrl"`
	TokenURL     string           `json:"tokenUrl"`
	AuthStyle    oauth2.AuthStyle `json:"authStyle,omitempty"`
	RedirectURL  string           `json:"redirectUrl"`
	Scopes       []string         `json:"scopes,omitempty"`
}

type storedOAuthToken struct {
	AccessToken  string    `json:"accessToken"`
	TokenType    string    `json:"tokenType,omitempty"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	Expiry       time.Time `json:"expiry,omitzero"`
}

func encodeOAuthSession(cfg *oauth2.Config, token *oauth2.Token) ([]byte, error) {
	if cfg == nil || token == nil {
		return nil, errors.New("mcp oauth: config and token are required")
	}
	session := storedOAuthSession{
		Version: oauthSessionVersion,
		Config: storedOAuthConfig{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			AuthURL:      cfg.Endpoint.AuthURL,
			TokenURL:     cfg.Endpoint.TokenURL,
			AuthStyle:    cfg.Endpoint.AuthStyle,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       slices.Clone(cfg.Scopes),
		},
		Token: storedOAuthToken{
			AccessToken:  token.AccessToken,
			TokenType:    token.TokenType,
			RefreshToken: token.RefreshToken,
			Expiry:       token.Expiry,
		},
	}
	if err := session.validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("mcp oauth: encode session: %w", err)
	}
	return payload, nil
}

func decodeOAuthSession(payload []byte) (*oauth2.Config, *oauth2.Token, error) {
	var session storedOAuthSession
	if err := json.Unmarshal(payload, &session, json.RejectUnknownMembers(true)); err != nil {
		return nil, nil, fmt.Errorf("mcp oauth: decode session: %w", err)
	}
	if err := session.validate(); err != nil {
		return nil, nil, err
	}
	return &oauth2.Config{
		ClientID:     session.Config.ClientID,
		ClientSecret: session.Config.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   session.Config.AuthURL,
			TokenURL:  session.Config.TokenURL,
			AuthStyle: session.Config.AuthStyle,
		},
		RedirectURL: session.Config.RedirectURL,
		Scopes:      slices.Clone(session.Config.Scopes),
	}, &oauth2.Token{
		AccessToken:  session.Token.AccessToken,
		TokenType:    session.Token.TokenType,
		RefreshToken: session.Token.RefreshToken,
		Expiry:       session.Token.Expiry,
	}, nil
}

func (s storedOAuthSession) validate() error {
	if s.Version != oauthSessionVersion {
		return fmt.Errorf("mcp oauth: unsupported session version %d", s.Version)
	}
	if err := s.Config.validate(); err != nil {
		return err
	}
	return s.Token.validate()
}

func (s storedOAuthConfig) validate() error {
	if s.ClientID == "" {
		return errors.New("mcp oauth: stored client id is empty")
	}
	if err := validateStoredOAuthURL("authorization URL", s.AuthURL); err != nil {
		return err
	}
	if err := validateStoredOAuthURL("token URL", s.TokenURL); err != nil {
		return err
	}
	return validateStoredOAuthURL("redirect URL", s.RedirectURL)
}

func (s storedOAuthToken) validate() error {
	if s.AccessToken == "" {
		return errors.New("mcp oauth: stored access token is empty")
	}
	return nil
}

func validateStoredOAuthURL(field, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("mcp oauth: stored %s is not an absolute HTTP(S) URL", field)
	}
	return nil
}

type oauthSession struct {
	store   OAuthSessionStore
	server  mcpserver.ID
	binding string
}

func (s *oauthSession) save(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) error {
	payload, err := encodeOAuthSession(cfg, token)
	if err != nil {
		return err
	}
	if err := s.store.SaveOAuthSession(ctx, s.server, s.binding, payload); err != nil {
		return fmt.Errorf("mcp oauth: persist session for %q: %w", s.server, err)
	}
	return nil
}

func (s *oauthSession) remove(ctx context.Context) error {
	return s.store.RemoveOAuthSession(ctx, s.server, s.binding)
}

type savingTokenSource struct {
	mu     sync.Mutex
	source oauth2.TokenSource
	config oauth2.Config
	last   *oauth2.Token
	save   func(*oauth2.Config, *oauth2.Token) error
}

type invalidatingTokenSource struct {
	mu          sync.Mutex
	source      oauth2.TokenSource
	lifetime    context.Context
	session     *oauthSession
	invalidated bool
}

func newSavingTokenSource(source oauth2.TokenSource, cfg *oauth2.Config, token *oauth2.Token, save func(*oauth2.Config, *oauth2.Token) error) oauth2.TokenSource {
	config := *cfg
	config.Scopes = slices.Clone(cfg.Scopes)
	return &savingTokenSource{source: source, config: config, last: optional.Clone(token), save: save}
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, err := s.source.Token()
	if err != nil {
		return nil, err
	}
	if sameOAuthToken(s.last, token) {
		return token, nil
	}
	if err := s.save(&s.config, token); err != nil {
		return nil, err
	}
	s.last = optional.Clone(token)
	return token, nil
}

func sameOAuthToken(left, right *oauth2.Token) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.AccessToken == right.AccessToken &&
		left.TokenType == right.TokenType &&
		left.RefreshToken == right.RefreshToken &&
		left.Expiry.Equal(right.Expiry)
}

func (i *invalidatingTokenSource) Token() (*oauth2.Token, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.invalidated {
		return nil, errors.Join(mcpserver.ErrAuthorizationRequired, errStoredOAuthRejected)
	}
	token, err := i.source.Token()
	if err == nil || !oauthCredentialRejected(err) {
		return token, err
	}
	i.invalidated = true
	removeErr := i.session.remove(i.lifetime)
	return nil, errors.Join(mcpserver.ErrAuthorizationRequired, errStoredOAuthRejected, err, removeErr)
}

func oauthCredentialRejected(err error) bool {
	retrieveErr, ok := errors.AsType[*oauth2.RetrieveError](err)
	if !ok {
		return false
	}
	switch retrieveErr.ErrorCode {
	case "invalid_grant", "invalid_client", "unauthorized_client":
		return true
	}
	return retrieveErr.Response != nil &&
		(retrieveErr.Response.StatusCode == http.StatusUnauthorized ||
			retrieveErr.Response.StatusCode == http.StatusForbidden)
}

func invalidateRejectedTokens(
	source oauth2.TokenSource,
	lifetime context.Context,
	session *oauthSession,
) oauth2.TokenSource {
	if session == nil {
		return source
	}
	return &invalidatingTokenSource{
		source: source, lifetime: lifetime, session: session,
	}
}

// restoredOAuthHandler deliberately does not start an interactive browser flow
// during boot. A rejected credential is removed and the dial is classified as
// needsAuth; the explicit Authorize command then owns user interaction.
type restoredOAuthHandler struct {
	mu      sync.RWMutex
	source  oauth2.TokenSource
	session *oauthSession
}

var _ auth.OAuthHandler = (*restoredOAuthHandler)(nil)

func (r *restoredOAuthHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.source, nil
}

func (r *restoredOAuthHandler) Authorize(ctx context.Context, _ *http.Request, response *http.Response) error {
	var responseErr error
	if response != nil && response.Body != nil {
		responseErr = response.Body.Close()
	}
	r.mu.Lock()
	r.source = nil
	r.mu.Unlock()
	removeErr := r.session.remove(ctx)
	return errors.Join(errStoredOAuthRejected, responseErr, removeErr)
}

func restoreOAuthHandler(
	ctx context.Context,
	lifetime context.Context,
	store OAuthSessionStore,
	target mcpserver.OAuthTarget,
) (auth.OAuthHandler, error) {
	if store == nil {
		return nil, nil
	}
	if lifetime == nil {
		return nil, errors.New("mcp oauth: lifetime is required")
	}
	payload, binding, found, err := store.LoadOAuthSession(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("mcp oauth: load session for %q: %w", target.ID(), err)
	}
	if !found {
		return nil, nil
	}
	cfg, token, err := decodeOAuthSession(payload)
	if err != nil {
		return nil, fmt.Errorf("mcp oauth: restore session for %q: %w", target.ID(), err)
	}
	session := &oauthSession{store: store, server: target.ID(), binding: binding}
	if token.RefreshToken == "" && !token.Valid() {
		if err := session.remove(ctx); err != nil {
			return nil, fmt.Errorf("mcp oauth: remove expired session for %q: %w", target.ID(), err)
		}
		return nil, nil
	}
	source := newSavingTokenSource(
		cfg.TokenSource(lifetime, token),
		cfg,
		token,
		func(updatedConfig *oauth2.Config, updatedToken *oauth2.Token) error {
			return session.save(lifetime, updatedConfig, updatedToken)
		},
	)
	source = invalidateRejectedTokens(source, lifetime, session)
	return &restoredOAuthHandler{source: source, session: session}, nil
}
