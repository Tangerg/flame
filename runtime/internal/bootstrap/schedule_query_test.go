package bootstrap

import (
	json "encoding/json/v2"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func scheduleToolsReady(tools []protocol.MCPTool) bool {
	return slices.ContainsFunc(tools, func(tool protocol.MCPTool) bool { return tool.Name == "list_schedules" })
}

func scheduleQuery(r *poMCPRuntime, query protocol.PageQuery) protocol.CallPluginRequest {
	r.t.Helper()
	providers := r.must(delivery.PluginsListProviders, struct{}{}, "").(*protocol.Page[protocol.PluginProviderSelection])
	installations := r.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation])
	contracts, err := extension.ParseCapability([]byte(testsupport.SchedulesCapabilityJSON))
	if err != nil {
		r.t.Fatal(err)
	}
	for _, provider := range providers.Data {
		if provider.Role != "schedules" {
			continue
		}
		for _, installation := range installations.Data {
			if installation.ID != provider.InstallationID {
				continue
			}
			input, err := json.Marshal(testsupport.SchedulesListV1Input{Cursor: query.Cursor, Limit: schedulePageLimit(query.Limit)})
			if err != nil {
				r.t.Fatal(err)
			}
			return protocol.CallPluginRequest{InstallationID: installation.ID, Digest: installation.Selected.Digest, Contribution: provider.Contribution, Contract: string(contracts.Identity().JSON()), Method: testsupport.SchedulesListV1Method, Target: extension.Target{Scope: "installation", Reference: installation.ID}, Input: string(input)}
		}
	}
	r.t.Fatal("selected schedule contribution unavailable")
	return protocol.CallPluginRequest{}
}

func readSchedulePage(r *poMCPRuntime, query protocol.PageQuery) *protocol.Page[testsupport.SchedulesCreateV1Output] {
	r.t.Helper()
	reply := r.must(delivery.PluginsCall, scheduleQuery(r, query), "").(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var page testsupport.SchedulesListV1Output
	if err := json.Unmarshal([]byte(reply.Output), &page); err != nil {
		r.t.Fatal(err)
	}
	values := make([]testsupport.SchedulesCreateV1Output, 0, len(page.Rows))
	for _, row := range page.Rows {
		out := testsupport.SchedulesCreateV1Output(row)
		values = append(values, out)
	}
	return protocol.NewPageWithCursor(values, page.NextCursor)
}

func scheduleRunRequest(r *poMCPRuntime, id string) protocol.CallPluginRequest {
	r.t.Helper()
	request := scheduleQuery(r, protocol.PageQuery{})
	input, err := json.Marshal(testsupport.SchedulesRunNowV1Input{ID: id})
	if err != nil {
		r.t.Fatal(err)
	}
	request.Method, request.Input = testsupport.SchedulesRunNowV1Method, string(input)
	return request
}

func readScheduleRun(r *poMCPRuntime, request protocol.CallPluginRequest, key string) *testsupport.SchedulesRunNowV1Output {
	r.t.Helper()
	reply := r.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var value testsupport.SchedulesRunNowV1Output
	if err := json.Unmarshal([]byte(reply.Output), &value, json.RejectUnknownMembers(true)); err != nil {
		r.t.Fatal(err)
	}
	return &value
}

func scheduleCreateRequest(r *poMCPRuntime, draft testsupport.SchedulesCreateV1Input) protocol.CallPluginRequest {
	r.t.Helper()
	request := scheduleQuery(r, protocol.PageQuery{})
	input, err := json.Marshal(draft)
	if err != nil {
		r.t.Fatal(err)
	}
	request.Method, request.Input = testsupport.SchedulesCreateV1Method, string(input)
	return request
}
func readScheduleCreate(r *poMCPRuntime, request protocol.CallPluginRequest, key string) *testsupport.SchedulesCreateV1Output {
	r.t.Helper()
	reply := r.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var snapshot testsupport.SchedulesCreateV1Output
	if err := json.Unmarshal([]byte(reply.Output), &snapshot); err != nil {
		r.t.Fatal(err)
	}
	return &snapshot
}
func scheduleUpdateRequest(r *poMCPRuntime, input testsupport.SchedulesUpdateV1Input) protocol.CallPluginRequest {
	r.t.Helper()
	request := scheduleQuery(r, protocol.PageQuery{})
	encoded, err := json.Marshal(input)
	if err != nil {
		r.t.Fatal(err)
	}
	request.Method, request.Input = testsupport.SchedulesUpdateV1Method, string(encoded)
	return request
}

func schedulePageLimit(limit *int) *int64 {
	if limit == nil {
		return nil
	}
	return new(int64(*limit))
}
