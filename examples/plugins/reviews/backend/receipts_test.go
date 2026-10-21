package reviews

import (
	json "encoding/json/v2"
	"errors"
	"reflect"
	"testing"

	contractsdk "github.com/Tangerg/flame/runtime/capability"

	"github.com/Tangerg/flame/runtime/jsoninput"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestPublicReceiptLookupRetainsOriginalOutcomeAcrossBackendRestart(t *testing.T) {
	directory := t.TempDir()
	store := openTestStore(t, directory)
	input := Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved}
	original, err := testUpdate(t.Context(), store, "original", input)
	if err != nil {
		t.Fatal(err)
	}
	refused, err := testUpdate(t.Context(), store, "refused", input)
	if err != nil || refused.Type != Conflict {
		t.Fatalf("business refusal = %+v, %v", refused, err)
	}
	later := Update{ID: ExampleReviewID, ExpectedRevision: 2, Status: Open}
	if _, err := testUpdate(t.Context(), store, "later", later); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openTestStore(t, directory)
	server, err := NewServer(store)
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "receipt-client"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	backend, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	capability, err := pluginsdk.NewCapabilityBinding(session, "reviews", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	updateMethod, err := contracts.Update.Bind(capability)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		id     string
		result UpdateResult
	}{{"original", original}, {"refused", refused}} {
		value, found, err := updateMethod.ReadReceipt(t.Context(), expected.id, input)
		if err != nil || !found || !reflect.DeepEqual(value, expected.result) {
			t.Fatalf("original receipt = %+v, %t, %v", value, found, err)
		}
	}
	if _, found, err := updateMethod.ReadReceipt(t.Context(), "missing", input); err != nil || found {
		t.Fatalf("missing receipt became a mutation/outcome: %t, %v", found, err)
	}
	for _, binding := range []struct {
		method string
		input  Update
	}{{contracts.List.Name(), input}, {contracts.Update.Name(), later}} {
		original, err := capability.Method(binding.method)
		if err != nil {
			t.Fatal(err)
		}
		admitted, err := jsoninput.Encode(binding.input)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = original.ReadReceipt(t.Context(), "original", admitted)
		if err == nil {
			t.Fatal("changed binding adopted an earlier receipt")
		}
		if _, known := errors.AsType[*contractsdk.Refusal](err); known {
			t.Fatalf("binding error fabricated a known mutation result: %v", err)
		}
	}
	current, err := store.List(t.Context())
	if err != nil || len(current) != 1 || current[0].Revision != 3 || current[0].Status != Open {
		t.Fatalf("receipt lookup changed current facts: %+v, %v", current, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := updateMethod.ReadReceipt(t.Context(), "original", input); err == nil {
		t.Fatal("closed owner invented receipt evidence")
	}
}

func TestMalformedPrivateReceiptCannotBecomeKnownEvidence(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	contracts, err := NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	input := Update{ID: ExampleReviewID, ExpectedRevision: 1, Status: Resolved}
	if _, err := testUpdate(t.Context(), store, "original", input); err != nil {
		t.Fatal(err)
	}
	digest, err := jsoninput.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range []string{`{"type":"updated"}`, `{"type":"unknown"}`, `{"type":"notFound","extra":true}`} {
		if _, err := store.db.ExecContext(t.Context(), `UPDATE invocations SET result=? WHERE id='original'`, []byte(result)); err != nil {
			t.Fatal(err)
		}
		receipt, err := store.Receipt(t.Context(), pluginsdk.ReceiptRequest{ID: "original", Method: contracts.Update.Name(), InputDigest: digest.Digest()}, contracts.Update.Name())
		if err == nil || receipt.Found {
			t.Fatalf("corrupt private result became receipt evidence: %s, %v", result, err)
		}
	}
	if _, err := store.db.ExecContext(t.Context(), `UPDATE invocations SET result=? WHERE id='original'`, []byte(`{"type":"notFound"}`)); err != nil {
		t.Fatal(err)
	}
	var changed map[string]any
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &changed); err != nil {
		t.Fatal(err)
	}
	changed["status"] = "invalid"
	data, err = json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(t.Context(), `UPDATE invocations SET input_json=? WHERE id='original'`, data); err != nil {
		t.Fatal(err)
	}
	if receipt, err := store.Receipt(t.Context(), pluginsdk.ReceiptRequest{ID: "original", Method: contracts.Update.Name(), InputDigest: digest.Digest()}, contracts.Update.Name()); err == nil || receipt.Found {
		t.Fatalf("corrupt binding became known evidence: %+v, %v", receipt, err)
	}
}
