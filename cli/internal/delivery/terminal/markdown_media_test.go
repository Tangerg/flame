package terminal

import (
	"context"
	"errors"
	"image"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
	"github.com/Tangerg/oolong/highlight"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

const diagramMarkdown = "Before diagram\n\n```mermaid\nflowchart TD\n A --> B\n```\n\nAfter diagram"

type markdownImageTransport struct {
	protocol    graphics.Protocol
	transmitted atomic.Int32
	released    atomic.Int32
}

func (m *markdownImageTransport) Protocol() graphics.Protocol { return m.protocol }
func (*markdownImageTransport) CellSize() (image.Point, bool) { return image.Pt(10, 20), true }
func (m *markdownImageTransport) Transmit([]byte) (graphics.Image, error) {
	return graphics.Image{ID: uint32(m.transmitted.Add(1)), Size: image.Pt(320, 160)}, nil
}
func (m *markdownImageTransport) Release(graphics.Image) error {
	m.released.Add(1)
	return nil
}

func markdownSession(t *testing.T, transport terminalImageTransport, prepare diagramPreparation) (*programtest.Host, func(func(*transcriptView))) {
	t.Helper()
	host := programtest.New(t, programtest.Config{Width: 80, Height: 24})
	ctx, cancel := context.WithCancel(t.Context())
	type owner struct {
		view     *transcriptView
		dispatch program.Dispatcher
	}
	ready, done := make(chan owner, 1), make(chan error, 1)
	go func() {
		done <- program.Run(ctx, program.Config{Host: host, Root: func(loop *program.Runtime) program.Component {
			view := newTranscriptView(kit.Dark(), kit.Unicode(), "", input.Wheel{}, highlight.New("github-dark"), 4, false, nil)
			view.images = newTerminalImagePresenter(transport)
			view.media = &markdownMedia{workers: newOperationOwner(ctx), dispatch: loop.Dispatcher(), images: view.images, syntax: view.syntax, prepare: prepare}
			ready <- owner{view, loop.Dispatcher()}
			return headless.NewRoot(view)
		}})
	}()
	owned := awaitValue(t, ready, "markdown owner")
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
		owned.view.Close()
	})
	return host, func(fn func(*transcriptView)) {
		t.Helper()
		if err := post(t.Context(), owned.dispatch, func() { fn(owned.view) }); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMermaidWaitsForCompleteSourceAndReleasesEveryPlacement(t *testing.T) {
	transport := &markdownImageTransport{protocol: graphics.Kitty}
	requested := make(chan string, 1)
	proceed := make(chan struct{})
	host, onOwner := markdownSession(t, transport, func(ctx context.Context, source string) ([]byte, error) {
		requested <- source
		select {
		case <-proceed:
			return []byte("prepared PNG"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	block := agent.Block{ID: "diagram", Kind: agent.BlockAssistant}
	onOwner(func(view *transcriptView) {
		if err := view.Apply(agent.BlockStarted{Block: block}, nil); err != nil {
			t.Error(err)
		}
		if err := view.Apply(agent.BlockDelta{BlockID: block.ID, Text: diagramMarkdown}, nil); err != nil {
			t.Error(err)
		}
	})
	host.Shows(t, "A --> B")
	select {
	case <-requested:
		t.Fatal("provisional Markdown started a browser job")
	default:
	}
	block.Text = diagramMarkdown + "\n\n```mermaid\nflowchart TD\n A --> B\n```"
	onOwner(func(view *transcriptView) {
		if err := view.Apply(agent.BlockCompleted{Block: block}, nil); err != nil {
			t.Error(err)
		}
		if view.content.Finished(view.content.FirstBlock()) {
			t.Error("pending diagram was finished")
		}
	})
	if source := awaitValue(t, requested, "Mermaid source"); source != "flowchart TD\n A --> B" {
		t.Fatalf("source = %q", source)
	}
	host.Shows(t, "Preparing Mermaid")
	close(proceed)
	host.Hides(t, "Preparing Mermaid")
	onOwner(func(view *transcriptView) {
		if !view.content.Finished(view.content.FirstBlock()) {
			t.Error("accepted diagram is not finished")
		}
		if transport.transmitted.Load() != 2 {
			t.Error("repeated diagrams did not get independent image handles")
		}
		view.Reset()
		if transport.released.Load() != 2 {
			t.Error("reset did not release every diagram")
		}
	})
	select {
	case <-requested:
		t.Fatal("identical source was prepared twice")
	default:
	}
}

func TestMermaidFailureKeepsSourceAndDiagnostic(t *testing.T) {
	transport := &markdownImageTransport{protocol: graphics.Kitty}
	host, onOwner := markdownSession(t, transport, func(context.Context, string) ([]byte, error) {
		return nil, errors.New("Mermaid syntax rejected")
	})
	onOwner(func(view *transcriptView) {
		block := &markdownBlock{theme: view.theme, speaker: "flame"}
		block.setSource(diagramMarkdown, view.look)
		view.Append(block)
	})
	host.Shows(t, "Mermaid syntax rejected")
	host.Shows(t, "A --> B")
	host.Shows(t, "After diagram")
	onOwner(func(view *transcriptView) {
		if !view.content.Finished(view.content.FirstBlock()) || transport.transmitted.Load() != 0 {
			t.Error("failed diagram did not settle without transmission")
		}
	})
}

func TestMermaidResetRejectsLateResultAndCancelsWorker(t *testing.T) {
	transport := &markdownImageTransport{protocol: graphics.Kitty}
	started, canceled := make(chan struct{}), make(chan struct{})
	_, onOwner := markdownSession(t, transport, func(ctx context.Context, _ string) ([]byte, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return []byte("late PNG"), nil
	})
	onOwner(func(view *transcriptView) {
		block := &markdownBlock{theme: view.theme, speaker: "flame"}
		block.setSource(diagramMarkdown, view.look)
		view.Append(block)
	})
	awaitValue(t, started, "diagram preparation")
	onOwner(func(view *transcriptView) { view.Reset() })
	awaitValue(t, canceled, "diagram cancellation")
	onOwner(func(view *transcriptView) {
		view.Close()
		if transport.transmitted.Load() != 0 || view.content.Len() != 0 {
			t.Error("late result reached a retired transcript")
		}
	})
}

func TestMermaidUnsupportedTerminalPreservesSourceWithoutStartingBackend(t *testing.T) {
	transport := &markdownImageTransport{}
	host, onOwner := markdownSession(t, transport, func(context.Context, string) ([]byte, error) {
		t.Error("unsupported terminal started Mermaid")
		return nil, errors.New("unexpected backend")
	})
	onOwner(func(view *transcriptView) {
		block := &markdownBlock{theme: view.theme, speaker: "flame"}
		block.setSource(diagramMarkdown, view.look)
		view.Append(block)
	})
	host.Shows(t, "Kitty graphics terminal")
	host.Shows(t, "A --> B")
}

func TestMermaidCloseJoinsPreparation(t *testing.T) {
	transport := &markdownImageTransport{protocol: graphics.Kitty}
	started, stopped := make(chan struct{}), make(chan struct{})
	_, onOwner := markdownSession(t, transport, func(ctx context.Context, _ string) ([]byte, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil, ctx.Err()
	})
	onOwner(func(view *transcriptView) {
		block := &markdownBlock{theme: view.theme, speaker: "flame"}
		block.setSource(diagramMarkdown, view.look)
		view.Append(block)
	})
	awaitValue(t, started, "diagram preparation")
	onOwner(func(view *transcriptView) { view.Close() })
	select {
	case <-stopped:
	default:
		t.Fatal("transcript closed before joining its renderer")
	}
	if transport.transmitted.Load() != 0 {
		t.Fatal("closed transcript transmitted an image")
	}
}

func TestMarkdownMathUsesNativeLayoutAndShowsDiagnostics(t *testing.T) {
	view := testTranscriptView(t)
	for _, source := range []string{"$$\n\\frac{a}{b}\n$$", "```math\n\\frac{a}{b}\n```"} {
		block := &markdownBlock{theme: view.theme, speaker: "flame"}
		block.setSource("# Formula\n\n"+source+"\n\n**after**", view.look)
		got := drawStatic(t, block, 40, block.HeightForWidth(40))
		if strings.Contains(got, `\frac`) || !strings.Contains(got, "─") || !strings.Contains(got, "after") {
			t.Errorf("formula did not use native layout:\n%s", got)
		}
		if len(block.Rows(40)) != block.HeightForWidth(40) {
			t.Error("math copy rows disagree with layout")
		}
	}
	block := &markdownBlock{theme: view.theme, speaker: "flame"}
	block.setSource("$$\n\\notacommand{x}\n$$\n\nafter", view.look)
	got := drawStatic(t, block, 80, block.HeightForWidth(80))
	if block.diagnostic == nil || !strings.Contains(got, "notacommand") || !strings.Contains(got, "after") {
		t.Fatalf("invalid math lost source, diagnostic or following prose:\n%s", got)
	}
}

func TestStreamingMathReplacesProvisionalDiagnostics(t *testing.T) {
	view := testTranscriptView(t)
	block := agent.Block{ID: "math", Kind: agent.BlockAssistant}
	if err := view.Apply(agent.BlockStarted{Block: block}, nil); err != nil {
		t.Fatal(err)
	}
	if err := view.Apply(agent.BlockDelta{BlockID: block.ID, Text: "$$\n\\fra"}, nil); err != nil {
		t.Fatal(err)
	}
	message := view.entries[view.content.FirstBlock()].content.(*markdownBlock)
	if message.diagnostic == nil {
		t.Fatal("invalid provisional formula has no diagnostic")
	}
	if err := view.Apply(agent.BlockDelta{BlockID: block.ID, Text: "c{a}{b}\n$$"}, nil); err != nil {
		t.Fatal(err)
	}
	if message.diagnostic != nil {
		t.Fatal("valid streamed formula retained an obsolete diagnostic")
	}
}

func TestTranscriptDiscardAndCloseReleaseInlineImages(t *testing.T) {
	view := testTranscriptView(t)
	view.retain = 4
	transport := &markdownImageTransport{protocol: graphics.Kitty}
	images := newTerminalImagePresenter(transport)
	for range 6 {
		block, err := images.transmit(view.theme, []byte("prepared PNG"), "image")
		if err != nil {
			t.Fatal(err)
		}
		view.Append(block)
	}
	drawRoot(t, view, 80, 24)
	view.DiscardExcess()
	if transport.released.Load() != 2 {
		t.Fatal("discard did not release the two retired images")
	}
	view.Close()
	if transport.released.Load() != 6 {
		t.Fatal("close did not release the retained images")
	}
}
