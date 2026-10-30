package terminal

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"
)

func TestHandledNoOpEditsCannotStarveDraftAutosave(t *testing.T) {
	backend := runtimefixture.New()
	backend.Instant = true
	stateDirectory := t.TempDir()
	host, stop, workbenchForHost := runUIFromConfig(t, Config{RuntimeProfile: featuredTerminalProfile(t), Runtime: backend, Workspace: t.TempDir(), OpenWorkbench: persistentTestWorkbench(t, stateDirectory)})
	host.Shows(t, "Ask flame")
	sessionID := firstRuntimeSession(t, backend)
	host.Type("draft survives no-op edits")
	for range len("draft survives no-op edits") {
		host.Send(input.Key{Code: input.Left})
	}

	started := time.Now()
	deadline := started.Add(2 * time.Second)
	var draft authoring.Message
	var found bool
	for time.Now().Before(deadline) {
		host.Send(input.Key{Code: input.Backspace})
		if time.Since(started) >= 5*draftPersistenceDelay {
			var err error
			draft, found, err = storedDraft(workbenchForHost, sessionID)
			if err != nil {
				t.Fatal(err)
			}
			if found && draft.Equal(authoring.Message{Text: "draft survives no-op edits"}) {
				break
			}
		}
		time.Sleep(draftPersistenceDelay / 10)
	}
	if !found || !draft.Equal(authoring.Message{Text: "draft survives no-op edits"}) {
		t.Fatalf("autosaved draft = (%+v, %v), want the authored value while no-op edits continue", draft, found)
	}
	stop()
}

func TestResolvedKeyTextSchedulesDraftAutosave(t *testing.T) {
	backend := runtimefixture.New()
	backend.Instant = true
	stateDirectory := t.TempDir()
	host, stop, workbenchForHost := runUIFromConfig(t, Config{RuntimeProfile: featuredTerminalProfile(t), Runtime: backend, Workspace: t.TempDir(), OpenWorkbench: persistentTestWorkbench(t, stateDirectory)})
	host.Shows(t, "Ask flame")
	sessionID := firstRuntimeSession(t, backend)

	host.Send(input.Key{Text: "你好"})
	host.Shows(t, "你好")
	awaitStoredDraft(t, workbenchForHost, sessionID, authoring.Message{Text: "你好"})
	stop()
}

func TestProgrammaticComposerEditsScheduleDraftAutosave(t *testing.T) {
	tests := []struct {
		name string
		edit func(*testing.T, *programtest.Host)
		want string
	}{
		{
			name: "command palette",
			edit: func(t *testing.T, host *programtest.Host) {
				host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl})
				host.Shows(t, "Commands")
				host.Type("rename")
				host.Press(input.Enter)
			},
			want: "/rename ",
		},
		{
			name: "completion",
			edit: func(_ *testing.T, host *programtest.Host) {
				host.Type("/sho")
				host.Press(input.Enter)
			},
			want: "/shortcuts",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := runtimefixture.New()
			backend.Instant = true
			stateDirectory := t.TempDir()
			host, stop, workbenchForHost := runUIFromConfig(t, Config{RuntimeProfile: featuredTerminalProfile(t), Runtime: backend, Workspace: t.TempDir(), OpenWorkbench: persistentTestWorkbench(t, stateDirectory)})
			host.Shows(t, "Ask flame")
			sessionID := firstRuntimeSession(t, backend)

			test.edit(t, host)
			host.Shows(t, test.want)
			awaitStoredDraft(t, workbenchForHost, sessionID, authoring.Message{Text: test.want})
			stop()
		})
	}
}

func awaitStoredDraft(t *testing.T, currentWorkbench func() *workbench.Store, sessionID string, want authoring.Message) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var draft authoring.Message
	var found bool
	var err error
	for time.Now().Before(deadline) {
		draft, found, err = storedDraft(currentWorkbench, sessionID)
		if err == nil && found && draft.Equal(want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("autosaved draft = (%+v, %v, %v), want %+v", draft, found, err, want)
}

func storedDraft(currentWorkbench func() *workbench.Store, sessionID string) (authoring.Message, bool, error) {
	store := currentWorkbench()
	if store == nil {
		return authoring.Message{}, false, errors.New("workbench has not opened")
	}
	message, found := store.Draft(sessionID)
	return message, found, nil
}
