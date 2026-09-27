package terminal

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/application/settings"
	"github.com/Tangerg/flame/cli/internal/application/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/queue"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func newTestQueue(t *testing.T, store *workbench.Store) *workbench.Queue {
	t.Helper()
	if store == nil {
		var err error
		store, err = workbench.OpenMemory(workbench.Config{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
	}
	prompts, err := workbench.NewQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	return prompts
}

// enqueueTestPrompt allocates the command identity before the queue sees it,
// exactly as the app does when it enqueues an authored prompt.
func enqueueTestPrompt(t *testing.T, prompts *workbench.Queue, sessionID string, message prompt.Message) queue.Entry {
	t.Helper()
	entry, err := prompts.EnqueueCommand(
		mutation.NewCommandID(), sessionID, message,
		prompt.RunOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func testQueueEntryID(t *testing.T, value uint64) queue.EntryID {
	t.Helper()
	prompts := newTestQueue(t, nil)
	var entry queue.Entry
	for index := uint64(0); index < value; index++ {
		entry = enqueueTestPrompt(t, prompts, "ses_test", prompt.Message{Text: "test entry"})
	}
	return entry.ID
}

type finishObservingRuntime struct {
	*recordingRuntime
	finished chan struct{}
	once     sync.Once
}

type blockingFirstStartRuntime struct {
	Runtime

	mu             sync.Mutex
	blocked        bool
	receiptCommand replay.CommandID
	receipt        conversation.SegmentStream
	receiptErr     error
	inputs         []prompt.StartRun
	started        chan prompt.StartRun
	release        chan struct{}
}

type gatedRecoveryStartRuntime struct {
	Runtime
	commandID replay.CommandID
	started   chan struct{}
	release   chan struct{}
}

func (g *gatedRecoveryStartRuntime) StartRun(ctx context.Context, request prompt.StartRun) (conversation.SegmentStream, error) {
	if request.CommandID == g.commandID {
		select {
		case g.started <- struct{}{}:
		default:
		}
		select {
		case <-g.release:
		case <-ctx.Done():
			return conversation.SegmentStream{}, context.Cause(ctx)
		}
	}
	return g.Runtime.StartRun(ctx, request)
}

func (b *blockingFirstStartRuntime) StartRun(ctx context.Context, request prompt.StartRun) (conversation.SegmentStream, error) {
	b.mu.Lock()
	if request.CommandID != "" && request.CommandID == b.receiptCommand {
		b.inputs = append(b.inputs, request.Clone())
		stream, err := b.receipt, b.receiptErr
		b.mu.Unlock()
		if err != nil {
			return conversation.SegmentStream{}, err
		}
		return stream, nil
	}
	first := !b.blocked
	b.blocked = true
	b.inputs = append(b.inputs, request.Clone())
	b.mu.Unlock()
	stream, err := b.Runtime.StartRun(ctx, request)
	if err != nil {
		receipt, accepted := conversation.AcceptedMutationReceipt(err)
		if !accepted {
			return conversation.SegmentStream{}, err
		}
		stream = receipt
	}
	if !first {
		return stream, err
	}
	b.mu.Lock()
	b.receiptCommand = request.CommandID
	b.receipt = stream
	b.receiptErr = err
	b.mu.Unlock()
	select {
	case b.started <- request.Clone():
	case <-ctx.Done():
		return conversation.SegmentStream{}, context.Cause(ctx)
	}
	select {
	case <-b.release:
		if err != nil {
			return conversation.SegmentStream{}, err
		}
		return stream, nil
	case <-ctx.Done():
		return conversation.SegmentStream{}, context.Cause(ctx)
	}
}

func (b *blockingFirstStartRuntime) startInputs() []prompt.StartRun {
	b.mu.Lock()
	defer b.mu.Unlock()
	inputs := make([]prompt.StartRun, len(b.inputs))
	for index, input := range b.inputs {
		inputs[index] = input.Clone()
	}
	return inputs
}

func (f *finishObservingRuntime) StartRun(ctx context.Context, request prompt.StartRun) (conversation.SegmentStream, error) {
	stream, err := f.recordingRuntime.StartRun(ctx, request)
	if err != nil {
		return conversation.SegmentStream{}, err
	}
	original := stream.Events
	stream.Events = func(yield func(conversation.RunEvent, error) bool) {
		for event, streamErr := range original {
			continued := yield(event, streamErr)
			if streamErr == nil {
				if _, finished := event.Event.(conversation.RunFinished); finished {
					f.once.Do(func() { close(f.finished) })
				}
			}
			if !continued {
				return
			}
		}
	}
	return stream, nil
}

func testQueueDrawer(t *testing.T, messages ...prompt.Message) (*queueDrawer, *workbench.Queue) {
	t.Helper()
	prompts := newTestQueue(t, nil)
	for _, message := range messages {
		enqueueTestPrompt(t, prompts, "session", message)
	}
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	drawer := newQueueDrawer(kit.Dark(), kit.Unicode(), bindings.editor, nil)
	sync := func() { drawer.Set(prompts.Snapshot("session")) }
	drawer.SetActions(queueDrawerActions{
		BeginEdit: func(entry queue.Entry) error {
			err := prompts.Hold(entry.SessionID, entry.ID)
			sync()
			return err
		},
		SaveEdit: func(entry queue.Entry, message prompt.Message, sendNow bool) error {
			if err := prompts.Update(entry.SessionID, entry.ID, message); err != nil {
				return err
			}
			if sendNow {
				if err := prompts.Promote(entry.SessionID, entry.ID); err != nil {
					return err
				}
			}
			sync()
			return nil
		},
		CancelEdit: func(entry queue.Entry) error {
			return prompts.Release(entry.SessionID, entry.ID)
		},
		Remove: func(id queue.EntryID) error {
			_, err := prompts.Remove("session", id)
			sync()
			return err
		},
		Move: func(id queue.EntryID, offset int) error {
			err := prompts.Move("session", id, offset)
			sync()
			return err
		},
		SendNow: func(id queue.EntryID) error {
			err := prompts.Promote("session", id)
			sync()
			return err
		},
	})
	sync()
	return drawer, prompts
}

func drawQueueDrawer(t *testing.T, drawer *queueDrawer, width, height int) (*headless.Root, *grid.Surface, string) {
	t.Helper()
	root := headless.NewRoot(drawer)
	surface := grid.NewSurface(width, height)
	root.Draw(surface.View())
	return root, surface, strings.Join(surface.Rows(), "\n")
}

// drawnTextOrigin finds a cell a click can be aimed at. A pointer test aims at
// what the user sees, so it reads the drawn screen instead of asking the drawer
// to publish a rectangle whose one consumer would be this assertion.
func drawnTextOrigin(t *testing.T, surface *grid.Surface, drawn string) image.Point {
	t.Helper()
	rows := surface.Rows()
	for y, row := range rows {
		if index := strings.Index(row, drawn); index >= 0 {
			return image.Pt(utf8.RuneCountInString(row[:index]), y)
		}
	}
	t.Fatalf("%q was not drawn:\n%s", drawn, strings.Join(rows, "\n"))
	return image.Point{}
}

func queueDrawerHit(t *testing.T, drawer *queueDrawer, target queueTarget) queueHit {
	t.Helper()
	for _, hit := range drawer.presentation.Value().hits {
		if hit.target == target {
			return hit
		}
	}
	t.Fatalf("queue target %+v is not presented", target)
	return queueHit{}
}

func TestQueueViewKeepsTheNextPromptAndOverflowVisible(t *testing.T) {
	view := newQueueView(kit.Dark(), kit.Unicode())
	view.Set(queue.Snapshot{Entries: []queue.Entry{
		{ID: testQueueEntryID(t, 1), Message: prompt.Message{Text: "first follow-up\nwith more detail"}},
		{ID: testQueueEntryID(t, 2), Message: prompt.Message{Text: "second follow-up"}},
		{ID: testQueueEntryID(t, 3), Message: prompt.Message{Text: "third follow-up"}},
		{ID: testQueueEntryID(t, 4), Message: prompt.Message{Text: "fourth follow-up"}},
	}})
	if got := view.HeightForWidth(queueMinWidth - 1); got != 0 {
		t.Fatalf("narrow queue height = %d", got)
	}
	if got := view.HeightForWidth(80); got != queueMaxRows {
		t.Fatalf("queue height = %d", got)
	}
	rendered := drawStatic(t, view, 48, queueMaxRows)
	for _, want := range []string{"Queue", "4 queued", "first follow-up", "3 more"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("queue does not contain %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "with more detail") {
		t.Fatalf("queue preview leaked later lines:\n%s", rendered)
	}
}

func TestQueueDrawerRendersPreviewActionsAndResponsiveFallback(t *testing.T) {
	drawer, _ := testQueueDrawer(t,
		prompt.Message{Text: "first line\nsecond line", Attachments: []prompt.Attachment{{ID: "a", Kind: protocol.ContentBlockText, Name: "context.txt", Path: "/tmp/context.txt"}}},
		prompt.Message{Text: "second prompt"},
	)
	_, _, rendered := drawQueueDrawer(t, drawer, 96, 9)
	for _, want := range []string{
		"Queue · 2 prompts", "Preview · full queued prompt", "second line",
		"[send now]", "[edit]", "[remove]", "J/K reorder",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("queue drawer does not contain %q:\n%s", want, rendered)
		}
	}
	_, _, narrow := drawQueueDrawer(t, drawer, 22, 5)
	if !strings.Contains(narrow, "first line") || strings.Contains(narrow, "[send now]") {
		t.Fatalf("narrow queue drawer did not preserve content priority:\n%s", narrow)
	}
}

func TestQueueDrawerRejectsCommandsAgainstAReplacedPresentation(t *testing.T) {
	bindings, err := configuredKeyBindings(settings.Default())
	if err != nil {
		t.Fatal(err)
	}
	drawer := newQueueDrawer(kit.Dark(), kit.Unicode(), bindings.editor, nil)
	active := true
	drawer.lifecycle.bind(func() bool { return active })
	removed := make([]queue.EntryID, 0, 1)
	drawer.SetActions(queueDrawerActions{Remove: func(id queue.EntryID) error {
		removed = append(removed, id)
		return nil
	}})
	drawer.Set(queue.Snapshot{Entries: []queue.Entry{{ID: testQueueEntryID(t, 1), Message: prompt.Message{Text: "visible"}}}})
	root := headless.NewRoot(drawer)
	surface := grid.NewSurface(72, 8)
	root.Draw(surface.View())

	replacementID := testQueueEntryID(t, 2)
	drawer.Set(queue.Snapshot{Entries: []queue.Entry{{ID: replacementID, Message: prompt.Message{Text: "replacement"}}}})
	if !root.Handle(input.Key{Code: input.Delete}) || len(removed) != 0 {
		t.Fatalf("undrawn replacement removed entries %v", removed)
	}
	root.Draw(surface.View())
	if !root.Handle(input.Key{Code: input.Delete}) || len(removed) != 1 || removed[0] != replacementID {
		t.Fatalf("visible replacement removed entries %v", removed)
	}

	active = false
	root.Handle(input.Key{Code: input.Delete})
	if len(removed) != 1 {
		t.Fatalf("closed drawer removed entries %v", removed)
	}
}

func TestQueueDrawerEditsMultilineTextAndKeepsAttachments(t *testing.T) {
	attachment := prompt.Attachment{ID: "a", Kind: protocol.ContentBlockText, Name: "context.txt", Path: "/tmp/context.txt"}
	drawer, prompts := testQueueDrawer(t, prompt.Message{Text: "original", Attachments: []prompt.Attachment{attachment}})
	drawer.Focus(true)
	drawer.Handle(input.Key{Code: input.Enter})
	if !drawer.Editing() {
		t.Fatal("Enter did not begin queue editing")
	}
	if held := prompts.Snapshot("session").Entries[0].Held; !held {
		t.Fatal("queue editor did not hold its entry")
	}
	drawer.Handle(input.Key{Code: input.Character, Rune: 'u', Mods: input.Ctrl})
	drawer.Handle(input.Paste{Text: "  edited"})
	drawer.Handle(input.Key{Code: input.Enter, Mods: input.Shift})
	drawer.Handle(input.Paste{Text: "second line  "})
	drawer.Handle(input.Key{Code: input.Enter})
	if drawer.Editing() {
		t.Fatal("Enter did not save queue editing")
	}
	entry, ok := prompts.BeginDispatch("session")
	if !ok || entry.Message.Text != "  edited\nsecond line  " || len(entry.Message.Attachments) != 1 || entry.Message.Attachments[0].ID != attachment.ID {
		t.Fatalf("saved queued message = %+v, %v", entry.Message, ok)
	}
	prompts.ReleaseDispatch("session")

	drawer.Handle(input.Key{Code: input.Enter})
	drawer.Handle(input.Paste{Text: " discarded"})
	if !drawer.Handle(input.Key{Code: input.Esc}) || drawer.Editing() {
		t.Fatal("first Esc did not discard the edit")
	}
	if drawer.Handle(input.Key{Code: input.Esc}) {
		t.Fatal("browse-mode Esc should be left for the dialog controller")
	}
	entries := prompts.Snapshot("session").Entries
	if len(entries) != 1 || entries[0].Message.Text != "  edited\nsecond line  " {
		t.Fatalf("discard changed queued entries to %+v", entries)
	}
}

func TestQueueDrawerPreservesItsEditThroughAnExtremeResize(t *testing.T) {
	drawer, prompts := testQueueDrawer(t, prompt.Message{Text: "original"})
	drawer.Focus(true)
	drawer.Handle(input.Key{Code: input.Enter})
	drawer.Handle(input.Key{Code: input.Character, Rune: 'u', Mods: input.Ctrl})
	drawer.Handle(input.Paste{Text: "first line"})
	drawer.Handle(input.Key{Code: input.Enter, Mods: input.Shift})
	drawer.Handle(input.Paste{Text: "second line"})

	_, _, tiny := drawQueueDrawer(t, drawer, 1, 1)
	if tiny == "" {
		t.Fatal("extreme queue layout did not retain ownership of its viewport")
	}
	drawer.Handle(input.Paste{Text: " while tiny"})
	_, _, restored := drawQueueDrawer(t, drawer, 72, 8)
	for _, want := range []string{"Editing queued prompt", "first line", "second line while tiny"} {
		if !strings.Contains(restored, want) {
			t.Fatalf("restored queue editor does not contain %q:\n%s", want, restored)
		}
	}

	drawer.Handle(input.Key{Code: input.Enter})
	entry, ok := prompts.BeginDispatch("session")
	if !ok || entry.Message.Text != "first line\nsecond line while tiny" {
		t.Fatalf("saved queue edit after resize = %+v, %v", entry.Message, ok)
	}
}

func TestClosingQueueDrawerReleasesItsEditedEntry(t *testing.T) {
	drawer, prompts := testQueueDrawer(t, prompt.Message{Text: "editable"})
	drawer.Focus(true)
	drawer.Handle(input.Key{Code: input.Enter})
	if !prompts.Snapshot("session").Entries[0].Held {
		t.Fatal("test did not hold the edited entry")
	}
	drawer.Closed()
	if drawer.Editing() || prompts.Snapshot("session").Entries[0].Held {
		t.Fatalf("closed queue drawer left editing=%v snapshot=%+v", drawer.Editing(), prompts.Snapshot("session"))
	}
}

func TestQueueDrawerReleasesTheOriginalSessionWhenSnapshotChanges(t *testing.T) {
	drawer, prompts := testQueueDrawer(t, prompt.Message{Text: "old session prompt"})
	enqueueTestPrompt(t, prompts, "next-session", prompt.Message{Text: "next session prompt"})
	drawer.lifecycle.bind(func() bool { return true })
	drawer.Focus(true)
	root, surface, _ := drawQueueDrawer(t, drawer, 72, 8)
	drawer.Handle(input.Key{Code: input.Enter})
	root.Draw(surface.View())
	if !prompts.Snapshot("session").Entries[0].Held {
		t.Fatal("test did not hold the original session entry")
	}

	drawer.Set(prompts.Snapshot("next-session"))
	drawer.Handle(input.Paste{Text: "stale editor input"})
	drawer.Handle(input.Key{Code: input.Enter})
	if drawer.Editing() {
		t.Fatal("snapshot replacement accepted input from the previous session's editor")
	}
	if prompts.Snapshot("session").Entries[0].Held {
		t.Fatal("snapshot replacement left the original session entry held")
	}
	if entries := prompts.Snapshot("session").Entries; len(entries) != 1 || entries[0].Message.Text != "old session prompt" || entries[0].Held {
		t.Fatalf("released original entry = %+v", entries)
	}
	_, _, rendered := drawQueueDrawer(t, drawer, 72, 7)
	if !strings.Contains(rendered, "next session prompt") || strings.Contains(rendered, "old session prompt") {
		t.Fatalf("replacement snapshot rendered the wrong session:\n%s", rendered)
	}
}

func TestQueueDrawerEditorOwnsPointerPlacement(t *testing.T) {
	drawer, _ := testQueueDrawer(t, prompt.Message{Text: "move this cursor"})
	drawer.Focus(true)
	drawer.Handle(input.Key{Code: input.Enter})
	root, surface, _ := drawQueueDrawer(t, drawer, 72, 8)
	first := drawnTextOrigin(t, surface, "move this cursor")
	_, before := drawer.editor.Editor().Cursor()
	if before == 0 {
		t.Fatal("queue editor cursor did not start at the end")
	}
	if !root.Handle(input.Mouse{Pos: first, Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("queue editor click was not routed")
	}
	line, column := drawer.editor.Editor().Cursor()
	if line != 0 || column != 0 {
		t.Fatalf("queue editor click moved cursor to %d:%d, want 0:0", line, column)
	}
}

func TestQueueDrawerReordersAndPromotesTheSelectedEntry(t *testing.T) {
	drawer, prompts := testQueueDrawer(t,
		prompt.Message{Text: "first"}, prompt.Message{Text: "second"}, prompt.Message{Text: "third"},
	)
	drawer.Handle(input.Key{Code: input.Down})
	drawer.Handle(input.Key{Code: input.Character, Rune: 'J', Mods: input.Shift})
	got := prompts.Snapshot("session").Entries
	if got[0].Message.Text != "first" || got[1].Message.Text != "third" || got[2].Message.Text != "second" {
		t.Fatalf("reordered queue = %+v", got)
	}
	drawer.Handle(input.Key{Code: input.Character, Rune: 's'})
	got = prompts.Snapshot("session").Entries
	if got[0].Message.Text != "second" {
		t.Fatalf("send-now promotion = %+v", got)
	}
}

func TestQueueDrawerMouseActionsCommitOnlyOnAnUndraggedMatchingRelease(t *testing.T) {
	drawer, prompts := testQueueDrawer(t, prompt.Message{Text: "first"}, prompt.Message{Text: "second"})
	root, surface, _ := drawQueueDrawer(t, drawer, 80, 5)
	firstID := prompts.Snapshot("session").Entries[0].ID
	remove := queueDrawerHit(t, drawer, queueTarget{kind: queueTargetRemove, id: firstID})
	edit := queueDrawerHit(t, drawer, queueTarget{kind: queueTargetEdit, id: firstID})

	root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseDown, Button: input.ButtonLeft})
	surface.Reset()
	root.Draw(surface.View())
	pressed, ok := surface.CellAt(remove.area.Min.X, remove.area.Min.Y)
	if !ok || !pressed.Style.Attr.Has(grid.Reverse) {
		t.Fatalf("pressed queue action has no pressed visual: %+v, %v", pressed, ok)
	}
	root.Handle(input.Mouse{Pos: edit.area.Min, Action: input.MouseUp, Button: input.ButtonLeft})
	if got := len(prompts.Snapshot("session").Entries); got != 2 {
		t.Fatalf("mismatched release removed an entry: %d", got)
	}

	root.Draw(surface.View())
	remove = queueDrawerHit(t, drawer, queueTarget{kind: queueTargetRemove, id: firstID})
	root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: image.Pt(0, 0), Action: input.MouseDrag, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseUp, Button: input.ButtonLeft})
	if got := len(prompts.Snapshot("session").Entries); got != 2 {
		t.Fatalf("dragged release removed an entry: %d", got)
	}

	root.Draw(surface.View())
	remove = queueDrawerHit(t, drawer, queueTarget{kind: queueTargetRemove, id: firstID})
	root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseUp, Button: input.ButtonLeft})
	if got := len(prompts.Snapshot("session").Entries); got != 1 {
		t.Fatalf("matching release left %d entries", got)
	}
}

func TestQueueDrawerCancelsAStalePointerGesture(t *testing.T) {
	tests := []struct {
		name      string
		interrupt func(*queueDrawer, *headless.Root, *workbench.Queue, image.Point)
	}{
		{
			name: "different button release",
			interrupt: func(_ *queueDrawer, root *headless.Root, _ *workbench.Queue, point image.Point) {
				root.Handle(input.Mouse{Pos: point, Action: input.MouseUp, Button: input.ButtonRight})
			},
		},
		{
			name: "different button press",
			interrupt: func(_ *queueDrawer, root *headless.Root, _ *workbench.Queue, point image.Point) {
				root.Handle(input.Mouse{Pos: point, Action: input.MouseDown, Button: input.ButtonRight})
			},
		},
		{
			name: "focus loss",
			interrupt: func(drawer *queueDrawer, _ *headless.Root, _ *workbench.Queue, _ image.Point) {
				drawer.Focus(false)
				drawer.Focus(true)
			},
		},
		{
			name: "snapshot replacement",
			interrupt: func(drawer *queueDrawer, root *headless.Root, prompts *workbench.Queue, _ image.Point) {
				drawer.Set(prompts.Snapshot("session"))
				root.Draw(grid.NewSurface(80, 5).View())
			},
		},
		{
			name: "keyboard navigation",
			interrupt: func(drawer *queueDrawer, _ *headless.Root, _ *workbench.Queue, _ image.Point) {
				drawer.Handle(input.Key{Code: input.Down})
			},
		},
		{
			name: "wheel navigation",
			interrupt: func(_ *queueDrawer, root *headless.Root, _ *workbench.Queue, point image.Point) {
				root.Handle(input.Mouse{Pos: point, Action: input.WheelDown})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			drawer, prompts := testQueueDrawer(t, prompt.Message{Text: "first"}, prompt.Message{Text: "second"})
			root, _, _ := drawQueueDrawer(t, drawer, 80, 5)
			firstID := prompts.Snapshot("session").Entries[0].ID
			remove := queueDrawerHit(t, drawer, queueTarget{kind: queueTargetRemove, id: firstID})

			root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseDown, Button: input.ButtonLeft})
			test.interrupt(drawer, root, prompts, remove.area.Min)
			root.Handle(input.Mouse{Pos: remove.area.Min, Action: input.MouseUp, Button: input.ButtonLeft})
			if got := len(prompts.Snapshot("session").Entries); got != 2 {
				t.Fatalf("stale pointer gesture left %d entries", got)
			}
		})
	}
}

func TestRunningTurnQueuesFollowUpsAndDrainsThemInFIFOOrder(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(authoredPrompt string) runtimefixture.Script {
		if authoredPrompt == "PRIMARY_RUN" {
			return runtimefixture.Script{Prelude: []runtimefixture.Step{
				{Delay: 500 * time.Millisecond, Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
			}}
		}
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Event: conversation.BlockCompleted{Block: conversation.Block{ID: "answer-" + authoredPrompt, Kind: conversation.BlockAssistant, Text: "RAN_" + authoredPrompt}}},
			{Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask flame")
	host.Type("PRIMARY_RUN")
	host.Press(input.Enter)
	host.Shows(t, "working")

	host.Type("FOLLOW_UP_ONE")
	host.Press(input.Enter)
	host.Type("FOLLOW_UP_TWO")
	host.Press(input.Enter)
	host.Shows(t, "2 queued")
	host.Shows(t, "FOLLOW_UP_ONE")
	host.Shows(t, "RAN_FOLLOW_UP_TWO")
	host.Shows(t, "complete")

	inputs := backend.startInputs()
	if len(inputs) != 3 {
		t.Fatalf("started %d runs: %+v", len(inputs), inputs)
	}
	for index, want := range []string{"PRIMARY_RUN", "FOLLOW_UP_ONE", "FOLLOW_UP_TWO"} {
		if got := inputs[index].Message.Text; got != want {
			t.Fatalf("run %d = %q, want %q", index+1, got, want)
		}
	}
	stop()
}

func TestAcceptedStartRetainsTheFIFOBoundaryUntilDurableSettlementRecovers(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(authoredPrompt string) runtimefixture.Script {
		finishDelay := time.Duration(0)
		if authoredPrompt == "FIRST_SETTLEMENT" {
			finishDelay = time.Second
		}
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Event: conversation.BlockCompleted{Block: conversation.Block{
				ID: "answer-" + authoredPrompt, Kind: conversation.BlockAssistant, Text: authoredPrompt + "_RAN",
			}}},
			{Delay: finishDelay, Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	gate := &blockingFirstStartRuntime{
		Runtime: base, started: make(chan prompt.StartRun, 1), release: make(chan struct{}),
	}
	stateDirectory := t.TempDir()
	host, stop := runUIWithState(t, gate, "/tmp/flame-cli-test", "ses_demo_1", stateDirectory)
	host.Shows(t, "Ask flame")
	host.Type("FIRST_SETTLEMENT")
	host.Press(input.Enter)
	first := awaitSignalValue(t, gate.started, "accepted start held before returning its receipt")
	host.Type("SECOND_SETTLEMENT")
	host.Press(input.Enter)

	var pending []workbench.PendingRun
	awaitState(t, "both runtime commands to become durable", func() bool {
		store, err := openTestWorkbench(stateDirectory)
		if err != nil {
			return false
		}
		pending = store.PendingRuns(first.SessionID)
		return len(pending) == 2 && pending[0].State == workbench.PendingRunDispatching
	})
	if pending[0].Command.CommandID != first.CommandID || pending[1].Command.Message.Text != "SECOND_SETTLEMENT" {
		t.Fatalf("durable FIFO before settlement = %+v", pending)
	}

	states, err := os.ReadDir(filepath.Join(stateDirectory, "sessions"))
	if err != nil || len(states) != 1 {
		t.Fatalf("session state files = %d, %v", len(states), err)
	}
	statePath := filepath.Join(stateDirectory, "sessions", states[0].Name())
	backupPath := statePath + ".backup"
	if renameErr := os.Rename(statePath, backupPath); renameErr != nil {
		t.Fatal(renameErr)
	}
	if mkdirErr := os.Mkdir(statePath, 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	blocker := filepath.Join(statePath, "blocker")
	if writeFileErr := os.WriteFile(blocker, []byte("block acknowledged start settlement"), 0o600); writeFileErr != nil {
		t.Fatal(writeFileErr)
	}

	close(gate.release)
	host.Shows(t, "workbench:")
	host.Shows(t, "FIRST_SETTLEMENT_RAN")
	if got := len(gate.startInputs()); got != 1 {
		t.Fatalf("second command crossed the failed settlement boundary: %d starts", got)
	}

	if removeErr := os.Remove(blocker); removeErr != nil {
		t.Fatal(removeErr)
	}
	if removeErr := os.Remove(statePath); removeErr != nil {
		t.Fatal(removeErr)
	}
	if renameErr := os.Rename(backupPath, statePath); renameErr != nil {
		t.Fatal(renameErr)
	}
	host.Shows(t, "SECOND_SETTLEMENT_RAN")
	host.Shows(t, "complete")
	if inputs := gate.startInputs(); len(inputs) != 2 || inputs[0].Message.Text != "FIRST_SETTLEMENT" ||
		inputs[1].Message.Text != "SECOND_SETTLEMENT" {
		t.Fatalf("starts after durable recovery = %+v", inputs)
	}

	reopened, err := openTestWorkbench(stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if remaining := reopened.PendingRuns(first.SessionID); len(remaining) != 0 {
		t.Fatalf("settled FIFO remains durable: %+v", remaining)
	}
	history := reopened.History()
	if len(history) != 2 || history[0].Text != "FIRST_SETTLEMENT" || history[1].Text != "SECOND_SETTLEMENT" {
		t.Fatalf("history after settlement recovery = %+v", history)
	}
	stop()
}

func TestAcceptedStartSettlementRecoveryRestoresTheTerminalStatusWithoutAFollowUp(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Event: conversation.BlockCompleted{Block: conversation.Block{ID: "answer", Kind: conversation.BlockAssistant, Text: "ONLY_SETTLEMENT_RAN"}}},
			{Delay: time.Second, Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	gate := &blockingFirstStartRuntime{
		Runtime: base, started: make(chan prompt.StartRun, 1), release: make(chan struct{}),
	}
	stateDirectory := t.TempDir()
	host, stop := runUIWithState(t, gate, "/tmp/flame-cli-test", "ses_demo_1", stateDirectory)
	host.Shows(t, "Ask flame")
	host.Type("ONLY_SETTLEMENT")
	host.Press(input.Enter)
	awaitSignalValue(t, gate.started, "accepted start held before its single settlement")

	states, err := os.ReadDir(filepath.Join(stateDirectory, "sessions"))
	if err != nil || len(states) != 1 {
		t.Fatalf("session state files = %d, %v", len(states), err)
	}
	statePath := filepath.Join(stateDirectory, "sessions", states[0].Name())
	backupPath := statePath + ".backup"
	if err := os.Rename(statePath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(statePath, "blocker")
	if err := os.WriteFile(blocker, []byte("block single settlement"), 0o600); err != nil {
		t.Fatal(err)
	}

	close(gate.release)
	host.Shows(t, "workbench:")
	host.Shows(t, "ONLY_SETTLEMENT_RAN")
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, statePath); err != nil {
		t.Fatal(err)
	}
	host.Shows(t, "complete")
	host.Hides(t, "workbench:")
	stop()
}

func TestCancelingARunDrainsItsQueuedFollowUpAfterCancellationSettles(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(authoredPrompt string) runtimefixture.Script {
		if authoredPrompt == "CANCEL_PRIMARY" {
			return runtimefixture.Script{Prelude: []runtimefixture.Step{
				{Delay: time.Hour, Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
			}}
		}
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Event: conversation.BlockCompleted{Block: conversation.Block{ID: "after-cancel", Kind: conversation.BlockAssistant, Text: "QUEUED_AFTER_CANCEL_RAN"}}},
			{Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask flame")
	host.Type("CANCEL_PRIMARY")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("AFTER_CANCEL")
	host.Press(input.Enter)
	host.Shows(t, "1 queued")
	host.Press(input.Esc)
	host.Shows(t, "QUEUED_AFTER_CANCEL_RAN")
	host.Shows(t, "complete")

	inputs := backend.startInputs()
	if len(inputs) != 2 || inputs[1].Message.Text != "AFTER_CANCEL" {
		t.Fatalf("runs after cancellation = %+v", inputs)
	}
	stop()
}

func TestQueueDrawerSendsTheSelectedFollowUpBeforeTheRestAndPreservesTheDraft(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(authoredPrompt string) runtimefixture.Script {
		if authoredPrompt == "INTERRUPTED_PRIMARY" {
			return runtimefixture.Script{Prelude: []runtimefixture.Step{
				{Delay: time.Hour, Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
			}}
		}
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Event: conversation.BlockCompleted{Block: conversation.Block{ID: "answer-" + authoredPrompt, Kind: conversation.BlockAssistant, Text: "RAN_" + authoredPrompt}}},
			{Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask flame")
	host.Type("INTERRUPTED_PRIMARY")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("FIRST_FOLLOW_UP")
	host.Press(input.Enter)
	host.Type("SECOND_FOLLOW_UP")
	host.Press(input.Enter)
	host.Type("UNSENT_DRAFT")

	host.Send(input.Key{Code: input.Character, Rune: ';', Mods: input.Ctrl})
	host.Shows(t, "Queue · 2 prompts")
	host.Press(input.Down)
	host.Send(input.Key{Code: input.Character, Rune: 's'})
	host.Shows(t, "RAN_SECOND_FOLLOW_UP")
	host.Shows(t, "RAN_FIRST_FOLLOW_UP")
	host.Shows(t, "UNSENT_DRAFT")
	host.Hides(t, "Queue ·")

	inputs := backend.startInputs()
	if len(inputs) != 3 {
		t.Fatalf("started %d runs: %+v", len(inputs), inputs)
	}
	for index, want := range []string{"INTERRUPTED_PRIMARY", "SECOND_FOLLOW_UP", "FIRST_FOLLOW_UP"} {
		if got := inputs[index].Message.Text; got != want {
			t.Fatalf("run %d = %q, want %q", index+1, got, want)
		}
	}
	stop()
}

func TestQueueDrawerReordersAndRemovesFollowUpsBeforeDispatch(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(authoredPrompt string) runtimefixture.Script {
		if authoredPrompt == "PRIMARY_FOR_QUEUE_MUTATION" {
			return runtimefixture.Script{Prelude: []runtimefixture.Step{
				{Delay: time.Hour, Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
			}}
		}
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Event: conversation.BlockCompleted{Block: conversation.Block{ID: "answer-" + authoredPrompt, Kind: conversation.BlockAssistant, Text: "RAN_" + authoredPrompt}}},
			{Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask flame")
	host.Type("PRIMARY_FOR_QUEUE_MUTATION")
	host.Press(input.Enter)
	host.Shows(t, "working")
	for _, authoredPrompt := range []string{"FOLLOW_UP_ONE", "FOLLOW_UP_TWO", "FOLLOW_UP_THREE"} {
		host.Type(authoredPrompt)
		host.Press(input.Enter)
	}
	host.Shows(t, "3 queued")
	host.Send(input.Key{Code: input.Character, Rune: ';', Mods: input.Ctrl})
	host.Shows(t, "Queue · 3 prompts")
	host.Press(input.Down)
	host.Send(input.Key{Code: input.Character, Rune: 'K', Mods: input.Shift})
	host.Shows(t, "queued prompt reordered")
	host.Press(input.Down)
	host.Send(input.Key{Code: input.Character, Rune: 'x'})
	host.Shows(t, "queued prompt removed")
	host.Press(input.Esc)
	host.Hides(t, "Queue · 2 prompts")
	host.Press(input.Esc)
	host.Shows(t, "RAN_FOLLOW_UP_TWO")
	host.Shows(t, "RAN_FOLLOW_UP_THREE")
	host.Shows(t, "complete")

	inputs := backend.startInputs()
	if len(inputs) != 3 {
		t.Fatalf("started %d runs after queue mutation: %+v", len(inputs), inputs)
	}
	for index, want := range []string{"PRIMARY_FOR_QUEUE_MUTATION", "FOLLOW_UP_TWO", "FOLLOW_UP_THREE"} {
		if got := inputs[index].Message.Text; got != want {
			t.Fatalf("run %d = %q, want %q", index+1, got, want)
		}
	}
	stop()
}

func TestEmptyEnterPromotesTheNextQueuedFollowUp(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(authoredPrompt string) runtimefixture.Script {
		if authoredPrompt == "PRIMARY_FOR_EMPTY_ENTER" {
			return runtimefixture.Script{Prelude: []runtimefixture.Step{
				{Delay: time.Hour, Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
			}}
		}
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Event: conversation.BlockCompleted{Block: conversation.Block{ID: "empty-enter-answer", Kind: conversation.BlockAssistant, Text: "EMPTY_ENTER_SENT_NEXT"}}},
			{Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask flame")
	host.Type("PRIMARY_FOR_EMPTY_ENTER")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("NEXT_FROM_EMPTY_ENTER")
	host.Press(input.Enter)
	host.Shows(t, "queue or send next")
	host.Press(input.Enter)
	host.Shows(t, "EMPTY_ENTER_SENT_NEXT")

	inputs := backend.startInputs()
	if len(inputs) != 2 || inputs[1].Message.Text != "NEXT_FROM_EMPTY_ENTER" {
		t.Fatalf("runs after empty Enter = %+v", inputs)
	}
	stop()
}

func TestQueueDrawerRemainsUsableOnAConstrainedTerminal(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Delay: time.Hour, Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	host, stop := runUIWith(t, base)
	host.Shows(t, "Ask flame")
	host.Type("PRIMARY_AT_NARROW_WIDTH")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("QUEUED_AT_NARROW_WIDTH")
	host.Press(input.Enter)
	if !host.Resize(32, 10) {
		t.Fatal("resize to constrained queue layout was refused")
	}
	host.Send(input.Key{Code: input.Character, Rune: ';', Mods: input.Ctrl})
	host.Shows(t, "Queue · 1 prompt")
	host.Press(input.Enter)
	host.Shows(t, "Editing queued prompt")
	host.Press(input.Esc)
	host.Shows(t, "edit discarded")
	host.Press(input.Esc)
	host.Hides(t, "Queue · 1 prompt")
	stop()
}

func TestEditingTheFrontPromptHoldsAutomaticDispatchUntilSave(t *testing.T) {
	base := runtimefixture.New()
	base.Script = func(authoredPrompt string) runtimefixture.Script {
		if authoredPrompt == "PRIMARY_BEFORE_QUEUE_EDIT" {
			return runtimefixture.Script{Prelude: []runtimefixture.Step{
				{Delay: 2 * time.Second, Event: conversation.BlockCompleted{Block: conversation.Block{ID: "primary-finished-marker", Kind: conversation.BlockAssistant, Text: "PRIMARY_FINISHED_WHILE_EDITING"}}},
				{Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
			}}
		}
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Event: conversation.BlockCompleted{Block: conversation.Block{ID: "edited-queue-answer", Kind: conversation.BlockAssistant, Text: "RAN_" + authoredPrompt}}},
			{Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	backend := &finishObservingRuntime{
		recordingRuntime: &recordingRuntime{Runtime: base},
		finished:         make(chan struct{}),
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask flame")
	host.Type("PRIMARY_BEFORE_QUEUE_EDIT")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("ORIGINAL_QUEUED_TEXT")
	host.Press(input.Enter)
	host.Send(input.Key{Code: input.Character, Rune: ';', Mods: input.Ctrl})
	host.Shows(t, "Queue · 1 prompt")
	host.Press(input.Enter)
	host.Shows(t, "Editing queued prompt")
	host.Shows(t, "PRIMARY_FINISHED_WHILE_EDITING")
	select {
	case <-backend.finished:
	case <-time.After(2 * time.Second):
		t.Fatal("primary run did not finish while the queue editor was open")
	}
	sessionID := backend.startInput().SessionID
	snapshot, err := backend.GetSession(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, active := snapshot.ActiveRun(); active {
		t.Fatal("primary run remained active after the UI presented its completed state")
	}
	if got := backend.startCount(); got != 1 {
		t.Fatalf("held queue entry started before save: %d runs", got)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'u', Mods: input.Ctrl})
	host.Type("EDITED_QUEUED_TEXT")
	host.Press(input.Enter)
	host.Shows(t, "RAN_EDITED_QUEUED_TEXT")
	inputs := backend.startInputs()
	if len(inputs) != 2 || inputs[1].Message.Text != "EDITED_QUEUED_TEXT" {
		t.Fatalf("runs after queue edit = %+v", inputs)
	}
	stop()
}

func TestPendingRunRecoveryPreservesAnEditedSuccessor(t *testing.T) {
	for _, outcome := range []string{"accepted", "rejected"} {
		t.Run(outcome, func(t *testing.T) {
			base := runtimefixture.New()
			base.Script = func(string) runtimefixture.Script {
				return runtimefixture.Script{Prelude: []runtimefixture.Step{{
					Delay: time.Hour,
					Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}},
				}}}
			}
			command := testStartRun("ses_demo_1", "RECOVERY_HEAD")
			command.CommandID = mutation.NewCommandID()
			var backend Runtime
			if outcome == "accepted" {
				idempotent := &idempotentStartRuntime{Runtime: base}
				if _, err := idempotent.StartRun(t.Context(), command); err != nil {
					t.Fatal(err)
				}
				backend = idempotent
			} else {
				if _, err := base.StartRun(t.Context(), testStartRun(command.SessionID, "EXISTING_ACTIVE_RUN")); err != nil {
					t.Fatal(err)
				}
				backend = &activeConflictRuntime{Runtime: base, conflict: command.CommandID}
			}
			gate := &gatedRecoveryStartRuntime{
				Runtime: backend, commandID: command.CommandID,
				started: make(chan struct{}, 1), release: make(chan struct{}),
			}
			stateDirectory := t.TempDir()
			store, err := openTestWorkbench(stateDirectory)
			if err != nil {
				t.Fatal(err)
			}
			stageDispatchingRun(t, store, command)
			successor := testStartRun(command.SessionID, "ORIGINAL_SUCCESSOR")
			successor.CommandID = mutation.NewCommandID()
			if err := store.StagePendingRun(workbench.PendingRun{
				State: workbench.PendingRunQueued, Command: successor,
				Replay: replay.UnprotectedGuard(), CancelReplay: replay.UnprotectedGuard(),
			}); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			readPending := func() []workbench.PendingRun {
				t.Helper()
				reopened, err := openTestWorkbench(stateDirectory)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				return reopened.PendingRuns(command.SessionID)
			}
			host, stop := runUIWithReplayState(t, gate, "/tmp/flame-cli-test", command.SessionID, stateDirectory)
			awaitState(t, "pending run recovery to start", func() bool {
				select {
				case <-gate.started:
					return true
				default:
					return false
				}
			})
			host.Send(input.Key{Code: input.Character, Rune: ';', Mods: input.Ctrl})
			host.Shows(t, "Queue · 2 prompts")
			host.Press(input.Down)
			host.Press(input.Enter)
			host.Shows(t, "Editing queued prompt")
			host.Send(input.Key{Code: input.Character, Rune: 'u', Mods: input.Ctrl})
			host.Type("EDITED_BEFORE_RECOVERY")
			host.Shows(t, "EDITED_BEFORE_RECOVERY")

			close(gate.release)
			awaitState(t, "pending run recovery to settle durably", func() bool {
				pending := readPending()
				if outcome == "accepted" {
					return len(pending) == 1 && pending[0].Command.CommandID == successor.CommandID
				}
				return len(pending) == 2 && pending[0].State == workbench.PendingRunQueued &&
					pending[0].Command.CommandID != command.CommandID
			})
			host.Type("_AND_AFTER_RECOVERY")
			host.Press(input.Enter)
			host.Shows(t, "queued prompt updated")
			pending := readPending()
			last := pending[len(pending)-1]
			if last.Command.Message.Text != "EDITED_BEFORE_RECOVERY_AND_AFTER_RECOVERY" || last.Command.CommandID == successor.CommandID {
				t.Fatalf("saved successor after recovery = %+v", last)
			}
			stop()
		})
	}
}

func TestQueuedFollowUpKeepsItsAttachmentIdentityUntilDispatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "context.txt"), []byte("queue context"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := runtimefixture.New()
	base.Script = func(authoredPrompt string) runtimefixture.Script {
		delay := time.Duration(0)
		if authoredPrompt == "ATTACHMENT_PRIMARY" {
			delay = 800 * time.Millisecond
		}
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Delay: delay, Event: conversation.RunFinished{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWithWorkspace(t, backend, workspace)
	host.Shows(t, "Ask flame")
	host.Type("ATTACHMENT_PRIMARY")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("/attach context.txt")
	host.Press(input.Enter)
	host.Shows(t, "attached context.txt")
	host.Type("ATTACHED_FOLLOW_UP")
	host.Press(input.Enter)
	host.Shows(t, "1 queued")
	host.Shows(t, "complete")

	inputs := backend.startInputs()
	if len(inputs) != 2 || len(inputs[1].Message.Attachments) != 1 {
		t.Fatalf("queued attachment inputs = %+v", inputs)
	}
	attachment := inputs[1].Message.Attachments[0]
	wantInfo, wantErr := os.Stat(filepath.Join(workspace, "context.txt"))
	gotInfo, gotErr := os.Stat(attachment.Path)
	if attachment.Name != "context.txt" || wantErr != nil || gotErr != nil || !os.SameFile(wantInfo, gotInfo) {
		t.Fatalf("queued attachment = %+v", attachment)
	}
	stop()
}
