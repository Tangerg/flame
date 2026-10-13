package bootstrap

import (
	json "encoding/json/v2"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	memoryrpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	"github.com/Tangerg/flame/runtime/protocol"
)

func memoryQuery(r *poMCPRuntime, input memoryrpc.ListInput) (protocol.CallPluginRequest, error) {
	providers := r.must(delivery.PluginsListProviders, struct{}{}, "").(*protocol.Page[protocol.PluginProviderSelection])
	installations := r.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation])
	contracts, err := memoryrpc.NewContracts()
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
			return protocol.CallPluginRequest{InstallationID: installation.ID, Digest: installation.Selected.Digest, Contribution: provider.Contribution, Contract: string(contracts.Capability.Identity().JSON()), Method: memoryrpc.List, Input: encoded, Target: capability.Target{Scope: "installation", Reference: installation.ID}}, nil
		}
	}
	return protocol.CallPluginRequest{}, protocol.ErrPluginUnavailable
}
func memoryQueryResult(r *poMCPRuntime, input memoryrpc.ListInput) delivery.Result {
	request, err := memoryQuery(r, input)
	if err != nil {
		return delivery.Result{Failure: delivery.NewFailure(err, err.Error())}
	}
	return r.endpoint.Invoke(r.t.Context(), delivery.PluginsCall, request, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
}
func readMemoryPage(r *poMCPRuntime, input memoryrpc.ListInput) *protocol.Page[protocol.AgentMemoryItem] {
	r.t.Helper()
	result := memoryQueryResult(r, input)
	if result.Failure != nil {
		r.t.Fatal(result.Failure)
	}
	reply := result.Value.(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var page memoryrpc.Page
	if err := json.Unmarshal(reply.Output, &page); err != nil {
		r.t.Fatal(err)
	}
	rows := make([]protocol.AgentMemoryItem, 0, len(page.Rows))
	for _, row := range page.Rows {
		rows = append(rows, protocol.AgentMemoryItem{ID: row.ID.String(), Scope: protocol.AgentMemoryScope(row.Scope), Content: row.Content, Origin: protocol.AgentMemoryOrigin(row.Origin), Status: protocol.AgentMemoryStatus(row.Status), Pinned: row.Pinned, SessionID: row.SessionID, Day: row.Day, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt})
	}
	return protocol.NewPageWithCursor(rows, page.NextCursor)
}

func memoryAddRequest(r *poMCPRuntime, input memoryrpc.AddInput) protocol.CallPluginRequest {
	r.t.Helper()
	request, err := memoryQuery(r, memoryrpc.ListInput{})
	if err != nil {
		r.t.Fatal(err)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		r.t.Fatal(err)
	}
	request.Method, request.Input = memoryrpc.Add, encoded
	return request
}
func mustMemoryAdd(r *poMCPRuntime, request protocol.CallPluginRequest, key string) *protocol.AgentMemoryItem {
	r.t.Helper()
	reply := r.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var result memoryrpc.AddResult
	if err := json.Unmarshal(reply.Output, &result); err != nil {
		r.t.Fatal(err)
	}
	row := result.Item
	return &protocol.AgentMemoryItem{ID: row.ID.String(), Scope: protocol.AgentMemoryScope(row.Scope), Content: row.Content, Origin: protocol.AgentMemoryOrigin(row.Origin), Status: protocol.AgentMemoryStatus(row.Status), Pinned: row.Pinned, SessionID: row.SessionID, Day: row.Day, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
