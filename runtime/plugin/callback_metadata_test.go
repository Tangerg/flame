package plugin

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
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
	var capturedInvocation capability.Invocation
	session := methodSession(t, func(backend *Backend) error {
		return RegisterCommand(backend, "owner.dispatch", nil, func(ctx context.Context, _ *sdk.ServerSession, command Command[map[string]string]) (string, error) {
			calls++
			ref, err := CallbackFromContext(ctx)
			if err != nil {
				return "", err
			}
			captured = ref
			capturedInvocation, err = InvocationFromContext(ctx)
			if err != nil {
				return "", err
			}
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
	invocation := capability.Invocation{ID: "original", Origin: capability.Model, Parent: "parent-tool", Target: capability.Target{Scope: binding.Descriptor().Target, Reference: "run_original"}}
	ctx, err := WithInvocation(t.Context(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	ref := CallbackRef{Endpoint: "http://127.0.0.1:1/", Credential: strings.Repeat("a", 64), Call: strings.Repeat("b", 64), MaxResponseBytes: 1<<53 + 1}
	for _, call := range []string{strings.Repeat("b", 64), strings.Repeat("c", 64)} {
		ref.Call = call
		metadata, err := CallbackMetadata(ref)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := binding.InvokeWithMetadata(ctx, "original", input, metadata); err != nil || captured != ref || capturedInvocation != invocation {
			t.Fatalf("captured callback=%+v error=%v", captured, err)
		}
	}
	if calls != 2 {
		t.Fatalf("business calls=%d", calls)
	}
	var invalid sdk.ParamsBase
	if err := invalid.Set(callbackMetadataKey, map[string]string{"endpoint": "https://example.test/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := binding.InvokeWithMetadata(ctx, "original", input, invalid); err == nil || calls != 2 {
		t.Fatalf("invalid metadata entered business execution: calls=%d error=%v", calls, err)
	}
	forged := invocation
	forged.Origin = capability.Human
	var supplied sdk.ParamsBase
	if err := supplied.Set(InvocationMetadataKey, forged); err != nil {
		t.Fatal(err)
	}
	if _, err := binding.InvokeWithMetadata(ctx, "original", input, supplied); err == nil || calls != 2 {
		t.Fatal("caller metadata replaced the model origin before dispatch")
	}
	if _, err := CallbackFromContext(t.Context()); err == nil {
		t.Fatal("callback reference escaped its admitted context")
	}
}

func TestCallbackMetadataRefusesMalformedAuthority(t *testing.T) {
	for _, raw := range []string{
		`{`,
		`null`,
		`{"endpoint":"https://example.test/","credential":%q,"call":%q,"maxResponseBytes":4096}`,
		`{"endpoint":"http://127.0.0.1:1/","credential":%q,"call":%q,"maxResponseBytes":4096,"permission":"replacement"}`,
		`{"endpoint":"http://127.0.0.1:1/","credential":%q,"call":%q,"maxResponseBytes":9223372036854775807}`,
		`{"endpoint":"http://127.0.0.1:1/","credential":%q,"call":%q,"maxResponseBytes":9223372036854775808}`,
	} {
		t.Run(raw, func(t *testing.T) {
			if strings.Contains(raw, "%q") {
				raw = fmt.Sprintf(raw, strings.Repeat("a", 64), strings.Repeat("b", 64))
			}
			if _, err := bindCallbackMetadata(t.Context(), sdk.Meta{callbackMetadataKey: []byte(raw)}); err == nil {
				t.Fatal("malformed callback authority was admitted")
			}
		})
	}
}
