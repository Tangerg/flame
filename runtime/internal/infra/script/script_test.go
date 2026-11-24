package script

import (
	"context"
	"encoding/json/jsontext"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
)

type blockingHost struct {
	started chan struct{}
	ended   chan error
}

func (blockingHost) Tools() []string { return []string{"wait"} }

func (blockingHost) Describe(string) (jsontext.Value, bool) { return nil, false }

func (h blockingHost) Call(ctx context.Context, _ int, _ Call) (Settlement, error) {
	h.started <- struct{}{}
	<-ctx.Done()
	h.ended <- context.Cause(ctx)
	return Settlement{Status: builtin.ScriptCallFailed, Message: "canceled"}, nil
}

func limits() Limits {
	return Limits{Deadline: 2 * time.Second, MemoryBytes: 16 << 20, Calls: 4, ConcurrentCalls: 1, OutputBytes: 1 << 10}
}

func TestScriptReachesNothingButItsTools(t *testing.T) {
	host := blockingHost{started: make(chan struct{}, 1), ended: make(chan error, 1)}
	outcome, err := Run(t.Context(), `
const reachable = ["require", "process", "fetch", "setTimeout", "setInterval", "std", "os", "import", "XMLHttpRequest", "WebSocket",
  "__flame_call", "__flame_describe", "__flame_print", "__flame_tools"].filter((name) => typeof globalThis[name] !== "undefined");
let dynamicImport = "refused";
try { await import("os"); dynamicImport = "loaded"; } catch {}
return { reachable, dynamicImport, tools: Object.keys(tools) };`, host, limits())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != builtin.ScriptCompleted || outcome.Output != `{"reachable":[],"dynamicImport":"refused","tools":["wait"]}` {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestScriptEndsAtEachLimit(t *testing.T) {
	short := limits()
	short.Deadline = 200 * time.Millisecond
	for name, test := range map[string]struct {
		source string
		limits Limits
		status builtin.ScriptStatus
	}{
		"deadline": {source: `while (true) {}`, limits: short, status: builtin.ScriptTimedOut},
		"output":   {source: `while (true) console.log("x".repeat(100));`, limits: limits(), status: builtin.ScriptOutputLimitExceeded},
		"memory":   {source: `const kept = []; while (true) kept.push("x".repeat(1 << 16) + kept.length);`, limits: limits(), status: builtin.ScriptFailed},
		"stall":    {source: `await new Promise(() => {});`, limits: limits(), status: builtin.ScriptFailed},
	} {
		t.Run(name, func(t *testing.T) {
			started := time.Now()
			outcome, err := Run(t.Context(), test.source, blockingHost{}, test.limits)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Status != test.status || time.Since(started) > 10*time.Second {
				t.Fatalf("outcome = %+v after %s", outcome, time.Since(started))
			}
		})
	}
}

func TestScriptCancelsCallsItCannotFinish(t *testing.T) {
	host := blockingHost{started: make(chan struct{}, 1), ended: make(chan error, 1)}
	bounded := limits()
	bounded.Calls = 2
	outcome, err := Run(t.Context(), `
const first = tools.wait();
const second = tools.wait();
await tools.wait();`, host, bounded)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-host.started:
	default:
		t.Fatal("the first call never started")
	}
	if cause := <-host.ended; cause == nil {
		t.Fatal("the running call was not canceled")
	}
	statuses := make([]builtin.ScriptCallStatus, len(outcome.Calls))
	for index, call := range outcome.Calls {
		statuses[index] = call.Status
	}
	if outcome.Status != builtin.ScriptCallLimitExceeded || strings.Join(callStatusText(statuses), ",") != "unconfirmed,notDispatched,notDispatched" {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func callStatusText(statuses []builtin.ScriptCallStatus) []string {
	text := make([]string, len(statuses))
	for index, status := range statuses {
		text[index] = string(status)
	}
	return text
}
