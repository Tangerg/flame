package mcp

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	framecall "github.com/Tangerg/frame/call"
)

type AuthorizationIntent struct {
	Source     extension.ContributionSource `json:"source"`
	Invocation framecall.Invocation         `json:"invocation"`
	Start      wire.Start                   `json:"start"`
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
	if i.Start.Grant == "" || i.Start.Epoch == "" {
		return errors.New("mcp: authorization start requires its grant and connection epoch")
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
	Finish    func(context.Context, AuthorizationAttemptStatus) error
	Complete  func(context.Context) error
}
