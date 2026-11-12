package client_test

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/client"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/frame/portable"
)

type bindingRecorder struct {
	numericPlugin
	requests []protocol.CallPluginRequest
}

func (p *bindingRecorder) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, options protocol.PluginCallOptions) (*protocol.CallPluginResponse, error) {
	p.requests = append(p.requests, request)
	return p.numericPlugin.CallPlugin(ctx, request, options)
}

func (p *bindingRecorder) StreamPlugin(ctx context.Context, request protocol.CallPluginRequest, options protocol.PluginCallOptions) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error) {
	p.requests = append(p.requests, request)
	return p.numericPlugin.StreamPlugin(ctx, request, options)
}

func replaceContract(t *testing.T, binding client.PluginBinding) {
	t.Helper()
	descriptor := binding.Contract.Descriptor()
	descriptor.Reference = "replacement"
	replacement, err := plugincontract.AdmitCapability(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	*binding.Contract = *replacement
}

func TestBoundPluginCallCapturesCapabilityBeforeCallerReplacement(t *testing.T) {
	method, err := pluginmcp.NewMethod[struct{}, map[string]any](portable.MethodConfig{Name: "numbers.read", Description: "Read numbers.", Kind: portable.Query, Target: "installation"})
	if err != nil {
		t.Fatal(err)
	}
	binding := numericBinding(t, method.Descriptor())
	original := binding.Contract.Identity().String()
	caller := &bindingRecorder{}
	bound, err := client.BindPluginMethod(caller, binding, method)
	if err != nil {
		t.Fatal(err)
	}
	replaceContract(t, binding)
	if _, err := bound.Call(t.Context(), struct{}{}, protocol.PluginCallOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(caller.requests) != 1 || caller.requests[0].Contract != original {
		t.Fatal("bound call used the caller's replacement contract")
	}
}

func TestBoundPluginStreamCapturesCapabilityBeforeLazyDelivery(t *testing.T) {
	method, err := pluginmcp.NewStreamingMethod[struct{}, map[string]any, map[string]any](portable.MethodConfig{Name: "numbers.stream", Description: "Stream numbers.", Kind: portable.Query, Target: "installation"})
	if err != nil {
		t.Fatal(err)
	}
	binding := numericBinding(t, method.Descriptor())
	original := binding.Contract.Identity().String()
	caller := &bindingRecorder{}
	bound, err := client.BindPluginStreamingMethod(caller, binding, method)
	if err != nil {
		t.Fatal(err)
	}
	stream := bound.Stream(t.Context(), struct{}{}, protocol.PluginCallOptions{})
	replaceContract(t, binding)
	for _, err := range stream {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(caller.requests) != 1 || caller.requests[0].Contract != original {
		t.Fatal("bound stream used the caller's replacement contract")
	}
}
