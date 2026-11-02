package mcp

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/qimu/portable"
)

// GrantProposal retains only the original command evidence needed to reconcile
// an interrupted directory write. Its opaque token is supplied by the vault.
type GrantProposal struct {
	Source     extension.ContributionSource `json:"source"`
	Invocation portable.Invocation          `json:"invocation"`
	Change     wire.Change                  `json:"change"`
}
