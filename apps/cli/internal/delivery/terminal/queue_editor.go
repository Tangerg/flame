package terminal

import (
	"errors"
	"fmt"
	"image"

	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/kit"
)

// queueRun is what editing the queue asks of Run admission.
type queueRun interface {
	drainQueue() bool
	queueSendBlocked() error
	dispatchPromoted() error
	message(label string)
}

// queueEditor presents the current session's prompt queue and edits it. An
// entry already being sent cannot be edited, removed or moved; dispatch and
// its settlement belong to Run admission.
type queueEditor struct {
	queue     *workbench.Queue
	session   *sessionState
	run       queueRun
	view      *queueView
	prompt    *promptView
	drawer    *queueDrawer
	dialog    *base.Dialog
	workspace *workspaceScene
}

type queueEditorConfig struct {
	queue   *workbench.Queue
	session *sessionState
	run     queueRun
	view    *queueView
	prompt  *promptView
	stack   *workspaceScene
	theme   kit.Theme
	glyphs  kit.Glyphs
	keys    *keymap.Map
	clip    program.Clipboard
}

func newQueueEditor(cfg queueEditorConfig) *queueEditor {
	q := &queueEditor{queue: cfg.queue, session: cfg.session, run: cfg.run, view: cfg.view, prompt: cfg.prompt, workspace: cfg.stack}
	q.drawer = newQueueDrawer(cfg.theme, cfg.glyphs, cfg.keys, cfg.clip)
	body := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: q.drawer.Element(), Size: layout.Flex(1)}}, Element: scene.ElementConfig{Preview: func(event scene.Event) scene.EventResult {
		if cfg.stack.preview != nil {
			return cfg.stack.preview(event)
		}
		return scene.EventResult{}
	}}})
	q.dialog = base.NewDialog(base.DialogConfig{Title: "Queue", Content: body.Element(), Place: func(space image.Point) image.Rectangle { return q.drawer.Place(space).In(space) }, Dismissed: func() { cfg.stack.dismissDialog(q.dialog.Element()); q.drawer.Closed() }})
	q.drawer.SetActions(queueDrawerActions{
		BeginEdit:  q.holdQueuedPrompt,
		SaveEdit:   q.saveQueuedPrompt,
		CancelEdit: q.releaseQueuedPrompt,
		Remove:     q.removeQueuedPrompt,
		Move:       q.moveQueuedPrompt,
		SendNow:    q.sendQueuedNow,
		Dismiss:    q.dialog.Dismiss,
	})
	return q
}

// closable reports whether cancel should close the drawer rather than reach
// the Run: it is open and not editing an entry.
func (q *queueEditor) closable() bool {
	return q.dialog.Open() && !q.drawer.Editing()
}

// sync shows the current session's queue in the view, prompt and drawer.
func (q *queueEditor) sync() authoring.QueueSnapshot {
	snapshot := q.queue.Snapshot(q.session.current.ID)
	q.view.Set(snapshot)
	q.prompt.SetQueued(len(snapshot.Entries))
	q.drawer.Set(snapshot)
	q.dialog.SetTitle("Queue · " + countedNoun(len(snapshot.Entries), "prompt"))
	q.dialog.SetDescription(fmt.Sprintf("Manage %s waiting behind the current turn", countedNoun(len(snapshot.Entries), "prompt")))
	if len(snapshot.Entries) == 0 && q.dialog.Open() {
		q.dialog.Dismiss()
	}
	return snapshot
}

func (q *queueEditor) ShowQueue() {
	snapshot := q.sync()
	if len(snapshot.Entries) == 0 {
		q.run.message("queue is empty")
		return
	}
	q.drawer.ResetNotice()
	q.dialog.Show()
	q.workspace.showDialog(q.dialog.Element())
	q.run.message(fmt.Sprintf("queue · %d waiting", len(snapshot.Entries)))
}

func (q *queueEditor) holdQueuedPrompt(entry authoring.QueueEntry) error {
	if entry.SessionID != q.session.current.ID {
		return errors.New("queued prompt belongs to another session")
	}
	if dispatching, ok := q.queue.Dispatching(entry.SessionID); ok && dispatching.ID == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := q.queue.Hold(entry.SessionID, entry.ID); err != nil {
		return err
	}
	q.sync()
	return nil
}
func (q *queueEditor) saveQueuedPrompt(entry authoring.QueueEntry, message authoring.Message, sendNow bool) error {
	if entry.SessionID != q.session.current.ID {
		return errors.New("queued prompt belongs to another session")
	}
	if dispatching, ok := q.queue.Dispatching(entry.SessionID); ok && dispatching.ID == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := q.queue.Update(entry.SessionID, entry.ID, message); err != nil {
		return err
	}
	q.sync()
	if sendNow {
		return q.sendQueuedNow(entry.ID)
	}
	q.run.drainQueue()
	return nil
}
func (q *queueEditor) releaseQueuedPrompt(entry authoring.QueueEntry) error {
	if err := q.queue.Release(entry.SessionID, entry.ID); err != nil {
		return err
	}
	if entry.SessionID != q.session.current.ID {
		return nil
	}
	q.sync()
	q.run.drainQueue()
	return nil
}
func (q *queueEditor) removeQueuedPrompt(id authoring.QueueEntryID) error {
	if entry, ok := q.queue.Dispatching(q.session.current.ID); ok && id == entry.ID {
		return errQueuedPromptDispatching
	}
	if _, err := q.queue.Remove(q.session.current.ID, id); err != nil {
		return err
	}
	snapshot := q.sync()
	if len(snapshot.Entries) == 0 {
		q.run.message("queue is empty")
	}
	return nil
}
func (q *queueEditor) moveQueuedPrompt(id authoring.QueueEntryID, offset int) error {
	if entry, ok := q.queue.Dispatching(q.session.current.ID); ok && id == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := q.queue.Move(q.session.current.ID, id, offset); err != nil {
		return err
	}
	q.sync()
	return nil
}

// sendQueuedNow persists priority in the queue before touching the active run.
// Cancellation and dispatch therefore remain resumable if either runtime control
// call is delayed or fails: the promoted entry is still the next FIFO item.
func (q *queueEditor) sendQueuedNow(id authoring.QueueEntryID) error {
	if entry, ok := q.queue.Dispatching(q.session.current.ID); ok && id == entry.ID {
		return errQueuedPromptDispatching
	}
	if err := q.run.queueSendBlocked(); err != nil {
		return err
	}
	if err := q.queue.Promote(q.session.current.ID, id); err != nil {
		return err
	}
	q.sync()
	return q.run.dispatchPromoted()
}
