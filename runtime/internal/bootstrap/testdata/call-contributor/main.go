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

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

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
	mode, trace := os.Args[1], os.Args[2]
	server := sdk.NewServer(&sdk.Implementation{Name: "independent-call-contributor"}, nil)
	backend, err := pluginsdk.NewBackend(server)
	if err != nil {
		return err
	}
	defer backend.Close()
	if mode == "outer" || mode == "inner" {
		return runObserver(ctx, server, backend, mode, trace)
	}
	if mode == "stream-drop" || mode == "describe-stream-drop" {
		return runDroppingStream(ctx, server, backend, mode, trace)
	}
	if mode == "gated-proposal" {
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
	}
	method, err := pluginsdk.ToolProposalMethod()
	if err != nil {
		return err
	}
	contract, err := pluginsdk.ToolProposalContract()
	if err != nil {
		return err
	}
	if err := method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[chat.ToolCall]) (chat.ToolCall, error) {
		owner, err := pluginsdk.InvocationFromContext(ctx)
		if err != nil || owner.Origin != capability.Model || owner.Parent == "" || owner.ID != command.Input.ID || command.ID != "" {
			return chat.ToolCall{}, errors.New("contributor: original Tool owner was replaced")
		}
		proposal := command.Input
		if proposal.Name != "reviews_update_review" {
			return proposal, nil
		}
		if err := record(ctx, trace, proposal); err != nil {
			return chat.ToolCall{}, err
		}
		switch mode {
		case "proposal", "gated-proposal":
			proposal.Arguments = `{"id":"example-review","expectedRevision":1,"status":"resolved"}`
		case "invalid-proposal":
			proposal.Arguments = `{"id":"example-review","expectedRevision":1,"status":17}`
		default:
			return chat.ToolCall{}, errors.New("contributor: unsupported mode")
		}
		return proposal, nil
	}); err != nil {
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

func runDroppingStream(ctx context.Context, server *sdk.Server, backend *pluginsdk.Backend, mode, trace string) error {
	method, err := pluginsdk.NewStreamingMethod[struct{}, string, bool](pluginsdk.MethodConfig{Name: "example.records/stream.v1", Kind: capability.Mutation, Target: "installation", Description: "Stream the captured command."})
	if err != nil {
		return err
	}
	contract, err := capability.New(capability.Config{Reference: "example.records/streaming.v1", Composition: capability.Selection, Scope: "installation"}, method.Descriptor())
	if err != nil {
		return err
	}
	if mode == "describe-stream-drop" {
		_, err := fmt.Fprintln(os.Stdout, contract.Identity().String())
		return err
	}
	if err := method.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[struct{}]) iter.Seq2[pluginsdk.StreamValue[string, bool], error] {
		return func(yield func(pluginsdk.StreamValue[string, bool], error) bool) {
			if err := record(ctx, trace, map[string]string{"phase": "started", "id": command.ID}); err != nil {
				yield(pluginsdk.StreamValue[string, bool]{}, err)
				return
			}
			if yield(pluginsdk.StreamValue[string, bool]{Chunk: new("witnessed output")}, nil) {
				os.Exit(29)
			}
		}
	}); err != nil {
		return err
	}
	if err := pluginsdk.RegisterServer(backend, pluginsdk.ReceiptMethod, nil, func(ctx context.Context, _ *sdk.ServerSession, request pluginsdk.ReceiptRequest) (pluginsdk.Receipt, error) {
		if err := record(ctx, trace, map[string]string{"phase": "receipt", "id": request.ID}); err != nil {
			return pluginsdk.Receipt{}, err
		}
		return pluginsdk.Receipt{}, nil
	}); err != nil {
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

func runObserver(ctx context.Context, server *sdk.Server, backend *pluginsdk.Backend, layer, trace string) error {
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		return err
	}
	mark := func(ctx context.Context, command pluginsdk.Command[chat.Request], phase string) error {
		if len(command.Input.Tools) == 0 {
			return nil
		}
		owner, err := pluginsdk.InvocationFromContext(ctx)
		if err != nil || owner.ID != command.ID || owner.Target.Reference != command.ID {
			return errors.New("observer: original model identity was replaced")
		}
		return record(ctx, trace, map[string]string{"layer": layer, "phase": phase, "id": command.ID})
	}
	method, err := pluginsdk.ModelMethod()
	if err != nil {
		return err
	}
	if err := method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[chat.Request]) (pluginsdk.ModelResult, error) {
		if err := mark(ctx, command, "before"); err != nil {
			return pluginsdk.ModelResult{}, err
		}
		next, err := pluginsdk.CallbackFromContext(ctx)
		if err != nil {
			return pluginsdk.ModelResult{}, err
		}
		response, err := pluginsdk.CallModel(ctx, next, command.ID, command.Input.Clone())
		if err != nil {
			return pluginsdk.ModelResult{}, err
		}
		if err := mark(ctx, command, "after"); err != nil {
			return pluginsdk.ModelResult{}, err
		}
		return pluginsdk.ModelResult{Response: response}, nil
	}); err != nil {
		return err
	}
	stream, err := pluginsdk.ModelStreamMethod()
	if err != nil {
		return err
	}
	if err := stream.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[chat.Request]) iter.Seq2[pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult], error] {
		return func(yield func(pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult], error) bool) {
			if err := mark(ctx, command, "before"); err != nil {
				yield(pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult]{}, err)
				return
			}
			next, err := pluginsdk.CallbackFromContext(ctx)
			if err != nil {
				yield(pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult]{}, err)
				return
			}
			for value, err := range pluginsdk.StreamModel(ctx, next, command.ID, command.Input.Clone()) {
				if !yield(value, err) || err != nil {
					return
				}
			}
			if err := mark(ctx, command, "after"); err != nil {
				yield(pluginsdk.StreamValue[chat.ResponseDelta, pluginsdk.ModelResult]{}, err)
			}
		}
	}); err != nil {
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
