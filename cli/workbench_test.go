package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/cli/internal/delivery/cmd"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
)

func TestWorkbenchFactoryIsLazyForHelpAndCompletion(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"completion", "bash"}} {
		t.Run(args[0], func(t *testing.T) {
			base := filepath.Join(t.TempDir(), "authoring")
			root := cmd.NewRoot(cmd.Dependencies{
				OpenRuntime: func(context.Context, string) (cmd.Runtime, cmd.RuntimeProfile, error) {
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
		if err := store.SaveDraft(sessionID, prompt.Message{Text: "target: " + endpoint}); err != nil {
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
