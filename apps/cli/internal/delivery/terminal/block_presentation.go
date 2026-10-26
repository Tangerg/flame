package terminal

import (
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/markdown"
)

// blockPresentation carries the terminal context for built-in transcript rendering.
type blockPresentation struct {
	Theme   kit.Theme
	Glyphs  kit.Glyphs
	Look    markdown.Look
	Syntax  highlight.Renderer
	Tools   []toolPresenter
	Speaker string
	Image   func(conversation.InlineImage) headless.Block
}
