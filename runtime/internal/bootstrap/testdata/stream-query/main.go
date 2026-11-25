package main

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) (err error) {
	method, err := mcp.NewStreamingMethod[struct{}, string, bool](frame.MethodConfig{
		Name: "example.records/read.v1", Kind: frame.Query, Target: extension.ScopeInstallation,
		Description: "Read ordered test records.",
	})
	if err != nil {
		return err
	}
	capability, err := contract.NewCapability(contract.CapabilityConfig{
		Reference:   "example.records/streaming.v1",
		Composition: contract.Selection,
		Scope:       extension.ScopeInstallation,
	}, method.Descriptor())
	if err != nil {
		return err
	}
	if len(os.Args) == 2 && os.Args[1] == "describe" {
		_, err := fmt.Fprintln(os.Stdout, capability.Identity().String())
		return err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "stream-query"}, nil)
	backend, err := mcp.NewBackend(server)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, backend.Close()) }()
	if err := method.RegisterServer(backend, read); err != nil {
		return err
	}
	if err := backend.BindCapability("stream", capability); err != nil {
		return err
	}
	if err := backend.Publish(); err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

func read(context.Context, *sdk.ServerSession, mcp.Command[struct{}]) iter.Seq2[mcp.StreamValue[string, bool], error] {
	return func(yield func(mcp.StreamValue[string, bool], error) bool) {
		for _, value := range []string{"first", "second"} {
			if !yield(mcp.StreamChunk[bool](value), nil) {
				return
			}
		}
		yield(mcp.StreamResult[string](true), nil)
	}
}
