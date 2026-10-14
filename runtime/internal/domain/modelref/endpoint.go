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
	ID        string                `json:"id"`
	Caller    capability.Invocation `json:"caller"`
	Provider  string                `json:"provider"`
	Model     string                `json:"model"`
	SessionID string                `json:"sessionId,omitzero"`
	Request   *chat.Request         `json:"request"`
	StartedAt time.Time             `json:"startedAt"`
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
	Response   *chat.Response
	FinishedAt time.Time
	Failure    string
}

type EndpointObservation struct {
	Invocation   EndpointInvocation
	Response     *chat.Response
	State        string
	Failure      string
	FinishedAt   time.Time
	RequestJSON  jsoninput.Input
	ResponseJSON jsoninput.Input
}
