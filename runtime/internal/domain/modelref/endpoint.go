package modelref

import (
	"errors"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/resourceid"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
)

// EndpointKind names the provider operation a physical endpoint records. Chat
// and embedding share one admission/evidence/accounting owner; each kind carries
// exactly its own request and response payload.
type EndpointKind string

const (
	EndpointChat      EndpointKind = "chat"
	EndpointEmbedding EndpointKind = "embedding"
)

func (k EndpointKind) Validate() error {
	switch k {
	case EndpointChat, EndpointEmbedding:
		return nil
	default:
		return errors.New("models: unknown endpoint kind")
	}
}

// EndpointInvocation is admission of one real provider request, distinct from
// its logical calling operation. Its immutable input is persisted before I/O.
// Kind selects which request payload is present; the other is nil.
type EndpointInvocation struct {
	ID        string
	Kind      EndpointKind
	Caller    capability.Invocation
	Provider  string
	Model     string
	SessionID string
	Request   *chat.Request
	Embedding *embedding.Request
	StartedAt time.Time
}

func (i EndpointInvocation) Validate() error {
	if err := identity.ValidateEffect(i.ID); err != nil {
		return err
	}
	if err := i.Caller.Validate(); err != nil {
		return err
	}
	if i.Caller.Target.Scope != capability.ScopeCall || i.Caller.Target.Reference != i.Caller.ID || i.Caller.ID == i.ID || i.StartedAt.IsZero() {
		return errors.New("models: endpoint invocation requires a distinct calling model operation")
	}
	if _, err := modelchoice.New(i.Provider, i.Model); err != nil {
		return err
	}
	if i.SessionID != "" {
		if err := resourceid.ValidateSession(i.SessionID); err != nil {
			return err
		}
	}
	switch i.Kind {
	case EndpointChat:
		if i.Request == nil || i.Embedding != nil {
			return errors.New("models: chat endpoint requires exactly a chat request")
		}
		return i.Request.Validate()
	case EndpointEmbedding:
		if i.Embedding == nil || i.Request != nil {
			return errors.New("models: embedding endpoint requires exactly an embedding request")
		}
		return i.Embedding.Validate()
	default:
		return errors.New("models: endpoint invocation has an unknown kind")
	}
}

// EndpointOutcome settles one admitted request. Response carries chat evidence
// and EmbeddingResponse carries embedding evidence; at most one is present and
// it matches the invocation kind. A missing response means uncertainty.
type EndpointOutcome struct {
	CostUSD           *float64
	Response          *chat.Response
	EmbeddingResponse *embedding.Response
	FinishedAt        time.Time
	Failure           string
}

// EndpointObservation is a settled endpoint projection. ServedModel and Usage
// are the normalized accounting facts both kinds expose, so recovery, reporting
// and delivery never decode a kind-specific response. Response holds the chat
// evidence the primary-conversation recovery needs; embedding evidence stays in
// ResponseJSON. Usage is nil exactly when the provider reported no usage.
type EndpointObservation struct {
	CostUSD        *float64
	Usage          *chat.Usage
	SessionID      string
	RunID          string
	SegmentID      string
	ServedModel    string
	Invocation     EndpointInvocation
	Response       *chat.Response
	State          string
	Failure        string
	FinishedAt     time.Time
	InvocationJSON jsoninput.Input
	RequestJSON    jsoninput.Input
	ResponseJSON   jsoninput.Input
}

// HasResponse reports whether known provider evidence was observed. It is the
// kind-agnostic signal consumers use instead of a chat-specific response pointer.
func (o EndpointObservation) HasResponse() bool {
	return len(o.ResponseJSON.String()) > 0
}

func (o EndpointObservation) Clone() EndpointObservation {
	if o.Invocation.Request != nil {
		o.Invocation.Request = o.Invocation.Request.Clone()
	}
	if o.Invocation.Embedding != nil {
		o.Invocation.Embedding = cloneEmbeddingRequest(o.Invocation.Embedding)
	}
	o.Response = o.Response.Clone()
	if o.CostUSD != nil {
		o.CostUSD = new(*o.CostUSD)
	}
	if o.Usage != nil {
		usage := o.Usage.Clone()
		o.Usage = &usage
	}
	return o
}

func cloneEmbeddingRequest(r *embedding.Request) *embedding.Request {
	return &embedding.Request{Texts: slices.Clone(r.Texts), Options: r.Options.Clone()}
}
