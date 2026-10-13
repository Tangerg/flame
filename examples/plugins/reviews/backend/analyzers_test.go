package reviews

import (
	"context"
	"strings"
	"testing"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
)

type analyzerCaller struct {
	value   protocol.PluginPointBinding
	finding Finding
	calls   []protocol.CallPluginRequest
}

func (c *analyzerCaller) ResolvePluginPoint(_ context.Context, _ protocol.ResolvePluginPointRequest, _ flameruntime.CallOptions) (*protocol.Page[protocol.PluginPointBinding], error) {
	return &protocol.Page[protocol.PluginPointBinding]{Data: []protocol.PluginPointBinding{c.value}}, nil
}

func (c *analyzerCaller) CallPlugin(_ context.Context, request protocol.CallPluginRequest, _ flameruntime.PluginCallOptions) (*protocol.CallPluginResponse, error) {
	c.calls = append(c.calls, request)
	output, err := jsoninput.Encode(Findings{Findings: []Finding{c.finding}})
	if err != nil {
		return nil, err
	}
	return &protocol.CallPluginResponse{Output: output.JSON()}, nil
}

func TestAnalysisUsesTheResolvedReleaseAndRejectsUnrelatedFindingEvidence(t *testing.T) {
	const provider = "00000000-0000-4000-8000-000000000001"
	const consumer = "00000000-0000-4000-8000-000000000002"
	review := Snapshot{ID: ExampleReviewID, Title: "Review evidence", Status: Open, Revision: 3}
	value, err := jsoninput.Encode(Analyzer{Title: "Evidence", Contribution: "analyzer"})
	if err != nil {
		t.Fatal(err)
	}
	caller := &analyzerCaller{value: protocol.PluginPointBinding{InstallationID: provider, Digest: strings.Repeat("a", 64), Contribution: "evidence", Value: value.JSON()}, finding: Finding{Code: "open", Message: "Inspect this evidence.", ReviewID: review.ID, Revision: review.Revision}}
	result, err := AnalyzeReview(t.Context(), caller, capability.Target{Scope: "installation", Reference: consumer}, review)
	if err != nil || result.Review != review || len(result.Findings) != 1 || len(caller.calls) != 1 {
		t.Fatalf("analysis: %+v, calls=%d, %v", result, len(caller.calls), err)
	}
	request := caller.calls[0]
	if request.InstallationID != provider || request.Digest != caller.value.Digest || request.Contribution != "analyzer" || request.Target.Reference != provider {
		t.Fatalf("analysis retargeted its captured point value: %+v", request)
	}
	caller.finding.Revision++
	if _, err := AnalyzeReview(t.Context(), caller, capability.Target{Scope: "installation", Reference: consumer}, review); err == nil {
		t.Fatal("accepted a finding about a different inspected revision")
	}
}
