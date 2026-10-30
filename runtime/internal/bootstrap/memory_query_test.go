package bootstrap

import (
	json "encoding/json/v2"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func memoryQuery(r *poMCPRuntime, input memorycontract.ListInput) (protocol.CallPluginRequest, error) {
	providers := r.must(delivery.PluginsListProviders, struct{}{}, "").(*protocol.Page[protocol.PluginProviderSelection])
	installations := r.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation])
	contracts, err := portable.ParseCapability([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	for _, provider := range providers.Data {
		if provider.Role != "memory" {
			continue
		}
		for _, installation := range installations.Data {
			if installation.ID != provider.InstallationID {
				continue
			}
			if !installation.Enabled {
				return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
			}
			return protocol.CallPluginRequest{InstallationID: installation.ID, Digest: installation.Selected.Digest, Contribution: provider.Contribution, Contract: string(contracts.Identity().JSON()), Method: memorycontract.ListV2Method, Input: string(encoded), Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installation.ID}}, nil
		}
	}
	return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
}
func memoryQueryResult(r *poMCPRuntime, input memorycontract.ListInput) delivery.Result {
	request, err := memoryQuery(r, input)
	if err != nil {
		return delivery.Result{Failure: protocol.NewFailure(err, err.Error())}
	}
	return r.endpoint.Invoke(r.t.Context(), delivery.PluginsCall, request, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
}
func readMemoryPage(r *poMCPRuntime, input memorycontract.ListInput) *protocol.Page[memorycontract.ItemSnapshot] {
	r.t.Helper()
	result := memoryQueryResult(r, input)
	if result.Failure != nil {
		r.t.Fatal(result.Failure)
	}
	reply := result.Value.(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var page memorycontract.Page
	if err := json.Unmarshal([]byte(reply.Output), &page); err != nil {
		r.t.Fatal(err)
	}
	values := make([]memorycontract.ItemSnapshot, len(page.Rows))
	for index, row := range page.Rows {
		values[index] = memorycontract.ItemSnapshot(row)
	}
	return protocol.NewPageWithCursor(values, page.NextCursor)
}

func memoryAddRequest(r *poMCPRuntime, input memorycontract.AddInput) protocol.CallPluginRequest {
	r.t.Helper()
	request, err := memoryQuery(r, memorycontract.ListInput{})
	if err != nil {
		r.t.Fatal(err)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		r.t.Fatal(err)
	}
	request.Method, request.Input = memorycontract.AddV2Method, string(encoded)
	return request
}
func mustMemoryAdd(r *poMCPRuntime, request protocol.CallPluginRequest, key string) *memorycontract.ItemSnapshot {
	r.t.Helper()
	reply := r.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var result memorycontract.AddResult
	if err := json.Unmarshal([]byte(reply.Output), &result); err != nil {
		r.t.Fatal(err)
	}
	item := memorycontract.ItemSnapshot(result.Item)
	return &item
}

func memoryToolsReady(tools []protocol.MCPTool) bool {
	return slices.ContainsFunc(tools, func(value protocol.MCPTool) bool { return value.Name == memorycontract.SearchToolName })
}
