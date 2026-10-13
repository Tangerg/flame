package bootstrap

import (
	json "encoding/json/v2"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	scheduleapp "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	scheduledomain "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	schedulerpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	"github.com/Tangerg/flame/runtime/protocol"
)

func scheduleQuery(r *poMCPRuntime, query protocol.PageQuery) protocol.CallPluginRequest {
	r.t.Helper()
	providers := r.must(delivery.PluginsListProviders, struct{}{}, "").(*protocol.Page[protocol.PluginProviderSelection])
	installations := r.must(delivery.PluginsList, struct{}{}, "").(*protocol.Page[protocol.PluginInstallation])
	contracts, err := schedulerpc.NewContracts()
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
			input, err := json.Marshal(schedulerpc.ListInput{Cursor: query.Cursor, Limit: query.Limit})
			if err != nil {
				r.t.Fatal(err)
			}
			return protocol.CallPluginRequest{InstallationID: installation.ID, Digest: installation.Selected.Digest, Contribution: provider.Contribution, Contract: string(contracts.Capability.Identity().JSON()), Method: schedulerpc.List, Target: capability.Target{Scope: "installation", Reference: installation.ID}, Input: input}
		}
	}
	r.t.Fatal("selected schedule contribution unavailable")
	return protocol.CallPluginRequest{}
}

func readSchedulePage(r *poMCPRuntime, query protocol.PageQuery) *protocol.Page[protocol.Schedule] {
	r.t.Helper()
	reply := r.must(delivery.PluginsCall, scheduleQuery(r, query), "").(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var page schedulerpc.Page
	if err := json.Unmarshal(reply.Output, &page); err != nil {
		r.t.Fatal(err)
	}
	values := make([]protocol.Schedule, 0, len(page.Rows))
	for _, row := range page.Rows {
		value, err := scheduledomain.Restore(row)
		if err != nil {
			r.t.Fatal(err)
		}
		out := protocol.Schedule{ID: value.ID(), Title: value.Title(), Instructions: value.Instructions(), Cron: value.Cron(), Revision: value.Revision(), CreatedAt: value.CreatedAt(), Enabled: value.Enabled(), Provider: value.ModelSelection().Provider(), Model: value.ModelSelection().Model(), ReasoningEffort: value.ModelSelection().ReasoningEffort()}
		if value.CWD() != "" {
			out.Workspace = &protocol.WorkspaceRef{Path: value.CWD()}
		}
		if !value.NextRunAt().IsZero() {
			out.NextRunAt = new(value.NextRunAt())
		}
		if !value.LastRunAt().IsZero() {
			out.LastRunAt = new(value.LastRunAt())
		}
		values = append(values, out)
	}
	return protocol.NewPageWithCursor(values, page.NextCursor)
}

func scheduleRunRequest(r *poMCPRuntime, id string) protocol.CallPluginRequest {
	r.t.Helper()
	request := scheduleQuery(r, protocol.PageQuery{})
	input, err := json.Marshal(schedulerpc.ItemInput{ID: id})
	if err != nil {
		r.t.Fatal(err)
	}
	request.Method, request.Input = schedulerpc.RunNow, input
	return request
}

func readScheduleRun(r *poMCPRuntime, request protocol.CallPluginRequest, key string) *scheduleapp.RunReference {
	r.t.Helper()
	reply := r.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse)
	if reply.Failure != nil {
		r.t.Fatal(reply.Failure)
	}
	var value scheduleapp.RunReference
	if err := json.Unmarshal(reply.Output, &value, json.RejectUnknownMembers(true)); err != nil {
		r.t.Fatal(err)
	}
	return &value
}
