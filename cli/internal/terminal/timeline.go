package terminal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/flame/cli/internal/agent"
)

type timelineEntry struct {
	Run          agent.Run
	RootPosition int
	RootTotal    int
	Depth        int
}

type timelinePane struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	picker *picker[timelineEntry]
	fork   func(timelineEntry)
	live   bool
}

func newTimelinePane(theme kit.Theme, glyphs kit.Glyphs, jump func(timelineEntry), fork func(timelineEntry)) *timelinePane {
	pane := &timelinePane{theme: theme, glyphs: glyphs, fork: fork}
	pane.picker = newPicker(theme, glyphs, "search runs",
		func(entry timelineEntry) string {
			if entry.Depth > 0 {
				return strings.Repeat("  ", entry.Depth-1) + glyphs.Bullet + " Subagent · " + shortIdentity(entry.Run.ID)
			}
			return fmt.Sprintf("Run %d of %d · %s", entry.RootPosition, entry.RootTotal, shortIdentity(entry.Run.ID))
		},
		func(entry timelineEntry) string {
			detail := string(entry.Run.Status)
			if entry.Run.Model != "" {
				detail = entry.Run.Model + " · " + detail
			}
			if entry.Depth > 0 {
				detail += " · parent " + shortIdentity(entry.Run.Lineage.ParentRunID())
			}
			return detail
		},
		jump,
	)
	return pane
}

func (t *timelinePane) SetRuns(runs []agent.Run) {
	t.picker.Reset()
	t.picker.SetItems(buildTimelineEntries(runs))
}

func (t *timelinePane) RefreshRuns(runs []agent.Run) {
	t.picker.SetItems(buildTimelineEntries(runs))
}

func (t *timelinePane) SetLive(live bool) { t.live = live }

func buildTimelineEntries(runs []agent.Run) []timelineEntry {
	children := make(map[string][]agent.Run)
	var roots []agent.Run
	for _, run := range runs {
		if run.Lineage.IsRoot() {
			roots = append(roots, run)
		} else {
			children[run.Lineage.ParentRunID()] = append(children[run.Lineage.ParentRunID()], run)
		}
	}
	entries := make([]timelineEntry, 0, len(runs))
	for index := len(roots) - 1; index >= 0; index-- {
		root := roots[index]
		entries = append(entries, timelineEntry{
			Run: root.Clone(), RootPosition: index + 1, RootTotal: len(roots),
		})
		appendTimelineDescendants(&entries, children, root.ID, index+1, len(roots), 1)
	}
	return entries
}

func appendTimelineDescendants(
	entries *[]timelineEntry,
	children map[string][]agent.Run,
	parentID string,
	rootPosition, rootTotal, depth int,
) {
	for _, child := range children[parentID] {
		*entries = append(*entries, timelineEntry{
			Run: child.Clone(), RootPosition: rootPosition, RootTotal: rootTotal, Depth: depth,
		})
		appendTimelineDescendants(entries, children, child.ID, rootPosition, rootTotal, depth+1)
	}
}

func (t *timelinePane) Draw(frame headless.Frame) {
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Flex(1)}, {Size: layout.Fixed(1)},
	}))
	t.picker.Draw(rows[0])
	hint := "enter jump to retained output · alt+f fork from run · esc close"
	if t.live {
		hint = "live run tree · enter jump to retained output · esc close"
	}
	kit.Label{Text: hint, Style: t.theme.Subtle, Ellipsis: t.glyphs.Ellipsis}.Draw(rows[1].View)
}

func (t *timelinePane) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() && key.Mods == input.Alt && key.Rune == 'f' {
		t.picker.interruptPointerGesture()
		if t.live {
			return true
		}
		if entry, selected := t.picker.Current(); selected && t.fork != nil {
			t.fork(entry)
		}
		return true
	}
	return t.picker.Handle(event)
}

func (t *timelinePane) Focus(has bool) { t.picker.Focus(has) }

func (a *app) buildTimeline(theme kit.Theme, glyphs kit.Glyphs) {
	a.timeline = newTimelinePane(theme, glyphs,
		func(entry timelineEntry) {
			if !a.timelineDialog.Open() {
				return
			}
			a.timelineDialog.Dismiss()
			if !a.transcript.JumpToRun(entry.Run.ID) {
				a.message("that run no longer has retained transcript output")
				return
			}
			a.shell.focus(transcriptPaneKey)
		},
		func(entry timelineEntry) {
			if !a.timelineDialog.Open() {
				return
			}
			if a.conversation.Busy() || a.following || a.pendingCancel != nil {
				a.message("finish or cancel the current run before forking")
				return
			}
			a.timelineDialog.Dismiss()
			a.forkSessionFromRun(entry.Run.ID)
		},
	)
	a.timelineDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Current session timeline", Body: a.timeline,
		Where: layout.Placement{Width: 88, Height: 20},
	})
	a.timeline.picker.cancel = a.timelineDialog.Dismiss
}

func (a *app) ShowTimeline() {
	runs := a.conversation.Runs()
	if len(runs) == 0 {
		a.message("the current session has no runs")
		return
	}
	a.timeline.SetLive(a.conversation.Busy() || a.following || a.pendingCancel != nil)
	a.timeline.SetRuns(runs)
	a.timelineDialog.Show()
}

func (a *app) refreshOpenTimeline() {
	if !a.timelineDialog.Open() {
		return
	}
	a.timeline.SetLive(a.conversation.Busy() || a.following || a.pendingCancel != nil)
	a.timeline.RefreshRuns(a.conversation.Runs())
}

func shortIdentity(identity string) string {
	identity = strings.TrimSpace(identity)
	if len(identity) <= 12 {
		return identity
	}
	return identity[:12]
}
