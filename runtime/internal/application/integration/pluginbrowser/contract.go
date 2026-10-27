// Package pluginbrowser defines Flame's trusted browser composition boundary.
// Qimu owns admission and execution; Flame supplies component authoring policy.
package pluginbrowser

import (
	"context"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/control"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type Session struct {
	Authority           control.Authority
	Realm               control.Realm
	Credential          string
	ControlEndpoint     string
	MaximumMessageBytes int
	ProofCapacity       int
	PollInterval        time.Duration
	ComponentContract   qimu.ContractRef
}

type OpenRequest struct {
	OperationID string
	ResumeToken string
	Resume      *control.Realm
}

type ComponentRequest struct {
	OperationID  string
	ParentUseID  string
	Realm        control.Realm
	Installation identity.InstallationID
	Digest       identity.Digest
	Component    plugin.ContributionID
}

// Service is invoked by the trusted application client. A unit's callback grant
// does not expose browser bootstrap or permission expansion through this port.
type Service interface {
	Open(context.Context, OpenRequest) (Session, error)
	Acquire(context.Context, ComponentRequest) (control.UseGrant, error)
	Close(context.Context, control.Realm) error
}
