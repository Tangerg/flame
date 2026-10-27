package bootstrap

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

const steerCrashKey = "original-steer-crash"

func TestSteerAcceptanceCrashHelper(t *testing.T) {
	phase := os.Getenv("FLAME_TEST_STEER_CRASH")
	if phase == "" {
		return
	}
	home := os.Getenv("FLAME_HOME")
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 1)
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
		if phase != "commit" {
			t.Error("original command query dispatched model work")
		}
		entered <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	})}
	if phase == "commit" {
		tx := stores.Transactor
		stores.Transactor = func(ctx context.Context, apply func(context.Context) error) error {
			if err := tx(ctx, apply); err != nil {
				return err
			}
			record, err := stores.CommandAcceptances.Lookup(ctx, steerCrashKey)
			if err != nil {
				return err
			}
			if record.Found {
				checkpoint, found, err := stores.ExecutorCheckpoints.LoadSessionCheckpoint(ctx, record.Acceptance.SessionID)
				if err != nil || !found || len(checkpoint.Payload) == 0 {
					t.Fatalf("accepted steer lost context: %t,%v", found, err)
				}
				if _, found, err := stores.ExecutorCheckpoints.LoadExecutionTree(ctx, record.Acceptance.SessionID, checkpoint.RootMemberID); err != nil || !found {
					t.Fatalf("accepted steer lost head: %t,%v", found, err)
				}
				os.Exit(commandCrashExit)
			}
			return nil
		}
	}
	host, api := buildProtocolRuntime(t, protocolRuntimeConfig(t, stores, model), home)
	defer host.Close()
	endpoint := host.Endpoint()
	options := acceptedCommandOptions()
	options.IdempotencyKey = steerCrashKey
	input := []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "retain this original steer"}}
	if phase == "commit" {
		session, err := api.CreateSession(t.Context(), protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: home}})
		if err != nil {
			t.Fatal(err)
		}
		started, events, err := api.StartRun(t.Context(), protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "wait"}}})
		if err != nil {
			t.Fatal(err)
		}
		_ = collectRunEvents(events)
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("model did not begin")
		}
		result := endpoint.Invoke(t.Context(), delivery.RunsSteer, protocol.SteerRunRequest{RunID: started.RunID, ExpectedSegmentID: started.SegmentID, Input: input}, options)
		t.Fatalf("steer crash cut not reached: %v", result.Failure)
	}
	receipt, err := endpoint.Call[protocol.GetRunAcceptanceRequest, *protocol.RunAcceptance](t.Context(), delivery.RunsGetAcceptance, protocol.GetRunAcceptanceRequest{Namespace: testsupport.IdempotencyNamespace, Key: steerCrashKey, Operation: protocol.RunCommandSteer}, delivery.Options{})
	if err != nil || receipt.Type != protocol.RunAcceptanceAccepted || receipt.Run.Status != protocol.RunStatusFinished || receipt.AppliedItem != nil || len(receipt.Input) != 1 || receipt.Input[0].Text != input[0].Text {
		t.Fatalf("cold receipt=%+v,%v", receipt, err)
	}
	snapshot, err := api.GetSessionSnapshot(t.Context(), protocol.GetSessionSnapshotRequest{SessionID: receipt.SessionID})
	if err != nil || len(snapshot.SteerInputs) != 1 || snapshot.SteerInputs[0].UserItemID != receipt.UserItemID {
		t.Fatalf("cold pending snapshot=%+v,%v", snapshot, err)
	}
	for range 2 {
		replay := endpoint.Invoke(t.Context(), delivery.RunsSteer, protocol.SteerRunRequest{RunID: receipt.RunID, ExpectedSegmentID: receipt.SegmentID, Input: input}, options)
		if replay.Failure != nil || replay.Value.(*protocol.SteerRunResponse).UserItemID != receipt.UserItemID {
			t.Fatalf("cold original replay=%+v,%v", replay.Value, replay.Failure)
		}
	}
	input[0].Text = "changed draft"
	if err := endpoint.Invoke(t.Context(), delivery.RunsSteer, protocol.SteerRunRequest{RunID: receipt.RunID, ExpectedSegmentID: receipt.SegmentID, Input: input}, options).Failure; !errors.Is(err, protocol.ErrIdempotencyConflict) {
		t.Fatalf("changed original command=%v", err)
	}
}

func TestSteerAcceptanceSurvivesCommitBeforeResponseCrash(t *testing.T) {
	t.Setenv("FLAME_HOME", t.TempDir())
	for _, phase := range []string{"commit", "query"} {
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSteerAcceptanceCrashHelper$")
		command.Env = append(os.Environ(), "FLAME_TEST_STEER_CRASH="+phase)
		output, err := command.CombinedOutput()
		cancel()
		if phase == "commit" {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != commandCrashExit {
				t.Fatalf("crash: %v\n%s", err, output)
			}
		} else if err != nil {
			t.Fatalf("cold query: %v\n%s", err, output)
		}
	}
}
