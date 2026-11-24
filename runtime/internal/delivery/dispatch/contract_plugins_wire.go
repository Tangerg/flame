package dispatch

import (
	"reflect"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framevalue "github.com/Tangerg/frame/value"
)

func registerPluginTerminalConditions(s *Shapes) {
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.PluginTerminalBinding](),
		Rules: []ConditionalRule{
			{When: whenEquals("source", string(extension.TerminalBindingValue)), Required: []string{"value"}, Forbidden: []string{"field", "context", "form"}},
			{When: whenEquals("source", string(extension.TerminalBindingRow)), Required: []string{"field"}, Forbidden: []string{"value", "context", "form"}},
			{When: whenEquals("source", string(extension.TerminalBindingContext)), Required: []string{"context"}, Forbidden: []string{"value", "field", "form"}},
			{When: whenEquals("source", string(extension.TerminalBindingForm)), Required: []string{"form"}, Forbidden: []string{"value", "field", "context"}},
		},
	})
	s.constraint(ObjectConstraintSpec{
		GoType: typeOf[protocol.PluginTerminalFormField](),
		Rules: []ConditionalRule{{
			When:     whenEquals("kind", string(extension.TerminalFormChoice)),
			Required: []string{"options"},
		}},
	})
}

func registerPluginUnions(s *Shapes) {
	s.union(UnionSpec{GoType: typeOf[protocol.PluginRendererSelector](), Variants: []VariantSpec{
		{Tag: string(extension.RendererToolName), Required: []string{"name"}},
		{Tag: string(extension.RendererResultShape), Required: []string{"shape"}},
		{Tag: string(extension.RendererFileExtension), Required: []string{"name"}},
	}})
	s.union(UnionSpec{GoType: typeOf[protocol.PluginValueChange](), Variants: []VariantSpec{{Tag: string(protocol.PluginValueSet), Required: []string{"value"}}, {Tag: string(protocol.PluginValueClear)}}})
	s.union(UnionSpec{GoType: typeOf[protocol.PluginRealization](), Variants: []VariantSpec{
		{Tag: string(protocol.PluginRealizationAvailable), Optional: []string{"unavailableBackends"}},
		{Tag: string(protocol.PluginRealizationReleaseUnavailable)},
	}})
	s.union(UnionSpec{GoType: typeOf[protocol.PluginServerDeclaration](), Variants: []VariantSpec{
		{Tag: string(protocol.PluginServerStdio), Required: []string{"name", "command"}, Optional: []string{"args", "env", "dir"}},
		{Tag: string(protocol.PluginServerHTTP), Required: []string{"name", "url"}, Optional: []string{"headers"}},
		{Tag: string(protocol.PluginServerNode), Required: []string{"name", "entry"}},
	}})
	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.PluginInput](), Rules: []ConditionalRule{
		{When: whenEquals("target", string(protocol.PluginInputEnvironment)), Required: []string{"key"}},
		{When: whenEquals("target", string(protocol.PluginInputHeader)), Required: []string{"key"}},
		{When: whenEquals("target", string(protocol.PluginInputAuthorization)), Forbidden: []string{"key"}},
	}})
	s.union(UnionSpec{GoType: typeOf[protocol.PluginInputState](), Variants: []VariantSpec{
		{Tag: string(protocol.PluginInputUnset)},
		{Tag: string(protocol.PluginInputConfigured)},
		{Tag: string(protocol.PluginInputValue), Required: []string{"value"}},
	}})
	s.union(UnionSpec{GoType: typeOf[protocol.PluginComponent](), Variants: []VariantSpec{
		{Tag: string(protocol.PluginComponentManifestField), Required: []string{"name"}},
		{Tag: string(protocol.PluginComponentFlameExtension)},
		{Tag: string(protocol.PluginComponentExtensionField), Required: []string{"name"}},
		{Tag: string(protocol.PluginComponentContribution), Required: []string{"name"}},
		{Tag: string(protocol.PluginComponentMCP)},
		{Tag: string(protocol.PluginComponentMCPServer), Required: []string{"name"}},
		{Tag: string(protocol.PluginComponentSkills)},
		{Tag: string(protocol.PluginComponentSkill), Required: []string{"name"}},
	}})
}

func registerPluginValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.OpenPluginBrowserRealmRequest](), Constraints: []FieldConstraint{
		{Field: "operationId", Kind: ConstraintNonEmpty}, {Field: "resumeToken", Kind: ConstraintNonEmpty},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.AcquirePluginComponentRequest](), Constraints: []FieldConstraint{
		{Field: "operationId", Kind: ConstraintNonEmpty}, {Field: "componentId", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginBrowserSession](), Constraints: []FieldConstraint{
		{Field: "credential", Kind: ConstraintNonEmpty}, {Field: "controlEndpoint", Kind: ConstraintNonEmpty},
		{Field: "maximumMessageBytes", Kind: ConstraintPositive}, {Field: "proofCapacity", Kind: ConstraintPositive}, {Field: "preparationCapacity", Kind: ConstraintPositive},
		{Field: "pollIntervalMilliseconds", Kind: ConstraintPositive},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginClientComponent](), Constraints: []FieldConstraint{
		{Field: "id", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
		{Field: "contribution", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
		{Field: "material", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
		{Field: "settingsIntent", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginTerminalView](), Constraints: []FieldConstraint{
		{Field: "id", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
		{Field: "contribution", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
		{Field: "title", Kind: ConstraintNonEmpty},
		{Field: "title", Kind: ConstraintMaxLength, Limit: plugin.MaxPresentationTitleBytes},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginTerminalAction](), Constraints: []FieldConstraint{
		{Field: "id", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
		{Field: "title", Kind: ConstraintNonEmpty},
		{Field: "title", Kind: ConstraintMaxLength, Limit: plugin.MaxPresentationTitleBytes},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.ReadPluginResourceRequest](), Constraints: []FieldConstraint{{Field: "componentId", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern}}})
	declared := func(field string) FieldConstraint {
		return FieldConstraint{Field: field, Kind: ConstraintPattern, Value: identity.DeclarationIDPattern}
	}
	nonEmpty := func(field string) FieldConstraint { return FieldConstraint{Field: field, Kind: ConstraintNonEmpty} }
	installation := FieldConstraint{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()}
	digest := FieldConstraint{Field: "digest", Kind: ConstraintPattern, Value: framevalue.DigestPattern()}
	localeTag := []FieldConstraint{nonEmpty("tag"), {Field: "tag", Kind: ConstraintMaxLength, Limit: plugin.MaxLocaleTagBytes}}
	title := []FieldConstraint{nonEmpty("title"), {Field: "title", Kind: ConstraintMaxLength, Limit: plugin.MaxPresentationTitleBytes}}
	for _, spec := range []FieldConstraintSpec{
		{GoType: typeOf[protocol.ReadPluginLocaleRequest](), Constraints: localeTag},
		{GoType: typeOf[protocol.PluginLocale](), Constraints: slices.Concat(localeTag, title)},
		{GoType: typeOf[protocol.PluginPoint](), Constraints: []FieldConstraint{nonEmpty("contract"), declared("id")}},
		{GoType: typeOf[protocol.PluginPointValue](), Constraints: []FieldConstraint{nonEmpty("contract"), declared("id"), nonEmpty("value")}},
		{GoType: typeOf[protocol.ResolvePluginPointRequest](), Constraints: []FieldConstraint{nonEmpty("contract"), nonEmpty("attributes")}},
		{GoType: typeOf[protocol.PluginPointReference](), Constraints: []FieldConstraint{installation, declared("contribution")}},
		{GoType: typeOf[protocol.PluginPointBinding](), Constraints: []FieldConstraint{installation, declared("contribution"), digest, nonEmpty("value")}},
		{GoType: typeOf[protocol.ResolvePluginRequirementsRequest](), Constraints: []FieldConstraint{installation, digest, declared("contribution")}},
		{GoType: typeOf[protocol.PluginOperationBinding](), Constraints: []FieldConstraint{installation, digest, declared("contribution"), nonEmpty("contract")}},
		{GoType: typeOf[protocol.PluginChangeNotice](), Constraints: []FieldConstraint{declared("contribution")}},
		{GoType: typeOf[protocol.PluginRequirement](), Constraints: []FieldConstraint{declared("id"), nonEmpty("contract")}},
		{GoType: typeOf[protocol.PluginCollaborator](), Constraints: []FieldConstraint{declared("id")}},
	} {
		s.valueConstraint(spec)
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.CallPluginRequest](), Constraints: []FieldConstraint{
		{Field: "contract", Kind: ConstraintNonEmpty},
		{Field: "input", Kind: ConstraintNonEmpty},
		{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()},
		{Field: "digest", Kind: ConstraintPattern, Value: framevalue.DigestPattern()},
		{Field: "contribution", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
		{Field: "method", Kind: ConstraintNonEmpty},
		{Field: "method", Kind: ConstraintMaxLength, Limit: plugin.MaximumCommandTextCharacters},
		{Field: "method", Kind: ConstraintIdentity},
		{Field: "target.scope", Kind: ConstraintNonEmpty},
		{Field: "target.scope", Kind: ConstraintIdentity},
		{Field: "target.scope", Kind: ConstraintMaxLength, Limit: identity.MaximumResourceCharacters},
		{Field: "target.reference", Kind: ConstraintNonEmpty},
		{Field: "target.reference", Kind: ConstraintPattern, Value: `^[^\p{Cc}]*$`},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginCallFailure](), Constraints: []FieldConstraint{
		{Field: "code", Kind: ConstraintNonEmpty},
		{Field: "code", Kind: ConstraintMaxLength, Limit: protocol.MaximumPluginFailureCodeCharacters},
		{Field: "message", Kind: ConstraintNonEmpty},
	}})
	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.CallPluginResponse](), Rules: []ConditionalRule{
		{RequiredAny: []string{"output", "failure"}},
		{When: whenPresent("failure"), Forbidden: []string{"output"}},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.CallPluginResponse](), Constraints: []FieldConstraint{{Field: "output", Kind: ConstraintNonEmpty}}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginStreamEvent](), Constraints: []FieldConstraint{{Field: "value", Kind: ConstraintNonEmpty}}})
	s.constraint(ObjectConstraintSpec{GoType: typeOf[protocol.PluginStreamEvent](), Rules: []ConditionalRule{
		{When: whenEquals("kind", string(protocol.PluginStreamChunk)), Required: []string{"value"}, Forbidden: []string{"failure"}},
		{When: whenEquals("kind", string(protocol.PluginStreamResult)), Required: []string{"value"}, Forbidden: []string{"failure"}},
		{When: whenEquals("kind", string(protocol.PluginStreamFailure)), Required: []string{"failure"}, Forbidden: []string{"value"}},
	}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginContribution](), Constraints: append(mcpServerName("server"), FieldConstraint{Field: "id", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern}, FieldConstraint{Field: "export", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern}, FieldConstraint{Field: "contract", Kind: ConstraintNonEmpty})})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginRendererSelector](), Constraints: []FieldConstraint{{Field: "name", Kind: ConstraintNonEmpty}, {Field: "name", Kind: ConstraintMaxLength, Limit: plugin.MaxRendererSelectorNameBytes}}})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.PluginThemeColors](),
		Constraints: []FieldConstraint{
			{Field: "background", Kind: ConstraintPattern, Value: plugin.ThemeColorPattern()},
			{Field: "foreground", Kind: ConstraintPattern, Value: plugin.ThemeColorPattern()},
			{Field: "accent", Kind: ConstraintPattern, Value: plugin.ThemeColorPattern()},
			{Field: "muted", Kind: ConstraintPattern, Value: plugin.ThemeColorPattern()},
			{Field: "border", Kind: ConstraintPattern, Value: plugin.ThemeColorPattern()},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.PluginRealization](),
		Constraints: []FieldConstraint{
			{Field: "unavailableBackends", Kind: ConstraintMinItems, Limit: 1},
			{Field: "unavailableBackends", Kind: ConstraintUniqueItems},
			{Field: "unavailableBackends", Kind: ConstraintPatternItems, Value: mcpserver.ServerNamePattern()},
		},
	})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginServerDeclaration](), Constraints: mcpServerName("name")})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginInput](), Constraints: mcpServerName("server")})
	for _, shape := range []reflect.Type{typeOf[protocol.SelectPluginProviderRequest](), typeOf[protocol.PluginProviderSelection]()} {
		constraints := []FieldConstraint{
			FieldConstraint{Field: "contribution", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
			FieldConstraint{Field: "role", Kind: ConstraintNonEmpty},
			FieldConstraint{Field: "role", Kind: ConstraintMaxLength, Limit: plugin.MaximumRoleCharacters},
			FieldConstraint{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()},
		}
		s.valueConstraint(FieldConstraintSpec{GoType: shape, Constraints: constraints})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginCompositionMember](), Constraints: []FieldConstraint{
		{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()},
		{Field: "contribution", Kind: ConstraintPattern, Value: identity.DeclarationIDPattern},
	}})
	for _, shape := range []reflect.Type{typeOf[protocol.PluginComposition](), typeOf[protocol.SetPluginCompositionRequest](), typeOf[protocol.ResolvePluginCompositionRequest]()} {
		constraints := []FieldConstraint{{Field: "contract", Kind: ConstraintNonEmpty}}
		if shape != typeOf[protocol.ResolvePluginCompositionRequest]() {
			constraints = append(constraints, FieldConstraint{Field: "members", Kind: ConstraintUniqueItems})
		}
		s.valueConstraint(FieldConstraintSpec{GoType: shape, Constraints: constraints})
	}
	for _, shape := range []reflect.Type{typeOf[protocol.StagePluginRequest](), typeOf[protocol.PluginRequest](), typeOf[protocol.PluginReleaseRequest](), typeOf[protocol.SetPluginEnablementRequest](), typeOf[protocol.ConfigurePluginRequest]()} {
		constraints := []FieldConstraint{{Field: "installationId", Kind: ConstraintPattern, Value: identity.InstallationIDPattern()}}
		switch shape {
		case typeOf[protocol.PluginReleaseRequest](), typeOf[protocol.ConfigurePluginRequest]():
			constraints = append(constraints, FieldConstraint{Field: "digest", Kind: ConstraintPattern, Value: framevalue.DigestPattern()})
		}
		switch shape {
		case typeOf[protocol.StagePluginRequest]():
			constraints = append(constraints, FieldConstraint{Field: "source", Kind: ConstraintNonEmpty}, FieldConstraint{Field: "source", Kind: ConstraintMaxLength, Limit: plugin.MaximumSourceCharacters})
		}
		s.valueConstraint(FieldConstraintSpec{GoType: shape, Constraints: constraints})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.InstallPluginRequest](), Constraints: []FieldConstraint{{Field: "source", Kind: ConstraintNonEmpty}, {Field: "source", Kind: ConstraintMaxLength, Limit: plugin.MaximumSourceCharacters}}})
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[protocol.PluginValueChange](), Constraints: []FieldConstraint{{Field: "value", Kind: ConstraintMaxLength, Limit: plugin.MaximumValueCharacters}}})
}
