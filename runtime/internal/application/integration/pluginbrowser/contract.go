package pluginbrowser

import (
	"context"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framebrowser "github.com/Tangerg/frame/browser"
	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
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
	ComponentContract   framehost.ContractRef
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

type Service interface {
	Open(context.Context, OpenRequest) (Session, error)
	Acquire(context.Context, ComponentRequest) (framebrowser.UseGrant, error)
	Close(context.Context, framebrowser.Realm) error
}
