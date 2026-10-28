package mcp

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"iter"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const InvokeMethod = "io.github.tangerg.qimu/operations.invoke"
const DescribeMethod = "io.github.tangerg.qimu/operations.describe"
const ValidateMethod = "io.github.tangerg.qimu/operations.validate"

type invokeParams struct {
	sdk.ParamsBase
	Protocol   string         `json:"protocol"`
	Method     string         `json:"method"`
	Contract   string         `json:"contract"`
	Export     string         `json:"export,omitzero"`
	Capability string         `json:"capability,omitzero"`
	Input      jsontext.Value `json:"input"`
	CommandID  string         `json:"commandId,omitzero"`
}

func (p *invokeParams) UnmarshalJSON(data []byte) error {
	type wire invokeParams
	var decoded wire
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if decoded.Protocol != portable.Protocol || decoded.Method == "" || len(decoded.Contract) != 64 || len(decoded.Input) == 0 {
		return errors.New("plugin: current backend protocol, method, contract and input are required")
	}
	if (decoded.Export == "") != (decoded.Capability == "") || decoded.Export != "" && (!portable.ValidExportName(decoded.Export) || len(decoded.Capability) != 64) {
		return errors.New("plugin: capability export and contract must be supplied together")
	}
	*p = invokeParams(decoded)
	return nil
}

type protocolParams struct {
	sdk.ParamsBase
	Protocol string `json:"protocol"`
}

func (p *protocolParams) UnmarshalJSON(data []byte) error {
	type wire protocolParams
	var decoded wire
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if decoded.Protocol != portable.Protocol {
		return errors.New("plugin: current backend protocol is required")
	}
	*p = protocolParams(decoded)
	return nil
}

type Description struct {
	Protocol     string                      `json:"protocol"`
	Methods      []portable.MethodDescriptor `json:"methods"`
	Capabilities []CapabilityExport          `json:"capabilities"`
}

type boundMethod struct {
	contract *portable.Method
	validate func(jsontext.Value) error
	invoke   func(context.Context, *sdk.ServerSession, *invokeParams) (*methodResult[jsontext.Value], error)
	stream   func(context.Context, *sdk.ServerSession, *invokeParams) (iter.Seq2[streamValue, error], error)
}

// Backend owns declaration/code binding and publication. Setup is explicit;
// after Publish no export can change. User code runs outside this owner's lock.
type Backend struct {
	server       *sdk.Server
	mu           sync.RWMutex
	methods      map[string]boundMethod
	capabilities map[string]*portable.Capability
	published    bool
	closeDone    chan struct{}
	closeErr     error
	closed       bool
	setup        func(context.Context, Setup) (io.Closer, error)
	activations  map[*sdk.ServerSession]*setupActivation
	setupJoins   sync.WaitGroup
}

func NewBackend(server *sdk.Server) (*Backend, error) {
	if server == nil {
		return nil, errors.New("plugin: MCP server is required")
	}
	b := &Backend{server: server, methods: map[string]boundMethod{}, capabilities: map[string]*portable.Capability{}, activations: map[*sdk.ServerSession]*setupActivation{}, closeDone: make(chan struct{})}
	if err := b.registerSetup(); err != nil {
		return nil, err
	}
	if err := sdk.AddReceivingCustomMethod(server, InvokeMethod, func(ctx context.Context, session *sdk.ServerSession, params *invokeParams) (*methodResult[jsontext.Value], error) {
		method, err := b.resolveMethod(params)
		if err != nil {
			return nil, err
		}
		if method.stream != nil {
			return invokeBackendStream(ctx, session, method, params)
		}
		return method.invoke(ctx, session, params)
	}); err != nil {
		return nil, err
	}
	if err := sdk.AddReceivingCustomMethod(server, ValidateMethod, func(_ context.Context, _ *sdk.ServerSession, params *invokeParams) (*methodResult[string], error) {
		method, err := b.resolveMethod(params)
		if err != nil {
			return nil, err
		}
		if params.CommandID != "" {
			return nil, errors.New("plugin: input validation cannot carry a command identity")
		}
		if err := method.validate(params.Input); err != nil {
			return reply(err.Error(), nil, nil)
		}
		return reply("", nil, nil)
	}); err != nil {
		return nil, err
	}
	if err := sdk.AddReceivingCustomMethod(server, DescribeMethod, func(_ context.Context, _ *sdk.ServerSession, params *protocolParams) (*methodResult[Description], error) {
		if params == nil || params.Protocol != portable.Protocol {
			return nil, errors.New("plugin: current backend protocol is required")
		}
		b.mu.RLock()
		defer b.mu.RUnlock()
		if !b.published || b.closed {
			return nil, errors.New("plugin: backend is not published")
		}
		methods := make([]portable.MethodDescriptor, 0, len(b.methods))
		for _, method := range b.methods {
			methods = append(methods, method.contract.Descriptor())
		}
		slices.SortFunc(methods, func(a, b portable.MethodDescriptor) int { return strings.Compare(a.Name, b.Name) })
		capabilities := make([]CapabilityExport, 0, len(b.capabilities))
		for name, capability := range b.capabilities {
			capabilities = append(capabilities, CapabilityExport{Name: name, Contract: capability.Descriptor()})
		}
		slices.SortFunc(capabilities, func(a, b CapabilityExport) int { return strings.Compare(a.Name, b.Name) })
		return reply(Description{Protocol: portable.Protocol, Methods: methods, Capabilities: capabilities}, nil, nil)
	}); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Backend) resolveMethod(params *invokeParams) (boundMethod, error) {
	if params == nil || params.Protocol != portable.Protocol {
		return boundMethod{}, errors.New("plugin: current backend protocol is required")
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	method, found := b.methods[params.Method]
	if !b.published || b.closed || !found {
		return boundMethod{}, errors.New("plugin: operation export is not published")
	}
	if params.Contract != method.contract.Identity().Digest() {
		return boundMethod{}, errors.New("plugin: operation contract mismatch")
	}
	if err := b.checkCapability(params); err != nil {
		return boundMethod{}, err
	}
	return method, nil
}

func (b *Backend) Publish() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.published || b.closed {
		return errors.New("plugin: backend is already published")
	}
	b.published = true
	return nil
}

func (b *Backend) register(method boundMethod) error {
	if b == nil {
		return errors.New("plugin: backend is required")
	}
	if method.contract == nil || method.validate == nil || method.invoke == nil && method.stream == nil {
		return errors.New("plugin: admitted method and handler are required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.published || b.closed {
		return errors.New("plugin: published backend cannot change exports")
	}
	if _, found := b.methods[method.contract.Name()]; found {
		return errors.New("plugin: duplicate operation export")
	}
	b.methods[method.contract.Name()] = method
	return nil
}

func Describe(ctx context.Context, session *sdk.ClientSession) (Description, error) {
	var zero Description
	if session == nil {
		return zero, errors.New("plugin: client session is required")
	}
	result, err := sdk.CallCustomMethod[*protocolParams, *methodResult[Description]](ctx, session, DescribeMethod, &protocolParams{Protocol: portable.Protocol})
	if err != nil {
		return zero, err
	}
	value, err := result.value(DescribeMethod)
	if err != nil {
		return zero, err
	}
	if value.Protocol != portable.Protocol {
		return zero, errors.New("plugin: incompatible backend protocol")
	}
	return value, nil
}
