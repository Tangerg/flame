package bootstrap

import (
	json "encoding/json/v2"
	"slices"
	"strconv"
	"testing"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"

	framevalue "github.com/Tangerg/frame/value"

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
	contracts, err := plugincontract.ParseCapability([]byte(testsupport.SchedulesCapabilityJSON))
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
			input, err := json.Marshal(testsupport.SchedulesListInput{Cursor: query.Cursor, Limit: schedulePageLimit(r.t, query.Limit)})
			if err != nil {
				r.t.Fatal(err)
			}
			return protocol.CallPluginRequest{InstallationID: installation.ID, Digest: installation.Selected.Digest, Contribution: provider.Contribution, Contract: string(contracts.Identity().Bytes()), Method: testsupport.SchedulesListV1Method, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installation.ID}, Input: string(input)}
		}
	}
	r.t.Fatal("selected schedule contribution unavailable")
	return protocol.CallPluginRequest{}
}

func readSchedulePage(r *poMCPRuntime, query protocol.PageQuery) *protocol.Page[*testsupport.SchedulesListV1OutputDomainSnapshot] {
	r.t.Helper()
	reply := r.must(delivery.PluginsCall, scheduleQuery(r, query), "").(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var page testsupport.SchedulesPage
	if err := json.Unmarshal([]byte(reply.Output), &page); err != nil {
		r.t.Fatal(err)
	}
	return protocol.NewPageWithCursor(page.Rows, page.NextCursor)
}

func scheduleRunRequest(r *poMCPRuntime, id string) protocol.CallPluginRequest {
	r.t.Helper()
	request := scheduleQuery(r, protocol.PageQuery{})
	input, err := json.Marshal(testsupport.SchedulesRunNowV1InputWireItemInput{ID: id})
	if err != nil {
		r.t.Fatal(err)
	}
	request.Method, request.Input = testsupport.SchedulesRunNowV1Method, string(input)
	return request
}

func readScheduleRun(r *poMCPRuntime, request protocol.CallPluginRequest, key string) *testsupport.SchedulesRunReference {
	r.t.Helper()
	reply := r.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var value testsupport.SchedulesRunReference
	if err := json.Unmarshal([]byte(reply.Output), &value, json.RejectUnknownMembers(true)); err != nil {
		r.t.Fatal(err)
	}
	return &value
}

func scheduleCreateRequest(r *poMCPRuntime, draft testsupport.SchedulesDraft) protocol.CallPluginRequest {
	r.t.Helper()
	request := scheduleQuery(r, protocol.PageQuery{})
	if draft.ModelSelection == nil {
		draft.ModelSelection = &testsupport.SchedulesCreateV1InputModelSelection{}
	}
	input, err := json.Marshal(draft)
	if err != nil {
		r.t.Fatal(err)
	}
	request.Method, request.Input = testsupport.SchedulesCreateV1Method, string(input)
	return request
}
func readScheduleCreate(r *poMCPRuntime, request protocol.CallPluginRequest, key string) *testsupport.SchedulesCreateV1OutputDomainSnapshot {
	r.t.Helper()
	reply := r.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var snapshot testsupport.SchedulesCreateV1OutputDomainSnapshot
	if err := json.Unmarshal([]byte(reply.Output), &snapshot); err != nil {
		r.t.Fatal(err)
	}
	return &snapshot
}
func scheduleUpdateRequest(r *poMCPRuntime, input testsupport.SchedulesUpdateInput) protocol.CallPluginRequest {
	r.t.Helper()
	request := scheduleQuery(r, protocol.PageQuery{})
	if input.ModelSelection == nil {
		input.ModelSelection = &testsupport.SchedulesModelPatch{}
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		r.t.Fatal(err)
	}
	request.Method, request.Input = testsupport.SchedulesUpdateV1Method, string(encoded)
	return request
}

func schedulePageLimit(t testing.TB, limit *int) framevalue.Number {
	if limit == nil {
		return framevalue.Number{}
	}
	return generatedNumber(t, strconv.Itoa(*limit))
}
