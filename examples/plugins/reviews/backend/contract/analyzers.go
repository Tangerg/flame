package reviews

import (
	"errors"

	"unicode/utf8"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/scope/core/jsonschema"
)

type Analysis struct {
	Review   Snapshot  `json:"review"`
	Findings []Finding `json:"findings"`
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
	Capability *portable.Capability
	Analyze    *qimumcp.Method[Snapshot, Findings]
}

func NewAnalyzerContracts() (AnalyzerContracts, error) {
	method, err := qimumcp.NewMethod[Snapshot, Findings](qimumcp.MethodConfig{
		Name: "io.github.tangerg.flame.examples/reviews.analyze", Kind: portable.Query, Target: "installation",
		Description: "Analyze an inspected review snapshot and return findings without changing its records.",
	})
	if err != nil {
		return AnalyzerContracts{}, err
	}
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "io.github.tangerg.flame.examples/reviews.analyzer", Composition: portable.Collection, Scope: "installation"}, method.Descriptor())
	if err != nil {
		return AnalyzerContracts{}, err
	}
	return AnalyzerContracts{Capability: contract, Analyze: method}, nil
}

func NewAnalyzerPoint() (*portable.Point, error) {
	schema, err := jsonschema.For[Analyzer]()
	if err != nil {
		return nil, err
	}
	applicability, err := jsonvalue.Encode(map[string]any{
		"type": "object", "required": []string{"attributes"},
		"properties": map[string]any{"attributes": map[string]any{
			"type": "object", "required": []string{"reviewStatus"},
			"properties": map[string]any{"reviewStatus": map[string]any{"const": string(Open)}},
		}},
	})
	if err != nil {
		return nil, err
	}
	return portable.AdmitPoint(portable.PointDescriptor{Protocol: portable.Protocol, Reference: "io.github.tangerg.flame.examples/reviews.analyzers", Composition: portable.Collection, Scope: "installation", Optional: true, Ordering: portable.CanonicalKeys, Value: schema.JSON(), Applicability: applicability.JSON()})
}
