package workspace

import "github.com/Tangerg/flame/packages/go-sdk/protocol"

type HookCatalog struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []protocol.HookInfo
}
