package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/frame"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

type callbackInput struct {
	Value int64 `json:"value"`
}
type callbackParentInput struct {
	Callback CallbackRef `json:"callback"`
	Value    int64       `json:"value"`
}
type callbackOwnerKey struct{}

func callbackTestMethod(t *testing.T, target string) *Method[callbackInput, int64] {
	t.Helper()
	method, err := NewMethod[callbackInput, int64](frame.MethodConfig{Name: "test.host/compute", Kind: frame.Query, Target: target, Description: "Compute for the captured owner."})
	if err != nil {
		t.Fatal(err)
	}
	return method
}

func newTestCallbacks(t *testing.T) *Callbacks {
	t.Helper()
	callbacks := NewCallbacks()
	t.Cleanup(func() {
		if err := callbacks.Close(); err != nil {
			t.Error(err)
		}
	})
	return callbacks
}

var callbackTestLimits = CallbackLimits{MaxRequestBytes: 4096, MaxResponseBytes: 4096}

func TestCallbackCancellationPreservesOwnerCauseAndCredentialPrivacy(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	if err := host.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	method := callbackTestMethod(t, "call")
	cause := errors.New("owning operation ended")
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	entered := make(chan struct{})
	observed := make(chan error, 1)
	callback, err := NewCallback(method, nil, func(ctx context.Context, _ Command[callbackInput]) (int64, error) {
		close(entered)
		<-ctx.Done()
		observed <- context.Cause(ctx)
		return 0, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(ctx, session, callbackTestLimits, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	result := make(chan error, 1)
	go func() { _, err := CallHost(ctx, ref, method, "", callbackInput{1}); result <- err }()
	<-entered
	cancel(cause)
	if err := <-observed; !errors.Is(err, cause) {
		t.Fatalf("owner cause changed: %v", err)
	}
	err = <-result
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("caller cause changed: %v", err)
	}
	if strings.Contains(err.Error(), ref.Credential) || strings.Contains(err.Error(), ref.Call) {
		t.Fatal("transport diagnostic exposed a callback credential")
	}
}

func TestCallbackBackendProcess(t *testing.T) {
	if os.Getenv("FLAME_CALLBACK_FIXTURE") != "1" {
		return
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "public-callback-author"}, nil)
	backend, err := NewBackend(server)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	method := callbackTestMethod(t, "captured")
	err = errors.Join(testMethod[callbackInput, int64](t, "test.backend/inner", frame.Query).RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command Command[callbackInput]) (int64, error) {
		input := command.Input
		return input.Value + 10, nil
	}), testMethod[callbackParentInput, int64](t, "test.backend/outer", frame.Query).RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command Command[callbackParentInput]) (int64, error) {
		input := command.Input
		return CallHost(ctx, input.Callback, method, "", callbackInput{input.Value})
	}),
	)
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

func TestPublicCallbacksAreReentrantThroughARealHeadlessBackend(t *testing.T) {
	host := newTestCallbacks(t)
	client := sdk.NewClient(&sdk.Implementation{Name: "headless-runtime"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	command := exec.Command(os.Args[0], "-test.run=^TestCallbackBackendProcess$")
	command.Env = append(os.Environ(), "FLAME_CALLBACK_FIXTURE=1")
	session, err := client.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		host.RevokeSession(session)
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := host.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	method := callbackTestMethod(t, "captured")
	callback, err := NewCallback(method, nil, func(ctx context.Context, command Command[callbackInput]) (int64, error) {
		if ctx.Value(callbackOwnerKey{}) != "captured parent" {
			return 0, errors.New("nested origin/parent changed")
		}
		return testMethod[callbackInput, int64](t, "test.backend/inner", frame.Query).Call(ctx, session, command.Input)
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := context.WithValue(ctx, callbackOwnerKey{}, "captured parent")
	ref, release, err := host.Open(owner, session, callbackTestLimits, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	value, err := testMethod[callbackParentInput, int64](t, "test.backend/outer", frame.Query).Call(ctx, session, callbackParentInput{ref, 5})
	if err != nil || value != 15 {
		t.Fatalf("nested headless invocation value=%d error=%v", value, err)
	}
}
