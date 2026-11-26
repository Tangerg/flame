package mcp

import (
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"

	framecall "github.com/Tangerg/frame/call"
)

type DirectoryIntent struct {
	Source     extension.ContributionSource `json:"source"`
	Invocation framecall.Invocation         `json:"invocation"`
	Change     wire.Change                  `json:"change"`
}

func (i DirectoryIntent) Validate() error {
	if err := i.Source.Validate(); err != nil {
		return err
	}
	if err := extension.ValidateInvocation(i.Invocation); err != nil {
		return err
	}
	if i.Invocation.Target != (framecall.Target{Scope: extension.ScopeInstallation, Reference: i.Source.InstallationID}) {
		return errors.New("mcp: directory invocation target differs from its connector")
	}
	if _, err := mcpserver.ParseServerName(i.Change.Name); err != nil {
		return err
	}
	if entry := i.Change.Replacement; entry != nil {
		if entry.Name != i.Change.Name || entry.Revision != "" || entry.LaunchGrant == "" || entry.HandshakeSeconds < 0 {
			return errors.New("mcp: invalid directory replacement")
		}
	} else if i.Change.Expected == "" {
		return errors.New("mcp: directory removal requires a revision")
	}
	return nil
}

type PendingDirectoryChange struct {
	Intent  DirectoryIntent
	Receipt operation.ReplayClaim
}

type LaunchGrant struct {
	Connection mcpserver.Connection
	Proposal   *DirectoryIntent
}
