package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"

	"github.com/Tangerg/flame/apps/cli/internal/delivery/cmd"
)

func TestWorkbenchFactoryIsLazyForHelpAndCompletion(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"completion", "bash"}} {
		t.Run(args[0], func(t *testing.T) {
			base := filepath.Join(t.TempDir(), "authoring")
			root := cmd.NewRoot(cmd.Dependencies{
				OpenRuntime: func(context.Context, settings.Runtime) (cmd.Runtime, cmd.RuntimeProfile, error) {
					t.Fatal("help or completion opened Runtime")
					return nil, nil, nil
				},
				OpenWorkbench: workbenchFactory(base),
			})
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(args)
			if err := root.ExecuteContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(base); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("help or completion created authoring state: %v", err)
			}
		})
	}
}

func TestWorkbenchFactoryIsolatesAndRecoversEachRuntimeTarget(t *testing.T) {
	open := workbenchFactory(t.TempDir())
	const sessionID = "ses_shared_identity"
	endpoints := []string{"", "https://one.example", "https://two.example"}
	for _, endpoint := range endpoints {
		store, err := open(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		if _, found := store.Draft(sessionID); found {
			t.Fatalf("target %q inherited another target's draft", endpoint)
		}
		if err := store.SaveDraft(sessionID, authoring.Message{Text: "target: " + endpoint}); err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, endpoint := range endpoints {
		store, err := open(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		draft, found := store.Draft(sessionID)
		if !found || draft.Text != "target: "+endpoint {
			t.Fatalf("target %q recovered draft %+v, found %v", endpoint, draft, found)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkbenchFactoryRetainsUnknownOpeningUnderOnePartitionOwner(t *testing.T) {
	open := workbenchFactory(t.TempDir())
	const endpoint = "https://runtime.example"
	first, err := open(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	const sessionID = "ses_shared"
	const commandID authoring.CommandID = "cli_00000000000000000000000000000001"
	pending := workbench.PendingRun{
		State:  workbench.PendingRunQueued,
		Replay: authoring.UnprotectedGuard(), CancelReplay: authoring.UnprotectedGuard(),
		Command: authoring.StartRun{CommandID: commandID, SessionID: sessionID, Message: authoring.Message{Text: "original intent"}},
	}
	if err := first.StagePendingRun(pending); err != nil {
		t.Fatal(err)
	}
	guard, err := authoring.NewProtectedGuard("idp_00000000000000000000000000000001", time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.MarkPendingRunDispatching(sessionID, commandID, guard, nil); err != nil {
		t.Fatal(err)
	}
	if second, err := open(endpoint); err == nil {
		_ = second.Close()
		t.Fatal("second partition writer admitted")
	}
	if err := first.SaveDraft(sessionID, authoring.Message{Text: "new draft"}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := open(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got := reopened.PendingRuns(sessionID)
	if len(got) != 1 || got[0].State != workbench.PendingRunDispatching || got[0].Command.CommandID != commandID || got[0].Command.Message.Text != "original intent" {
		t.Fatalf("unknown opening changed: %+v", got)
	}
}
