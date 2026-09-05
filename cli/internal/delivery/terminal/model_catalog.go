package terminal

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/runtime/protocol"
)

func (a *app) ShowModels() {
	a.executeRuntimeReaderQuery(a.modelsReaderQuery())
}

func (a *app) modelsReaderQuery() runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading model catalog", mode: runtimeReaderModels,
		read: func(ctx context.Context) (readerDocument, error) {
			models, err := a.runtime.ListModels(ctx)
			if err != nil && len(models) == 0 {
				return readerDocument{}, err
			}
			document := modelCatalogDocument(models)
			if err != nil {
				document.Detail += " · incomplete catalog"
				document.Sections = append([]ToolSection{{
					Title: "Provider discovery failed", Style: toolSectionCode, Language: "text", Text: err.Error(),
				}}, document.Sections...)
			}
			return document, nil
		},
	}
}

func modelCatalogDocument(models []protocol.Model) readerDocument {
	if len(models) == 0 {
		return paragraphDocument("Models", "none available", []string{"The runtime did not advertise any models."})
	}
	sections := make([]ToolSection, 0, len(models))
	for _, model := range models {
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
	return readerDocument{Title: "Models", Detail: fmt.Sprintf("%d available", len(models)), Sections: sections}
}

func modelCatalogLines(model protocol.Model) []string {
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

func modelTokenLimits(model protocol.Model) string {
	if model.TokenLimits == nil {
		return ""
	}
	var limits []string
	for _, limit := range []struct {
		name  string
		value *int64
	}{
		{name: "context", value: model.TokenLimits.ContextWindow},
		{name: "input", value: model.TokenLimits.MaxInputTokens},
		{name: "output", value: model.TokenLimits.MaxOutputTokens},
	} {
		if limit.value != nil {
			limits = append(limits, limit.name+" "+formatThousands(*limit.value))
		}
	}
	return strings.Join(limits, " · ")
}

func modelCapabilityLines(capabilities protocol.ModelCapabilities) []string {
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

func joinModalities(modalities []protocol.Modality) string {
	values := make([]string, len(modalities))
	for index, modality := range modalities {
		values[index] = string(modality)
	}
	return strings.Join(values, ", ")
}

func modelPricingText(pricing protocol.ModelPricing) string {
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
