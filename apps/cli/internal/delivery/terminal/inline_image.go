package terminal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/kit"
)

type terminalImagePresenter struct {
	transport terminalImageTransport
}

type terminalImageTransport interface {
	Protocol() graphics.Protocol
	CellSize() (image.Point, bool)
	Transmit(context.Context, []byte) (<-chan graphics.UploadResult, error)
	Release(graphics.Resource) error
}

func newTerminalImagePresenter(transport terminalImageTransport) *terminalImagePresenter {
	return &terminalImagePresenter{transport: transport}
}

func (t *terminalImagePresenter) Present(theme kit.Theme, image conversation.InlineImage) base.Block {
	fallback := fallbackInlineImage(theme, image)
	if t == nil || !t.transport.Protocol().Supports(graphics.Live) {
		return fallback
	}
	_, available := t.transport.CellSize()
	if !available {
		return fallback
	}
	data, err := normalizePNG(image.Data)
	if err != nil {
		return presentError(theme, fmt.Sprintf("%s: %v", inlineImageLabel(image), err))
	}
	return &terminalImageBlock{transport: t.transport, data: bytes.Clone(data), alt: inlineImageLabel(image), theme: theme}
}

func (t *terminalImagePresenter) transmit(ctx context.Context, theme kit.Theme, data []byte, alt string) (*terminalImageBlock, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	results, err := t.transport.Transmit(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("transmit %s: %w", alt, err)
	}
	// An accepted transmission settles exactly once, including cancellation. Read
	// that settlement before deciding whether this owner can retain its resource.
	result, ok := <-results
	if !ok {
		return nil, fmt.Errorf("transmit %s: upload ended without a result", alt)
	}
	if result.Err != nil {
		return nil, fmt.Errorf("transmit %s: %w", alt, result.Err)
	}
	if cause := context.Cause(ctx); cause != nil {
		return nil, errors.Join(cause, t.transport.Release(result.Resource))
	}
	return &terminalImageBlock{transport: t.transport, handle: result.Resource, alt: alt, theme: theme}, nil
}

func (t *transcriptView) prepareImage(id transcriptEntryID, pending *terminalImageBlock) {
	if len(pending.data) == 0 {
		return
	}
	entry := t.entries[id]
	entry.settled = false
	if t.media == nil {
		entry.content = presentError(t.theme, "inline image upload has no active presentation owner")
		t.projectEntry(id)
		t.settleEntry(id)
		return
	}
	owner, dispatch := t.media.workers, t.media.dispatch
	slot := operationSlot(fmt.Sprintf("image-%p", pending))
	started := owner.Go(slot, false, func(ctx context.Context, lease operationLease) {
		image, err := t.images.transmit(ctx, pending.theme, pending.data, pending.alt)
		transferred := false
		delivery := post(ctx, dispatch, func() {
			if !owner.Current(lease) || !owner.Release(lease) || t.entries[id] != entry {
				return
			}
			pending.cancel = nil
			if err != nil {
				entry.content = presentError(t.theme, err.Error())
			} else {
				entry.content = image
				transferred = true
			}
			t.projectEntry(id)
			t.settleEntry(id)
			t.refreshSearch()
		})
		owner.failDelivery(delivery)
		if image != nil && !transferred {
			owner.failDelivery(image.Close())
		}
	})
	if !started {
		entry.content = presentError(t.theme, "inline image worker is unavailable")
		t.projectEntry(id)
		t.settleEntry(id)
		return
	}
	pending.cancel = func() { owner.Cancel(slot) }
}

type terminalImageBlock struct {
	transport terminalImageTransport
	handle    graphics.Resource
	data      []byte
	cancel    func()
	alt       string
	theme     kit.Theme
}

func (t *terminalImageBlock) HeightForWidth(width int) int { return t.view().HeightForWidth(width) }

func (t *terminalImageBlock) Draw(view grid.View) { t.view().Draw(view) }

func (t *terminalImageBlock) Close() error {
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	t.data = nil
	if !t.handle.Live() {
		return nil
	}
	err := t.transport.Release(t.handle)
	if err == nil {
		t.handle = graphics.Resource{}
	}
	return err
}

func (t *terminalImageBlock) view() kit.Image {
	cell, _ := t.transport.CellSize()
	return kit.Image{Of: t.handle, Cell: cell, Alt: t.alt, Theme: t.theme}
}

func fallbackInlineImage(theme kit.Theme, image conversation.InlineImage) base.Block {
	return &kit.Image{Alt: inlineImageLabel(image), Theme: theme}
}

func inlineImageLabel(image conversation.InlineImage) string {
	return fmt.Sprintf("@ %s (%s, %d bytes)", image.Name, image.MIMEType, len(image.Data))
}

func normalizePNG(data []byte) ([]byte, error) {
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err == nil {
		return data, nil
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode inline image: %w", err)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, decoded); err != nil {
		return nil, fmt.Errorf("encode inline image as PNG: %w", err)
	}
	return encoded.Bytes(), nil
}
