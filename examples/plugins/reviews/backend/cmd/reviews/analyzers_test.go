package main

import (
	"context"
	"strings"
	"testing"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type observedAnalyzerCaller struct {
	value   protocol.PluginPointBinding
	finding reviews.Finding
	calls   []protocol.CallPluginRequest
}

func (c *observedAnalyzerCaller) ResolvePluginPoint(_ context.Context, _ protocol.ResolvePluginPointRequest, _ protocol.CallOptions) (*protocol.Page[protocol.PluginPointBinding], error) {
	return &protocol.Page[protocol.PluginPointBinding]{Data: []protocol.PluginPointBinding{c.value}}, nil
}

func (c *observedAnalyzerCaller) CallPlugin(_ context.Context, request protocol.CallPluginRequest, _ protocol.PluginCallOptions) (*protocol.CallPluginResponse, error) {
	c.calls = append(c.calls, request)
	output, err := jsonvalue.Encode(reviews.Findings{Findings: []reviews.Finding{c.finding}})
	if err != nil {
		return nil, err
	}
	return &protocol.CallPluginResponse{Output: string(output.JSON())}, nil
}

func TestAnalysisUsesTheResolvedReleaseAndRejectsUnrelatedFindingEvidence(t *testing.T) {
	const provider = "00000000-0000-4000-8000-000000000001"
	const consumer = "00000000-0000-4000-8000-000000000002"
	review := reviews.Snapshot{ID: reviews.ExampleReviewID, Title: "Review evidence", Status: reviews.Open, Revision: 3}
	value, err := jsonvalue.Encode(reviews.Analyzer{Title: "Evidence", Contribution: "analyzer"})
	if err != nil {
		t.Fatal(err)
	}
	caller := &observedAnalyzerCaller{value: protocol.PluginPointBinding{InstallationID: provider, Digest: strings.Repeat("a", 64), Contribution: "evidence", Value: string(value.JSON())}, finding: reviews.Finding{Code: "open", Message: "Inspect this evidence.", ReviewID: review.ID, Revision: review.Revision}}
	result, err := analyzeReview(t.Context(), caller, extension.Target{Scope: "installation", Reference: consumer}, review)
	if err != nil || result.Review != review || len(result.Findings) != 1 || len(caller.calls) != 1 {
		t.Fatalf("analysis: %+v, calls=%d, %v", result, len(caller.calls), err)
	}
	request := caller.calls[0]
	if request.InstallationID != provider || request.Digest != caller.value.Digest || request.Contribution != "analyzer" || request.Target.Reference != provider {
		t.Fatalf("analysis retargeted its captured point value: %+v", request)
	}
	caller.finding.Revision++
	if _, err := analyzeReview(t.Context(), caller, extension.Target{Scope: "installation", Reference: consumer}, review); err == nil {
		t.Fatal("accepted a finding about a different inspected revision")
	}
}
