package toolset

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/script"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

var scriptLimits = script.Limits{
	Deadline:        3 * time.Minute,
	MemoryBytes:     64 << 20,
	Calls:           256,
	ConcurrentCalls: 8,
	OutputBytes:     256 << 10,
}

// Results the script received are retained for clients up to this many bytes
// per script; the record states the size of each result it omits.
const scriptRecordBytes = 256 << 10

const scriptDescription = `Run a JavaScript program that calls this Run's read-only tools, then return only what you need. Use it to page through, filter, join or count tool results without reading every intermediate result yourself; call a tool directly for a single lookup.

The script is the body of an async function: use await and return. ` + "`tools`" + ` maps each callable tool's exact name to an async function taking its arguments object, for example ` + "`await tools.read({ path: \"go.mod\" })`" + ` or ` + "`await tools[\"name-with-dash\"]({})`" + `. A call resolves to the tool's structured result, or to its text when it has none, and rejects with an Error whose status is "failed" or "rejected". ` + "`Object.keys(tools)`" + ` lists the callable tools and ` + "`describeTool(name)`" + ` returns one tool's description, input schema and source. ` + "`console.log`" + ` lines and the returned value form the output.

Only tools that cannot change anything are callable. Each call passes the same hooks and approval policy as a direct call; a call that would need the user's approval is rejected, so make that call directly. There is no filesystem, network, environment, timer or module access. A script ends after 3 minutes, 256 tool calls, 64 MiB of memory or 256 KiB of output, and at most 8 calls run at once.`

type scriptInput struct {
	Summary string `json:"summary" jsonschema:"minLength=1,maxLength=80,pattern=^[^[:space:]](.*[^[:space:]])?$" jsonschema_description:"Concise 3-5 word label for what the script reads, at most 80 characters, without leading or trailing whitespace."`
	Script  string `json:"script" jsonschema:"minLength=1,maxLength=65536" jsonschema_description:"JavaScript run as the body of an async function. Return the result the conversation needs."`
}

// ScriptTools is the Run's read-only Tool catalog for one run_script call. Each
// Call passes the Run's admission for that Tool before executing it.
type ScriptTools interface {
	Names() []string
	Describe(name string) (jsontext.Value, bool)
	Call(ctx context.Context, index int, call script.Call) (ScriptToolCall, error)
}

// ScriptToolCall is what Runtime knows about one script call it settled. Source
// is zero when the script named no callable Tool, and Arguments is nil until the
// call's arguments were admitted.
type ScriptToolCall struct {
	Source     tool.Ref
	Arguments  *tool.Arguments
	Settlement script.Settlement
}

type scriptToolsKey struct{}

func WithScriptTools(ctx context.Context, tools ScriptTools) context.Context {
	return context.WithValue(ctx, scriptToolsKey{}, tools)
}

type readAuthorityWithheldKey struct{}

// The model sees only what a script returns, so a read inside a script must not
// satisfy the read-before-write guard.
func readAuthorityWithheld(ctx context.Context) bool {
	withheld, _ := ctx.Value(readAuthorityWithheldKey{}).(bool)
	return withheld
}

func newScriptTool() (toolcontract.Tool, error) {
	return toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: string(builtincontract.RunScript), Description: scriptDescription, Effect: toolcontract.EffectSafe,
	}, runScript)
}

func runScript(ctx context.Context, input scriptInput) (toolcontract.Output, error) {
	tools, ok := ctx.Value(scriptToolsKey{}).(ScriptTools)
	if !ok {
		return toolcontract.Output{}, errors.New("toolset: run_script has no Run Tool catalog")
	}
	host := &scriptHost{tools: tools, calls: make(map[int]ScriptToolCall)}
	outcome, err := script.Run(context.WithValue(ctx, readAuthorityWithheldKey{}, true), input.Script, host, scriptLimits)
	if err != nil {
		return toolcontract.Output{}, fmt.Errorf("toolset: run script: %w", err)
	}
	result := host.result(outcome)
	details, err := json.Marshal(result)
	if err != nil {
		return toolcontract.Output{}, fmt.Errorf("toolset: encode script result: %w", err)
	}
	output := toolcontract.Output{Content: toolcontract.NewTextOutput(scriptReport(result)).Content, Details: details}
	switch outcome.Status {
	case builtincontract.ScriptCompleted:
		return output, nil
	case builtincontract.ScriptCanceled:
		cause := errors.Join(ctx.Err(), context.Cause(ctx))
		if cause == nil {
			cause = errors.New("toolset: script canceled")
		}
		callErr, err := toolcontract.NewCallError(toolcontract.CallErrorConfig{Cause: cause, Evidence: output})
		if err != nil {
			return toolcontract.Output{}, errors.Join(cause, err)
		}
		return toolcontract.Output{}, callErr
	default:
		failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{
			Kind: toolcontract.FailureKindFailed, Output: output, Cause: fmt.Errorf("toolset: script %s", outcome.Status),
		})
		if err != nil {
			return toolcontract.Output{}, err
		}
		return toolcontract.Output{}, failure
	}
}

type scriptHost struct {
	tools ScriptTools
	mu    sync.Mutex
	calls map[int]ScriptToolCall
}

func (h *scriptHost) Tools() []string { return h.tools.Names() }

func (h *scriptHost) Describe(name string) (jsontext.Value, bool) { return h.tools.Describe(name) }

func (h *scriptHost) Call(ctx context.Context, index int, call script.Call) (script.Settlement, error) {
	settled, err := h.tools.Call(ctx, index, call)
	if err != nil {
		return script.Settlement{}, err
	}
	h.mu.Lock()
	h.calls[index] = settled
	h.mu.Unlock()
	return settled.Settlement, nil
}

func (h *scriptHost) result(outcome script.Outcome) builtincontract.ScriptResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := builtincontract.ScriptResult{
		Status: outcome.Status, Error: outcome.Error, Output: outcome.Output,
		Calls: make([]builtincontract.ScriptCall, len(outcome.Calls)),
	}
	retained := 0
	for index, record := range outcome.Calls {
		call := builtincontract.ScriptCall{Tool: record.Call.Tool, Status: record.Status, Error: record.Message}
		if requested, err := tool.ParseArguments(record.Call.Arguments); err == nil {
			call.Arguments = requested.Map()
		}
		settled, found := h.calls[index]
		if found && settled.Source != (tool.Ref{}) {
			call.Source = settled.Source.String()
		}
		if found && settled.Arguments != nil {
			call.Arguments = settled.Arguments.Map()
		}
		if found && record.Status == builtincontract.ScriptCallSucceeded {
			value := settled.Settlement.Value
			if retained+len(value) <= scriptRecordBytes {
				retained += len(value)
				call.Result = value
			} else {
				call.OmittedBytes = len(value)
			}
		}
		result.Calls[index] = call
	}
	return result
}

func scriptReport(result builtincontract.ScriptResult) string {
	var report strings.Builder
	switch result.Status {
	case builtincontract.ScriptCompleted:
	case builtincontract.ScriptFailed:
		fmt.Fprintf(&report, "The script failed: %s\n", result.Error)
	case builtincontract.ScriptTimedOut:
		fmt.Fprintf(&report, "The script exceeded its %s deadline and was stopped.\n", scriptLimits.Deadline)
	case builtincontract.ScriptCallLimitExceeded:
		fmt.Fprintf(&report, "The script requested more than %d tool calls and was stopped.\n", scriptLimits.Calls)
	case builtincontract.ScriptOutputLimitExceeded:
		fmt.Fprintf(&report, "The script produced more than %d bytes of output and was stopped.\n", scriptLimits.OutputBytes)
	case builtincontract.ScriptCanceled:
		report.WriteString("The script was canceled.\n")
	}
	switch {
	case result.Output != "" && report.Len() > 0:
		fmt.Fprintf(&report, "Output before it ended:\n%s\n", result.Output)
	case result.Output != "":
		report.WriteString(result.Output + "\n")
	case report.Len() == 0:
		report.WriteString("The script returned no output.\n")
	}
	if counts := scriptCallCounts(result.Calls); counts != "" {
		fmt.Fprintf(&report, "\nTool calls: %s.", counts)
	}
	return strings.TrimRight(report.String(), "\n")
}

func scriptCallCounts(calls []builtincontract.ScriptCall) string {
	counts := make(map[builtincontract.ScriptCallStatus]int)
	for _, call := range calls {
		counts[call.Status]++
	}
	var parts []string
	for _, status := range builtincontract.ScriptCallStatus("").Values() {
		if count := counts[builtincontract.ScriptCallStatus(status)]; count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, scriptCallStatusText(builtincontract.ScriptCallStatus(status))))
		}
	}
	return strings.Join(parts, ", ")
}

func scriptCallStatusText(status builtincontract.ScriptCallStatus) string {
	if status == builtincontract.ScriptCallNotDispatched {
		return "not dispatched"
	}
	return string(status)
}
