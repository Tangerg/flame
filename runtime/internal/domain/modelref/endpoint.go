package modelref

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/scope/core/chat"
)

// EndpointInvocation is admission of one real provider request, distinct from
// its logical calling operation. Its immutable input is persisted before I/O.
type EndpointInvocation struct {
	ID        string
	Caller    capability.Invocation
	Provider  string
	Model     string
	SessionID string
	Request   *chat.Request
	StartedAt time.Time
}

func (i EndpointInvocation) Validate() error {
	if err := identity.ValidateEffect(i.ID); err != nil {
		return err
	}
	if err := i.Caller.Validate(); err != nil {
		return err
	}
	if i.Caller.Target.Scope != "call" || i.Caller.Target.Reference != i.Caller.ID || i.Caller.ID == i.ID || i.StartedAt.IsZero() {
		return errors.New("models: endpoint invocation requires a distinct calling model operation")
	}
	if _, err := New(i.Provider, i.Model); err != nil {
		return err
	}
	if i.SessionID != "" {
		if err := resourceid.ValidateSession(i.SessionID); err != nil {
			return err
		}
	}
	return i.Request.Validate()
}

type EndpointOutcome struct {
	CostUSD    *float64
	Response   *chat.Response
	FinishedAt time.Time
	Failure    string
}

type EndpointObservation struct {
	CostUSD        *float64
	SessionID      string
	RunID          string
	Invocation     EndpointInvocation
	Response       *chat.Response
	State          string
	Failure        string
	FinishedAt     time.Time
	InvocationJSON jsoninput.Input
	RequestJSON    jsoninput.Input
	ResponseJSON   jsoninput.Input
}

func (o EndpointObservation) Clone() EndpointObservation {
	o.Invocation.Request = o.Invocation.Request.Clone()
	o.Response = o.Response.Clone()
	if o.CostUSD != nil {
		o.CostUSD = new(*o.CostUSD)
	}
	return o
}
