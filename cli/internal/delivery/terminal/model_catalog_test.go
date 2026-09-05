package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/flame/cli/internal/runtimefixture"
)

type partialModelCatalog struct {
	Runtime
}

func (p partialModelCatalog) ListModels(ctx context.Context) ([]protocol.Model, error) {
	models, err := p.Runtime.ListModels(ctx)
	return models, errors.Join(err, errors.New("offline: discovery unavailable"))
}

func TestModelCatalogDisplaysPartialResultsAndDiscoveryErrors(t *testing.T) {
	for _, command := range []string{"/models", "/model"} {
		t.Run(command, func(t *testing.T) {
			host, stop := runUIWithRuntimeServices(t, Config{Runtime: partialModelCatalog{Runtime: runtimefixture.New()}})
			t.Cleanup(stop)
			host.Shows(t, "Ask flame")
			host.Type(command)
			host.Press(input.Enter)
			host.Shows(t, "Models")
			host.Shows(t, "mock/balanced")
			host.Shows(t, "offline: discovery unavailable")
		})
	}
}

func TestModelCatalogDocumentConsumesCompleteModelMetadata(t *testing.T) {
	t.Parallel()
	contextWindow, maxInput, maxOutput := int64(200_000), int64(180_000), int64(20_000)
	limits := &protocol.ModelTokenLimits{
		ContextWindow: &contextWindow, MaxInputTokens: &maxInput, MaxOutputTokens: &maxOutput,
	}

	document := modelCatalogDocument([]protocol.Model{{
		ID: "reasoner", Provider: "provider", DisplayName: "Reasoner", Deprecated: true,
		TokenLimits:     limits,
		KnowledgeCutoff: "2026-01-31",
		Capabilities: &protocol.ModelCapabilities{
			Reasoning: true, ReasoningLevels: []string{"low", "high"}, ReasoningDefaultLevel: "high",
			Multimodal: true, InputModalities: []protocol.Modality{protocol.ModalityText, protocol.ModalityImage},
			OutputModalities: []protocol.Modality{protocol.ModalityText}, ToolUse: true, StructuredOutput: true,
		},
		Pricing: &protocol.ModelPricing{
			InputUSDPerMillionTokens: 0.2, OutputUSDPerMillionTokens: 0.8,
			CacheReadUSDPerMillionTokens: 0.02, CacheWriteUSDPerMillionTokens: 0.1,
		},
	}})
	if document.Title != "Models" || document.Detail != "1 available" || len(document.Sections) != 1 {
		t.Fatalf("document = %+v", document)
	}
	output := document.Sections[0].Title + "\n" + document.Sections[0].Text
	for _, want := range []string{
		"provider/reasoner · Reasoner · deprecated",
		"context 200,000 · input 180,000 · output 20,000",
		"knowledge     through 2026-01-31",
		"reasoning [low, high] default high · multimodal · tool use · structured output",
		"input         text, image",
		"output        text",
		"input $0.2/M · output $0.8/M · cache read $0.02/M · cache write $0.1/M",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("model document omitted %q:\n%s", want, output)
		}
	}
}

func TestModelsCommandOpensTheRuntimeCatalog(t *testing.T) {
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: runtimefixture.New()})
	host.Shows(t, "Ask flame")
	host.Type("/models")
	host.Press(input.Enter)
	host.Shows(t, "Models")
	host.Shows(t, "mock/balanced · Mock Balanced")
	host.Shows(t, "reasoning [low, medium, high] default medium")
	host.Shows(t, "input         text, image")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	stop()
}
