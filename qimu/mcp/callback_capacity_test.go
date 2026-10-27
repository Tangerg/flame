package mcp

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"weak"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestCallbacksLimitsRejectInvalidCapacity(t *testing.T) {
	for _, limits := range []CallbacksLimits{{}, {-1, 1, 1}, {1, 0, 1}, {1, 1, -1}} {
		if callbacks, err := NewCallbacksWithLimits(limits); callbacks != nil || err == nil {
			t.Fatalf("invalid capacities admitted: %+v, %v", limits, err)
		}
	}
}

func TestCallbackCapacityRemainsOwnedUntilOriginalWorkJoins(t *testing.T) {
	host, err := NewCallbacksWithLimits(CallbacksLimits{MaxSessions: 1, MaxCalls: 1, MaxPendingRequests: 1})
	if err != nil {
		t.Fatal(err)
	}
	finish := make(chan struct{})
	var finishOnce sync.Once
	defer func() {
		finishOnce.Do(func() { close(finish) })
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
	first, second := new(sdk.ClientSession), new(sdk.ClientSession)
	if err := host.AdmitSession(first); err != nil {
		t.Fatal(err)
	}
	if err := host.AdmitSession(first); err != nil {
		t.Fatalf("original session consumed another admission: %v", err)
	}
	if err := host.AdmitSession(second); !errors.Is(err, ErrCallbackCapacity) {
		t.Fatalf("full session budget: %v", err)
	}
	method := callbackTestMethod(t, "capacity")
	entered, canceled := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	callback, err := NewCallback(method, nil, func(ctx context.Context, command Command[callbackInput]) (int64, error) {
		calls.Add(1)
		if command.Input.Value == 1 {
			close(entered)
			<-ctx.Done()
			close(canceled)
			<-finish
		}
		return command.Input.Value, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(t.Context(), first, callbackTestLimits, callback)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := host.Open(t.Context(), first, callbackTestLimits, callback); !errors.Is(err, ErrCallbackCapacity) {
		t.Fatalf("full call budget: %v", err)
	}
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		_, _ = CallHost(t.Context(), ref, method, "", callbackInput{Value: 1})
	}()
	<-entered
	if _, err := CallHost(t.Context(), ref, method, "", callbackInput{Value: 2}); !errors.Is(err, ErrCallbackCapacity) {
		t.Fatalf("full pending request budget: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatal("refused request entered the callback handler")
	}
	released := make(chan struct{})
	go func() {
		release()
		close(released)
	}()
	<-canceled
	select {
	case <-released:
		t.Fatal("release returned before the original handler joined")
	default:
	}
	if _, _, err := host.Open(t.Context(), first, callbackTestLimits, callback); !errors.Is(err, ErrCallbackCapacity) {
		t.Fatalf("unconfirmed call was evicted from its capacity: %v", err)
	}
	finishOnce.Do(func() { close(finish) })
	<-released
	<-requestDone
	next, closeNext, err := host.Open(t.Context(), first, callbackTestLimits, callback)
	if err != nil {
		t.Fatalf("joined call did not release capacity: %v", err)
	}
	if value, err := CallHost(t.Context(), next, method, "", callbackInput{Value: 2}); err != nil || value != 2 {
		t.Fatalf("released request capacity was unavailable: %d, %v", value, err)
	}
	closeNext()
	host.RevokeSession(first)
	if err := host.AdmitSession(second); err != nil {
		t.Fatalf("retired session did not release capacity: %v", err)
	}
}

type callbackRetainedPayload struct{ bytes [1 << 20]byte }

func retiredCallback(t *testing.T) (func(), weak.Pointer[Callbacks], weak.Pointer[callbackRetainedPayload]) {
	t.Helper()
	host := NewCallbacks()
	session := new(sdk.ClientSession)
	if err := host.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	value := new(callbackRetainedPayload)
	value.bytes[0] = 7
	method := callbackTestMethod(t, "retention")
	callback, err := NewCallback(method, nil, func(context.Context, Command[callbackInput]) (int64, error) {
		return int64(value.bytes[0]), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(context.Background(), session, callbackTestLimits, callback)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := CallHost(t.Context(), ref, method, "", callbackInput{}); err != nil || result != 7 {
		t.Fatalf("original callback: %d, %v", result, err)
	}
	host.RevokeSession(session)
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	return release, weak.Make(host), weak.Make(value)
}

func TestTerminalCallbackReleaseDoesNotRetainHostOrNativeObject(t *testing.T) {
	release, host, value := retiredCallback(t)
	collected := false
	for range 100 {
		runtime.GC()
		if host.Value() == nil && value.Value() == nil {
			collected = true
			break
		}
		runtime.Gosched()
	}
	if !collected {
		t.Fatal("terminal callback release retained its former owner or native object")
	}
	release()
	runtime.KeepAlive(release)
}
