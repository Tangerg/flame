package plugin_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	private "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

func TestExecutableCommandDecoratorFailurePreservesCommittedInnerReceipt(t *testing.T) {
	method, err := pluginmcp.NewMethod[string, int64](portable.MethodConfig{Name: "example.records/update.v1", Kind: portable.Command, Target: "workspace", Description: "Update the private record."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "example.records/decoration.v1", Composition: plugincontract.Decoration, Scope: "workspace"}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("FLAME_TEST_COMMAND_DECORATOR") == "1" {
		server := sdk.NewServer(&sdk.Implementation{Name: "independent-command-decorator"}, nil)
		backend, err := pluginmcp.NewBackend(server)
		if err == nil {
			err = method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[string]) (int64, error) {
				next, err := pluginmcp.CallbackFromContext(ctx)
				if err != nil {
					return 0, err
				}
				value, err := pluginmcp.CallHost(ctx, next, method, command.ID, command.Input)
				if err != nil || value != 1 {
					return 0, errors.Join(err, errors.New("inner original outcome was replaced"))
				}
				return 0, errors.New("outer decorator failed after the private commit")
			})
		}
		if err == nil {
			err = backend.BindCapability("decorator", contract)
		}
		if err == nil {
			err = backend.Publish()
		}
		if err == nil {
			err = server.Run(context.Background(), &sdk.StdioTransport{})
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	path := filepath.Join(t.TempDir(), "private.db")
	schema := []string{private.CommandsSchema, `CREATE TABLE IF NOT EXISTS counter(value INTEGER NOT NULL)`, `INSERT INTO counter SELECT 0 WHERE NOT EXISTS(SELECT 1 FROM counter)`}
	db, err := sqlite.OpenSchema(t.Context(), path, schema)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	original := framemcp.Invocation{ID: "original-command", Origin: extension.Model, Parent: "run_original", Target: portable.Target{Scope: "workspace", Reference: "original-workspace"}}
	calling, err := pluginmcp.WithInvocation(t.Context(), original)
	if err != nil {
		t.Fatal(err)
	}
	callback, err := pluginmcp.NewCallback(method, nil, func(ctx context.Context, command pluginmcp.Command[string]) (int64, error) {
		owner, err := pluginmcp.InvocationFromContext(ctx)
		if err != nil || owner != original {
			return 0, errors.New("inner calling owner was replaced")
		}
		if command.ID != "original-command" || command.Input != "original-input" {
			return 0, errors.New("inner command identity/input was changed")
		}
		input, err := command.RequestInput()
		if err != nil {
			return 0, err
		}
		return private.Command(ctx, db, method.Name(), string(command.ID), input, nil, func(ctx context.Context, _ string) (private.CommandPlan[int64], error) {
			var previous int64
			if err := sqlite.Connection(ctx, db).QueryRowContext(ctx, `SELECT value FROM counter`).Scan(&previous); err != nil {
				return private.CommandPlan[int64]{}, err
			}
			return private.NewCommandPlan(previous+1, func(ctx context.Context) error {
				_, err := sqlite.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=?`, previous+1)
				return err
			})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "command-caller"}, nil)
	if err := pluginmcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestExecutableCommandDecoratorFailurePreservesCommittedInnerReceipt$", "-test.count=1")
	command.Env = append(os.Environ(), "FLAME_TEST_COMMAND_DECORATOR=1")
	session, err := client.Connect(t.Context(), &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	export, err := pluginmcp.ResolveCapability(t.Context(), session, "decorator", contract)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := export.Method(method.Name())
	if err != nil {
		t.Fatal(err)
	}
	callbacks := pluginmcp.NewCallbacks()
	defer callbacks.Close()
	if err := callbacks.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	ref, release, err := callbacks.Open(calling, session, pluginmcp.CallbackLimits{MaxRequestBytes: 4096, MaxResponseBytes: 4096}, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	metadata, err := pluginmcp.CallbackMetadata(ref)
	if err != nil {
		t.Fatal(err)
	}
	input, err := framevalue.EncodeJSON("original-input")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := bound.InvokeWithMetadata(calling, "original-command", input, metadata); err == nil {
			t.Fatal("outer failure appeared successful")
		} else if _, known := errors.AsType[*framemcp.Refusal](err); known {
			t.Fatalf("outer failure claimed a known business refusal: %v", err)
		}
	}
	release()
	if _, err := pluginmcp.CallHost(t.Context(), ref, method, "original-command", "original-input"); !errors.Is(err, pluginmcp.ErrStaleCall) {
		t.Fatalf("finished continuation entered a replacement lifetime: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.OpenSchema(t.Context(), path, schema)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	receipt, err := private.ReadReceipt(t.Context(), reopened, pluginmcp.ReceiptRequest{ID: "original-command", Method: method.Name(), InputDigest: input.Digest()})
	if err != nil || !receipt.Found {
		t.Fatalf("outer failure erased inner receipt after reopen: %+v, %v", receipt, err)
	}
	value, err := pluginmcp.DecodeReply[int64](receipt.Reply)
	if err != nil || value != 1 {
		t.Fatalf("original inner receipt changed: %d, %v", value, err)
	}
	var count int
	if err := reopened.Read(t.Context(), func(reader sqlite.Reader) error {
		return reader.QueryRowContext(t.Context(), `SELECT value FROM counter`).Scan(&count)
	}); err != nil || count != 1 {
		t.Fatalf("command redelivery repeated the mutation: count=%d, %v", count, err)
	}
}
