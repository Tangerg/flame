package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	contracts, err := reviews.NewAnalyzerContracts()
	if err != nil {
		return err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame-review-analyzer-example", Version: "1.0.0"}, nil)
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		return err
	}
	if err := contracts.Analyze.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command qimumcp.Command[reviews.Snapshot]) (reviews.Findings, error) {
		return analyze(command.Input)
	}); err != nil {
		return err
	}
	if err := backend.BindCapability("analyzer", contracts.Capability); err != nil {
		return err
	}
	if err := backend.Publish(); err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

func analyze(snapshot reviews.Snapshot) (reviews.Findings, error) {
	if err := snapshot.Validate(); err != nil {
		return reviews.Findings{}, err
	}
	findings := make([]reviews.Finding, 0, 1)
	if snapshot.Status == reviews.Open {
		findings = append(findings, reviews.Finding{Code: "unresolvedReview", Message: "The inspected review remains open; verify its evidence before resolving it.", ReviewID: snapshot.ID, Revision: snapshot.Revision})
	}
	return reviews.Findings{Findings: findings}, nil
}
