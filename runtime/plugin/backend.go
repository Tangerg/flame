package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"slices"
	"strings"
	"sync"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

// BackendProtocol identifies public wire behavior, independently of private
// implementation releases. There is one supported contract and no old reader.
const BackendProtocol = "io.github.tangerg.flame/backend/2026-10-13.1"

const invokeMethod = "io.github.tangerg.flame/operations.invoke"
const describeMethod = "io.github.tangerg.flame/operations.describe"

type invokeParams struct {
	sdk.ParamsBase
	Protocol  string         `json:"protocol"`
	Method    string         `json:"method"`
	Input     jsontext.Value `json:"input"`
	CommandID string         `json:"commandId,omitzero"`
}

func (p *invokeParams) UnmarshalJSON(data []byte) error {
	type wire invokeParams
	var decoded wire
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if decoded.Protocol != BackendProtocol || decoded.Method == "" || len(decoded.Input) == 0 {
		return errors.New("plugin: current backend protocol, method and input are required")
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
	if decoded.Protocol != BackendProtocol {
		return errors.New("plugin: current backend protocol is required")
	}
	*p = describeParams(decoded)
	return nil
}

type Description struct {
	Protocol string             `json:"protocol"`
	Methods  []MethodDescriptor `json:"methods"`
}

type boundMethod struct {
	descriptor MethodDescriptor
	invoke     func(context.Context, *sdk.ServerSession, *invokeParams) (*Result[jsontext.Value], error)
}

// Backend owns declaration/code binding and publication. Setup is explicit;
// after Publish no export can change. User code runs outside this owner's lock.
type Backend struct {
	server    *sdk.Server
	mu        sync.RWMutex
	methods   map[string]boundMethod
	published bool
}

func NewBackend(server *sdk.Server) (*Backend, error) {
	if server == nil {
		return nil, errors.New("plugin: MCP server is required")
	}
	b := &Backend{server: server, methods: map[string]boundMethod{}}
	if err := sdk.AddReceivingCustomMethod(server, invokeMethod, func(ctx context.Context, session *sdk.ServerSession, params *invokeParams) (*Result[jsontext.Value], error) {
		if params == nil || params.Protocol != BackendProtocol {
			return nil, errors.New("plugin: current backend protocol is required")
		}
		b.mu.RLock()
		method, found := b.methods[params.Method]
		ready := b.published
		b.mu.RUnlock()
		if !ready || !found {
			return nil, errors.New("plugin: operation export is not published")
		}
		return method.invoke(ctx, session, params)
	}); err != nil {
		return nil, err
	}
	if err := sdk.AddReceivingCustomMethod(server, describeMethod, func(_ context.Context, _ *sdk.ServerSession, params *describeParams) (*Result[Description], error) {
		if params == nil || params.Protocol != BackendProtocol {
			return nil, errors.New("plugin: current backend protocol is required")
		}
		b.mu.RLock()
		defer b.mu.RUnlock()
		if !b.published {
			return nil, errors.New("plugin: backend is not published")
		}
		methods := make([]MethodDescriptor, 0, len(b.methods))
		for _, method := range b.methods {
			methods = append(methods, cloneDescriptor(method.descriptor))
		}
		slices.SortFunc(methods, func(a, b MethodDescriptor) int { return strings.Compare(a.Name, b.Name) })
		return Reply(Description{Protocol: BackendProtocol, Methods: methods}, nil, nil)
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
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.published {
		return errors.New("plugin: published backend cannot change exports")
	}
	if _, found := b.methods[method.descriptor.Name]; found {
		return errors.New("plugin: duplicate operation export")
	}
	b.methods[method.descriptor.Name] = method
	return nil
}

func cloneDescriptor(value MethodDescriptor) MethodDescriptor {
	value.Input = append(jsontext.Value(nil), value.Input...)
	value.Output = append(jsontext.Value(nil), value.Output...)
	return value
}

func Describe(ctx context.Context, session *sdk.ClientSession) (Description, error) {
	var zero Description
	if session == nil {
		return zero, errors.New("plugin: client session is required")
	}
	result, err := sdk.CallCustomMethod[*describeParams, *Result[Description]](ctx, session, describeMethod, &describeParams{Protocol: BackendProtocol})
	if err != nil {
		return zero, err
	}
	value, err := result.value(describeMethod)
	if err != nil {
		return zero, err
	}
	if value.Protocol != BackendProtocol {
		return zero, errors.New("plugin: incompatible backend protocol")
	}
	return value, nil
}
