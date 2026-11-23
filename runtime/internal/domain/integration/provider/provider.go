package provider

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/secret"
)

var (
	errIDRequired      = errors.New("provider: id is required")
	errIDInvalid       = errors.New("provider: id is invalid")
	ErrAPIKeyRequired  = errors.New("provider: API key is required")
	errAPIKeyInvalid   = errors.New("provider: API key cannot have surrounding whitespace or contain control characters")
	errBaseURLInvalid  = errors.New("provider: base URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	errChangeCorrupted = errors.New("provider: change is corrupted")
)

// No String method, so formatting a Provider cannot print a key.
type APIKey struct {
	value string
}

func NewAPIKey(value string) (APIKey, error) {
	if strings.TrimSpace(value) == "" {
		return APIKey{}, ErrAPIKeyRequired
	}
	if strings.TrimSpace(value) != value || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return APIKey{}, errAPIKeyInvalid
	}
	return APIKey{value: value}, nil
}

func (k APIKey) Present() bool { return k.value != "" }

func (k APIKey) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte(secret.Redacted))
}

// Reveal is an explicit sensitive-data boundary. Callers must not log or
// project its result.
func (k APIKey) Reveal() string { return k.value }

type BaseURL struct {
	value string
}

func NewBaseURL(value string) (BaseURL, error) {
	if strings.ContainsRune(value, '\\') ||
		strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return BaseURL{}, errBaseURLInvalid
	}
	endpoint, err := httporigin.ParseEndpoint(value)
	if err != nil || endpoint.HasQuery() || endpoint.HasFragment() {
		return BaseURL{}, errBaseURLInvalid
	}
	parsed := endpoint.URL()
	parsed.Host = strings.ToLower(parsed.Host)
	normalized := parsed.String()
	return BaseURL{value: strings.TrimRight(normalized, "/")}, nil
}

func (u BaseURL) Present() bool  { return u.value != "" }
func (u BaseURL) String() string { return u.value }

type KeySource string

const (
	KeyStored      KeySource = "stored"
	KeyEnvironment KeySource = "environment"
)

type Credential struct {
	key    APIKey
	source KeySource
}

func (c Credential) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte(secret.Presence(c.Configured())))
}

func storedCredential(key APIKey) Credential {
	if !key.Present() {
		return Credential{}
	}
	return Credential{key: key, source: KeyStored}
}

func environmentCredential(key APIKey) Credential {
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

type Provider struct {
	id         flamemodel.ProviderIdentity
	credential Credential
	baseURL    BaseURL
}

func (p Provider) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(
		state,
		"Provider{id:%q, credential:%v, baseURL:%q}",
		p.ID(),
		p.credential,
		p.baseURL.String(),
	)
}

func New(id string) (Provider, error) {
	if strings.TrimSpace(id) == "" {
		return Provider{}, errIDRequired
	}
	identity, err := flamemodel.NewProviderIdentity(id)
	if err != nil {
		return Provider{}, fmt.Errorf("%w: %w", errIDInvalid, err)
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

func (p Provider) WithEnvironmentFallback(key APIKey) Provider {
	if p.credential.Configured() || !key.Present() {
		return p
	}
	p.credential = environmentCredential(key)
	return p
}

type changeKind uint8

const (
	changePreserve changeKind = iota
	changeSet
	changeClear
)

type Change[T any] struct {
	kind  changeKind
	value T
}

func (c Change[T]) Format(state fmt.State, _ rune) {
	label := "invalid"
	switch c.kind {
	case changePreserve:
		label = "preserve"
	case changeSet:
		label = "set"
	case changeClear:
		label = "clear"
	}
	_, _ = fmt.Fprintf(state, "Change{%s}", label)
}

func Preserve[T any]() Change[T] { return Change[T]{} }

func Set[T any](value T) Change[T] {
	return Change[T]{kind: changeSet, value: value}
}

func Clear[T any]() Change[T] { return Change[T]{kind: changeClear} }

func (c Change[T]) Empty() bool { return c.kind == changePreserve }

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
		return cleared, errChangeCorrupted
	}
}

type Patch struct {
	APIKey  Change[APIKey]
	BaseURL Change[BaseURL]
}

func (p Patch) Empty() bool { return p.APIKey.Empty() && p.BaseURL.Empty() }

func (p Patch) validate() error {
	if p.APIKey.kind == changeSet && !p.APIKey.value.Present() {
		return fmt.Errorf("%w: set API key", errChangeCorrupted)
	}
	if p.BaseURL.kind == changeSet && !p.BaseURL.value.Present() {
		return fmt.Errorf("%w: set base URL", errChangeCorrupted)
	}
	if p.APIKey.kind > changeClear || p.BaseURL.kind > changeClear {
		return errChangeCorrupted
	}
	return nil
}

func (p Provider) Apply(patch Patch) (Provider, error) {
	if err := patch.validate(); err != nil {
		return Provider{}, err
	}
	switch patch.APIKey.kind {
	case changePreserve:
	case changeSet:
		p.credential = storedCredential(patch.APIKey.value)
	case changeClear:
		p.credential = Credential{}
	default:
		return Provider{}, errChangeCorrupted
	}
	baseURL, err := patch.BaseURL.Resolve(p.baseURL)
	if err != nil {
		return Provider{}, err
	}
	p.baseURL = baseURL
	return p, nil
}
