package bootstrap

import (
	"path/filepath"
	"slices"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestPluginCompositionCommitsWithItsOriginalAcknowledgement(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "guardrails"))
	contract, err := pluginsdk.ModelInputContract()
	if err != nil {
		t.Fatal(err)
	}
	const (
		key                = "configure-original-composition"
		listCompositions   = delivery.Name("plugins.listCompositions")
		resolveComposition = delivery.Name("plugins.resolveComposition")
	)
	request := protocol.SetPluginCompositionRequest{
		Contract: contract.Identity().String(),
		Members:  []protocol.PluginCompositionMember{{InstallationID: installed.ID, Contribution: "guardrails-input"}},
	}
	allowReceipt := refuseCommandReceipt(t, cfg.stores.DataDirectory)
	failed := r.endpoint.Invoke(t.Context(), delivery.PluginsSetComposition, request, protocol.CallMetadata{
		IdempotencyKey: key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version},
	})
	allowReceipt()
	if failed.Failure == nil {
		t.Fatal("set composition succeeded with a refused receipt")
	}

	r = reopenPluginRuntime(t, r, &cfg)
	page := r.must(listCompositions, struct{}{}, "").(*protocol.Page[protocol.PluginComposition])
	if len(page.Data) != 0 {
		t.Fatalf("%d compositions escaped the acknowledgement transaction", len(page.Data))
	}
	first := r.must(delivery.PluginsSetComposition, request, key).(*protocol.PluginComposition)
	resolution := protocol.ResolvePluginCompositionRequest{Contract: request.Contract}
	bindings := r.must(resolveComposition, resolution, "").(*protocol.Page[protocol.PluginOperationBinding])
	if len(bindings.Data) != 1 || bindings.Data[0].InstallationID != installed.ID || bindings.Data[0].Digest != installed.Selected.Digest {
		t.Fatal("composition did not resolve the exact installed release")
	}
	clear := protocol.SetPluginCompositionRequest{Contract: request.Contract, Members: []protocol.PluginCompositionMember{}}
	r.must(delivery.PluginsSetComposition, clear, "clear-after-original-composition")
	replayed := r.must(delivery.PluginsSetComposition, request, key).(*protocol.PluginComposition)
	if first.Contract != request.Contract || replayed.Contract != first.Contract ||
		!slices.Equal(first.Members, request.Members) || !slices.Equal(replayed.Members, first.Members) {
		t.Fatalf("original composition members = %+v, replayed = %+v", first.Members, replayed.Members)
	}
	page = r.must(listCompositions, struct{}{}, "").(*protocol.Page[protocol.PluginComposition])
	if len(page.Data) != 1 || page.Data[0].Contract != request.Contract || len(page.Data[0].Members) != 0 {
		t.Fatal("replay changed the later cleared composition")
	}
	if bindings := r.must(resolveComposition, resolution, "").(*protocol.Page[protocol.PluginOperationBinding]); len(bindings.Data) != 0 {
		t.Fatal("replay restored the original executable composition")
	}
}
