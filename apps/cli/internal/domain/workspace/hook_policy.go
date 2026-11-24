package workspace

import "github.com/Tangerg/flame/packages/go-sdk/protocol"

type HookCatalog struct {
	RepositoryRoot    string
	RepositoryTrusted bool
	Hooks             []protocol.HookInfo
}
