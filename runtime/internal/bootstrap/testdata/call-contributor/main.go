package main

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/scopewire"

	"github.com/Tangerg/frame"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

// mode selects the one contribution this process publishes; observer modes
// also name the decorator layer recorded in the trace.
type mode string

const (
	outerObserver          mode = "outer"
	innerObserver          mode = "inner"
	droppingStream         mode = "stream-drop"
	describeDroppingStream mode = "describe-stream-drop"
	rewritingProposal      mode = "proposal"
	gatedProposal          mode = "gated-proposal"
	invalidProposal        mode = "invalid-proposal"
)

const proposedTool = "reviews_update_review"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	if len(os.Args) != 3 {
		return errors.New("contributor: mode and trace endpoint are required")
	}
	selected, trace := mode(os.Args[1]), os.Args[2]
	if selected == describeDroppingStream {
		_, contract, err := droppingStreamContract()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, contract.Identity().String())
		return err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "independent-call-contributor"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return err
	}
	defer backend.Close()
	contract, err := register(ctx, backend, selected, trace)
	if err != nil {
		return err
	}
	if err := backend.BindCapability("contributor", contract); err != nil {
		return err
	}
	if err := backend.Publish(); err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

func register(ctx context.Context, backend *pluginmcp.Backend, selected mode, trace string) (*plugincontract.Capability, error) {
	switch selected {
	case outerObserver, innerObserver:
		return registerObserver(backend, selected, trace)
	case droppingStream:
		return registerDroppingStream(backend, trace)
	case gatedProposal:
		if err := awaitSource(ctx, trace); err != nil {
			return nil, err
		}
		return registerProposal(backend, selected, trace)
	case rewritingProposal, invalidProposal:
		return registerProposal(backend, selected, trace)
	default:
		return nil, fmt.Errorf("contributor: unsupported mode %q", selected)
	}
}

// awaitSource makes startup depend on the test's trace server, so the test
// can fail the contributor's launch on demand.
func awaitSource(ctx context.Context, trace string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, trace+"/startup", nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	if err := response.Body.Close(); err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return errors.New("contributor: source is unavailable")
	}
	return nil
}

func registerProposal(backend *pluginmcp.Backend, selected mode, trace string) (*plugincontract.Capability, error) {
	method, err := pluginsdk.ToolProposalMethod()
	if err != nil {
		return nil, err
	}
	contract, err := pluginsdk.ToolProposalContract()
	if err != nil {
		return nil, err
	}
	arguments := `{"id":"example-review","expectedRevision":1,"status":"resolved"}`
	if selected == invalidProposal {
		arguments = `{"id":"example-review","expectedRevision":1,"status":17}`
	}
	err = method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[chat.ToolCall]) (chat.ToolCall, error) {
		owner, err := pluginmcp.InvocationFromContext(ctx)
		if err != nil || owner.Origin != extension.Model || owner.Parent == "" || owner.ID != command.Input.ID || command.ID != "" {
			return chat.ToolCall{}, errors.New("contributor: original Tool owner was replaced")
		}
		proposal := command.Input
		if proposal.Name != proposedTool {
			return proposal, nil
		}
		if err := record(ctx, trace, proposal); err != nil {
			return chat.ToolCall{}, err
		}
		proposal.Arguments = arguments
		return proposal, nil
	})
	return contract, err
}

func droppingStreamContract() (*pluginmcp.StreamingMethod[struct{}, string, bool], *plugincontract.Capability, error) {
	method, err := pluginmcp.NewStreamingMethod[struct{}, string, bool](frame.MethodConfig{Name: "example.records/stream.v1", Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Stream the captured command."})
	if err != nil {
		return nil, nil, err
	}
	contract, err := plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "example.records/streaming.v1", Composition: plugincontract.Selection, Scope: extension.ScopeInstallation}, method.Descriptor())
	if err != nil {
		return nil, nil, err
	}
	return method, contract, nil
}

func registerDroppingStream(backend *pluginmcp.Backend, trace string) (*plugincontract.Capability, error) {
	method, contract, err := droppingStreamContract()
	if err != nil {
		return nil, err
	}
	if err := method.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[struct{}]) iter.Seq2[pluginmcp.StreamValue[string, bool], error] {
		return func(yield func(pluginmcp.StreamValue[string, bool], error) bool) {
			if err := record(ctx, trace, map[string]string{"phase": "started", "id": string(command.ID)}); err != nil {
				yield(pluginmcp.StreamValue[string, bool]{}, err)
				return
			}
			// Exiting once the chunk is sent drops the stream after witnessed
			// output and before its result.
			if yield(pluginmcp.StreamChunk[bool]("witnessed output"), nil) {
				os.Exit(29)
			}
		}
	}); err != nil {
		return nil, err
	}
	receipt, err := pluginmcp.ReceiptQuery()
	if err != nil {
		return nil, err
	}
	err = receipt.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[pluginmcp.ReceiptRequest]) (pluginmcp.Receipt, error) {
		if err := record(ctx, trace, map[string]string{"phase": "receipt", "id": string(command.Input.ID)}); err != nil {
			return pluginmcp.Receipt{}, err
		}
		return pluginmcp.Receipt{}, nil
	})
	return contract, err
}

// observer records each layer's entry and exit around the host continuation,
// refusing a call whose model identity is not the original one.
type observer struct {
	layer mode
	trace string
}

func (o observer) mark(ctx context.Context, command pluginmcp.Command[scopewire.Request], phase, method string) error {
	owner, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil || owner.ID != string(command.ID) || owner.Target.Reference != string(command.ID) {
		return errors.New("observer: original model identity was replaced")
	}
	return record(ctx, o.trace, map[string]string{"layer": string(o.layer), "phase": phase, "method": method, "id": string(command.ID)})
}

func registerObserver(backend *pluginmcp.Backend, layer mode, trace string) (*plugincontract.Capability, error) {
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		return nil, err
	}
	observing := observer{layer: layer, trace: trace}
	if err := observing.registerCall(backend); err != nil {
		return nil, err
	}
	return contract, observing.registerStream(backend)
}

func (o observer) registerCall(backend *pluginmcp.Backend) error {
	continuation, err := pluginsdk.ModelMethod()
	if err != nil {
		return err
	}
	method, err := pluginsdk.ModelObserverCallMethod()
	if err != nil {
		return err
	}
	name := method.Descriptor().Name
	return method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[scopewire.Request]) (struct{}, error) {
		if err := o.mark(ctx, command, "before", name); err != nil {
			return struct{}{}, err
		}
		next, err := pluginmcp.CallbackFromContext(ctx)
		if err != nil {
			return struct{}{}, err
		}
		if _, err := pluginmcp.CallHost(ctx, next, continuation, command.ID, command.Input); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, o.mark(ctx, command, "after", name)
	})
}

type observationFrame = pluginmcp.StreamValue[scopewire.ResponseDelta, struct{}]

func (o observer) registerStream(backend *pluginmcp.Backend) error {
	stream, err := pluginsdk.ModelObserverStreamMethod()
	if err != nil {
		return err
	}
	name := stream.Descriptor().Name
	return stream.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[scopewire.Request]) iter.Seq2[observationFrame, error] {
		return func(yield func(observationFrame, error) bool) {
			if err := o.observeStream(ctx, command, name, yield); err != nil {
				yield(observationFrame{}, err)
			}
		}
	})
}

func (o observer) observeStream(ctx context.Context, command pluginmcp.Command[scopewire.Request], name string, yield func(observationFrame, error) bool) error {
	if err := o.mark(ctx, command, "before", name); err != nil {
		return err
	}
	next, err := pluginmcp.CallbackFromContext(ctx)
	if err != nil {
		return err
	}
	for value, err := range pluginsdk.StreamModel(ctx, next, command.ID, (*chat.Request)(&command.Input).Clone()) {
		if err != nil {
			return err
		}
		if chunk, ok := value.Chunk(); ok && !yield(pluginmcp.StreamChunk[struct{}](chunk), nil) {
			return nil
		}
	}
	if err := o.mark(ctx, command, "after", name); err != nil {
		return err
	}
	yield(pluginmcp.StreamResult[scopewire.ResponseDelta](struct{}{}), nil)
	return nil
}

func record(ctx context.Context, endpoint string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return errors.New("contributor: trace was refused")
	}
	return nil
}
