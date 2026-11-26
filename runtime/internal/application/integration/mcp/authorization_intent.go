package mcp

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
)

var ErrAuthorizationStartRetired = errors.New("mcp: sign-in did not start before the connection ended")

type AuthorizationIntent struct {
	Source     extension.ContributionSource `json:"source"`
	Invocation framecall.Invocation         `json:"invocation"`
	Start      wire.Start                   `json:"start"`
	Recipient  framevalue.Digest            `json:"recipient"`
}

func (i AuthorizationIntent) Validate() error {
	if err := i.Source.Validate(); err != nil {
		return err
	}
	if err := extension.ValidateInvocation(i.Invocation); err != nil {
		return err
	}
	if i.Invocation.Target != (framecall.Target{Scope: extension.ScopeInstallation, Reference: i.Source.InstallationID}) {
		return errors.New("mcp: authorization invocation target differs from its connector")
	}
	if i.Start.Grant == "" || i.Start.Epoch == "" || i.Recipient.IsZero() {
		return errors.New("mcp: authorization start requires its grant, connection epoch and recipient")
	}
	if _, err := mcpserver.ParseServerName(i.Start.Name); err != nil {
		return err
	}
	_, err := ParseAuthorizationAttemptID(i.Start.AttemptID)
	return err
}

type PendingAuthorizationStart struct {
	Intent  AuthorizationIntent
	Receipt operation.ReplayClaim
}

type AuthorizationStartSettlement struct {
	AttemptID AuthorizationAttemptID
	Server    mcpserver.ID
	Refusal   error
	Transport *AuthorizationTransport
	Complete  func(context.Context) error
}

type AuthorizationTransport struct {
	Recipient    framevalue.Digest
	CheckPending bool
	Report       func(context.Context, AuthorizationAttemptStatus) error
}

type AuthorizationRoute struct {
	Source extension.ContributionSource
	Name   mcpserver.ServerName
	Epoch  string
}

type AuthorizationFinish struct {
	ID         AuthorizationAttemptID
	Outcome    wire.Outcome
	Invocation framecall.Invocation
}

type PendingAuthorizationFinish struct {
	AuthorizationFinish
	Route AuthorizationRoute
}
