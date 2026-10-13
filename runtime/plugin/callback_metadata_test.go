package plugin

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestCallbackMetadataChangesWithoutChangingAdmittedBusinessInput(t *testing.T) {
	input, err := jsoninput.Encode(map[string]string{"runId": "run_original"})
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	var captured CallbackRef
	session := methodSession(t, func(backend *Backend) error {
		return RegisterCommand(backend, "owner.dispatch", nil, func(ctx context.Context, _ *sdk.ServerSession, command Command[map[string]string]) (string, error) {
			calls++
			ref, err := CallbackFromContext(ctx)
			if err != nil {
				return "", err
			}
			captured = ref
			original, err := command.RequestInput()
			if err != nil || original != input || command.ID != "original" {
				t.Fatalf("callback metadata changed admitted command: %v, %v", original, err)
			}
			return command.Input["runId"], nil
		})
	}, "owner.dispatch")
	binding, err := Resolve(t.Context(), session, "owner.dispatch")
	if err != nil {
		t.Fatal(err)
	}
	ref := CallbackRef{Endpoint: "http://127.0.0.1:1/", Credential: strings.Repeat("a", 64), Call: strings.Repeat("b", 64), MaxResponseBytes: 4096}
	for _, call := range []string{strings.Repeat("b", 64), strings.Repeat("c", 64)} {
		ref.Call = call
		metadata, err := CallbackMetadata(ref)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := binding.InvokeWithMetadata(t.Context(), "original", input, metadata); err != nil || captured != ref {
			t.Fatalf("captured callback=%+v error=%v", captured, err)
		}
	}
	if calls != 2 {
		t.Fatalf("business calls=%d", calls)
	}
	invalid := sdk.ParamsBase{Meta: sdk.Meta{callbackMetadataKey: map[string]string{"endpoint": "https://example.test/"}}}
	if _, err := binding.InvokeWithMetadata(t.Context(), "original", input, invalid); err == nil || calls != 2 {
		t.Fatalf("invalid metadata entered business execution: calls=%d error=%v", calls, err)
	}
	if _, err := CallbackFromContext(t.Context()); err == nil {
		t.Fatal("callback reference escaped its admitted context")
	}
}
