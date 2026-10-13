package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/flame/runtime/capability"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const invokeMethod = "io.github.tangerg.flame/operations.invoke"
const describeMethod = "io.github.tangerg.flame/operations.describe"

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
	if decoded.Protocol != capability.Protocol || decoded.Method == "" || len(decoded.Contract) != 64 || len(decoded.Input) == 0 {
		return errors.New("plugin: current backend protocol, method, contract and input are required")
	}
	if (decoded.Export == "") != (decoded.Capability == "") || decoded.Export != "" && (!capability.ValidExportName(decoded.Export) || len(decoded.Capability) != 64) {
		return errors.New("plugin: capability export and contract must be supplied together")
	}
	*p = invokeParams(decoded)
	return nil
}

type describeParams struct {
	sdk.ParamsBase
	Protocol string `json:"protocol"`
}

func (p *describeParams) UnmarshalJSON(data []byte) error {
	type wire describeParams
	var decoded wire
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if decoded.Protocol != capability.Protocol {
		return errors.New("plugin: current backend protocol is required")
	}
	*p = describeParams(decoded)
	return nil
}

type Description struct {
	Protocol     string                        `json:"protocol"`
	Methods      []capability.MethodDescriptor `json:"methods"`
	Capabilities []CapabilityExport            `json:"capabilities"`
}

type boundMethod struct {
	contract *capability.Method
	invoke   func(context.Context, *sdk.ServerSession, *invokeParams) (*Result[jsontext.Value], error)
}

// Backend owns declaration/code binding and publication. Setup is explicit;
// after Publish no export can change. User code runs outside this owner's lock.
type Backend struct {
	server       *sdk.Server
	mu           sync.RWMutex
	methods      map[string]boundMethod
	capabilities map[string]*capability.Contract
	published    bool
}

func NewBackend(server *sdk.Server) (*Backend, error) {
	if server == nil {
		return nil, errors.New("plugin: MCP server is required")
	}
	b := &Backend{server: server, methods: map[string]boundMethod{}, capabilities: map[string]*capability.Contract{}}
	if err := sdk.AddReceivingCustomMethod(server, invokeMethod, func(ctx context.Context, session *sdk.ServerSession, params *invokeParams) (*Result[jsontext.Value], error) {
		if params == nil || params.Protocol != capability.Protocol {
			return nil, errors.New("plugin: current backend protocol is required")
		}
		b.mu.RLock()
		method, found := b.methods[params.Method]
		ready := b.published
		exportErr := b.checkCapability(params)
		b.mu.RUnlock()
		if !ready || !found {
			return nil, errors.New("plugin: operation export is not published")
		}
		if params.Contract != method.contract.Identity().Digest() {
			return nil, errors.New("plugin: operation contract mismatch")
		}
		if exportErr != nil {
			return nil, exportErr
		}
		return method.invoke(ctx, session, params)
	}); err != nil {
		return nil, err
	}
	if err := sdk.AddReceivingCustomMethod(server, describeMethod, func(_ context.Context, _ *sdk.ServerSession, params *describeParams) (*Result[Description], error) {
		if params == nil || params.Protocol != capability.Protocol {
			return nil, errors.New("plugin: current backend protocol is required")
		}
		b.mu.RLock()
		defer b.mu.RUnlock()
		if !b.published {
			return nil, errors.New("plugin: backend is not published")
		}
		methods := make([]capability.MethodDescriptor, 0, len(b.methods))
		for _, method := range b.methods {
			methods = append(methods, method.contract.Descriptor())
		}
		slices.SortFunc(methods, func(a, b capability.MethodDescriptor) int { return strings.Compare(a.Name, b.Name) })
		capabilities := make([]CapabilityExport, 0, len(b.capabilities))
		for name, capability := range b.capabilities {
			capabilities = append(capabilities, CapabilityExport{Name: name, Contract: capability.Descriptor()})
		}
		slices.SortFunc(capabilities, func(a, b CapabilityExport) int { return strings.Compare(a.Name, b.Name) })
		return Reply(Description{Protocol: capability.Protocol, Methods: methods, Capabilities: capabilities}, nil, nil)
	}); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Backend) Publish() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.published {
		return errors.New("plugin: backend is already published")
	}
	if len(b.methods) == 0 {
		return errors.New("plugin: backend requires an operation export")
	}
	b.published = true
	return nil
}

func (b *Backend) register(method boundMethod) error {
	if b == nil {
		return errors.New("plugin: backend is required")
	}
	if method.contract == nil || method.invoke == nil {
		return errors.New("plugin: admitted method and handler are required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.published {
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
	result, err := sdk.CallCustomMethod[*describeParams, *Result[Description]](ctx, session, describeMethod, &describeParams{Protocol: capability.Protocol})
	if err != nil {
		return zero, err
	}
	value, err := result.value(describeMethod)
	if err != nil {
		return zero, err
	}
	if value.Protocol != capability.Protocol {
		return zero, errors.New("plugin: incompatible backend protocol")
	}
	return value, nil
}
