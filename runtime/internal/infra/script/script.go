// Package script runs a JavaScript program in a QuickJS virtual machine whose
// only route to the outside world is the host calls it is given.
package script

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
)

type Limits struct {
	Deadline        time.Duration
	MemoryBytes     uintptr
	Calls           int
	ConcurrentCalls int
	OutputBytes     int
}

func (l Limits) validate() error {
	if l.Deadline <= 0 || l.MemoryBytes == 0 || l.Calls <= 0 || l.ConcurrentCalls <= 0 || l.OutputBytes <= 0 {
		return errors.New("script: every limit must be positive")
	}
	return nil
}

// Call is one host call the script requested. Arguments is the JSON text the
// script supplied; the host validates it.
type Call struct {
	Tool      string
	Arguments string
}

// Settlement is a host's definite outcome for one call.
type Settlement struct {
	Status builtin.ScriptCallStatus
	// Value is the JSON value a succeeded call resolves to.
	Value jsontext.Value
	// Message is the error text a failed or rejected call rejects with.
	Message string
}

func (s Settlement) validate() error {
	switch s.Status {
	case builtin.ScriptCallSucceeded:
		if !s.Value.IsValid() {
			return errors.New("script: succeeded call value is not JSON")
		}
	case builtin.ScriptCallFailed, builtin.ScriptCallRejected:
		if s.Message == "" {
			return errors.New("script: unsuccessful call requires a message")
		}
	default:
		return fmt.Errorf("script: call settlement has status %q", s.Status)
	}
	return nil
}

type Host interface {
	Tools() []string
	// Describe returns the JSON description of a callable tool.
	Describe(name string) (jsontext.Value, bool)
	// Call runs on its own goroutine and must return once ctx ends. An error is
	// a host failure that ends the script; it is never the call's outcome.
	Call(ctx context.Context, index int, call Call) (Settlement, error)
}

type CallRecord struct {
	Call    Call
	Status  builtin.ScriptCallStatus
	Message string
}

type Outcome struct {
	Status builtin.ScriptStatus
	// Error is the uncaught error of a failed script.
	Error  string
	Output string
	Calls  []CallRecord
}

// Run returns an error only when the engine or the host fails; every script
// ending, including its own errors and exceeded limits, is an Outcome.
func Run(ctx context.Context, source string, host Host, limits Limits) (Outcome, error) {
	if err := limits.validate(); err != nil {
		return Outcome{}, err
	}
	tools := host.Tools()
	if slices.ContainsFunc(tools, func(name string) bool { return name == "" }) {
		return Outcome{}, errors.New("script: tool names must not be empty")
	}
	runCtx, cancel := context.WithTimeoutCause(ctx, limits.Deadline, errDeadline)
	defer cancel()
	machine, err := start(runCtx, tools, host, limits)
	if err != nil {
		return Outcome{}, err
	}
	outcome, err := machine.run(source)
	cancel()
	machine.close()
	if err != nil {
		return Outcome{}, err
	}
	return outcome, nil
}
