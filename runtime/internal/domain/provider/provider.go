// Package provider owns model-provider identity, credential provenance,
// endpoint configuration, and update semantics. Persistence, environment
// lookup, model metadata, and client construction remain outside this package.
package provider

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

var (
	ErrIDRequired      = errors.New("provider: id is required")
	ErrIDInvalid       = errors.New("provider: id is invalid")
	ErrAPIKeyRequired  = errors.New("provider: API key is required")
	ErrBaseURLInvalid  = errors.New("provider: base URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	ErrChangeCorrupted = errors.New("provider: change is corrupted")
)

// APIKey is a validated opaque provider credential. It deliberately exposes
// no String method so formatting a Provider cannot accidentally print a key.
type APIKey struct {
	value string
}

func NewAPIKey(value string) (APIKey, error) {
	if strings.TrimSpace(value) == "" {
		return APIKey{}, ErrAPIKeyRequired
	}
	return APIKey{value: value}, nil
}

func (k APIKey) Present() bool { return k.value != "" }

// Reveal is an explicit sensitive-data boundary. Callers must not log or
// project its result.
func (k APIKey) Reveal() string { return k.value }

// BaseURL is an optional validated provider endpoint. Its zero value means the
// provider-owned default endpoint, rather than an empty-string convention.
type BaseURL struct {
	value string
}

func NewBaseURL(value string) (BaseURL, error) {
	if strings.TrimSpace(value) != value || value == "" || strings.ContainsAny(value, "?#\\") ||
		strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return BaseURL{}, ErrBaseURLInvalid
	}
	scheme, remainder, found := strings.Cut(value, "://")
	scheme = strings.ToLower(scheme)
	if !found || (scheme != "http" && scheme != "https") {
		return BaseURL{}, ErrBaseURLInvalid
	}
	authority, path, hasPath := strings.Cut(remainder, "/")
	if !validHTTPAuthority(authority) {
		return BaseURL{}, ErrBaseURLInvalid
	}
	normalized := scheme + "://" + strings.ToLower(authority)
	if hasPath {
		normalized += "/" + path
	}
	return BaseURL{value: strings.TrimRight(normalized, "/")}, nil
}

const maximumTCPPort = 65_535

func validHTTPAuthority(authority string) bool {
	if authority == "" || strings.Contains(authority, "@") {
		return false
	}
	if strings.HasPrefix(authority, "[") {
		closingBracket := strings.IndexByte(authority, ']')
		if closingBracket <= 1 || !strings.Contains(authority[1:closingBracket], ":") {
			return false
		}
		suffix := authority[closingBracket+1:]
		return suffix == "" || (strings.HasPrefix(suffix, ":") && validPort(suffix[1:]))
	}
	host, port, hasPort := strings.Cut(authority, ":")
	if host == "" || strings.ContainsAny(host, "[]") || strings.Contains(port, ":") {
		return false
	}
	return !hasPort || validPort(port)
}

func validPort(raw string) bool {
	port, err := strconv.Atoi(raw)
	return err == nil && port > 0 && port <= maximumTCPPort
}

func (u BaseURL) Present() bool  { return u.value != "" }
func (u BaseURL) String() string { return u.value }

// KeySource is the closed set of credential origins. Credential absence is
// represented by an unconfigured Credential, never by an empty KeySource.
type KeySource string

const (
	KeyStored      KeySource = "stored"
	KeyEnvironment KeySource = "environment"
)

// Credential keeps the secret and its provenance inseparable. Its zero value
// is the explicit unconfigured state.
type Credential struct {
	key    APIKey
	source KeySource
}

func StoredCredential(key APIKey) Credential {
	if !key.Present() {
		return Credential{}
	}
	return Credential{key: key, source: KeyStored}
}

func EnvironmentCredential(key APIKey) Credential {
	if !key.Present() {
		return Credential{}
	}
	return Credential{key: key, source: KeyEnvironment}
}

func (c Credential) Configured() bool { return c.key.Present() }

func (c Credential) APIKey() (APIKey, bool) {
	return c.key, c.Configured()
}

func (c Credential) Source() (KeySource, bool) {
	if !c.Configured() {
		var absent KeySource
		return absent, false
	}
	return c.source, true
}

// Provider is one registry entry. Its fields are private so invalid primitive
// combinations cannot leak between the Domain, storage, protocol, and UI.
type Provider struct {
	id         modelref.ProviderIdentity
	credential Credential
	baseURL    BaseURL
}

func New(id string) (Provider, error) {
	if strings.TrimSpace(id) == "" {
		return Provider{}, ErrIDRequired
	}
	identity, err := modelref.NewProviderIdentity(id)
	if err != nil {
		return Provider{}, fmt.Errorf("%w: %v", ErrIDInvalid, err)
	}
	return Provider{id: identity}, nil
}

func (p Provider) ID() string { return p.id.String() }
func (p Provider) BaseURL() (BaseURL, bool) {
	return p.baseURL, p.baseURL.Present()
}
func (p Provider) Credential() (Credential, bool) {
	return p.credential, p.credential.Configured()
}
func (p Provider) APIKey() (APIKey, bool) {
	return p.credential.APIKey()
}

// WithEnvironmentFallback supplies an environment credential only when no
// stored credential exists. Stored credentials therefore win by construction.
func (p Provider) WithEnvironmentFallback(key APIKey) Provider {
	if p.credential.Configured() || !key.Present() {
		return p
	}
	p.credential = EnvironmentCredential(key)
	return p
}

type changeKind uint8

const (
	changePreserve changeKind = iota
	changeSet
	changeClear
)

// Change models preserve, set, and clear as distinct states. This removes the
// former pointer/empty-string convention from every update boundary.
type Change[T any] struct {
	kind  changeKind
	value T
}

func Preserve[T any]() Change[T] { return Change[T]{} }
func Set[T any](value T) Change[T] {
	return Change[T]{kind: changeSet, value: value}
}
func Clear[T any]() Change[T] { return Change[T]{kind: changeClear} }

func (c Change[T]) Empty() bool { return c.kind == changePreserve }

// Resolve applies the change to current without exposing its internal tag.
func (c Change[T]) Resolve(current T) (T, error) {
	switch c.kind {
	case changePreserve:
		return current, nil
	case changeSet:
		return c.value, nil
	case changeClear:
		var cleared T
		return cleared, nil
	default:
		var cleared T
		return cleared, ErrChangeCorrupted
	}
}

// Patch is an atomic partial update to persisted provider configuration.
type Patch struct {
	APIKey  Change[APIKey]
	BaseURL Change[BaseURL]
}

func (p Patch) Empty() bool { return p.APIKey.Empty() && p.BaseURL.Empty() }

func (p Patch) validate() error {
	if p.APIKey.kind == changeSet && !p.APIKey.value.Present() {
		return fmt.Errorf("%w: set API key", ErrChangeCorrupted)
	}
	if p.BaseURL.kind == changeSet && !p.BaseURL.value.Present() {
		return fmt.Errorf("%w: set base URL", ErrChangeCorrupted)
	}
	if p.APIKey.kind > changeClear || p.BaseURL.kind > changeClear {
		return ErrChangeCorrupted
	}
	return nil
}

// Apply returns a new Provider with patch applied. Environment credentials are
// intentionally converted to stored credentials only by an explicit key set;
// the environment registry never persists its effective overlay.
func (p Provider) Apply(patch Patch) (Provider, error) {
	if err := patch.validate(); err != nil {
		return Provider{}, err
	}
	switch patch.APIKey.kind {
	case changePreserve:
	case changeSet:
		p.credential = StoredCredential(patch.APIKey.value)
	case changeClear:
		p.credential = Credential{}
	default:
		return Provider{}, ErrChangeCorrupted
	}
	baseURL, err := patch.BaseURL.Resolve(p.baseURL)
	if err != nil {
		return Provider{}, err
	}
	p.baseURL = baseURL
	return p, nil
}
