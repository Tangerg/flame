package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/extensions/highlight"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/kit"
)

// blockPresentation carries the terminal context for built-in transcript rendering.
type blockPresentation struct {
	Theme   kit.Theme
	Glyphs  kit.Glyphs
	Look    markdown.Look
	Syntax  highlight.Renderer
	Tools   []toolPresenter
	Speaker string
	Image   func(conversation.InlineImage) base.Block
}
