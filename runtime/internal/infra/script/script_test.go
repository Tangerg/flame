package script

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"maps"
	"slices"
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

type resultHost map[string]jsontext.Value

func (h resultHost) Tools() []string { return slices.Sorted(maps.Keys(h)) }

func (resultHost) Describe(string) (jsontext.Value, bool) { return nil, false }

func (h resultHost) Call(_ context.Context, _ int, call Call) (Settlement, error) {
	return Settlement{Status: builtin.ScriptCallSucceeded, Value: h[call.Tool]}, nil
}

func limits() Limits {
	return Limits{Deadline: 2 * time.Second, MemoryBytes: 16 << 20, Calls: 4, ConcurrentCalls: 1, OutputBytes: 1 << 10, ResultBytes: 1 << 10}
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
	page := resultHost{"page": jsontext.Value(`"` + strings.Repeat("x", 400) + `"`)}
	for name, test := range map[string]struct {
		source string
		host   Host
		limits Limits
		status builtin.ScriptStatus
	}{
		"deadline": {source: `while (true) {}`, host: blockingHost{}, limits: short, status: builtin.ScriptTimedOut},
		"output":   {source: `while (true) console.log("x".repeat(100));`, host: blockingHost{}, limits: limits(), status: builtin.ScriptOutputLimitExceeded},
		"results":  {source: `while (true) await tools.page();`, host: page, limits: limits(), status: builtin.ScriptResultLimitExceeded},
		"memory":   {source: `const kept = []; while (true) kept.push("x".repeat(1 << 16) + kept.length);`, host: blockingHost{}, limits: limits(), status: builtin.ScriptFailed},
		"stall":    {source: `await new Promise(() => {});`, host: blockingHost{}, limits: limits(), status: builtin.ScriptFailed},
	} {
		t.Run(name, func(t *testing.T) {
			started := time.Now()
			outcome, err := Run(t.Context(), test.source, test.host, test.limits)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Status != test.status || time.Since(started) > 10*time.Second {
				t.Fatalf("outcome = %+v after %s", outcome, time.Since(started))
			}
		})
	}
}

// A number the script cannot hold exactly arrives as its decimal text, and the
// record keeps the exact result the script received.
func TestScriptNeverRoundsWhatItReceives(t *testing.T) {
	issue := jsontext.Value(`{"id":12345678901234567890,"number":42,"ratio":0.1,"tiny":1e-400}`)
	outcome, err := Run(t.Context(), `
const issue = await tools.issue();
return [typeof issue.id, issue.id, issue.number + 1, issue.ratio, issue.tiny];`, resultHost{"issue": issue}, limits())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != builtin.ScriptCompleted || outcome.Output != `["string","12345678901234567890",43,0.1,"1e-400"]` ||
		len(outcome.Calls) != 1 || !bytes.Equal(outcome.Calls[0].Result, issue) {
		t.Fatalf("outcome = %+v", outcome)
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
