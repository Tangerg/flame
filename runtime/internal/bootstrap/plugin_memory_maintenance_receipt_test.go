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

	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestMemoryMaintenanceCommittedReplyLossRecoversWithoutAnotherModelCall(t *testing.T) {
	source, evidence := managedMessageLossPackage(t, "memory", memorycontract.MaintainV1Method, "response")
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
			command = exec.CommandContext(ctx, filepath.Join(source, "bin", "message-loss"), evidence, binary, memorycontract.MaintainV1Method, "", "response")
		} else {
			command = exec.CommandContext(ctx, binary)
		}
		command.Env = append(os.Environ(), "PLUGIN_ROOT="+source, "PLUGIN_DATA="+data)
		client := sdk.NewClient(&sdk.Implementation{Name: "maintenance-receipt"}, nil)
		if err := qimumcp.RegisterClient(client); err != nil {
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
	broker := qimumcp.NewCallbacks()
	t.Cleanup(func() {
		if err := broker.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := broker.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	grant, release, err := pluginservices.GrantMemoryModels(ctx, broker, session, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		return completedTextResponse("- retained publication"), nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	input := memorycontract.MaintainV1Input{SessionID: "ses_maintenance_receipt", Project: project, Compacted: true}
	input.Conversation.MessageCount = 4
	input.Conversation.Transcript = "captured conversation"
	metadata, err := qimumcp.CallbackMetadata(grant)
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := portable.ParseCapability([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	exported, err := qimumcp.NewCapabilityBinding(session, "memory", contracts)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := exported.Method(memorycontract.MaintainV1Method)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = binding.InvokeWithMetadata(ctx, "original", admitted, metadata)
	if err == nil {
		t.Fatal("withheld maintenance response became observed success")
	}
	if _, known := errors.AsType[*portable.Refusal](err); known {
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
	result, found, err := testsupport.ReceiptCapability[memorycontract.MaintainV1Input, memorycontract.MaintainV1Output](ctx, session, memoryContract(t), "memory", memorycontract.MaintainV1Method, "original", input)
	if err != nil || !found || !result.Published {
		t.Fatalf("original publication receipt=%+v, %t, %v", result, found, err)
	}
	replayed, err := testsupport.CallCapability[memorycontract.MaintainV1Input, memorycontract.MaintainV1Output](ctx, session, memoryContract(t), "memory", memorycontract.MaintainV1Method, "original", input, sdk.ParamsBase{})
	if err != nil || replayed != result || calls.Load() != 2 {
		t.Fatalf("receipt replay restarted models: %+v, %d, %v", replayed, calls.Load(), err)
	}
	input.Compacted = false
	if _, err := testsupport.CallCapability[memorycontract.MaintainV1Input, memorycontract.MaintainV1Output](ctx, session, memoryContract(t), "memory", memorycontract.MaintainV1Method, "original", input, sdk.ParamsBase{}); err == nil {
		t.Fatal("same identity adopted changed maintenance input")
	}
	listed, err := testsupport.CallCapability[memorycontract.ListV2Input, memorycontract.ListV2Output](ctx, session, memoryContract(t), "memory", memorycontract.ListV2Method, "", memorycontract.ListV2Input{Scope: memorycontract.ListV2InputScopeProject, CWD: project}, sdk.ParamsBase{})
	if err != nil || len(listed.Rows) != 1 || listed.Rows[0].Content != "retained publication" {
		t.Fatalf("recovered private memory=%+v, %v", listed, err)
	}
}

func memoryContract(t *testing.T) *portable.Capability {
	t.Helper()
	contracts, err := portable.ParseCapability([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	return contracts
}
