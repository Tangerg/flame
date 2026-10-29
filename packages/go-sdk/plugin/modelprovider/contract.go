// Package modelprovider publishes the installed model-adapter boundary.
// The caller retains selection, admission, accounting and recovery authority.
package modelprovider

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/models/catalog"
)

const Reference = "flame.model-provider.v1"

type Authentication string

const (
	KeyRequired Authentication = "keyRequired"
	KeyOptional Authentication = "keyOptional"
)

func (Authentication) JSONSchemaEnum() []string {
	return []string{string(KeyRequired), string(KeyOptional)}
}

type ModelSource string

const (
	BundledModels  ModelSource = "bundled"
	EndpointModels ModelSource = "endpoint"
)

func (ModelSource) JSONSchemaEnum() []string {
	return []string{string(BundledModels), string(EndpointModels)}
}

type EmbeddingProfile struct {
	DefaultModel string `json:"defaultModel,omitempty"`
}

type Profile struct {
	ID               string            `json:"id"`
	Authentication   Authentication    `json:"authentication"`
	EndpointRequired bool              `json:"endpointRequired"`
	ModelSource      ModelSource       `json:"modelSource"`
	DefaultModel     string            `json:"defaultModel,omitempty"`
	Embedding        *EmbeddingProfile `json:"embedding,omitzero"`
	Models           []catalog.Model   `json:"models"`
}

func (p Profile) Validate() error {
	if _, err := modelchoice.NewProviderIdentity(p.ID); err != nil {
		return err
	}
	if p.Authentication != KeyRequired && p.Authentication != KeyOptional {
		return errors.New("model provider: invalid authentication policy")
	}
	switch p.ModelSource {
	case BundledModels:
		if _, err := modelchoice.NewModelIdentity(p.DefaultModel); err != nil {
			return err
		}
	case EndpointModels:
		if p.DefaultModel != "" {
			return errors.New("model provider: endpoint-owned models cannot declare a bundled default")
		}
	default:
		return errors.New("model provider: invalid model source")
	}
	if p.Embedding != nil && p.Embedding.DefaultModel != "" {
		if _, err := modelchoice.NewModelIdentity(p.Embedding.DefaultModel); err != nil {
			return err
		}
	}
	seen := make(map[string]struct{}, len(p.Models))
	for _, model := range p.Models {
		if err := model.Validate(); err != nil {
			return err
		}
		if _, err := modelchoice.NewModelIdentity(model.ID); err != nil {
			return err
		}
		if _, exists := seen[model.ID]; exists {
			return errors.New("model provider: repeated catalog identity")
		}
		seen[model.ID] = struct{}{}
	}
	return nil
}

// Connection is an immutable credential snapshot, not another mutable registry.
// It crosses only the private installed-backend channel.
type Connection struct {
	Provider string `json:"provider"`
	APIKey   string `json:"apiKey,omitempty"`
	BaseURL  string `json:"baseURL,omitempty"`
}

func (c Connection) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "Connection{Provider:%q, CredentialPresent:%t, EndpointPresent:%t}", c.Provider, c.APIKey != "", c.BaseURL != "")
}

func (c Connection) Validate() error {
	if _, err := modelchoice.NewProviderIdentity(c.Provider); err != nil {
		return err
	}
	if strings.ContainsFunc(c.APIKey, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return errors.New("model provider: credential contains invalid characters")
	}
	if c.BaseURL == "" {
		return nil
	}
	endpoint, err := url.Parse(c.BaseURL)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return errors.New("model provider: endpoint must be an absolute HTTP(S) URL without userinfo or fragment")
	}
	return nil
}

type Open struct {
	Connection Connection `json:"connection"`
	Model      string     `json:"model"`
}

func (o Open) Validate() error {
	if err := o.Connection.Validate(); err != nil {
		return err
	}
	_, err := modelchoice.NewModelIdentity(o.Model)
	return err
}

type Handle struct {
	ID string `json:"id" jsonschema:"pattern=^[a-f0-9]{64}$"`
}

func (h Handle) Validate() error {
	if len(h.ID) != 64 || strings.ContainsFunc(h.ID, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') }) {
		return errors.New("model provider: invalid model handle")
	}
	return nil
}

type Chat struct {
	Handle
	Original      string         `json:"original" jsonschema:"pattern=\\S,maxLength=256"`
	Provider      string         `json:"provider"`
	Model         string         `json:"model"`
	Streaming     bool           `json:"streaming"`
	TokenCounting bool           `json:"tokenCounting"`
	Metadata      *catalog.Model `json:"metadata,omitzero"`
}

func (c Chat) Validate() error {
	if _, err := modelchoice.NewProviderIdentity(c.Provider); err != nil {
		return err
	}
	if _, err := modelchoice.NewModelIdentity(c.Model); err != nil {
		return err
	}
	if err := errors.Join(c.Handle.Validate(), qimumcp.ValidateCommandID(c.Original)); err != nil {
		return err
	}
	if c.Metadata != nil {
		if err := c.Metadata.Validate(); err != nil {
			return err
		}
		if c.Metadata.ID != c.Model {
			return errors.New("model provider: metadata belongs to another model")
		}
	}
	return nil
}

type Reopen struct {
	Original string `json:"original" jsonschema:"pattern=\\S,maxLength=256"`
}

func (r Reopen) Validate() error { return qimumcp.ValidateCommandID(r.Original) }

type Embedder struct {
	Handle
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Space    string `json:"space" jsonschema:"pattern=\\S"`
}

func (e Embedder) Validate() error {
	if err := e.Handle.Validate(); err != nil {
		return err
	}
	if _, err := modelchoice.NewProviderIdentity(e.Provider); err != nil {
		return err
	}
	if _, err := modelchoice.NewModelIdentity(e.Model); err != nil {
		return err
	}
	if strings.TrimSpace(e.Space) == "" {
		return errors.New("model provider: vector space is required")
	}
	return nil
}

type ChatRequest struct {
	Handle  Handle       `json:"handle"`
	Request chat.Request `json:"request"`
}

func (r ChatRequest) Validate() error {
	return errors.Join(r.Handle.Validate(), r.Request.Validate())
}

type EmbeddingRequest struct {
	Handle  Handle         `json:"handle"`
	Request jsontext.Value `json:"request"`
}

// Scope's embedding codec owns admission of this complete envelope. Keeping it
// opaque avoids deriving a competing schema for a custom native codec.
func (r EmbeddingRequest) Native() (*embedding.Request, error) {
	if err := r.Handle.Validate(); err != nil {
		return nil, err
	}
	var request embedding.Request
	if err := json.Unmarshal(r.Request, &request, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	return &request, request.Validate()
}

type FailureKind string

const (
	Internal            FailureKind = "internal"
	Canceled            FailureKind = "canceled"
	Timeout             FailureKind = "timeout"
	InvalidCredentials  FailureKind = "invalid_credentials"
	RateLimited         FailureKind = "rate_limited"
	ProviderRejected    FailureKind = "provider_rejected"
	ProviderUnavailable FailureKind = "provider_unavailable"
)

func (FailureKind) JSONSchemaEnum() []string {
	return []string{string(Internal), string(Canceled), string(Timeout), string(InvalidCredentials), string(RateLimited), string(ProviderRejected), string(ProviderUnavailable)}
}

// Failure preserves execution evidence separately from the failure category.
// No category or receipt authorizes another provider dispatch.
type Failure struct {
	Kind             FailureKind          `json:"kind"`
	Execution        chat.CallFailureKind `json:"execution"`
	RetryAfterMillis int64                `json:"retryAfterMillis,omitzero" jsonschema:"minimum=0"`
}

func (f Failure) Validate() error {
	if !slices.Contains(f.Kind.JSONSchemaEnum(), string(f.Kind)) || !f.Execution.Valid() || f.RetryAfterMillis < 0 {
		return errors.New("model provider: invalid failure evidence")
	}
	return nil
}

type ChatResult struct {
	Response *chat.Response `json:"response,omitzero"`
	Failure  *Failure       `json:"failure,omitzero"`
}

type EmbeddingResult struct {
	Response jsontext.Value `json:"response,omitzero"`
	Failure  *Failure       `json:"failure,omitzero"`
}

type TokenCount struct {
	Count   *int64   `json:"count,omitzero" jsonschema:"minimum=0"`
	Failure *Failure `json:"failure,omitzero"`
}

func (r ChatResult) Validate() error {
	if r.Failure != nil {
		return r.Failure.Validate()
	}
	if r.Response == nil {
		return errors.New("model provider: missing chat outcome")
	}
	return r.Response.Validate()
}

func (r EmbeddingResult) Validate() error {
	if r.Failure != nil {
		return r.Failure.Validate()
	}
	if r.Response == nil {
		return errors.New("model provider: missing embedding outcome")
	}
	_, err := r.Native()
	return err
}

func (r EmbeddingResult) Native() (*embedding.Response, error) {
	if r.Response == nil {
		return nil, nil
	}
	var response embedding.Response
	if err := json.Unmarshal(r.Response, &response, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	return &response, response.Validate()
}

func (r TokenCount) Validate() error {
	if r.Failure != nil {
		if r.Count != nil {
			return errors.New("model provider: conflicting token count outcomes")
		}
		return r.Failure.Validate()
	}
	if r.Count == nil || *r.Count < 0 {
		return errors.New("model provider: missing or negative token count")
	}
	return nil
}

func ProfilesMethod() (*qimumcp.Method[struct{}, []Profile], error) {
	return qimumcp.NewMethod[struct{}, []Profile](qimumcp.MethodConfig{Name: "flame.model-provider/profiles.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read this installed adapter's provider policies and published model facts."})
}

func RemoteModelsMethod() (*qimumcp.Method[Connection, []string], error) {
	return qimumcp.NewMethod[Connection, []string](qimumcp.MethodConfig{Name: "flame.model-provider/remote-models.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read model identities from the exact configured provider endpoint."})
}

func OpenChatMethod() (*qimumcp.Method[Open, Chat], error) {
	return qimumcp.NewMethod[Open, Chat](qimumcp.MethodConfig{Name: "flame.model-provider/open-chat.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Capture one immutable provider client shared by chat, streaming and optional token counting."})
}

func OpenEmbeddingMethod() (*qimumcp.Method[Open, Embedder], error) {
	return qimumcp.NewMethod[Open, Embedder](qimumcp.MethodConfig{Name: "flame.model-provider/open-embedding.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Capture an immutable embedding client and its vector-space identity."})
}

func ReopenChatMethod() (*qimumcp.Method[Reopen, Chat], error) {
	return qimumcp.NewMethod[Reopen, Chat](qimumcp.MethodConfig{Name: "flame.model-provider/reopen-chat.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Reconstruct a fresh client from the original confirmed open receipt, without reading current credentials or authorizing generation."})
}

func CloseMethod() (*qimumcp.Method[Handle, struct{}], error) {
	return qimumcp.NewMethod[Handle, struct{}](qimumcp.MethodConfig{Name: "flame.model-provider/close.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Retire the captured client after its accepted calls have joined."})
}

func CallMethod() (*qimumcp.Method[ChatRequest, ChatResult], error) {
	return qimumcp.NewMethod[ChatRequest, ChatResult](qimumcp.MethodConfig{Name: "flame.model-provider/call.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Execute one original provider exchange using the captured client and preserve its complete outcome."})
}

func StreamMethod() (*qimumcp.StreamingMethod[ChatRequest, chat.ResponseDelta, ChatResult], error) {
	return qimumcp.NewStreamingMethod[ChatRequest, chat.ResponseDelta, ChatResult](qimumcp.MethodConfig{Name: "flame.model-provider/stream.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Stream the original provider deltas and retain the final outcome without replaying an unconfirmed exchange."})
}

func CountMethod() (*qimumcp.Method[ChatRequest, TokenCount], error) {
	return qimumcp.NewMethod[ChatRequest, TokenCount](qimumcp.MethodConfig{Name: "flame.model-provider/count.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Count the complete request using the same captured provider instance, only when that capability was advertised."})
}

func EmbedMethod() (*qimumcp.Method[EmbeddingRequest, EmbeddingResult], error) {
	return qimumcp.NewMethod[EmbeddingRequest, EmbeddingResult](qimumcp.MethodConfig{Name: "flame.model-provider/embed.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Execute one original embedding exchange and preserve the complete provider response."})
}

func Contract() (*portable.Capability, error) {
	profiles, err := ProfilesMethod()
	if err != nil {
		return nil, err
	}
	remote, err := RemoteModelsMethod()
	if err != nil {
		return nil, err
	}
	openChat, err := OpenChatMethod()
	if err != nil {
		return nil, err
	}
	openEmbedding, err := OpenEmbeddingMethod()
	if err != nil {
		return nil, err
	}
	reopenChat, err := ReopenChatMethod()
	if err != nil {
		return nil, err
	}
	close, err := CloseMethod()
	if err != nil {
		return nil, err
	}
	call, err := CallMethod()
	if err != nil {
		return nil, err
	}
	stream, err := StreamMethod()
	if err != nil {
		return nil, err
	}
	count, err := CountMethod()
	if err != nil {
		return nil, err
	}
	embed, err := EmbedMethod()
	if err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: Reference, Composition: portable.Selection, Scope: extension.ScopeInstallation}, profiles.Descriptor(), remote.Descriptor(), openChat.Descriptor(), openEmbedding.Descriptor(), reopenChat.Descriptor(), close.Descriptor(), call.Descriptor(), stream.Descriptor(), count.Descriptor(), embed.Descriptor())
}
