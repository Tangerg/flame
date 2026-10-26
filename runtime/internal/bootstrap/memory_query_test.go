package bootstrap

import (
	json "encoding/json/v2"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func memoryQuery(r *poMCPRuntime, input memorycontract.ListV1Input) (protocol.CallPluginRequest, error) {
	providers := r.must(delivery.PluginsListProviders, struct{}{}, "").(*protocol.Page[protocol.PluginProviderSelection])
	installations := r.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation])
	contracts, err := extension.ParseCapability([]byte(memorycontract.CapabilityJSON))
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
			return protocol.CallPluginRequest{InstallationID: installation.ID, Digest: installation.Selected.Digest, Contribution: provider.Contribution, Contract: string(contracts.Identity().JSON()), Method: memorycontract.ListV1Method, Input: string(encoded), Target: extension.Target{Scope: "installation", Reference: installation.ID}}, nil
		}
	}
	return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
}
func memoryQueryResult(r *poMCPRuntime, input memorycontract.ListV1Input) delivery.Result {
	request, err := memoryQuery(r, input)
	if err != nil {
		return delivery.Result{Failure: protocol.NewFailure(err, err.Error())}
	}
	return r.endpoint.Invoke(r.t.Context(), delivery.PluginsCall, request, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
}
func readMemoryPage(r *poMCPRuntime, input memorycontract.ListV1Input) *protocol.Page[memorycontract.UpdateV1Output] {
	r.t.Helper()
	result := memoryQueryResult(r, input)
	if result.Failure != nil {
		r.t.Fatal(result.Failure)
	}
	reply := result.Value.(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var page memorycontract.ListV1Output
	if err := json.Unmarshal([]byte(reply.Output), &page); err != nil {
		r.t.Fatal(err)
	}
	values := make([]memorycontract.UpdateV1Output, len(page.Rows))
	for index, row := range page.Rows {
		values[index] = memorycontract.UpdateV1Output(row)
	}
	return protocol.NewPageWithCursor(values, page.NextCursor)
}

func memoryAddRequest(r *poMCPRuntime, input memorycontract.AddV1Input) protocol.CallPluginRequest {
	r.t.Helper()
	request, err := memoryQuery(r, memorycontract.ListV1Input{})
	if err != nil {
		r.t.Fatal(err)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		r.t.Fatal(err)
	}
	request.Method, request.Input = memorycontract.AddV1Method, string(encoded)
	return request
}
func mustMemoryAdd(r *poMCPRuntime, request protocol.CallPluginRequest, key string) *memorycontract.UpdateV1Output {
	r.t.Helper()
	reply := r.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var result memorycontract.AddV1Output
	if err := json.Unmarshal([]byte(reply.Output), &result); err != nil {
		r.t.Fatal(err)
	}
	item := memorycontract.UpdateV1Output(result.Item)
	return &item
}
