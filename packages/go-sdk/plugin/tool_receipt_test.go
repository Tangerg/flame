package plugin

import (
	"context"
	"errors"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"testing"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

func TestToolReceiptReadsOnlyTheOriginalOwnerOutput(t *testing.T) {
	output, err := chat.NewJSONToolOutput([]byte(`{"ordinal":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	known, err := NewToolReceipt(output, false)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := NewToolReceipt(chat.NewTextToolOutput("record refused"), true)
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsonvalue.Parse([]byte(`{"ordinal":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name                   string
		receipt                ToolReceipt
		found, failed, invalid bool
		queryErr               error
	}{
		{name: "absent"},
		{name: "original unrelated Tool", receipt: known, found: true},
		{name: "original refusal", receipt: failed, found: true, failed: true},
		{name: "missing output", receipt: ToolReceipt{Found: true}, invalid: true},
		{name: "null output", receipt: ToolReceipt{Found: true, Output: "null"}, invalid: true},
		{name: "corrupt output", receipt: ToolReceipt{Found: true, Output: `{"content":[{"kind":"invalid"}]}`}, invalid: true},
		{name: "undeclared output", receipt: ToolReceipt{Output: known.Output}, invalid: true},
		{name: "undeclared failure", receipt: ToolReceipt{IsError: true}, invalid: true},
		{name: "lookup conflict", queryErr: &portable.Refusal{Code: "command_conflict", Message: "changed input"}, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := sdk.NewServer(&sdk.Implementation{Name: "independent records"}, nil)
			backend := testBackend(t, server)
			if err := testMethod[qimumcp.ReceiptRequest, ToolReceipt](t, ToolReceiptMethod, portable.Query).RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command qimumcp.Command[qimumcp.ReceiptRequest]) (ToolReceipt, error) {
				request := command.Input
				if request.ID != "tool:original" || request.Method != "append_record" || request.InputDigest != input.Digest() {
					t.Fatalf("read changed original identity: %+v", request)
				}
				return test.receipt, test.queryErr
			}); err != nil {
				t.Fatal(err)
			}
			if err := testMethod[string, string](t, "append_record", portable.Mutation).RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, qimumcp.Command[string]) (string, error) {
				t.Error("receipt lookup replayed the mutation")
				return "", nil
			}); err != nil {
				t.Fatal(err)
			}
			client := sdk.NewClient(&sdk.Implementation{Name: "Core"}, nil)
			if err := qimumcp.RegisterClient(client); err != nil {
				t.Fatal(err)
			}
			session := connectPublishedBackend(t, backend, server, client)
			actual, err := ReadToolReceipt(t.Context(), session, "original", "append_record", input)
			_, failed := errors.AsType[*tool.Failure](err)
			if (actual != nil) != test.found || failed != test.failed {
				t.Fatalf("receipt changed outcome: %+v, %v", actual, err)
			}
			if test.invalid != (err != nil && !failed) {
				t.Fatalf("invalid evidence classified as an outcome: %v", err)
			}
			if test.found && !test.failed && string(actual.Details) != string(output.Details) {
				t.Fatalf("receipt narrowed original JSON: %s", actual.Details)
			}
			if test.failed {
				text, textOnly := actual.Text()
				if !textOnly || text != "record refused" {
					t.Fatalf("lost original refusal output: %+v", actual)
				}
			}
		})
	}
}
