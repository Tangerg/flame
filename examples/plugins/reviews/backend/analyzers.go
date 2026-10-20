package reviews

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	runtimeclient "github.com/Tangerg/flame/packages/runtime-client/go"
	"github.com/Tangerg/flame/runtime/protocol"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/point"
	"github.com/Tangerg/scope/core/jsonschema"
)

type Analysis struct {
	Review   Snapshot  `json:"review"`
	Findings []Finding `json:"findings"`
}

type AnalyzerCaller interface {
	runtimeclient.PointResolver
	runtimeclient.PluginCaller
}

func AnalyzeReview(ctx context.Context, caller AnalyzerCaller, target capability.Target, review Snapshot) (Analysis, error) {
	contract, err := NewAnalyzerPoint()
	if err != nil {
		return Analysis{}, err
	}
	values, err := runtimeclient.ResolvePointValues[Analyzer](ctx, caller, contract, target, map[string]any{"reviewStatus": review.Status}, nil, protocol.CallOptions{})
	if err != nil {
		return Analysis{}, err
	}
	contracts, err := NewAnalyzerContracts()
	if err != nil {
		return Analysis{}, err
	}
	findings := make([]Finding, 0)
	for _, value := range values {
		method, err := runtimeclient.BindPluginMethod(caller, runtimeclient.PluginBinding{
			InstallationID: value.InstallationID, Digest: value.Digest,
			Contribution: value.Value.Contribution, Contract: contracts.Capability,
			Target: capability.Target{Scope: "installation", Reference: value.InstallationID},
		}, contracts.Analyze)
		if err != nil {
			return Analysis{}, err
		}
		result, err := method.Call(ctx, review, protocol.PluginCallOptions{})
		if err != nil {
			return Analysis{}, fmt.Errorf("reviews: analyzer %s: %w", value.Value.Title, err)
		}
		for _, finding := range result.Findings {
			if err := finding.Validate(review); err != nil {
				return Analysis{}, err
			}
		}
		findings = append(findings, result.Findings...)
	}
	return Analysis{Review: review, Findings: findings}, nil
}

// Analyzer names a callable contribution in the same installed release as the
// point value. Reviews interprets this reference; the loader has no business enum.
type Analyzer struct {
	Title        string `json:"title"`
	Contribution string `json:"contribution"`
}

type Finding struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	ReviewID string `json:"reviewId"`
	Revision int64  `json:"revision"`
}

func (f Finding) Validate(review Snapshot) error {
	if f.ReviewID != review.ID || f.Revision != review.Revision || f.Code == "" || len(f.Code) > 128 || f.Message == "" || len(f.Message) > 4096 || !utf8.ValidString(f.Code) || !utf8.ValidString(f.Message) {
		return errors.New("reviews: analyzer finding differs from the inspected review or exceeds its bounds")
	}
	return nil
}

type Findings struct {
	Findings []Finding `json:"findings"`
}

type AnalyzerContracts struct {
	Capability *capability.Contract
	Analyze    *pluginsdk.Method[Snapshot, Findings]
}

func NewAnalyzerContracts() (AnalyzerContracts, error) {
	method, err := pluginsdk.NewMethod[Snapshot, Findings](pluginsdk.MethodConfig{
		Name: "io.github.tangerg.flame.examples/reviews.analyze", Kind: capability.Query, Target: "installation",
		Description: "Analyze an inspected review snapshot and return findings without changing its records.",
	})
	if err != nil {
		return AnalyzerContracts{}, err
	}
	contract, err := capability.New(capability.Config{Reference: "io.github.tangerg.flame.examples/reviews.analyzer", Composition: capability.Collection, Scope: "installation"}, method.Descriptor())
	if err != nil {
		return AnalyzerContracts{}, err
	}
	return AnalyzerContracts{Capability: contract, Analyze: method}, nil
}

func NewAnalyzerPoint() (*point.Contract, error) {
	schema, err := jsonschema.For[Analyzer]()
	if err != nil {
		return nil, err
	}
	applicability, err := jsoninput.Encode(map[string]any{
		"type": "object", "required": []string{"attributes"},
		"properties": map[string]any{"attributes": map[string]any{
			"type": "object", "required": []string{"reviewStatus"},
			"properties": map[string]any{"reviewStatus": map[string]any{"const": string(Open)}},
		}},
	})
	if err != nil {
		return nil, err
	}
	return point.Admit(point.Definition{Protocol: capability.Protocol, Reference: "io.github.tangerg.flame.examples/reviews.analyzers", Composition: capability.Collection, Scope: "installation", Optional: true, Ordering: point.CanonicalKeys, Value: schema.JSON(), Applicability: applicability.JSON()})
}
