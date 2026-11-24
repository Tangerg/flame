package script

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"

	"modernc.org/quickjs"
)

var errDeadline = errors.New("script: deadline exceeded")

// The prelude captures the host functions and removes their global names, so a
// script reaches the host only through the frozen objects defined here.
const prelude = `(() => {
  const call = __flame_call, describe = __flame_describe, print = __flame_print;
  const names = JSON.parse(__flame_tools());
  for (const name of ["__flame_call", "__flame_describe", "__flame_print", "__flame_tools"]) delete globalThis[name];
  const format = (value) => {
    if (typeof value === "string") return value;
    if (value === undefined) return "undefined";
    try {
      const text = JSON.stringify(value);
      return text === undefined ? String(value) : text;
    } catch {
      return String(value);
    }
  };
  const rejection = (raw) => {
    const failure = JSON.parse(raw);
    const error = new Error(failure.message);
    error.status = failure.status;
    return error;
  };
  const tools = Object.create(null);
  for (const name of names) {
    tools[name] = (args = {}) => call(name, JSON.stringify(args)).then(JSON.parse, (raw) => { throw rejection(raw); });
  }
  const log = (...values) => print(values.map(format).join(" "));
  Object.defineProperties(globalThis, {
    tools: { value: Object.freeze(tools) },
    describeTool: { value: (name) => { const raw = describe(String(name)); return raw === "" ? undefined : JSON.parse(raw); } },
    console: { value: Object.freeze({ log, info: log, warn: log, error: log, debug: log }) },
  });
  return format;
})()`

type completion struct {
	index      int
	settlement Settlement
	err        error
}

// A machine is confined to the goroutine that runs it; only host calls run
// elsewhere and report back through completions.
type machine struct {
	ctx    context.Context
	host   Host
	limits Limits
	tools  string
	vm     *quickjs.VM
	format quickjs.Value

	records     []CallRecord
	promises    map[int]*quickjs.PromiseCapability
	queue       []int
	running     int
	completions chan completion
	calls       sync.WaitGroup
	unwatch     func() bool

	received int
	output   strings.Builder
	result   string
	finished bool
	status   builtin.ScriptStatus
	failure  string
	hostErr  error
}

func start(ctx context.Context, tools []string, host Host, limits Limits) (_ *machine, err error) {
	names, err := json.Marshal(tools)
	if err != nil {
		return nil, fmt.Errorf("script: encode tool names: %w", err)
	}
	vm, err := quickjs.NewVM()
	if err != nil {
		return nil, fmt.Errorf("script: start engine: %w", err)
	}
	vm.SetMemoryLimit(limits.MemoryBytes)
	m := &machine{
		ctx: ctx, host: host, limits: limits, tools: string(names), vm: vm,
		promises:    make(map[int]*quickjs.PromiseCapability),
		completions: make(chan completion, limits.Calls),
	}
	m.unwatch = context.AfterFunc(ctx, vm.Interrupt)
	defer func() {
		if err != nil {
			m.close()
		}
	}()
	for name, function := range map[string]any{
		"__flame_call": m.call, "__flame_describe": m.describe,
		"__flame_print": m.print, "__flame_tools": m.toolNames,
	} {
		if err := vm.RegisterFunc(name, function, false); err != nil {
			return nil, fmt.Errorf("script: register %s: %w", name, err)
		}
	}
	m.format, err = vm.EvalValue(prelude, quickjs.EvalGlobal)
	if err != nil {
		return nil, fmt.Errorf("script: install prelude: %w", err)
	}
	return m, nil
}

func (m *machine) run(source string) (Outcome, error) {
	promise, err := m.vm.EvalValue("(async () => {"+source+"\n})()", quickjs.EvalGlobal)
	if err != nil {
		m.fail(errorText(err))
		return m.outcome(), nil
	}
	defer promise.Free()
	settled, err := promise.Then(m.fulfilled, m.rejected)
	if err != nil {
		m.fail(errorText(err))
		return m.outcome(), nil
	}
	settled.Free()
	for {
		if _, err := m.vm.ExecutePendingJobs(); err != nil {
			m.fail(errorText(err))
		}
		if m.hostErr != nil {
			return Outcome{}, m.hostErr
		}
		if m.finished || m.status != "" {
			break
		}
		if m.running == 0 && len(m.queue) == 0 {
			m.fail("the script awaits a promise that can never settle")
			break
		}
		select {
		case done := <-m.completions:
			m.settle(done)
		case <-m.ctx.Done():
			m.halt(m.ended())
		}
	}
	// Outcomes that arrived before the script ended are definite even though
	// the script never observed them.
	for {
		select {
		case done := <-m.completions:
			if done.err != nil {
				return Outcome{}, done.err
			}
			m.record(done)
		default:
			return m.outcome(), nil
		}
	}
}

func (m *machine) outcome() Outcome {
	status := m.status
	if status == "" {
		status = builtin.ScriptCompleted
	}
	output := m.output.String()
	if m.result != "" {
		output = strings.TrimPrefix(output+"\n"+m.result, "\n")
	}
	return Outcome{Status: status, Error: m.failure, Output: output, Calls: append([]CallRecord(nil), m.records...)}
}

func (m *machine) close() {
	if m.unwatch != nil {
		m.unwatch()
	}
	m.calls.Wait()
	for index, promise := range m.promises {
		promise.Free()
		delete(m.promises, index)
	}
	if m.format.VM() != nil {
		m.format.Free()
	}
	_ = m.vm.Close()
}

func (m *machine) fulfilled(value quickjs.Value) {
	m.finished = true
	if m.status != "" || value.IsUndefined() {
		return
	}
	formatted, err := m.format.Call(quickjs.UndefinedValue, value)
	if err != nil {
		m.fail(errorText(err))
		return
	}
	text, _ := formatted.(string)
	if !m.fits(text) {
		m.halt(builtin.ScriptOutputLimitExceeded)
		return
	}
	m.result = text
}

func (m *machine) rejected(value quickjs.Value) {
	m.finished = true
	m.fail(describeError(quickjs.ErrorFromValue(value)))
}

// An interrupt is uncatchable, so the script cannot continue past a limit.
func (m *machine) halt(status builtin.ScriptStatus) {
	if m.status == "" {
		m.status = status
	}
	m.vm.Interrupt()
}

// The engine reports an interrupt as the script's own error, so a failure
// after the context ends is that ending.
func (m *machine) fail(text string) {
	if m.status != "" {
		return
	}
	if m.ctx.Err() != nil {
		m.status = m.ended()
		return
	}
	m.status = builtin.ScriptFailed
	m.failure = text
}

func (m *machine) ended() builtin.ScriptStatus {
	if errors.Is(context.Cause(m.ctx), errDeadline) {
		return builtin.ScriptTimedOut
	}
	return builtin.ScriptCanceled
}

func (m *machine) fits(text string) bool {
	size := m.output.Len() + len(text)
	if m.output.Len() > 0 {
		size++
	}
	return size <= m.limits.OutputBytes
}

func (m *machine) print(text string) {
	if m.status != "" {
		return
	}
	if !m.fits(text) {
		m.halt(builtin.ScriptOutputLimitExceeded)
		return
	}
	if m.output.Len() > 0 {
		m.output.WriteByte('\n')
	}
	m.output.WriteString(text)
}

func (m *machine) toolNames() string { return m.tools }

func (m *machine) describe(name string) string {
	description, found := m.host.Describe(name)
	if !found {
		return ""
	}
	return string(description)
}

func (m *machine) call(name, arguments string) quickjs.Value {
	index := len(m.records)
	m.records = append(m.records, CallRecord{Call: Call{Tool: name, Arguments: arguments}, Status: builtin.ScriptCallNotDispatched})
	promise, err := m.vm.NewPromiseCapability()
	if err != nil {
		m.fail(errorText(err))
		m.vm.Interrupt()
		return quickjs.UndefinedValue
	}
	if index >= m.limits.Calls {
		m.halt(builtin.ScriptCallLimitExceeded)
	}
	if m.status != "" {
		pending := promise.Promise.Dup()
		promise.Free()
		return pending
	}
	m.promises[index] = promise
	m.queue = append(m.queue, index)
	m.dispatch()
	return promise.Promise.Dup()
}

func (m *machine) dispatch() {
	for m.status == "" && m.running < m.limits.ConcurrentCalls && len(m.queue) > 0 {
		index := m.queue[0]
		m.queue = m.queue[1:]
		m.records[index].Status = builtin.ScriptCallUnconfirmed
		m.running++
		call := m.records[index].Call
		m.calls.Go(func() {
			settlement, err := m.host.Call(m.ctx, index, call)
			m.completions <- completion{index: index, settlement: settlement, err: err}
		})
	}
}

func (m *machine) settle(done completion) {
	m.running--
	if done.err != nil {
		m.hostErr = done.err
		return
	}
	if !m.record(done) {
		return
	}
	promise := m.promises[done.index]
	delete(m.promises, done.index)
	defer promise.Free()
	var err error
	if value := done.settlement.Value; done.settlement.Status == builtin.ScriptCallSucceeded {
		if m.received+len(value) > m.limits.ResultBytes {
			m.halt(builtin.ScriptResultLimitExceeded)
			return
		}
		var delivered string
		if delivered, err = scriptValue(value); err != nil {
			m.hostErr = fmt.Errorf("script: project call result: %w", err)
			return
		}
		m.received += len(value)
		m.records[done.index].Result = value
		_, err = promise.Resolve.Call(quickjs.UndefinedValue, delivered)
	} else {
		var rejection []byte
		rejection, err = json.Marshal(map[string]string{"status": string(done.settlement.Status), "message": done.settlement.Message})
		if err == nil {
			_, err = promise.Reject.Call(quickjs.UndefinedValue, string(rejection))
		}
	}
	if err != nil {
		m.fail(errorText(err))
	}
	m.dispatch()
}

func (m *machine) record(done completion) bool {
	if err := done.settlement.validate(); err != nil {
		m.hostErr = err
		return false
	}
	m.records[done.index].Status = done.settlement.Status
	m.records[done.index].Message = done.settlement.Message
	return true
}

func errorText(err error) string {
	if engine, ok := errors.AsType[*quickjs.Error](err); ok {
		return describeError(engine)
	}
	return err.Error()
}

func describeError(engine *quickjs.Error) string {
	text := engine.Error()
	if stack := strings.TrimRight(engine.Stack, "\n"); stack != "" && !strings.Contains(text, stack) {
		text += "\n" + stack
	}
	return text
}
