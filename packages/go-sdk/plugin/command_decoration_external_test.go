package plugin_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	private "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/packages/go-sdk/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestExecutableCommandDecoratorFailurePreservesCommittedInnerReceipt(t *testing.T) {
	method, err := pluginsdk.NewMethod[string, int64](pluginsdk.MethodConfig{Name: "example.records/update.v1", Kind: extension.Mutation, Target: "workspace", Description: "Update the private record."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := extension.NewCapability(extension.CapabilityConfig{Reference: "example.records/decoration.v1", Composition: extension.Decoration, Scope: "workspace"}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("FLAME_TEST_COMMAND_DECORATOR") == "1" {
		server := sdk.NewServer(&sdk.Implementation{Name: "independent-command-decorator"}, nil)
		backend, err := pluginsdk.NewBackend(server)
		if err == nil {
			err = method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[string]) (int64, error) {
				next, err := pluginsdk.CallbackFromContext(ctx)
				if err != nil {
					return 0, err
				}
				value, err := pluginsdk.CallHost(ctx, next, method, command.ID, command.Input)
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
	original := extension.Invocation{ID: "original-command", Origin: extension.Model, Parent: "run_original", Target: extension.Target{Scope: "workspace", Reference: "original-workspace"}}
	calling, err := pluginsdk.WithInvocation(t.Context(), original)
	if err != nil {
		t.Fatal(err)
	}
	callback, err := pluginsdk.NewCallback(method, nil, func(ctx context.Context, command pluginsdk.Command[string]) (int64, error) {
		owner, err := pluginsdk.InvocationFromContext(ctx)
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
		return private.Command(ctx, db, method.Name(), command.ID, command.Input, input, nil, func(ctx context.Context, _ string) (int64, error) {
			connection := sqlite.Connection(ctx, db)
			if _, err := connection.ExecContext(ctx, `UPDATE counter SET value=value+1`); err != nil {
				return 0, err
			}
			var value int64
			err := connection.QueryRowContext(ctx, `SELECT value FROM counter`).Scan(&value)
			return value, err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "command-caller"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestExecutableCommandDecoratorFailurePreservesCommittedInnerReceipt$", "-test.count=1")
	command.Env = append(os.Environ(), "FLAME_TEST_COMMAND_DECORATOR=1")
	session, err := client.Connect(t.Context(), &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	export, err := pluginsdk.ResolveCapability(t.Context(), session, "decorator", contract)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := export.Method(method.Name())
	if err != nil {
		t.Fatal(err)
	}
	callbacks := pluginsdk.NewCallbacks()
	defer callbacks.Close()
	callbacks.AdmitSession(session)
	ref, release, err := callbacks.Open(calling, session, pluginsdk.CallbackLimits{MaxRequestBytes: 4096, MaxResponseBytes: 4096}, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	metadata, err := pluginsdk.CallbackMetadata(ref)
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsonvalue.Encode("original-input")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := bound.InvokeWithMetadata(calling, "original-command", input, metadata); err == nil {
			t.Fatal("outer failure appeared successful")
		} else if _, known := errors.AsType[*extension.Refusal](err); known {
			t.Fatalf("outer failure claimed a known business refusal: %v", err)
		}
	}
	release()
	if _, err := pluginsdk.CallHost(t.Context(), ref, method, "original-command", "original-input"); !errors.Is(err, pluginsdk.ErrStaleCall) {
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
	receipt, err := private.ReadReceipt(t.Context(), reopened, pluginsdk.ReceiptRequest{ID: "original-command", Method: method.Name(), InputDigest: input.Digest()})
	if err != nil || !receipt.Found {
		t.Fatalf("outer failure erased inner receipt after reopen: %+v, %v", receipt, err)
	}
	value, err := pluginsdk.DecodeReply[int64](receipt.Reply)
	if err != nil || value != 1 {
		t.Fatalf("original inner receipt changed: %d, %v", value, err)
	}
	var count int
	if err := reopened.QueryRowContext(t.Context(), `SELECT value FROM counter`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("command redelivery repeated the mutation: count=%d, %v", count, err)
	}
}
