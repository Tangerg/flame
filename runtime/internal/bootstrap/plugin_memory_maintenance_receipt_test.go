package bootstrap

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	memoryrpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestMemoryMaintenanceCommittedReplyLossRecoversWithoutAnotherModelCall(t *testing.T) {
	source, evidence := managedMessageLossPackage(t, "memory", memoryrpc.Maintain, "response")
	data := t.TempDir()
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	connect := func(loss bool) *sdk.ClientSession {
		t.Helper()
		binary := filepath.Join(source, "bin", "memory")
		var command *exec.Cmd
		if loss {
			command = exec.CommandContext(ctx, filepath.Join(source, "bin", "message-loss"), evidence, binary, memoryrpc.Maintain, "", "response")
		} else {
			command = exec.CommandContext(ctx, binary)
		}
		command.Env = append(os.Environ(), "PLUGIN_ROOT="+source, "PLUGIN_DATA="+data)
		client := sdk.NewClient(&sdk.Implementation{Name: "maintenance-receipt"}, nil)
		if err := pluginsdk.RegisterClient(client); err != nil {
			t.Fatal(err)
		}
		session, err := client.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		return session
	}
	session := connect(true)
	broker := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	broker.AdmitSession(session)
	var calls atomic.Int32
	grant, release, err := memoryrpc.GrantModels(ctx, broker, session, func(context.Context, memoryrpc.Prompt) (string, error) {
		calls.Add(1)
		return "- retained publication", nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	input := memoryrpc.MaintenanceInput{SessionID: "ses_maintenance_receipt", Project: project, Compacted: true, Grant: grant, Conversation: memoryrpc.Conversation{MessageCount: 4, Transcript: "captured conversation"}}
	_, err = pluginsdk.CallCommand[memoryrpc.MaintenanceInput, memoryrpc.MaintenanceResult](ctx, session, memoryrpc.Maintain, "original", input)
	if err == nil {
		t.Fatal("withheld maintenance response became observed success")
	}
	if _, known := errors.AsType[*pluginsdk.Failure](err); known {
		t.Fatalf("transport loss became known refusal: %v", err)
	}
	if _, err := os.Stat(evidence); err != nil {
		t.Fatalf("no committed response witnessed: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("actual extraction/curation calls=%d", calls.Load())
	}
	release()
	broker.RevokeSession(session)
	_ = session.Close()
	session = connect(false)
	result, found, err := pluginsdk.ReadReceipt[memoryrpc.MaintenanceInput, memoryrpc.MaintenanceResult](ctx, session, "original", memoryrpc.Maintain, input)
	if err != nil || !found || !result.Published {
		t.Fatalf("original publication receipt=%+v, %t, %v", result, found, err)
	}
	replayed, err := pluginsdk.CallCommand[memoryrpc.MaintenanceInput, memoryrpc.MaintenanceResult](ctx, session, memoryrpc.Maintain, "original", input)
	if err != nil || replayed != result || calls.Load() != 2 {
		t.Fatalf("receipt replay restarted models: %+v, %d, %v", replayed, calls.Load(), err)
	}
	input.Compacted = false
	if _, err := pluginsdk.CallCommand[memoryrpc.MaintenanceInput, memoryrpc.MaintenanceResult](ctx, session, memoryrpc.Maintain, "original", input); err == nil {
		t.Fatal("same identity adopted changed maintenance input")
	}
	listed, err := pluginsdk.Call[memoryrpc.ListInput, memoryrpc.Page](ctx, session, memoryrpc.List, memoryrpc.ListInput{Scope: "project", CWD: project})
	if err != nil || len(listed.Rows) != 1 || listed.Rows[0].Content != "retained publication" {
		t.Fatalf("recovered private memory=%+v, %v", listed, err)
	}
}
