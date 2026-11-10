package terminal

import (
	"context"
	"image"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"

	"github.com/Tangerg/flame/apps/cli/internal/application/extensions"
)

const attachmentCompletionPrefix = "@"

// completionGate remembers a token whose popup was explicitly closed. A key-up
// event, repaint, rejected candidate, or late async result must not immediately
// reopen it; editing the token or leaving it explicitly starts a new query.
type completionGate struct {
	suppressed *base.Token
}

func (c *completionGate) Allow(query base.Token) bool {
	if c.suppressed == nil {
		return true
	}
	if *c.suppressed == query {
		return false
	}
	c.Reset()
	return true
}

func (c *completionGate) Suppress(query base.Token) {
	c.suppressed = &query
}

func (c *completionGate) Reset() {
	c.suppressed = nil
}

// promptCompletion offers command names and workspace files for the token
// under the prompt's cursor. gate keeps an explicitly closed offer closed until
// its token changes.
type promptCompletion struct {
	popup    *base.Completion
	group    *scene.Group
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
	keys := base.DefaultCompletionKeys()
	keys.Bind(base.Accept, input.Chord{Code: input.Enter})
	p.popup = base.NewCompletion(base.CompletionConfig{
		Look: p.theme.Look(p.glyphs), Keys: keys,
		CurrentToken: p.currentToken,
		Accept: func(candidate base.Candidate, token base.Token) {
			// Acceptance closes both halves of completion: Oolong has already
			// dismissed the list, while the application must retire its async
			// producer so an older file lookup cannot reopen the accepted token.
			p.tasks.operations.Cancel(completionOperation)
			if token.Trigger.Prefix == attachmentCompletionPrefix {
				p.acceptAttachmentCompletion(candidate.Text, token)
				return
			}
			p.gate.Reset()
			p.composer.Controller().Replace(token.Line, token.Start, token.End, candidate.Text)
			p.composer.scheduleDraftPersistence()
		},
	})
	return p
}

func (c *promptCompletion) refreshCompletion() {
	c.tasks.operations.Cancel(completionOperation)
	token, ok := c.currentToken()
	if !ok {
		c.gate.Reset()
		c.popup.Dismiss()
		return
	}
	if !c.gate.Allow(token) {
		c.popup.Dismiss()
		return
	}
	if token.Trigger.Prefix == attachmentCompletionPrefix {
		c.completeFiles(token)
		return
	}
	found := c.commands.find(token.Query)
	candidates := make([]base.Candidate, 0, len(found))
	for _, match := range found {
		availability := c.availability(match.Command.Name)
		detail := match.Command.Title
		if !availability.Enabled() {
			detail = "unavailable: " + availability.Reason()
		}
		candidates = append(candidates, base.Candidate{
			Text: match.Command.Name, Label: match.Command.Name,
			Detail: detail, Matched: match.At,
		})
	}
	c.popup.Offer(token, candidates)
}

func (c *promptCompletion) currentToken() (base.Token, bool) {
	return c.composer.Controller().TokenAtCursor(
		base.Trigger{Prefix: commandPrefix, AtStart: true},
		base.Trigger{Prefix: attachmentCompletionPrefix},
	)
}

func (c *promptCompletion) handleCompletion(event input.Event) bool {
	_, wasOpen := c.popup.Token()
	handled := c.popup.Handle(event)
	if !handled || !wasOpen || c.popup.Open() {
		return handled
	}
	if query, ok := c.currentToken(); ok {
		c.gate.Suppress(query)
	} else {
		c.gate.Reset()
	}
	return true
}

func (c *promptCompletion) completeFiles(query base.Token) {
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
		func(ctx context.Context) ([]base.Candidate, error) {
			matches, err := resolver.Complete(ctx, query.Query)
			candidates := make([]base.Candidate, 0, len(matches))
			for _, match := range matches {
				candidates = append(candidates, base.Candidate{
					Text: match.Path, Label: match.Path, Detail: match.Detail, Matched: match.Matched,
				})
			}
			return candidates, err
		},
		func(candidates []base.Candidate, err error) {
			if !c.gate.Allow(query) {
				c.popup.Dismiss()
				return
			}
			if err != nil {
				c.popup.Dismiss()
				c.notify(err.Error())
				return
			}
			c.popup.Offer(query, candidates)
		},
	)
}

func (c *promptCompletion) exactCommandCompletion() bool {
	token, open := c.popup.Token()
	if !open || token.Trigger.Prefix != commandPrefix || token.Query == "" {
		return false
	}
	command, found := c.commands.lookup(token.Query)
	if !found {
		return false
	}
	candidate, selected := c.popup.Current()
	return selected && candidate.Text == command.Name
}

func (c *promptCompletion) Element() scene.Element {
	if c.group == nil {
		c.group = scene.NewGroup(scene.GroupConfig{
			Items: []scene.Item{{Element: c.popup.Element(), Size: layout.Flex(1)}},
			Element: scene.ElementConfig{Enabled: c.popup.Open, Paint: func(frame scene.Frame) {
				box, area := c.placement(frame.Bounds().Size())
				box.Draw(frame.View.Sub(area))
			}},
			Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
				box, area := c.placement(space)
				return []image.Rectangle{box.InnerRect(area.Size()).Add(area.Min)}
			},
		})
	}
	return c.group.Element()
}

func (c *promptCompletion) placement(space image.Point) (kit.Box, image.Rectangle) {
	width, height := space.X, space.Y
	rows := c.popup.HeightForWidth(width)
	promptHeight := c.prompt.HeightForWidth(width)
	availableHeight := height - promptHeight
	if width <= 2 || availableHeight <= 2 || rows <= 0 {
		return kit.Box{}, image.Rectangle{}
	}
	title, footer := "commands", "enter complete"
	if token, ok := c.popup.Token(); ok && token.Trigger.Prefix == attachmentCompletionPrefix {
		title, footer = "workspace files", "enter attach"
	} else if c.exactCommandCompletion() {
		footer = "enter run"
	}
	box := kit.Box{Theme: c.theme, Glyphs: c.glyphs, Padding: layout.Symmetric(0, 1), Title: title, Footer: footer, FooterAlign: layout.End}
	area := (layout.Placement{Anchor: layout.BottomLeft, Width: max(c.popup.Width()+4, 32), Height: rows + 2}).In(image.Pt(width-2, availableHeight))
	return box, area.Add(image.Pt(1, 0))
}
