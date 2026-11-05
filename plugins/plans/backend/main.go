package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	planmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/planworkflow/mcp"

	"github.com/Tangerg/flame/packages/go-sdk/planworkflow"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func author(proposal planworkflow.Proposal) (planworkflow.Decision, error) {
	active := false
	for _, step := range proposal.Steps {
		if err := step.Validate(); err != nil {
			return planworkflow.Decision{Refusal: err.Error()}, nil
		}
		if step.Status == planworkflow.PlanStatusInProgress {
			if active {
				return planworkflow.Decision{Refusal: "at most one step may be in progress"}, nil
			}
			active = true
		}
	}
	document, err := planworkflow.Seal(proposal)
	return planworkflow.Decision{Document: &document}, err
}

func review(document planworkflow.Document) (string, error) {
	if err := document.Verify(); err != nil {
		return "", err
	}
	var text strings.Builder
	for _, step := range document.Steps {
		if err := step.Validate(); err != nil {
			return "", err
		}
		mark := "[ ]"
		switch step.Status {
		case planworkflow.PlanStatusCompleted:
			mark = "[x]"
		case planworkflow.PlanStatusInProgress:
			mark = "[~]"
		}
		fmt.Fprintf(&text, "%s %s\n", mark, step.Description)
	}
	return text.String(), nil
}

func run(ctx context.Context, _ string) (err error) {
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.plans", Version: "1.0.0"}, nil)
	host, err := pluginmcp.NewBackend(server)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, host.Close()) }()
	method, err := planmcp.AuthorMethod()
	if err != nil {
		return err
	}
	if err := method.RegisterServer(host, nil, func(_ context.Context, _ *sdk.ServerSession, call pluginmcp.Command[planworkflow.Proposal]) (planworkflow.Decision, error) {
		return author(call.Input)
	}); err != nil {
		return err
	}
	reviewer, err := planmcp.ReviewMethod()
	if err != nil {
		return err
	}
	if err := reviewer.RegisterServer(host, nil, func(_ context.Context, _ *sdk.ServerSession, call pluginmcp.Command[planworkflow.Document]) (string, error) {
		return review(call.Input)
	}); err != nil {
		return err
	}
	contract, err := planmcp.Contract()
	if err != nil {
		return err
	}
	if err := host.BindCapability("plans", contract); err != nil {
		return err
	}
	if err := host.Publish(); err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

func main() { backend.Main("plan workflow", run) }
