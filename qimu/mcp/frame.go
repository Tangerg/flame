package mcp

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"math"
	"strings"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/go-sdk/jsonrpc"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

// ProcessRequestFrameBytes bounds a NewProcessClient frame without opening a session.
// The SDK's int64 request counter and, for streams, any valid callback grant
// are included, so callers can reduce input before allocating or dispatching it.
func ProcessRequestFrameBytes(ctx context.Context, contract *portable.Capability, export, method, id string, input jsonvalue.Value, metadata sdk.ParamsBase) (int, error) {
	if contract == nil || !portable.ValidExportName(export) {
		return 0, errors.New("plugin: frame requires an exact capability export")
	}
	operation, found := contract.Method(method)
	if !found {
		return 0, errors.New("plugin: frame method is not in its capability")
	}
	cloned := sdk.Meta{}
	for key, value := range metadata.Meta {
		cloned[key] = append([]byte(nil), value...)
	}
	metadata.Meta = cloned
	version := ""
	for _, supported := range sdk.SupportedProtocolVersions() {
		if len(supported) > len(version) {
			version = supported
		}
	}
	for key, value := range map[string]any{sdk.MetaKeyProtocolVersion: version, sdk.MetaKeyClientInfo: processClientInfo(), sdk.MetaKeyClientCapabilities: processClientCapabilities()} {
		if _, present := metadata.Meta[key]; !present {
			if err := metadata.Set(key, value); err != nil {
				return 0, err
			}
		}
	}
	if len(operation.Descriptor().Stream) != 0 {
		grant := CallbackRef{Endpoint: "http://127.0.0.1:65535/", Credential: strings.Repeat("0", 64), Call: strings.Repeat("0", 64), MaxResponseBytes: math.MaxInt64 - 1}
		if err := metadata.Set(StreamMetadataKey, grant); err != nil {
			return 0, err
		}
	}
	binding := &Binding{method: operation, export: export, capability: contract.Identity().Digest()}
	params, err := binding.invocationParams(ctx, id, input, metadata)
	if err != nil {
		return 0, err
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return 0, err
	}
	requestID, err := jsonrpc.MakeID(float64(math.MinInt64))
	if err != nil {
		return 0, err
	}
	frame, err := jsonrpc.EncodeMessage(&jsonrpc.Request{ID: requestID, Method: InvokeMethod, Params: encoded})
	return len(frame) + 1, err
}

// NewProcessClient fixes the SDK profile used by installed process connections
// and their request-capacity admission. Caller-specific clients use RegisterClient.
func NewProcessClient() (*sdk.Client, error) {
	client := sdk.NewClient(processClientInfo(), &sdk.ClientOptions{Capabilities: processClientCapabilities()})
	if err := RegisterClient(client); err != nil {
		return nil, err
	}
	return client, nil
}
func processClientInfo() *sdk.Implementation { return &sdk.Implementation{Name: "qimu", Version: "1"} }
func processClientCapabilities() *sdk.ClientCapabilities {
	caps := &sdk.ClientCapabilities{RootsV2: &sdk.RootCapabilities{ListChanged: true}}
	caps.Roots.ListChanged = true
	return caps
}
