package mcp

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"

	framecall "github.com/Tangerg/frame/call"
)

type GrantProposal struct {
	Source     extension.ContributionSource `json:"source"`
	Invocation framecall.Invocation         `json:"invocation"`
	Change     wire.Change                  `json:"change"`
}
