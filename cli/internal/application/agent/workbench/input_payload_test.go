package workbench

import (
	"context"
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestPreparedInputPreservesMaximumAttachmentOutsideRecordBudget(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	data := base64.StdEncoding.EncodeToString(make([]byte, agent.MaxAttachmentBytes))
	command := agent.StartRun{
		CommandID: "cli_11111111111111111111111111111111", SessionID: "ses_large",
		Message: agent.Message{Attachments: []agent.Attachment{{
			ID: "image", Kind: protocol.ContentBlockImage, Name: "image.png",
			Path: filepath.Join(t.TempDir(), "image.png"), MimeType: "image/png", Size: agent.MaxAttachmentBytes,
		}}},
	}
	input := []protocol.ContentBlock{{Type: protocol.ContentBlockImage, Mime: "image/png", Data: data}}
	if err := store.StagePendingRun(queuedPendingRun(command)); err != nil {
		t.Fatal(err)
	}
	guard := protectedReplayGuard(t, "runtime-test", time.Now().Add(time.Hour))
	if err := store.MarkPendingRunDispatching(command.SessionID, command.CommandID, guard, preparedTestInput(t, store, command.Message, input)); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(filepath.Join(directory, store.sessionStateName(command.SessionID)))
	if err != nil || stat.Size() >= maximumStateBytes {
		t.Fatalf("authoring record exceeded its original budget: %v, %v", stat, err)
	}
	input[0].Data = "mutated caller"
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	pending := reopened.PendingRuns(command.SessionID)[0]
	replayed, err := pending.ReplayCommand()
	if err != nil || replayed.CommandID != command.CommandID || pending.Replay != guard || replayed.Input[0].Data != data {
		t.Fatalf("maximum attachment did not survive restart: %v", err)
	}
	changed := reopened.PendingRuns(command.SessionID)
	changed[0].Command.Input[0].Data = "AA=="
	if err := reopened.SavePendingRuns(command.SessionID, changed); err == nil {
		t.Fatal("queue edit replaced the frozen content of a dispatched command")
	}
	if got, err := reopened.PendingRuns(command.SessionID)[0].ReplayCommand(); err != nil || got.Input[0].Data != data {
		t.Fatal("rejected queue edit changed the original payload")
	}
}

func TestUnavailableInputPreservesOriginalRunResumeAndSteer(t *testing.T) {
	for _, mode := range []string{"legacy-path-only", "missing", "corrupt"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			store, err := OpenDirectory(directory, Config{})
			if err != nil {
				t.Fatal(err)
			}
			attachment := steerTestAttachment(t.TempDir())
			steer := steerTestPending(t, "ses_steer", attachment)
			message := steer.Message()
			input := steer.Command().Input
			start := agent.StartRun{
				CommandID: "cli_22222222222222222222222222222222", SessionID: "ses_start", Message: message,
			}
			if err := store.StagePendingRun(queuedPendingRun(start)); err != nil {
				t.Fatal(err)
			}
			if err := store.MarkPendingRunDispatching(start.SessionID, start.CommandID, steer.Replay(), preparedTestInput(t, store, message, input)); err != nil {
				t.Fatal(err)
			}
			approval := agent.Approval{
				RunID: "run_resume", ItemID: "item_approval", Title: "Proceed?",
				Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning},
			}
			resume := PendingResume{
				Command: agent.ResumeRun{
					CommandID: "cli_33333333333333333333333333333333", RunID: approval.RunID,
					Answers: []agent.InterruptAnswer{{ItemID: approval.ItemID, Answer: agent.ApprovalAnswer{Decision: protocol.ApprovalDeny}}},
					Message: &message, Input: input,
				},
				Interactions: []agent.Interaction{approval}, Replay: steer.Replay(),
			}
			if err := store.StagePendingResume("ses_resume", resume, preparedTestInput(t, store, message, input)); err != nil {
				t.Fatal(err)
			}
			source := agent.Message{Text: "/steer " + message.Text, Attachments: slices.Clone(message.Attachments)}
			if err := store.SaveDraft(steer.SessionID(), source); err != nil {
				t.Fatal(err)
			}
			if err := store.StagePendingSteer(steer, source, preparedTestInput(t, store, message, input)); err != nil {
				t.Fatal(err)
			}
			original := store.PendingRuns(start.SessionID)[0]
			if mode == "legacy-path-only" {
				original.InputDigest, original.Command.Input = "", nil
				resume.Command.Input = nil
				steer.command.Input = nil
				for _, state := range []sessionState{
					{SessionID: start.SessionID, PendingRuns: []PendingRun{original}},
					{SessionID: "ses_resume", PendingResume: &resume},
					{SessionID: steer.SessionID(), PendingSteer: new(steer.record())},
				} {
					if err := store.save(store.sessionStateName(state.SessionID), state); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				path := filepath.Join(directory, original.InputDigest.name())
				if mode == "missing" {
					err = os.Remove(path)
				} else {
					err = os.WriteFile(path, []byte("[]"), 0o600)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenDirectory(directory, Config{})
			if err != nil {
				t.Fatalf("unavailable input prevented preservation of the authoring state: %v", err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			run := reopened.PendingRuns(start.SessionID)[0]
			if _, err := run.ReplayCommand(); !errors.Is(err, agent.ErrCommandInputUnavailable) {
				t.Fatalf("start replay error = %v", err)
			}
			if _, err := reopened.RequeuePendingRun(start.SessionID, start.CommandID); !errors.Is(err, agent.ErrCommandInputUnavailable) {
				t.Fatalf("unavailable start received a fresh command identity: %v", err)
			}
			recoveredResume, found := reopened.PendingResume("ses_resume")
			if !found || recoveredResume.Command.CommandID != resume.Command.CommandID {
				t.Fatal("resume identity was lost")
			}
			if _, err := recoveredResume.ReplayCommand(); !errors.Is(err, agent.ErrCommandInputUnavailable) {
				t.Fatalf("resume replay error = %v", err)
			}
			if _, err := reopened.RequeuePendingResume("ses_resume", resume.Command.CommandID, resume.Replay); !errors.Is(err, agent.ErrCommandInputUnavailable) {
				t.Fatalf("unavailable resume received a fresh command identity: %v", err)
			}
			recoveredSteer, found := reopened.PendingSteer(steer.SessionID())
			if !found || recoveredSteer.CommandID() != steer.CommandID() {
				t.Fatal("steer identity was lost")
			}
			if _, err := recoveredSteer.ReplayCommand(); !errors.Is(err, agent.ErrCommandInputUnavailable) {
				t.Fatalf("steer replay error = %v", err)
			}
			if err := reopened.SaveDraft(start.SessionID, agent.Message{Text: "new work remains editable"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type inputWriteFailure struct {
	Persistence
	prefix string
}

func (p inputWriteFailure) Replace(name string, body []byte) error {
	if strings.HasPrefix(name, p.prefix) {
		return errors.New("injected input publication failure")
	}
	return p.Persistence.Replace(name, body)
}

func (p inputWriteFailure) Close() error { return closePersistence(p.Persistence) }

func TestInputPublicationFailureLeavesQueuedCommandEditable(t *testing.T) {
	for _, prefix := range []string{"inputs", "sessions"} {
		t.Run(prefix, func(t *testing.T) {
			directory := t.TempDir()
			store, err := OpenDirectory(directory, Config{})
			if err != nil {
				t.Fatal(err)
			}
			command := agent.StartRun{
				CommandID: "cli_44444444444444444444444444444444", SessionID: "ses_failure", Message: agent.Message{Text: "original"},
			}
			if err := store.StagePendingRun(queuedPendingRun(command)); err != nil {
				t.Fatal(err)
			}
			store.persistence = inputWriteFailure{Persistence: store.persistence, prefix: prefix}
			input := []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: command.Message.Text}}
			prepared, err := store.PrepareInput(t.Context(), command.Message, input)
			if err == nil {
				err = store.MarkPendingRunDispatching(command.SessionID, command.CommandID, commandreplay.UnprotectedGuard(), prepared)
			}
			if err == nil {
				t.Fatal("injected publication failure was ignored")
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenDirectory(directory, Config{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			pending := reopened.PendingRuns(command.SessionID)
			if len(pending) != 1 || pending[0].State != PendingRunQueued || pending[0].InputDigest != "" || pending[0].Command.Input != nil {
				t.Fatalf("failed publication changed delivery state: %+v", pending)
			}
			pending[0].Command.Message.Text = "editable after failure"
			if err := reopened.SavePendingRuns(command.SessionID, pending); err != nil {
				t.Fatal(err)
			}
			names, err := reopened.persistence.ListFiles("inputs", ".json")
			wantFiles := 0
			if prefix == "sessions" {
				wantFiles = 1
			}
			if len(names) != wantFiles || (err != nil && !errors.Is(err, fs.ErrNotExist)) {
				t.Fatalf("opening deleted an unpublished input: %v, %v", names, err)
			}
		})
	}
}

func preparedTestInput(t *testing.T, store *Store, message agent.Message, blocks []protocol.ContentBlock) *PreparedInput {
	t.Helper()
	input, err := store.PrepareInput(t.Context(), message, blocks)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestOpeningAnotherStoreDoesNotDeleteInputAwaitingPublication(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	pending := steerTestPending(t, "ses_preparing", steerTestAttachment(t.TempDir()))
	prepared := preparedTestInput(t, store, pending.Message(), pending.Command().Input)
	reader, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	source := agent.Message{Text: "/steer " + pending.Message().Text, Attachments: pending.Message().Attachments}
	if err := store.SaveDraft(pending.SessionID(), source); err != nil {
		t.Fatal(err)
	}
	if err := store.StagePendingSteer(pending, source, prepared); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	replayed, found := reopened.PendingSteer(pending.SessionID())
	command, err := replayed.ReplayCommand()
	if !found || err != nil || !command.Equal(pending.Command()) {
		t.Fatalf("concurrent opening removed active preparation: %t, %v", found, err)
	}
}

type pausedInputPersistence struct {
	Persistence
	entered chan struct{}
	release chan struct{}
}

func (p pausedInputPersistence) Replace(name string, body []byte) error {
	if strings.HasPrefix(name, "inputs/") {
		close(p.entered)
		<-p.release
	}
	return p.Persistence.Replace(name, body)
}

func (p pausedInputPersistence) Close() error { return closePersistence(p.Persistence) }

func TestInputPreparationDoesNotHoldTheAuthoringLock(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	persistence := pausedInputPersistence{Persistence: store.persistence, entered: make(chan struct{}), release: make(chan struct{})}
	store.persistence = persistence
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := store.PrepareInput(ctx, agent.Message{Text: "input"}, []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "input"}})
		finished <- err
	}()
	<-persistence.entered
	// A draft write must finish while the large immutable write is still paused.
	draftSaved := make(chan error, 1)
	go func() { draftSaved <- store.SaveDraft("ses_edit", agent.Message{Text: "still editable"}) }()
	var saveErr error
	select {
	case saveErr = <-draftSaved:
	case <-time.After(time.Second):
		close(persistence.release)
		<-finished
		t.Fatal("immutable content write held the authoring lock")
	}
	cancel()
	close(persistence.release)
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled preparation returned a publishable value: %v", err)
	}
	if saveErr != nil {
		t.Fatal(saveErr)
	}
	if pending := store.PendingRuns("ses_edit"); len(pending) != 0 {
		t.Fatal("preparation published a command journal")
	}
}
