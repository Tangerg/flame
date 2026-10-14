package bootstrap

import (
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestInstalledReviewsPointUsesCapturedCollaboratorsAndCurrentConsumerLifetime(t *testing.T) {
	r := poMCPOpen(t, poMCPConfig(t))
	activate := func(source, key string) *protocol.PluginInstallation {
		t.Helper()
		installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-"+key).(*protocol.PluginInstallation)
		return r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-"+key).(*protocol.PluginInstallation)
	}
	wait := func(installed *protocol.PluginInstallation, name string) protocol.MCPServerID {
		t.Helper()
		id := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: name}
		deadline := time.Now().Add(10 * time.Second)
		for {
			page := r.must(delivery.MCPServersList, struct{}{}, "").(*protocol.Page[protocol.MCPServer])
			for _, server := range page.Data {
				if server.ID == id && server.Status.Type == protocol.MCPServerConnected {
					return id
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("installed author never connected: %s, %+v", id.Name, page.Data)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	analyzerSource := reviewAuthorPackage(t, filepath.Join("analyzer", "package"), "review-analyzer")
	analyzer := activate(analyzerSource, "analyzer")
	wait(analyzer, "analyzer")
	consumerSource := reviewExamplePackage(t)
	consumer := activate(consumerSource, "consumer")
	consumerServer := wait(consumer, "reviews")
	contribution := consumer.Selected.Contributions[0]
	request := protocol.CallPluginRequest{InstallationID: consumer.ID, Digest: consumer.Selected.Digest, Contribution: contribution.ID, Contract: contribution.Contract, Method: "io.github.tangerg.flame.examples/reviews.analyzers", Input: `{}`, Target: capability.Target{Scope: "installation", Reference: consumer.ID}}
	type analysis struct {
		Review struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Status   string `json:"status"`
			Revision int64  `json:"revision"`
		} `json:"review"`
		Findings []struct {
			Code     string `json:"code"`
			Message  string `json:"message"`
			ReviewID string `json:"reviewId"`
			Revision int64  `json:"revision"`
		} `json:"findings"`
	}
	read := func(want int) analysis {
		t.Helper()
		reply := r.must(delivery.PluginsCall, request, "").(*protocol.CallPluginResponse)
		if reply.Failure != nil {
			t.Fatalf("installed analyzer refused: %+v", reply.Failure)
		}
		var value analysis
		if err := json.Unmarshal([]byte(reply.Output), &value, json.RejectUnknownMembers(true)); err != nil {
			t.Fatal(err)
		}
		if value.Review.ID != "example-review" || value.Review.Revision != 1 || value.Review.Status != "open" || len(value.Findings) != want {
			t.Fatalf("installed analysis lost its inspected owner: %+v", value)
		}
		for _, finding := range value.Findings {
			if finding.Code != "unresolvedReview" || finding.ReviewID != value.Review.ID || finding.Revision != value.Review.Revision {
				t.Fatalf("finding differs from inspected evidence: %+v", finding)
			}
		}
		return value
	}
	read(1)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: analyzer.ID}, "disable-analyzer")
	read(0)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: analyzer.ID, Enabled: true}, "reenable-analyzer")
	wait(analyzer, "analyzer")
	read(1)
	// New point values cannot grant authority absent from the original setup.
	second := activate(analyzerSource, "second-analyzer")
	wait(second, "analyzer")
	result := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
	if result.Failure == nil {
		t.Fatal("current point values bypassed the consumer's frozen collaborators")
	}
	r.must(delivery.MCPServersReconnect, protocol.MCPServerRequest{Server: consumerServer}, "refresh-consumer")
	r.poMCPTools(&consumerServer, func(tools []protocol.MCPTool) bool { return len(tools) == 2 })
	read(2)
	other := activate(consumerSource, "other-consumer")
	wait(other, "reviews")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: consumer.ID}, "disable-consumer")
	point := protocol.ResolvePluginPointRequest{Contract: consumer.Selected.Points[0].Contract, Target: request.Target, Attributes: `{"reviewStatus":"open"}`, Order: []protocol.PluginPointReference{}}
	result = r.endpoint.Invoke(t.Context(), delivery.PluginsResolvePoint, point, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
	if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrPluginUnavailable) {
		t.Fatalf("disabled consumer borrowed another installation's declaration: %v", result.Failure)
	}
	point.Target.Reference = other.ID
	values := r.must(delivery.PluginsResolvePoint, point, "").(*protocol.Page[protocol.PluginPointBinding])
	if len(values.Data) != 2 {
		t.Fatalf("current consumer lost independent point contributors: %+v", values.Data)
	}
}
