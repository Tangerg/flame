// Package pluginbrowser defines Flame's trusted browser composition boundary.
// Frame owns admission and execution; Flame supplies component authoring policy.
package pluginbrowser

import (
	framebrowser "github.com/Tangerg/frame/browser"
	framevalue "github.com/Tangerg/frame/value"

	"context"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"
)

type Session struct {
	Authority           framebrowser.Authority
	Realm               framebrowser.Realm
	Credential          string
	ControlEndpoint     string
	MaximumMessageBytes int
	ProofCapacity       int
	PreparationCapacity int
	PollInterval        time.Duration
	ComponentContract   frame.ContractRef
}

type OpenRequest struct {
	OperationID string
	ResumeToken string
	Resume      *framebrowser.Realm
}

type ComponentRequest struct {
	OperationID  string
	ParentUseID  string
	Realm        framebrowser.Realm
	Installation identity.InstallationID
	Digest       framevalue.Digest
	Component    plugin.ContributionID
}

// Service is invoked by the trusted application client. A unit's callback grant
// does not expose browser bootstrap or permission expansion through this port.
type Service interface {
	Open(context.Context, OpenRequest) (Session, error)
	Acquire(context.Context, ComponentRequest) (framebrowser.UseGrant, error)
	Close(context.Context, framebrowser.Realm) error
}
