package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tangerg/flame/runtime/capability"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
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
		case "proposal":
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
