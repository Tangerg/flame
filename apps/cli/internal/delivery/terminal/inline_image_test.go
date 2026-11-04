package terminal

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestInlineImageFallbackPreservesOutputIdentity(t *testing.T) {
	block := fallbackInlineImage(kit.Dark(), conversation.InlineImage{
		Name: "chart.png", MIMEType: "image/png", Data: []byte("1234"),
	})
	picture, ok := block.(*kit.Image)
	if !ok || picture.Alt != "@ chart.png (image/png, 4 bytes)" {
		t.Fatalf("fallback image = %#v", block)
	}
}

type imageTransportStub struct {
	owner       graphics.ImageOwner
	cell        image.Point
	transmitted int
}

func (*imageTransportStub) Protocol() graphics.Protocol { return graphics.Kitty }
func (i *imageTransportStub) CellSize() (image.Point, bool) {
	return i.cell, i.cell.X > 0 && i.cell.Y > 0
}
func (i *imageTransportStub) Transmit(context.Context, []byte) (<-chan graphics.UploadResult, error) {
	i.transmitted++
	out := make(chan graphics.UploadResult, 1)
	out <- graphics.UploadResult{Resource: i.owner.Bind(graphics.Image{ID: uint32(i.transmitted), Size: image.Pt(200, 100)})}
	close(out)
	return out, nil
}
func (i *imageTransportStub) Release(resource graphics.Resource) error {
	return i.owner.Release(resource, func(graphics.Image) error { return nil })
}

func TestTerminalImageBlockReflowsWithTheHostCellSize(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 1))
	var data bytes.Buffer
	if err := png.Encode(&data, source); err != nil {
		t.Fatal(err)
	}
	transport := &imageTransportStub{cell: image.Pt(10, 20)}
	presenter := newTerminalImagePresenter(transport)
	block := presenter.Present(kit.Dark(), conversation.InlineImage{
		Name: "chart.png", MIMEType: "image/png", Data: data.Bytes(),
	})
	pending := block.(*terminalImageBlock)
	uploaded, err := presenter.transmit(t.Context(), pending.theme, pending.data, pending.alt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := uploaded.Close(); err != nil {
			t.Error(err)
		}
	})
	before := uploaded.HeightForWidth(40)
	transport.cell = image.Pt(20, 10)
	after := uploaded.HeightForWidth(40)
	if before == after || transport.transmitted != 1 {
		t.Fatalf("responsive image = rows %d -> %d, transmissions %d", before, after, transport.transmitted)
	}
}

func TestNormalizePNGConvertsSupportedImageFormats(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 1))
	source.Set(0, 0, color.RGBA{R: 255, A: 255})
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, source, nil); err != nil {
		t.Fatal(err)
	}
	converted, err := normalizePNG(jpegData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(converted))
	if err != nil || config.Width != 2 || config.Height != 1 {
		t.Fatalf("converted PNG = config %+v, error %v", config, err)
	}
}
