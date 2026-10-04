package bootstrap

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/infra/process/teardown"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestInstanceShutdownOwnsReverseOrderAndIsIdempotentAcrossCopies(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []string
	)
	record := func(name string, err error) func() error {
		return func() error {
			mu.Lock()
			calls = append(calls, name)
			mu.Unlock()
			return err
		}
	}
	recordStop := func(name string) func() {
		return func() { _ = record("stop "+name, nil)() }
	}
	recordWait := func(name string) func(context.Context) error {
		return func(context.Context) error { return record("wait "+name, nil)() }
	}
	host := Instance{
		lifetime: &runtimeLifetime{
			context: t.Context(), delivery: testEndpoint(t),
			shutdownWait:   defaultShutdownWaitPolicy(),
			goalDriver:     shutdownFunc{stop: recordStop("goals"), wait: recordWait("goals")},
			mcpCoordinator: shutdownFunc{stop: recordStop("mcp"), wait: recordWait("mcp")},
			runCoordinator: shutdownFunc{stop: recordStop("active-runs"), wait: recordWait("active-runs")},
			executor:       shutdownFunc{stop: recordStop("active-execution-tree"), wait: recordWait("active-execution-tree")},
			runEffectTasks: shutdownFunc{
				drain: func(context.Context) error { return record("drain effects", nil)() },
				stop:  recordStop("effects"),
				wait:  recordWait("effects"),
			},
			toolResources: terminalClosers([]func() error{
				closerFunc(record("tool-1", nil)),
				closerFunc(record("tool-2", nil)),
			}),
			hostResources: terminalClosers([]func() error{
				closerFunc(record("resource-1", nil)),
				closerFunc(record("resource-2", nil)),
			}),
		},
	}
	copyOfInstance := host

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for index := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if index%2 == 0 {
				errs <- host.Close()
				return
			}
			errs <- copyOfInstance.Close()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Close error = %v, want nil", err)
		}
	}
	wantCalls := []string{
		"stop goals",
		"stop mcp",
		"stop active-runs",
		"wait goals",
		"wait mcp",
		"wait active-runs",
		"drain effects",
		"stop effects",
		"wait effects",
		"stop active-execution-tree",
		"wait active-execution-tree",
		"tool-2",
		"tool-1",
		"resource-2",
		"resource-1",
	}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("close calls = %v, want %v", calls, wantCalls)
	}
}

func TestInstanceShutdownAdvancesPastCompletedCloserError(t *testing.T) {
	closeErr := errors.New("terminal close diagnostic")
	var toolCalls, resourceCalls int
	oneShotToolClose := sync.OnceValue(func() error {
		toolCalls++
		return closeErr
	})
	host := Instance{lifetime: &runtimeLifetime{
		context: t.Context(), delivery: testEndpoint(t),
		shutdownWait: defaultShutdownWaitPolicy(),
		// A2A, LSP, Shells and SQLite all use this one-shot close shape: the
		// resource reaches its terminal state on the first call even when that
		// call reports a diagnostic. Replaying the same cached error can never
		// make more cleanup progress.
		toolResources: terminalClosers([]func() error{oneShotToolClose}),
		hostResources: terminalClosers([]func() error{func() error {
			resourceCalls++
			return nil
		}}),
	}}

	if err := host.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("first Close error = %v, want terminal diagnostic", err)
	}
	if toolCalls != 1 {
		t.Fatalf("one-shot tool close calls = %d, want 1", toolCalls)
	}
	if resourceCalls != 1 {
		t.Fatalf("dependent resource close calls = %d, want 1 after tool reached its terminal state", resourceCalls)
	}
	if err := host.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("second Close = %v, want retained terminal diagnostic", err)
	}
	if toolCalls != 1 || resourceCalls != 1 {
		t.Fatalf("second Close replayed terminal work: tool=%d resource=%d", toolCalls, resourceCalls)
	}
}

func TestInstanceShutdownContinuesGraphAfterCallerTimeout(t *testing.T) {
	releaseComponent := make(chan struct{})
	toolClosed := make(chan struct{})
	host := Instance{lifetime: &runtimeLifetime{
		context: t.Context(), delivery: testEndpoint(t),
		shutdownWait: testShutdownWait(t, time.Millisecond),
		runCoordinator: shutdownFunc{
			wait: func(ctx context.Context) error {
				select {
				case <-releaseComponent:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		},
		toolResources: terminalClosers([]func() error{func() error {
			close(toolClosed)
			return nil
		}}),
	}}
	if err := host.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close error = %v, want deadline exceeded", err)
	}
	select {
	case <-toolClosed:
		t.Fatal("tool dependency closed despite an unjoined component")
	default:
	}

	// A failed Open does not return the Instance, so no external caller exists to
	// issue another Close. The Instance generation itself must retain this graph and
	// advance once the component finishes after the caller's wait deadline.
	close(releaseComponent)
	select {
	case <-toolClosed:
	case <-time.After(time.Second):
		t.Fatal("Instance abandoned its dependent resource graph after caller timeout")
	}
}

func TestInstanceShutdownStartsNewGenerationAfterComponentError(t *testing.T) {
	want := errors.New("component did not settle")
	var stops, attempts, closed int
	host := Instance{lifetime: &runtimeLifetime{
		context: t.Context(), delivery: testEndpoint(t),
		shutdownWait: defaultShutdownWaitPolicy(),
		runCoordinator: shutdownFunc{
			stop: func() { stops++ },
			wait: func(context.Context) error {
				attempts++
				if attempts == 1 {
					return want
				}
				return nil
			},
		},
		toolResources: terminalClosers([]func() error{func() error { closed++; return nil }}),
	}}
	if err := host.Close(); !errors.Is(err, want) {
		t.Fatalf("first Close error = %v, want component failure", err)
	}
	if stops != 1 || attempts != 1 || closed != 0 {
		t.Fatalf("after failed generation: stops=%d attempts=%d closed=%d, want 1/1/0", stops, attempts, closed)
	}
	if err := host.Close(); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	if stops != 1 || attempts != 2 || closed != 1 {
		t.Fatalf("after retry generation: stops=%d attempts=%d closed=%d, want 1/2/1", stops, attempts, closed)
	}
}

func TestInstanceShutdownBoundsNonCooperativeToolCloserWithoutConcurrentRetry(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	host := Instance{lifetime: &runtimeLifetime{
		context: t.Context(), delivery: testEndpoint(t),
		shutdownWait: testShutdownWait(t, time.Millisecond),
		toolResources: []*teardown.Step{teardown.Terminal(func(context.Context) error {
			calls.Add(1)
			close(started)
			<-release // Models a third-party Close with no cancellation support.
			return nil
		})},
	}}
	if err := host.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Close error = %v, want deadline exceeded", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("non-cooperative closer did not start")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("closer calls after deadline = %d, want 1", got)
	}

	close(release)
	host.lifetime.shutdownWait = testShutdownWait(t, time.Second)
	if err := host.Close(); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("retry launched a second closer = %d, want 1", got)
	}
}

type componentInvocation struct {
	tasks    taskgroup.Group
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (c *componentInvocation) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{Capabilities: protocol.ServerCapabilities{Features: map[string]protocol.FeatureCapability{
		protocol.FeaturePlugins: {Enabled: true},
	}}}, nil
}

func (c *componentInvocation) RevokePlugin(ctx context.Context, _ protocol.PluginRequest) (*protocol.PluginInstallation, error) {
	ownerCtx, release, ok := c.tasks.Attach(ctx)
	if !ok {
		return nil, context.Canceled
	}
	defer release()
	close(c.started)
	<-ownerCtx.Done()
	close(c.canceled)
	<-c.release
	return nil, ownerCtx.Err()
}

func (c *componentInvocation) BeginShutdown()                          { c.tasks.Cancel() }
func (c *componentInvocation) AwaitShutdown(ctx context.Context) error { return c.tasks.Wait(ctx) }

func TestInstanceShutdownCancelsComponentsBeforeJoiningDelivery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lifetime, stopRuntime := context.WithCancel(t.Context())
		defer stopRuntime()
		component := &componentInvocation{
			started: make(chan struct{}), canceled: make(chan struct{}),
			release: make(chan struct{}),
		}
		endpoint, err := delivery.NewEndpoint(component, delivery.EndpointConfig{Lifetime: lifetime})
		if err != nil {
			t.Fatal(err)
		}
		resourceClosed := false
		host := Instance{lifetime: &runtimeLifetime{
			context: lifetime, stopRuntime: stopRuntime, delivery: endpoint,
			shutdownWait: defaultShutdownWaitPolicy(), mcpCoordinator: component,
			hostResources: terminalClosers([]func() error{func() error {
				resourceClosed = true
				return nil
			}}),
		}}
		invoked := make(chan delivery.Result, 1)
		go func() {
			invoked <- endpoint.Invoke(t.Context(), delivery.PluginsRevoke, protocol.PluginRequest{InstallationID: "11111111-1111-1111-1111-111111111111"}, delivery.Options{})
		}()
		synctest.Wait()
		select {
		case <-component.started:
		default:
			t.Fatal("component invocation was not admitted")
		}
		closed := make(chan error, 1)
		go func() { closed <- host.Close() }()
		synctest.Wait()
		select {
		case <-component.canceled:
		default:
			component.BeginShutdown()
			close(component.release)
			synctest.Wait()
			t.Fatal("shutdown waited on delivery before canceling its component work")
		}
		if resourceClosed {
			close(component.release)
			synctest.Wait()
			t.Fatal("shutdown closed dependencies before the accepted invocation returned")
		}
		close(component.release)
		synctest.Wait()
		if err := <-closed; err != nil {
			t.Fatalf("Close: %v", err)
		}
		if result := <-invoked; !errors.Is(result.Failure, context.Canceled) {
			t.Fatalf("retired invocation = %+v, want cancellation", result)
		}
		if !resourceClosed {
			t.Fatal("shutdown did not advance after the invocation returned")
		}
	})
}

type closerFunc func() error

func testShutdownWait(t *testing.T, timeout time.Duration) shutdownWaitPolicy {
	t.Helper()
	policy, err := newShutdownWaitPolicy(timeout)
	if err != nil {
		t.Fatalf("newShutdownWaitPolicy(%v): %v", timeout, err)
	}
	return policy
}

func (c closerFunc) Close() error { return c() }

type shutdownFunc struct {
	stop  func()
	wait  func(context.Context) error
	drain func(context.Context) error
}

func (s shutdownFunc) BeginShutdown() {
	if s.stop != nil {
		s.stop()
	}
}

func (s shutdownFunc) AwaitShutdown(ctx context.Context) error {
	if s.wait == nil {
		return nil
	}
	return s.wait(ctx)
}

func (s shutdownFunc) Drain(ctx context.Context) error {
	if s.drain == nil {
		return nil
	}
	return s.drain(ctx)
}

func (s shutdownFunc) Cancel() { s.BeginShutdown() }

func (s shutdownFunc) Wait(ctx context.Context) error { return s.AwaitShutdown(ctx) }

// testEndpoint builds the delivery entrypoint every assembled lifetime owns, so
// a shutdown test drives the real graph instead of a half-built one.
func testEndpoint(t *testing.T) *delivery.Endpoint {
	t.Helper()
	endpoint, err := delivery.NewEndpoint(struct{}{}, delivery.EndpointConfig{Lifetime: t.Context()})
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}
