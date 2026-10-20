package terminal

import (
	"errors"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/latex"
	"github.com/Tangerg/oolong/markdown"
)

const userMessageInset = 1

// userMessageBlock gives the user's request a quiet surface without turning it
// into a dialog. The transcript still owns scrolling, selection, and retention;
// this block owns only the visual hierarchy of one durable message.
type userMessageBlock struct {
	box     kit.Box
	message *kit.Entry
}

var (
	_ headless.Block         = (*userMessageBlock)(nil)
	_ headless.TextProjector = (*userMessageBlock)(nil)
)

// selfSpeaker labels the operator's own turn. A block is styled as the
// operator's exactly when it carries this label, so the two cannot disagree.
const selfSpeaker = "you"

func newUserMessageBlock(theme kit.Theme, speaker, body string) *userMessageBlock {
	message := &kit.Entry{Theme: theme, Label: speaker, Body: body}
	if speaker == selfSpeaker {
		message.LabelStyle = theme.Accent
	}
	return &userMessageBlock{
		box: kit.Box{
			Theme:   theme,
			Bare:    true,
			Padding: layout.Symmetric(0, userMessageInset),
		},
		message: message,
	}
}

func (u *userMessageBlock) HeightForWidth(width int) int {
	innerWidth, _ := u.geometry(width)
	return u.message.HeightForWidth(innerWidth)
}

func (u *userMessageBlock) Draw(view grid.View) {
	width, _ := view.Size()
	if _, inset := u.geometry(width); inset == 0 {
		u.message.Draw(view)
		return
	}
	u.message.Draw(u.box.Draw(view))
}

func (u *userMessageBlock) Rows(width int) []text.Row {
	innerWidth, inset := u.geometry(width)
	rows := u.message.Rows(innerWidth)
	for index := range rows {
		rows[index].Offset += inset
	}
	return rows
}

func (u *userMessageBlock) geometry(width int) (innerWidth, inset int) {
	overhead := u.box.Overhead().X
	if width <= overhead {
		return max(width, 1), 0
	}
	return width - overhead, userMessageInset
}

type markdownBlock struct {
	theme      kit.Theme
	speaker    string
	doc        markdown.Doc
	source     string
	look       markdown.Look
	diagnostic *text.Block
	cancel     func()
	images     []*terminalImageBlock
}

func (m *markdownBlock) setSource(source string, look markdown.Look) {
	m.source, m.look = source, look
	m.setBlocks(markdown.Render(source, look))
}

func (m *markdownBlock) setBlocks(blocks []markdown.Block, err error) {
	m.doc.SetBlocks(blocks)
	m.diagnostic = nil
	if err != nil {
		var lines []text.Line
		for line := range strings.SplitSeq(err.Error(), "\n") {
			lines = append(lines, text.Of(line, m.theme.Danger))
		}
		m.diagnostic = text.NewBlock(text.BlockConfig{Lines: lines, Wrap: true})
	}
}

func (m *markdownBlock) Close() error {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.doc.SetBlocks(nil)
	var err error
	for _, image := range m.images {
		err = errors.Join(err, image.Close())
	}
	m.images = nil
	return err
}

func (m *markdownBlock) HeightForWidth(width int) int {
	innerWidth := max(width-2, 1)
	rows := layout.Sum(1, m.doc.HeightForWidth(innerWidth), 1)
	if m.diagnostic != nil {
		rows = layout.Sum(rows, m.diagnostic.HeightForWidth(innerWidth))
	}
	return rows
}

func (m *markdownBlock) Draw(view grid.View) {
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	view.Text(0, 0, m.speaker, m.theme.Muted)
	m.doc.Draw(view.Sub(grid.Area(2, 1, max(width-2, 0), max(height-2, 0))))
	if m.diagnostic != nil {
		y := 1 + m.doc.HeightForWidth(max(width-2, 1))
		m.diagnostic.Draw(view.Sub(grid.Area(2, y, max(width-2, 0), max(height-y-1, 0))))
	}
}

func (m *markdownBlock) Rows(width int) []text.Row {
	rows := []text.Row{{Text: m.speaker}}
	body := m.doc.Rows(max(width-2, 1))
	if m.diagnostic != nil {
		body = append(body, m.diagnostic.Rows(max(width-2, 1))...)
	}
	for _, row := range body {
		row.Offset += 2
		rows = append(rows, row)
	}
	return append(rows, text.Row{})
}

func markdownLook(theme kit.Theme, glyphs kit.Glyphs, locale string, syntax highlight.Renderer) markdown.Look {
	look := markdown.Look{
		Text: theme.Text, Headings: []grid.Style{theme.Heading, theme.Strong},
		Strong: theme.Strong, Emphasis: grid.Style{Attr: grid.Italic},
		Struck: theme.Muted, Code: theme.Info, Block: theme.Sunken,
		Link: theme.Accent, Quote: theme.Muted, Rail: theme.Subtle,
		Marker: theme.Accent, Rule: theme.Divider,
		Glyphs: markdown.Glyphs{
			Bullet: glyphs.Bullet, Bar: glyphs.Vertical, Divider: glyphs.Horizontal,
			Checked: glyphs.Taken, Unchecked: glyphs.Free,
		},
	}
	look.SetRenderer(markdown.FencedCode, markdownFences(syntax, nil))
	look.SetRenderer(markdown.DisplayMath, func(_ string, source string) (grid.Drawable, error) {
		formula := latex.Render(source, latex.Look{Text: theme.Text, Rule: theme.Subtle, Error: theme.Danger, Glyphs: latex.GlyphsFor(locale)})
		return formula, formula.Err()
	})
	return look
}

func presentError(theme kit.Theme, message string) headless.Block {
	danger := theme
	danger.Text = theme.Danger
	return &kit.Entry{Theme: danger, Label: "runtime", Body: message}
}
