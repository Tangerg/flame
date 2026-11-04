package terminal

import (
	"context"
	"slices"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/apps/cli/internal/application/retry"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/extensions/highlight"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

// readerQuery is one authoritative read the reader can show. follows reports
// which change events make the shown document stale; it is nil for a document
// no event updates. A query without read only identifies a document its owner
// updates in place.
type readerQuery struct {
	status  string
	read    func(context.Context) (readerDocument, error)
	follows func(changefeed.Event) bool
}

// followsTopic reads a topic's own change event and a resync that names it.
func followsTopic(topics ...protocol.RuntimeTopic) func(changefeed.Event) bool {
	return func(event changefeed.Event) bool {
		if event.Type == protocol.RuntimeResync {
			return slices.ContainsFunc(topics, func(topic protocol.RuntimeTopic) bool { return slices.Contains(event.Topics, topic) })
		}
		return slices.ContainsFunc(topics, func(topic protocol.RuntimeTopic) bool { return event.Type == protocol.RuntimeEventType(topic) })
	}
}

// readerSurface owns the reader dialog and the query behind its document, so
// a change event re-reads exactly what is shown and no caller remembers the
// selection that produced it.
type readerSurface struct {
	pane         *readerPane
	dialog       *presentationDialog
	searchDialog *presentationDialog
	searchField  *base.Text
	shown        *readerQuery

	tasks   taskRunner
	status  *statusView
	message func(string)
}

type readerSurfaceConfig struct {
	theme   kit.Theme
	glyphs  kit.Glyphs
	syntax  highlight.Renderer
	stack   *workspaceScene
	loop    *program.Runtime
	tasks   taskRunner
	status  *statusView
	message func(string)
}

func newReaderSurface(cfg readerSurfaceConfig) *readerSurface {
	r := &readerSurface{tasks: cfg.tasks, status: cfg.status, message: cfg.message}
	r.pane = newReaderPane(cfg.theme, cfg.glyphs, cfg.syntax, cfg.loop.Environment().Wheel(), cfg.loop.Clipboard())
	r.dialog = newPresentationDialog(cfg.stack, kit.DialogConfig{Theme: cfg.theme, Glyphs: cfg.glyphs, Title: "Reader", Body: r.pane.Element(),
		Where: layout.Placement{},
	})
	r.pane.dismiss = r.dismiss
	r.pane.openSearch = func() {
		if r.dialog.Open() {
			r.searchField.SetText("")
			r.searchDialog.Show()
		}
	}
	r.pane.onCopied = func() { r.status.note("copied reader text") }

	field := base.NewText(base.TextConfig{Label: "Find in the reader", Placeholder: "text", Initial: "", Check: requiredText})
	r.searchField = field
	form := kit.FormConfig{Fields: []base.Field{field}, Theme: cfg.theme, Glyphs: cfg.glyphs,
		Hints: []keymap.Action{base.Submit, base.Cancel}}
	form.Keys = base.DefaultFormKeys()
	form.Submit = func(base.SubmissionID) (scene.ActionState, error) {
		if !r.dialog.Open() || !r.searchDialog.Open() {
			return scene.Unchanged, nil
		}
		r.searchDialog.Dismiss()
		r.pane.Find(field.Text())
		return scene.Applied, nil
	}

	dressed := kit.NewForm(form)
	r.searchDialog = newPresentationDialog(cfg.stack, kit.DialogConfig{Theme: cfg.theme, Glyphs: cfg.glyphs, Title: "Search reader", Body: formNavigation(dressed.Element(), func() { r.searchDialog.Dismiss() }),
		Where: layout.Placement{Width: 68, Height: 7},
	})
	r.listenForSearch()
	return r
}

func (r *readerSurface) listenForSearch() {
	results := r.pane.SearchResults()
	operations := r.tasks.operations
	operations.Go(readerSearchOperation, true, func(ctx context.Context, lease operationLease) {
		for {
			select {
			case result, ok := <-results:
				if !ok {
					return
				}
				if err := post(ctx, r.tasks.dispatch, func() {
					if operations.Current(lease) {
						r.pane.AcceptSearch(result)
					}
				}); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	})
}

// show reads query in the background and opens its document.
func (r *readerSurface) show(query *readerQuery) {
	r.load(query, query.read)
}

func (r *readerSurface) load(query *readerQuery, read func(context.Context) (readerDocument, error)) {
	r.status.note(query.status)
	r.tasks.session(readerDocumentOperation, true, read, func(document readerDocument, err error) {
		if err != nil {
			message := query.status + " failed: " + err.Error()
			if r.showing(query) {
				r.pane.RefreshFailed(message)
			}
			r.message(message)
			return
		}
		r.open(query, document)
		r.status.note(strings.ToLower(document.Title))
	})
}

// open shows a document query produced; query is nil for a static document.
func (r *readerSurface) open(query *readerQuery, document readerDocument) {
	r.openTarget(readerTarget{document: document})
	r.shown = query
}

func (r *readerSurface) openTarget(target readerTarget) {
	r.tasks.operations.Cancel(readerDocumentOperation)
	r.shown = nil
	r.pane.Open(target)
	r.dialog.Controller().SetDescription(target.document.Title)
	r.dialog.Show()
}

func (r *readerSurface) showing(query *readerQuery) bool {
	return query != nil && r.shown == query && r.dialog.Open()
}

// update replaces the document query opened, keeping the reader's place.
func (r *readerSurface) update(query *readerQuery, document readerDocument) {
	if r.showing(query) {
		r.pane.replace(document, true, r.pane.scroll.FollowingEnd())
	}
}

// follow re-reads the shown document when event makes it stale.
func (r *readerSurface) follow(event changefeed.Event) {
	if r.shown != nil && r.shown.follows != nil && r.shown.follows(event) {
		r.reload()
	}
}

// reload re-reads the shown document, retrying while the Runtime reconnects.
func (r *readerSurface) reload() {
	if r.shown == nil || r.shown.read == nil || !r.dialog.Open() {
		return
	}
	read := r.shown.read
	r.load(r.shown, func(ctx context.Context) (readerDocument, error) {
		failures := 0
		for {
			document, err := read(ctx)
			if err == nil || !retry.IsReconnectable(err) {
				return document, err
			}
			failures++
			if err := runtimeRecoveryBackoff.Wait(ctx, failures); err != nil {
				return readerDocument{}, err
			}
		}
	})
}

func (r *readerSurface) dismiss() {
	r.tasks.operations.Cancel(readerDocumentOperation)
	r.shown = nil
	r.pane.CloseDocument()
	r.searchDialog.Dismiss()
	r.dialog.Dismiss()
}

func (r *readerSurface) observingSource() bool { return r.pane.ObservingSource() }

func (r *readerSurface) shutdown() { r.pane.Shutdown() }

var _ interface{ Element() scene.Element } = (*readerPane)(nil)

func (a *app) OpenReader() {
	target, ok := a.transcript.selectedReaderTarget()
	if !ok {
		a.status.note("select a readable transcript entry")
		return
	}
	a.reader.openTarget(target)
}
