package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"testing"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestReceiptLookupIsReadOnlyAndPreservesOutcomeEvidence(t *testing.T) {
	for _, test := range []struct {
		name    string
		receipt Receipt
		found   bool
		value   string
		failure string
		invalid bool
	}{
		{name: "missing"},
		{name: "success", receipt: Receipt{Found: true, Reply: []byte(`{"value":"saved"}`)}, found: true, value: "saved"},
		{name: "refusal", receipt: Receipt{Found: true, Reply: []byte(`{"failure":{"code":"conflict","message":"revision changed"}}`)}, found: true, failure: "conflict"},
		{name: "missing evidence", receipt: Receipt{Found: true}, invalid: true},
		{name: "undeclared evidence", receipt: Receipt{Reply: []byte(`{"value":"saved"}`)}, invalid: true},
		{name: "null evidence", receipt: Receipt{Found: true, Reply: []byte(`null`)}, found: true, invalid: true},
		{name: "ambiguous evidence", receipt: Receipt{Found: true, Reply: []byte(`{"value":"saved","failure":{"code":"conflict","message":"failed"}}`)}, found: true, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
			if err := RegisterServer(server, ReceiptMethod, nil, func(_ context.Context, _ *sdk.ServerSession, input ReceiptRequest) (Receipt, error) {
				digest, err := InputDigest("input")
				if err != nil || input.ID != "original" || input.Method != "review.update" || input.InputDigest != digest {
					t.Fatalf("lookup changed original binding: %+v, %v", input, err)
				}
				return test.receipt, nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := RegisterCommand(server, "review.update", nil, func(context.Context, *sdk.ServerSession, Command[string]) (string, error) {
				t.Error("receipt lookup executed mutation")
				return "", nil
			}); err != nil {
				t.Fatal(err)
			}
			client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
			if err := RegisterClient[ReceiptRequest, Receipt](client, ReceiptMethod); err != nil {
				t.Fatal(err)
			}
			session := connectMethodSessions(t, server, client)
			value, found, err := ReadReceipt[string, string](t.Context(), session, "original", "review.update", "input")
			if found != test.found || value != test.value {
				t.Fatalf("receipt = %q, %t, %v", value, found, err)
			}
			failure, known := errors.AsType[*Failure](err)
			switch {
			case test.failure != "":
				if !known || failure.Code != test.failure {
					t.Fatalf("refusal = %v", err)
				}
			case test.invalid:
				if err == nil || known {
					t.Fatalf("malformed evidence became a known outcome: %v", err)
				}
			case err != nil:
				t.Fatal(err)
			}
		})
	}
}

func TestReceiptRequestRefusesMissingAndInvalidBindings(t *testing.T) {
	digest, err := InputDigest("input")
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		`{}`, `{"id":"original","method":"review.update","inputDigest":"short"}`,
		`{"id":"original","method":"","inputDigest":"` + digest + `"}`,
		`{"id":"","method":"review.update","inputDigest":"` + digest + `"}`,
		`{"id":"original","method":"review.update","inputDigest":"` + digest + `","extra":true}`,
	} {
		var request ReceiptRequest
		if err := json.Unmarshal([]byte(data), &request); err == nil {
			t.Fatalf("invalid binding admitted: %s", data)
		}
	}
}
