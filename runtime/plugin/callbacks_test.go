package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contractsdk "github.com/Tangerg/flame/runtime/capability"
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
	method, err := NewMethod[callbackInput, int64](MethodConfig{Name: "test.host/compute", Kind: contractsdk.Query, Target: target, Description: "Compute for the captured owner."})
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

func TestRPCPublicationPrecedesReferencesAndCannotBeReplaced(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	if _, _, err := host.OpenRPC(t.Context(), session, callbackTestLimits); err == nil {
		t.Fatal("unpublished transport issued a usable reference")
	}
	var absent http.HandlerFunc
	for _, handler := range []http.Handler{nil, absent} {
		if err := host.PublishRPC(handler); err == nil {
			t.Fatal("absent transport was published")
		}
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	if err := host.PublishRPC(handler); err != nil {
		t.Fatal(err)
	}
	if err := host.PublishRPC(handler); err == nil {
		t.Fatal("published transport was replaced")
	}
	_, release, err := host.OpenRPC(t.Context(), session, callbackTestLimits)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	if err := host.PublishRPC(handler); err == nil {
		t.Fatal("closed transport was republished")
	}
	if _, _, err := host.OpenRPC(t.Context(), session, callbackTestLimits); !errors.Is(err, ErrStaleCall) {
		t.Fatalf("closed transport issued a reference: %v", err)
	}
}

func TestCallbacksShareEndpointAndRetainCallAuthority(t *testing.T) {
	host := newTestCallbacks(t)
	method := callbackTestMethod(t, "captured")
	var calls atomic.Int32
	callback, err := NewCallback(method, nil, func(ctx context.Context, command Command[callbackInput]) (int64, error) {
		if ctx.Value(callbackOwnerKey{}) != "origin/target/parent" {
			return 0, errors.New("owner context was replaced")
		}
		input, err := command.RequestInput()
		if err != nil || input.String() != `{"value":9007199254740993}` {
			return 0, errors.New("original input was changed")
		}
		calls.Add(1)
		return command.Input.Value, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(t.Context(), callbackOwnerKey{}, "origin/target/parent")
	firstSession, secondSession := new(sdk.ClientSession), new(sdk.ClientSession)
	host.AdmitSession(firstSession)
	host.AdmitSession(secondSession)
	first, releaseFirst, err := host.Open(ctx, firstSession, callbackTestLimits, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()
	second, releaseSecond, err := host.Open(ctx, secondSession, callbackTestLimits, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	if first.Endpoint != second.Endpoint || first.Credential == second.Credential || first.Call == second.Call {
		t.Fatal("endpoint or activation/call ownership is wrong")
	}
	for range 5 {
		value, err := CallHost(t.Context(), first, method, "", callbackInput{9007199254740993})
		if err != nil || value != 9007199254740993 {
			t.Fatalf("callback value=%d error=%v", value, err)
		}
	}
	foreign := first
	foreign.Credential = second.Credential
	if _, err := CallHost(t.Context(), foreign, method, "", callbackInput{9007199254740993}); !errors.Is(err, ErrStaleCall) {
		t.Fatalf("foreign activation accepted a call: %v", err)
	}
	if _, err := CallHost(t.Context(), first, callbackTestMethod(t, "different"), "", callbackInput{9007199254740993}); err == nil {
		t.Fatal("different exact contract was accepted")
	}
	if calls.Load() != 5 {
		t.Fatalf("refused calls reached the owner: %d", calls.Load())
	}
	releaseFirst()
	if _, err := CallHost(t.Context(), first, method, "", callbackInput{9007199254740993}); !errors.Is(err, ErrStaleCall) {
		t.Fatalf("released call accepted: %v", err)
	}
	if _, err := CallHost(t.Context(), second, method, "", callbackInput{9007199254740993}); err != nil {
		t.Fatalf("releasing one call affected another: %v", err)
	}
}

func TestCallbackRefusalAndUncertaintyUseTheMethodContract(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	method := callbackTestMethod(t, "call")
	callback, err := NewCallback(method, nil, func(_ context.Context, command Command[callbackInput]) (int64, error) {
		if command.Input.Value == 1 {
			return 0, &Failure{Code: "conflict", Message: "original decision"}
		}
		return 0, errors.New("unconfirmed storage failure")
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(t.Context(), session, callbackTestLimits, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, err = CallHost(t.Context(), ref, method, "", callbackInput{1})
	failure, known := errors.AsType[*Failure](err)
	if !known || failure.Code != "conflict" || failure.Message != "original decision" {
		t.Fatalf("known refusal changed: %v", err)
	}
	_, err = CallHost(t.Context(), ref, method, "", callbackInput{2})
	if err == nil || errors.As(err, &failure) {
		t.Fatalf("unknown effect became a business refusal: %v", err)
	}
}

func TestCallbackCapacitiesDoNotInventAnOwnerResult(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	method := callbackTestMethod(t, "call")
	var calls atomic.Int32
	callback, err := NewCallback(method, nil, func(context.Context, Command[callbackInput]) (int64, error) { calls.Add(1); return 42, nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		limits    CallbackLimits
		wantCalls int32
	}{
		{"input", CallbackLimits{MaxRequestBytes: 1, MaxResponseBytes: 4096}, 0},
		{"output", CallbackLimits{MaxRequestBytes: 4096, MaxResponseBytes: 1}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			ref, release, err := host.Open(t.Context(), session, test.limits, callback)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			_, err = CallHost(t.Context(), ref, method, "", callbackInput{1})
			var failure *Failure
			if err == nil || errors.As(err, &failure) {
				t.Fatalf("capacity failure became a known business result: %v", err)
			}
			if calls.Load() != test.wantCalls {
				t.Fatalf("owner calls=%d want=%d", calls.Load(), test.wantCalls)
			}
		})
	}
}

func TestCallbackRetirementClosesAnAdmittedIncompleteRequest(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	method := callbackTestMethod(t, "call")
	var calls atomic.Int32
	callback, err := NewCallback(method, nil, func(context.Context, Command[callbackInput]) (int64, error) { calls.Add(1); return 42, nil })
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(t.Context(), session, callbackTestLimits, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	connection, err := net.Dial("tcp", strings.TrimSuffix(strings.TrimPrefix(ref.Endpoint, "http://"), "/"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_, err = fmt.Fprintf(connection, "POST / HTTP/1.1\r\nHost: localhost\r\nAuthorization: %s\r\nFlame-Call: %s\r\nContent-Length: 1024\r\nExpect: 100-continue\r\n\r\n", ref.Credential, ref.Call)
	if err != nil {
		t.Fatal(err)
	}
	// The server sends 100 Continue only after this authorized body read starts.
	if err := connection.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	continued := make([]byte, len("HTTP/1.1 100 Continue\r\n\r\n"))
	if _, err := io.ReadFull(connection, continued); err != nil {
		t.Fatal(err)
	}
	if string(continued) != "HTTP/1.1 100 Continue\r\n\r\n" {
		t.Fatalf("unexpected request admission: %q", continued)
	}
	done := make(chan struct{})
	go func() { host.RevokeSession(session); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("retirement waited for an incomplete request body")
	}
	if calls.Load() != 0 {
		t.Fatal("incomplete input entered user code")
	}
}

func TestCallbackRetirementCancelsAllBeforeJoining(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	method := callbackTestMethod(t, "call")
	entered := make(chan struct{}, 2)
	canceled := make(chan struct{}, 2)
	finish := make(chan struct{})
	var finishOnce sync.Once
	finishHandler := func() { finishOnce.Do(func() { close(finish) }) }
	defer finishHandler()
	callback, err := NewCallback(method, nil, func(ctx context.Context, _ Command[callbackInput]) (int64, error) {
		entered <- struct{}{}
		<-ctx.Done()
		canceled <- struct{}{}
		<-finish
		return 0, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	refs := make([]CallbackRef, 2)
	releases := make([]func(), 2)
	results := make(chan error, 2)
	for index := range refs {
		refs[index], releases[index], err = host.Open(t.Context(), session, callbackTestLimits, callback)
		if err != nil {
			t.Fatal(err)
		}
		go func(ref CallbackRef) {
			_, err := CallHost(t.Context(), ref, method, "", callbackInput{1})
			results <- err
		}(refs[index])
	}
	for range refs {
		<-entered
	}
	firstReleased := make(chan struct{})
	go func() { releases[0](); close(firstReleased) }()
	<-canceled
	retired := make(chan struct{})
	go func() { host.RevokeSession(session); close(retired) }()
	// Revocation must cancel the other call even while the first is settling.
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("retirement joined one call before canceling all")
	}
	select {
	case <-retired:
		t.Fatal("retirement lost the already-releasing call")
	default:
	}
	select {
	case <-firstReleased:
		t.Fatal("release returned before its handler settled")
	default:
	}
	if _, release, err := host.Open(t.Context(), session, callbackTestLimits, callback); !errors.Is(err, ErrStaleCall) {
		if release != nil {
			release()
		}
		t.Fatalf("retiring activation admitted new callbacks: %v", err)
	}
	finishHandler()
	<-retired
	<-firstReleased
	for range refs {
		if err := <-results; err == nil {
			t.Fatal("retired handler became a successful callback")
		}
	}
	for _, release := range releases {
		release()
	}
}

func TestCallbackCancellationPreservesOwnerCauseAndCredentialPrivacy(t *testing.T) {
	host := newTestCallbacks(t)
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
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
	err = errors.Join(
		RegisterServer(backend, "test.backend/inner", nil, func(_ context.Context, _ *sdk.ServerSession, input callbackInput) (int64, error) {
			return input.Value + 10, nil
		}),
		RegisterServer(backend, "test.backend/outer", nil, func(ctx context.Context, _ *sdk.ServerSession, input callbackParentInput) (int64, error) {
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
	host.AdmitSession(session)
	method := callbackTestMethod(t, "captured")
	callback, err := NewCallback(method, nil, func(ctx context.Context, command Command[callbackInput]) (int64, error) {
		if ctx.Value(callbackOwnerKey{}) != "captured parent" {
			return 0, errors.New("nested origin/parent changed")
		}
		return Call[callbackInput, int64](ctx, session, "test.backend/inner", command.Input)
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
	value, err := Call[callbackParentInput, int64](ctx, session, "test.backend/outer", callbackParentInput{ref, 5})
	if err != nil || value != 15 {
		t.Fatalf("nested headless invocation value=%d error=%v", value, err)
	}
}
