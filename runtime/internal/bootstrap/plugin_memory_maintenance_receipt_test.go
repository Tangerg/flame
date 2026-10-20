package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	memoryrpc "github.com/Tangerg/flame/plugins/memory/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
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
	grant, release, err := pluginservices.GrantMemoryModels(ctx, broker, session, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		return completedTextResponse("- retained publication"), nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	input := memoryrpc.MaintenanceInput{SessionID: "ses_maintenance_receipt", Project: project, Compacted: true, Conversation: memoryrpc.Conversation{MessageCount: 4, Transcript: "captured conversation"}}
	metadata, err := pluginsdk.CallbackMetadata(grant)
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := memoryrpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	exported, err := pluginsdk.NewCapabilityBinding(session, "memory", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := exported.Method(memoryrpc.Maintain)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = binding.InvokeWithMetadata(ctx, "original", admitted, metadata)
	if err == nil {
		t.Fatal("withheld maintenance response became observed success")
	}
	if _, known := errors.AsType[*capability.Refusal](err); known {
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
	db, err := sql.Open("sqlite", filepath.Join(data, "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var persisted string
	if err := db.QueryRowContext(ctx, `SELECT business_input_json FROM commands WHERE id='original'`).Scan(&persisted); err != nil || persisted != admitted.String() {
		t.Fatalf("private receipt did not retain only the original business input: %s, %v", persisted, err)
	}
	result, found, err := testsupport.ReceiptCapability[memoryrpc.MaintenanceInput, memoryrpc.MaintenanceResult](ctx, session, memoryContract(t), "memory", memoryrpc.Maintain, "original", input)
	if err != nil || !found || !result.Published {
		t.Fatalf("original publication receipt=%+v, %t, %v", result, found, err)
	}
	replayed, err := testsupport.CallCapability[memoryrpc.MaintenanceInput, memoryrpc.MaintenanceResult](ctx, session, memoryContract(t), "memory", memoryrpc.Maintain, "original", input, sdk.ParamsBase{})
	if err != nil || replayed != result || calls.Load() != 2 {
		t.Fatalf("receipt replay restarted models: %+v, %d, %v", replayed, calls.Load(), err)
	}
	input.Compacted = false
	if _, err := testsupport.CallCapability[memoryrpc.MaintenanceInput, memoryrpc.MaintenanceResult](ctx, session, memoryContract(t), "memory", memoryrpc.Maintain, "original", input, sdk.ParamsBase{}); err == nil {
		t.Fatal("same identity adopted changed maintenance input")
	}
	listed, err := testsupport.CallCapability[memoryrpc.ListInput, memoryrpc.Page](ctx, session, memoryContract(t), "memory", memoryrpc.List, "", memoryrpc.ListInput{Scope: "project", CWD: project}, sdk.ParamsBase{})
	if err != nil || len(listed.Rows) != 1 || listed.Rows[0].Content != "retained publication" {
		t.Fatalf("recovered private memory=%+v, %v", listed, err)
	}
}

func memoryContract(t *testing.T) *capability.Contract {
	t.Helper()
	contracts, err := memoryrpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	return contracts.Capability
}
