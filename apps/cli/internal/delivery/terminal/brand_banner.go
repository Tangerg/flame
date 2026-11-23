package terminal

import (
	"math"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

const brandTopMarginDivisor = 3

const (
	brandName         = "FLAME"
	brandMarkMinWidth = 40
)

// The mask marks filled cells; the active glyph set decides how a filled cell is drawn.
var brandMark = []string{
	"█████ █      ███  █   █ █████",
	"█     █     █   █ ██ ██ █    ",
	"████  █     █████ █ █ █ ████ ",
	"█     █     █   █ █   █ █    ",
	"█     █████ █   █ █   █ █████",
}

type brandBanner struct {
	theme     kit.Theme
	glyphs    kit.Glyphs
	version   string
	model     string
	workspace string
}

var _ grid.Drawable = (*brandBanner)(nil)

type brandLine struct {
	text  string
	style grid.Style
}

func newBrandBanner(
	theme kit.Theme,
	glyphs kit.Glyphs,
	version string,
	session conversation.Session,
	options authoring.RunOptions,
) *brandBanner {
	banner := &brandBanner{theme: theme, glyphs: glyphs, version: version}
	banner.SetSession(session)
	banner.SetOptions(options)
	return banner
}

func (b *brandBanner) SetSession(session conversation.Session) {
	if b != nil {
		b.workspace = displayWorkspace(session.Workspace)
	}
}

func (b *brandBanner) SetOptions(options authoring.RunOptions) {
	if b != nil {
		b.model = modelLabel(options)
	}
}

func (b *brandBanner) HeightForWidth(width int) int {
	if b == nil || width <= 0 {
		return 0
	}
	return len(b.lines(width, math.MaxInt))
}

func (b *brandBanner) Draw(view grid.View) {
	if b == nil {
		return
	}
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	lines := b.lines(width, height)
	top := max((height-len(lines))/brandTopMarginDivisor, 0)
	for row, line := range lines {
		value := text.Of(line.text, line.style).Truncate(width, b.glyphs.Ellipsis)
		x := max((width-value.Width())/2, 0)
		value.Draw(view, x, top+row)
	}
}

func (b *brandBanner) lines(width, height int) []brandLine {
	if width <= 0 || height <= 0 {
		return nil
	}
	if width < brandMarkMinWidth || height < len(brandMark)+1 {
		return b.compactLines(height)
	}

	lines := make([]brandLine, 0, len(brandMark))
	for _, row := range brandMark {
		lines = append(lines, brandLine{text: strings.ReplaceAll(row, "█", b.glyphs.BarFull.String()), style: b.theme.Accent})
	}
	if len(lines) < height-2 {
		lines = append(lines, brandLine{})
	}
	return append(lines, b.detailLines(height-len(lines))...)
}

func (b *brandBanner) compactLines(height int) []brandLine {
	lines := []brandLine{{text: brandName, style: b.theme.Accent}}
	return append(lines, b.detailLines(height-len(lines))...)
}

func (b *brandBanner) detailLines(capacity int) []brandLine {
	if capacity <= 0 {
		return nil
	}
	candidates := []brandLine{
		{text: "Flame CLI  " + b.version, style: b.theme.Heading},
		{text: b.model, style: b.theme.Muted},
		{text: b.workspace, style: b.theme.Subtle},
	}
	return candidates[:min(capacity, len(candidates))]
}
