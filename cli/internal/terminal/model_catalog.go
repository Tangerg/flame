package terminal

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/cli/internal/agent"
)

func (a *app) ShowModels() {
	a.executeRuntimeReaderQuery(a.modelsReaderQuery())
}

func (a *app) modelsReaderQuery() runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading model catalog", mode: runtimeReaderModels,
		read: func(ctx context.Context) (readerDocument, error) {
			models, err := a.runtime.ListModels(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return modelCatalogDocument(models), nil
		},
	}
}

func modelCatalogDocument(models []agent.Model) readerDocument {
	if len(models) == 0 {
		return paragraphDocument("Models", "none available", []string{"The runtime did not advertise any models."})
	}
	ordered := slices.Clone(models)
	slices.SortFunc(ordered, func(left, right agent.Model) int {
		if compared := strings.Compare(left.Provider, right.Provider); compared != 0 {
			return compared
		}
		return strings.Compare(left.ID, right.ID)
	})
	sections := make([]ToolSection, 0, len(ordered))
	for _, model := range ordered {
		title := model.Provider + "/" + model.ID
		if model.DisplayName != "" && model.DisplayName != model.ID {
			title += " · " + model.DisplayName
		}
		if model.Deprecated {
			title += " · deprecated"
		}
		sections = append(sections, ToolSection{
			Title: title, Style: toolSectionCode, Language: "text",
			Text: strings.Join(modelCatalogLines(model), "\n"),
		})
	}
	return readerDocument{Title: "Models", Detail: fmt.Sprintf("%d available", len(ordered)), Sections: sections}
}

func modelCatalogLines(model agent.Model) []string {
	lines := make([]string, 0, 7)
	if limits := modelTokenLimits(model); limits != "" {
		lines = append(lines, "tokens        "+limits)
	}
	if model.KnowledgeCutoff != "" {
		lines = append(lines, "knowledge     through "+model.KnowledgeCutoff)
	}
	if model.Capabilities == nil {
		lines = append(lines, "capabilities  not advertised")
	} else {
		lines = append(lines, modelCapabilityLines(*model.Capabilities)...)
	}
	if model.Pricing != nil {
		lines = append(lines, "pricing       "+modelPricingText(*model.Pricing))
	}
	if len(lines) == 0 {
		lines = append(lines, "No additional metadata was advertised.")
	}
	return lines
}

func modelTokenLimits(model agent.Model) string {
	var limits []string
	contextWindow, contextKnown := model.TokenLimits.ContextWindow()
	maxInput, inputKnown := model.TokenLimits.MaxInputTokens()
	maxOutput, outputKnown := model.TokenLimits.MaxOutputTokens()
	for _, limit := range []struct {
		name    string
		value   int64
		present bool
	}{
		{name: "context", value: contextWindow, present: contextKnown},
		{name: "input", value: maxInput, present: inputKnown},
		{name: "output", value: maxOutput, present: outputKnown},
	} {
		if limit.present {
			limits = append(limits, limit.name+" "+formatThousands(limit.value))
		}
	}
	return strings.Join(limits, " · ")
}

func modelCapabilityLines(capabilities agent.ModelCapabilities) []string {
	features := make([]string, 0, 4)
	if capabilities.Reasoning {
		reasoning := "reasoning"
		if len(capabilities.ReasoningLevels) > 0 {
			reasoning += " [" + strings.Join(capabilities.ReasoningLevels, ", ") + "]"
			if capabilities.ReasoningDefaultLevel != "" {
				reasoning += " default " + capabilities.ReasoningDefaultLevel
			}
		}
		features = append(features, reasoning)
	}
	if capabilities.Multimodal {
		features = append(features, "multimodal")
	}
	if capabilities.ToolUse {
		features = append(features, "tool use")
	}
	if capabilities.StructuredOutput {
		features = append(features, "structured output")
	}
	if len(features) == 0 {
		features = append(features, "none advertised")
	}
	lines := []string{"capabilities  " + strings.Join(features, " · ")}
	if len(capabilities.InputModalities) > 0 {
		lines = append(lines, "input         "+joinModalities(capabilities.InputModalities))
	}
	if len(capabilities.OutputModalities) > 0 {
		lines = append(lines, "output        "+joinModalities(capabilities.OutputModalities))
	}
	return lines
}

func joinModalities(modalities []agent.ModelModality) string {
	values := make([]string, len(modalities))
	for index, modality := range modalities {
		values[index] = string(modality)
	}
	return strings.Join(values, ", ")
}

func modelPricingText(pricing agent.ModelPricing) string {
	rates := []string{
		"input $" + formatModelRate(pricing.InputUSDPerMillionTokens) + "/M",
		"output $" + formatModelRate(pricing.OutputUSDPerMillionTokens) + "/M",
	}
	if pricing.CacheReadUSDPerMillionTokens > 0 {
		rates = append(rates, "cache read $"+formatModelRate(pricing.CacheReadUSDPerMillionTokens)+"/M")
	}
	if pricing.CacheWriteUSDPerMillionTokens > 0 {
		rates = append(rates, "cache write $"+formatModelRate(pricing.CacheWriteUSDPerMillionTokens)+"/M")
	}
	return strings.Join(rates, " · ")
}

func formatModelRate(rate float64) string {
	return strconv.FormatFloat(rate, 'f', -1, 64)
}
