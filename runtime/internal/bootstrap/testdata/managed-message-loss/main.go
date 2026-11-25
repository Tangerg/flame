package main

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	framecall "github.com/Tangerg/frame/call"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

// faultMode is how the probe disturbs the stdio stream between Runtime and
// the managed backend it launches.
type faultMode string

const (
	withholdRequest  faultMode = "request"
	withholdResponse faultMode = "response"
	durableCut       faultMode = "durable"
	replaceResponse  faultMode = "replace-response"
)

// stage is the durable cut the test selects in the "<evidence>.phase" file
// before each Runtime process it starts.
type stage testsupport.ScheduleCrashPhase

const (
	commitCut          = stage(testsupport.ScheduleCrashCommit)
	editedCut          = stage(testsupport.ScheduleCrashEdited)
	queryCut           = stage(testsupport.ScheduleCrashQuery)
	absentReceipt      = stage(testsupport.ScheduleCrashAbsent)
	conflictingReceipt = stage(testsupport.ScheduleCrashConflict)
	malformedReceipt   = stage(testsupport.ScheduleCrashMalformed)
)

const (
	callToolMethod     = "tools/call"
	createScheduleTool = "create_schedule"
)

var errUsage = errors.New("message probe requires a file, backend, method, tool and fault mode")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (err error) {
	probe, err := newProbe(os.Args[1:])
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	command := exec.CommandContext(ctx, probe.backend)
	command.Stderr = os.Stderr
	input, err := command.StdinPipe()
	if err != nil {
		return err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	defer func() {
		cancel()
		_ = input.Close()
		err = errors.Join(err, command.Wait())
	}()
	forwarded := make(chan error, 1)
	go func() {
		err := probe.forwardRequests(input)
		forwarded <- err
		// A SIGKILLed Runtime reaches the probe only as stdin EOF; cancelling
		// releases a halted cut and stops the backend.
		if err != nil || probe.mode == durableCut {
			cancel()
		}
	}()
	if err := probe.relayResponses(ctx, output); err != nil {
		return err
	}
	select {
	case err := <-forwarded:
		return err
	default:
		return nil
	}
}

type probe struct {
	evidence string
	backend  string
	method   string
	tool     string
	mode     faultMode
	// retained holds each matched request line until its response arrives.
	retained sync.Map
}

func newProbe(args []string) (*probe, error) {
	if len(args) != 5 {
		return nil, errUsage
	}
	mode := faultMode(args[4])
	switch mode {
	case withholdRequest, withholdResponse, durableCut, replaceResponse:
	default:
		return nil, errUsage
	}
	return &probe{evidence: args[0], backend: args[1], method: args[2], tool: args[3], mode: mode}, nil
}

type request struct {
	ID     jsontext.Value `json:"id"`
	Method string         `json:"method"`
	Params struct {
		Name   string `json:"name"`
		Method string `json:"method"`
	} `json:"params"`
}

// semantic unwraps Flame's invoke envelope to the method the backend implements.
func (r request) semantic() string {
	if r.Method == pluginmcp.InvokeMethod {
		return r.Params.Method
	}
	return r.Method
}

func (p *probe) matches(r request) bool {
	return r.semantic() == p.method && (p.tool == "" || r.Params.Name == p.tool)
}

func (p *probe) forwardRequests(backend io.WriteCloser) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(nil, sdk.DefaultMaxLineLength)
	for scanner.Scan() {
		if err := p.forward(backend, append([]byte(nil), scanner.Bytes()...)); err != nil {
			return err
		}
	}
	return errors.Join(scanner.Err(), backend.Close())
}

func (p *probe) forward(backend io.Writer, line []byte) error {
	var message request
	if err := json.Unmarshal(line, &message); err != nil {
		return err
	}
	switch p.mode {
	case withholdRequest:
		if p.matches(message) {
			withheld, err := p.withhold(line, nil)
			if err != nil || withheld {
				return errors.Join(err, errors.New("message loss probe withheld request"))
			}
		}
	case durableCut:
		cut, err := p.stage()
		if err != nil {
			return err
		}
		if cut.retains(message) {
			p.retained.Store(string(message.ID), jsontext.Value(line))
		}
	default:
		if p.matches(message) {
			p.retained.Store(string(message.ID), jsontext.Value(line))
		}
	}
	_, err := fmt.Fprintln(backend, string(line))
	return err
}

func (p *probe) relayResponses(ctx context.Context, backend io.Reader) error {
	responses := bufio.NewScanner(backend)
	responses.Buffer(nil, sdk.DefaultMaxLineLength)
	for responses.Scan() {
		line, err := p.relay(ctx, responses.Bytes())
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(os.Stdout, string(line)); err != nil {
			return err
		}
	}
	return responses.Err()
}

func (p *probe) relay(ctx context.Context, line []byte) ([]byte, error) {
	var response struct {
		ID jsontext.Value `json:"id"`
	}
	if err := json.Unmarshal(line, &response); err != nil {
		return nil, err
	}
	retained, ok := p.retained.LoadAndDelete(string(response.ID))
	if !ok {
		return line, nil
	}
	original := retained.(jsontext.Value)
	switch p.mode {
	case durableCut:
		return p.cut(ctx, original, line)
	case replaceResponse:
		replacement, err := os.ReadFile(p.evidence)
		if err != nil {
			return nil, err
		}
		return replaceReply(line, replacement)
	default:
		withheld, err := p.withhold(original, line)
		if err != nil {
			return nil, err
		}
		if withheld {
			return nil, errors.New("message loss probe withheld committed response")
		}
		return line, nil
	}
}

// cut records the exchange at the selected stage. A halting stage then parks
// the probe so the test can SIGKILL Runtime after the backend committed.
func (p *probe) cut(ctx context.Context, original jsontext.Value, line []byte) ([]byte, error) {
	cut, err := p.stage()
	if err != nil {
		return nil, err
	}
	line, err = cut.fault(line)
	if err != nil {
		return nil, err
	}
	record, err := json.Marshal(struct {
		Request       jsontext.Value `json:"request"`
		Response      jsontext.Value `json:"response"`
		DataDirectory string         `json:"dataDirectory"`
	}{original, jsontext.Value(line), os.Getenv("PLUGIN_DATA")})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(p.evidence+"."+string(cut), record, 0600); err != nil {
		return nil, err
	}
	if !cut.halts() {
		return line, nil
	}
	if err := p.signalReady(); err != nil {
		return nil, err
	}
	<-ctx.Done()
	return nil, context.Cause(ctx)
}

func (p *probe) stage() (stage, error) {
	phase, err := os.ReadFile(p.evidence + ".phase")
	if err != nil {
		return "", err
	}
	return stage(strings.TrimSpace(string(phase))), nil
}

func (p *probe) signalReady() error {
	ready, err := os.OpenFile(p.evidence+".ready", os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := ready.WriteString("ready\n")
	return errors.Join(writeErr, ready.Close())
}

// withhold claims the evidence file exclusively, so only the first matching
// message is lost and every later one, including after a restart, passes.
func (p *probe) withhold(request, response jsontext.Value) (bool, error) {
	evidence, err := json.Marshal(struct {
		DataDirectory string         `json:"dataDirectory"`
		Request       jsontext.Value `json:"request"`
		Response      jsontext.Value `json:"response,omitempty"`
	}{os.Getenv("PLUGIN_DATA"), request, response})
	if err != nil {
		return false, err
	}
	file, err := os.OpenFile(p.evidence, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, writeErr := file.Write(evidence)
	return true, errors.Join(writeErr, file.Close())
}

// cutsCreate reports whether the stage interrupts the create call itself
// rather than the receipt query that recovers it.
func (s stage) cutsCreate() bool {
	return s == commitCut || s == editedCut
}

func (s stage) halts() bool {
	return s.cutsCreate() || s == queryCut
}

func (s stage) retains(r request) bool {
	if s.cutsCreate() {
		return r.Method == callToolMethod && r.Params.Name == createScheduleTool
	}
	return r.semantic() == pluginsdk.ToolReceiptMethod
}

// fault rewrites the receipt reply the way a negative stage requires; every
// other stage forwards the backend's reply intact.
func (s stage) fault(response []byte) ([]byte, error) {
	var result jsontext.Value
	switch s {
	case absentReceipt:
		encoded, err := pluginmcp.EncodeReply(pluginsdk.ToolReceipt{}, nil, nil)
		if err != nil {
			return nil, err
		}
		result = encoded.Bytes()
	case conflictingReceipt:
		encoded, err := pluginmcp.EncodeReply(pluginsdk.ToolReceipt{}, framecall.ErrConflict, nil)
		if err != nil {
			return nil, err
		}
		result = encoded.Bytes()
	case malformedReceipt:
		result = jsontext.Value(`{"unexpected":"result"}`)
	default:
		return response, nil
	}
	return replaceReply(response, result)
}

func replaceReply(response []byte, result jsontext.Value) ([]byte, error) {
	var envelope map[string]jsontext.Value
	if err := json.Unmarshal(response, &envelope); err != nil {
		return nil, err
	}
	envelope["result"] = result
	delete(envelope, "error")
	return json.Marshal(envelope)
}
