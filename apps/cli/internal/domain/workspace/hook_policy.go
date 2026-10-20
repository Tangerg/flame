package workspace

import "github.com/Tangerg/flame/runtime/protocol"

type HookCatalog struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []protocol.HookInfo
}
