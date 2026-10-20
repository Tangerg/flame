package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/markdown"
	"github.com/Tangerg/oolong/mermaid"
)

type diagramPreparation func(context.Context, string) ([]byte, error)

func prepareMermaid(theme string) diagramPreparation {
	backend := sync.OnceValues(func() (*mermaid.Renderer, error) {
		return mermaid.New(mermaid.Config{Theme: theme})
	})
	return func(ctx context.Context, source string) ([]byte, error) {
		renderer, err := backend()
		if err != nil {
			return nil, err
		}
		image, err := renderer.Render(ctx, source)
		if err != nil {
			return nil, err
		}
		return image.PNG(), nil
	}
}

// Workers prepare neutral PNGs. Only the owning transcript may accept a result,
// upload it, and replace immutable Markdown content before finishing the block.
type markdownMedia struct {
	workers  *operationOwner
	dispatch program.Dispatcher
	images   *terminalImagePresenter
	syntax   highlight.Renderer
	prepare  diagramPreparation
}

func (a *app) configureMarkdown(transcript *transcriptView) {
	transcript.media = &markdownMedia{
		workers: newOperationOwner(a.ctx), dispatch: a.loop.Dispatcher(),
		images: transcript.images, syntax: a.syntax, prepare: a.prepareDiagram,
	}
	transcript.onRenderError = func(err error) { a.message(err.Error()) }
}

func markdownFences(syntax highlight.Renderer, diagram markdown.Renderer) markdown.Renderer {
	return func(info, source string) (grid.Drawable, error) {
		language := strings.Fields(info)
		if len(language) > 0 && language[0] == "mermaid" && diagram != nil {
			return diagram(info, source)
		}
		return text.NewBlock(text.BlockConfig{Lines: syntax.Lines(info, source), Wrap: true}), nil
	}
}

type preparedDiagram struct {
	png []byte
	err error
}

func (m *markdownMedia) start(block *markdownBlock, changed func()) bool {
	var sources []string
	seen := make(map[string]bool)
	cell, known := m.images.transport.CellSize()
	available := m.images.transport.Protocol().Supports(graphics.Live) && known && cell.X > 0 && cell.Y > 0
	look := block.look
	look.SetRenderer(markdown.FencedCode, markdownFences(m.syntax, func(_ string, source string) (grid.Drawable, error) {
		if !available {
			return nil, errors.New("mermaid: inline diagrams require a Kitty graphics terminal with cell dimensions; source is shown")
		}
		if !seen[source] {
			seen[source] = true
			sources = append(sources, source)
		}
		lines := []text.Line{text.Of("Preparing Mermaid…", block.theme.Muted)}
		for line := range strings.SplitSeq(source, "\n") {
			lines = append(lines, text.Of(line, block.theme.Text))
		}
		return text.NewBlock(text.BlockConfig{Lines: lines, Wrap: true}), nil
	}))
	block.setBlocks(markdown.Render(block.source, look))
	if len(sources) == 0 {
		return false
	}
	slot := operationSlot(fmt.Sprintf("markdown-%p", block))
	started := m.workers.Go(slot, false, func(ctx context.Context, lease operationLease) {
		prepared := make(map[string]preparedDiagram, len(sources))
		for _, source := range sources {
			if ctx.Err() != nil {
				return
			}
			data, err := m.prepare(ctx, source)
			prepared[source] = preparedDiagram{png: data, err: err}
		}
		_ = post(ctx, m.dispatch, func() {
			if !m.workers.Current(lease) || !m.workers.Release(lease) {
				return
			}
			block.cancel = nil
			look := block.look
			look.SetRenderer(markdown.FencedCode, markdownFences(m.syntax, func(_ string, source string) (grid.Drawable, error) {
				result := prepared[source]
				if result.err != nil {
					return nil, result.err
				}
				image, err := m.images.transmit(block.theme, result.png, "Mermaid diagram")
				if err != nil {
					return nil, err
				}
				block.images = append(block.images, image)
				return image, nil
			}))
			block.setBlocks(markdown.Render(block.source, look))
			changed()
		})
	})
	if !started {
		blocks, err := markdown.Render(block.source, block.look)
		block.setBlocks(blocks, errors.Join(err, errors.New("mermaid: diagram worker is unavailable")))
		return false
	}
	block.cancel = func() { m.workers.Cancel(slot) }
	return true
}
