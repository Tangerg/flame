package terminal

import (
	"context"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

type completionQuery struct {
	line  int
	token headless.Token
}

// completionGate remembers a token whose popup was explicitly closed. A key-up
// event, repaint, rejected candidate, or late async result must not immediately
// reopen it; editing the token or leaving it explicitly starts a new query.
type completionGate struct {
	suppressed completionQuery
	active     bool
}

func (c *completionGate) Allow(query completionQuery) bool {
	if !c.active {
		return true
	}
	if c.suppressed == query {
		return false
	}
	c.Reset()
	return true
}

func (c *completionGate) Suppress(query completionQuery) {
	c.suppressed, c.active = query, true
}

func (c *completionGate) Reset() {
	c.suppressed, c.active = completionQuery{}, false
}

// promptCompletion offers command names and workspace files for the token
// under the prompt's cursor. gate keeps an explicitly closed offer closed until
// its token changes.
type promptCompletion struct {
	popup    headless.Completion
	gate     completionGate
	composer *promptComposer
	tasks    taskRunner
	commands *commandCatalog
	prompt   *promptView
	theme    kit.Theme
	glyphs   kit.Glyphs

	notify       func(string)
	availability func(name string) extensions.CommandAvailability
}

type promptCompletionConfig struct {
	composer     *promptComposer
	tasks        taskRunner
	commands     *commandCatalog
	theme        kit.Theme
	glyphs       kit.Glyphs
	notify       func(string)
	availability func(name string) extensions.CommandAvailability
}

func newPromptCompletion(cfg promptCompletionConfig) *promptCompletion {
	p := &promptCompletion{
		composer: cfg.composer, tasks: cfg.tasks, commands: cfg.commands, theme: cfg.theme, glyphs: cfg.glyphs,
		notify: cfg.notify, availability: cfg.availability,
	}
	keys := headless.DefaultCompletionKeys()
	keys.Bind(headless.Accept, input.Chord{Code: input.Enter})
	p.popup = headless.Completion{
		Look: p.theme.Look(p.glyphs), Keys: keys,
		Accept: func(candidate headless.Candidate, token headless.Token) {
			// Acceptance closes both halves of completion: Oolong has already
			// dismissed the list, while the application must retire its async
			// producer so an older file lookup cannot reopen the accepted token.
			p.tasks.operations.Cancel(completionOperation)
			if token.Trigger.Prefix == "@" {
				p.acceptAttachmentCompletion(candidate.Text, token)
				return
			}
			p.gate.Reset()
			p.composer.Editor().Replace(token.Start, token.End, candidate.Text)
			p.composer.scheduleDraftPersistence()
		},
	}
	return p
}

func (c *promptCompletion) refreshCompletion() {
	c.tasks.operations.Cancel(completionOperation)
	query, ok := c.currentCompletionQuery()
	if !ok {
		c.gate.Reset()
		c.popup.Dismiss()
		return
	}
	if !c.gate.Allow(query) {
		c.popup.Dismiss()
		return
	}
	token := query.token
	if token.Trigger.Prefix == "@" {
		c.completeFiles(query)
		return
	}
	found := c.commands.find(token.Query)
	candidates := make([]headless.Candidate, 0, len(found))
	for _, match := range found {
		availability := c.availability(match.Command.Name)
		detail := match.Command.Title
		if !availability.Enabled() {
			detail = "unavailable: " + availability.Reason()
		}
		candidates = append(candidates, headless.Candidate{
			Text: match.Command.Name, Label: match.Command.Name,
			Detail: detail, Matched: match.At,
		})
	}
	c.popup.Offer(token, candidates)
}

func (c *promptCompletion) currentCompletionQuery() (completionQuery, bool) {
	lines := strings.Split(c.composer.Editor().Text(), "\n")
	line, column := c.composer.Editor().Cursor()
	if line < 0 || line >= len(lines) {
		return completionQuery{}, false
	}
	token, ok := headless.TokenAt(lines[line], column,
		headless.Trigger{Prefix: "/", AtStart: true},
		headless.Trigger{Prefix: "@"},
	)
	if !ok {
		return completionQuery{}, false
	}
	return completionQuery{line: line, token: token}, true
}

func (c *promptCompletion) handleCompletion(event input.Event) bool {
	_, wasOpen := c.popup.Token()
	handled := c.popup.Handle(event)
	if !handled || !wasOpen || c.popup.Open() {
		return handled
	}
	if query, ok := c.currentCompletionQuery(); ok {
		c.gate.Suppress(query)
	} else {
		c.gate.Reset()
	}
	return true
}

func (c *promptCompletion) completeFiles(query completionQuery) {
	// The previous offer carries its query's replacement range and must stop
	// accepting input before the next asynchronous lookup can publish a result.
	if !c.popup.Dismiss() {
		return
	}
	if c.composer.attachments == nil {
		return
	}
	resolver := c.composer.attachments
	c.tasks.session(completionOperation, true,
		func(ctx context.Context) ([]headless.Candidate, error) {
			matches, err := resolver.Complete(ctx, query.token.Query)
			candidates := make([]headless.Candidate, 0, len(matches))
			for _, match := range matches {
				candidates = append(candidates, headless.Candidate{
					Text: match.Path, Label: match.Path, Detail: match.Detail, Matched: match.Matched,
				})
			}
			return candidates, err
		},
		func(candidates []headless.Candidate, err error) {
			if !c.gate.Allow(query) {
				c.popup.Dismiss()
				return
			}
			if err != nil {
				c.popup.Dismiss()
				c.notify(err.Error())
				return
			}
			c.popup.Offer(query.token, candidates)
		},
	)
}

func (c *promptCompletion) exactCommandCompletion() bool {
	token, open := c.popup.Token()
	if !open || token.Trigger.Prefix != "/" || token.Query == "" {
		return false
	}
	command, found := c.commands.lookup(token.Query)
	if !found {
		return false
	}
	candidate, selected := c.popup.Current()
	return selected && candidate.Text == command.Name
}

func (c *promptCompletion) drawCompletion(frame headless.Frame) {
	width, height := frame.Size()
	rows := c.popup.HeightForWidth(width)
	promptHeight := c.prompt.HeightForWidth(width)
	availableHeight := height - promptHeight
	if width <= 2 || availableHeight <= 2 || rows <= 0 {
		return
	}
	title := "commands"
	footer := "enter complete"
	if token, ok := c.popup.Token(); ok && token.Trigger.Prefix == "@" {
		title = "workspace files"
		footer = "enter attach"
	} else if c.exactCommandCompletion() {
		footer = "enter run"
	}
	box := kit.Box{
		Theme: c.theme, Glyphs: c.glyphs,
		Padding: layout.Symmetric(0, 1), Title: title, Footer: footer,
		FooterAlign: layout.End,
	}
	space := frame.Sub(grid.Area(1, 0, width-2, availableHeight))
	area := (layout.Placement{
		Anchor: layout.BottomLeft, Width: max(c.popup.Width()+4, 32), Height: rows + 2,
	}).In(space.Bounds().Size())
	inner := box.InnerRect(area.Size())
	box.Draw(space.View.Sub(area))
	c.popup.Draw(space.Sub(area).Sub(inner))
}
