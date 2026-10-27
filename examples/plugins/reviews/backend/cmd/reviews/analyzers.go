package main

import (
	"context"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"

	"fmt"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/portable"
)

type analyzerCaller interface {
	runtimeclient.PointResolver
	runtimeclient.PluginCaller
}

func analyzeReview(ctx context.Context, caller analyzerCaller, target portable.Target, review reviews.Snapshot) (reviews.Analysis, error) {
	contract, err := reviews.NewAnalyzerPoint()
	if err != nil {
		return reviews.Analysis{}, err
	}
	values, err := runtimeclient.ResolvePointValues[reviews.Analyzer](ctx, caller, contract, target, map[string]any{"reviewStatus": review.Status}, nil, protocol.CallOptions{})
	if err != nil {
		return reviews.Analysis{}, err
	}
	contracts, err := reviews.NewAnalyzerContracts()
	if err != nil {
		return reviews.Analysis{}, err
	}
	findings := make([]reviews.Finding, 0)
	for _, value := range values {
		method, err := runtimeclient.BindPluginMethod(caller, runtimeclient.PluginBinding{
			InstallationID: value.InstallationID, Digest: value.Digest,
			Contribution: value.Value.Contribution, Contract: contracts.Capability,
			Target: portable.Target{Scope: "installation", Reference: value.InstallationID},
		}, contracts.Analyze)
		if err != nil {
			return reviews.Analysis{}, err
		}
		result, err := method.Call(ctx, review, protocol.PluginCallOptions{})
		if err != nil {
			return reviews.Analysis{}, fmt.Errorf("reviews: analyzer %s: %w", value.Value.Title, err)
		}
		for _, finding := range result.Findings {
			if err := finding.Validate(review); err != nil {
				return reviews.Analysis{}, err
			}
		}
		findings = append(findings, result.Findings...)
	}
	return reviews.Analysis{Review: review, Findings: findings}, nil
}
