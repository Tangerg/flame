package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

const startCrashKey = "start-opening-crash"

func TestStartAcceptanceCrashHelper(t *testing.T) {
	phase := os.Getenv("FLAME_TEST_START_CRASH")
	if phase == "" {
		return
	}
	home := os.Getenv("FLAME_HOME")
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	if phase == "commit" {
		tx := stores.Transactor
		stores.Transactor = func(ctx context.Context, apply func(context.Context) error) error {
			if err := tx(ctx, apply); err != nil {
				return err
			}
			record, err := stores.CommandAcceptances.Lookup(ctx, startCrashKey)
			if err != nil {
				return err
			}
			if record.Found {
				os.Exit(commandCrashExit)
			}
			return nil
		}
	}
	model := delegateRestartModel{chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		if err := os.WriteFile(filepath.Join(home, "unexpected-model-call"), []byte("called"), 0o600); err != nil {
			return nil, err
		}
		return nil, errors.New("start crash fixture must not dispatch a model")
	})}
	host, api := buildProtocolRuntime(t, protocolRuntimeConfig(t, stores, model), home)
	defer host.Close()
	endpoint := acceptedCommandEndpoint(t, api, stores)
	if phase == "commit" {
		session, err := api.CreateSession(t.Context(), protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
		if err != nil {
			t.Fatal(err)
		}
		options := acceptedCommandOptions()
		options.IdempotencyKey = startCrashKey
		result := endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{
			SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "retain the original opening"}},
		}, options)
		t.Fatalf("start commit crash was not reached: %v", result.Failure)
	}
	ack, err := endpoint.Call[protocol.GetRunAcceptanceRequest, *protocol.RunAcceptance](t.Context(), delivery.RunsGetAcceptance,
		protocol.GetRunAcceptanceRequest{Namespace: testsupport.IdempotencyNamespace, Key: startCrashKey, Operation: protocol.RunCommandStart}, delivery.Options{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := api.ListRuns(t.Context(), protocol.ListRunsRequest{SessionID: ack.SessionID})
	if err != nil || len(page.Data) != 1 || page.Data[0].ID != ack.RunID {
		t.Fatalf("lookup changed original Run history: %+v,%v", page, err)
	}
	encoded, err := json.Marshal(ack)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "start-acceptance.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStartAcceptanceSurvivesCommitBeforeResponseCrash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	invoke := func(phase string) error {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStartAcceptanceCrashHelper$")
		command.Env = append(os.Environ(), "FLAME_TEST_START_CRASH="+phase)
		output, err := command.CombinedOutput()
		if phase == "query" && err != nil {
			t.Fatalf("fresh Runtime query: %v\n%s", err, output)
		}
		return err
	}
	var exit *exec.ExitError
	if err := invoke("commit"); !errors.As(err, &exit) || exit.ExitCode() != commandCrashExit {
		t.Fatalf("start crash: %v", err)
	}
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	record, err := stores.CommandAcceptances.Lookup(t.Context(), startCrashKey)
	if err != nil || !record.Found || record.CurrentRun == nil {
		t.Fatalf("committed opening: %+v,%v", record, err)
	}
	if err := stores.Close(); err != nil {
		t.Fatal(err)
	}
	_ = invoke("query")
	encoded, err := os.ReadFile(filepath.Join(home, "start-acceptance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ack protocol.RunAcceptance
	if err := json.Unmarshal(encoded, &ack); err != nil {
		t.Fatal(err)
	}
	if ack.Type != protocol.RunAcceptanceAccepted || ack.RunID != record.Acceptance.RunID || ack.SegmentID != record.Acceptance.SegmentID || ack.UserItemID != record.Acceptance.UserItemID || ack.SessionID != record.Acceptance.SessionID {
		t.Fatalf("fresh query changed acceptance: %+v, original=%+v", ack, record.Acceptance)
	}
	if _, err := os.Stat(filepath.Join(home, "unexpected-model-call")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("acceptance query dispatched model work: %v", err)
	}
}
