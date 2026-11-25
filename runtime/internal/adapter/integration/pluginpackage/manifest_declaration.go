package pluginpackage

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framevalue "github.com/Tangerg/frame/value"
)

const (
	protocolMember        = "protocol"
	managedBackendsMember = "managedBackends"
	contributesMember     = "contributes"
)

// Each entry is the only reference to its member name, so the accepted member set is exactly the
// admitted set and a diagnostic names the member it admits.
type declarationMember struct {
	name  string
	admit func(raw jsontext.Value)
}

func contributionMember[T any](release *plugin.Builder, kind plugin.ComponentKind, name string, admit func(T) error) declarationMember {
	return declarationMember{name: name, admit: func(raw jsontext.Value) {
		admitContributions(raw, release, plugin.Component{Kind: kind, Name: name}, admit)
	}}
}

func parseExtensions(raw jsontext.Value, release *plugin.Builder, ctx context.Context, root *os.Root, nativeServers []string) error {
	resources := declarationResources{root: root}
	contributions := []declarationMember{
		contributionMember(release, plugin.ComponentContribution, "contributions", func(w wireContribution) error {
			value, err := resources.contribution(ctx, w)
			if err != nil {
				return err
			}
			return release.AdmitContribution(value)
		}),
		contributionMember(release, plugin.ComponentContribution, "points", func(w wirePoint) error {
			contract, err := resources.point(ctx, w.Contract)
			if err != nil {
				return err
			}
			return release.AdmitPoint(plugin.Point{ID: w.ID, Contract: contract.Identity()})
		}),
		contributionMember(release, plugin.ComponentContribution, "pointValues", func(w wirePointValue) error {
			contract, err := resources.point(ctx, w.Contract)
			if err != nil {
				return err
			}
			value, err := framevalue.ParseJSON(w.Value)
			if err != nil {
				return err
			}
			return release.AdmitPointValue(plugin.PointValue{ID: w.ID, Contract: contract.Identity(), Value: value})
		}),
		contributionMember(release, plugin.ComponentExtensionField, "inputs", func(w wireInput) error { return release.AdmitInput(w.domain()) }),
		contributionMember(release, plugin.ComponentContribution, "components", func(w wireComponent) error {
			component, err := resources.component(ctx, w)
			if err != nil {
				return err
			}
			return release.AdmitComponent(component)
		}),
		contributionMember(release, plugin.ComponentContribution, "terminalViews", func(w wireTerminalView) error {
			view, err := w.domain()
			if err != nil {
				return err
			}
			return release.AdmitTerminalView(view)
		}),
	}
	presentation := []declarationMember{
		contributionMember(release, plugin.ComponentContribution, "locales", func(w wireLocale) error {
			locale, err := resources.locale(ctx, w)
			if err != nil {
				return err
			}
			return release.AdmitLocale(locale)
		}),
		contributionMember(release, plugin.ComponentContribution, "themes", func(w wireTheme) error {
			theme, err := w.domain()
			if err != nil {
				return err
			}
			return release.AdmitTheme(theme)
		}),
	}
	members, found := loadDeclaration(ctx, raw, release, resources, contributions)
	if !found {
		return nil
	}
	if raw, found := members[managedBackendsMember]; found {
		if err := admitManagedBackends(raw, release, root, nativeServers); err != nil {
			return err
		}
	}
	contributes, ok := decodePresentation(members[contributesMember], presentation)
	if !ok {
		report(release, plugin.Component{Kind: plugin.ComponentFlameExtension}, plugin.DiagnosticInvalidDeclaration)
		return nil
	}
	for _, member := range contributions {
		member.admit(members[member.name])
	}
	for _, member := range presentation {
		member.admit(contributes[member.name])
	}
	return nil
}

func loadDeclaration(ctx context.Context, raw jsontext.Value, release *plugin.Builder, resources declarationResources, contributions []declarationMember) (map[string]jsontext.Value, bool) {
	var namespaces map[string]jsontext.Value
	if err := json.Unmarshal(raw, &namespaces); err != nil || namespaces == nil {
		report(release, plugin.Component{Kind: plugin.ComponentManifestField, Name: "extensions"}, plugin.DiagnosticInvalidDeclaration)
		return nil, false
	}
	raw, found := namespaces[plugin.Namespace]
	if !found {
		return nil, false
	}
	var reference struct {
		Declaration string `json:"declaration"`
	}
	var members map[string]jsontext.Value
	var requiredProtocol string
	err := json.Unmarshal(raw, &reference, json.RejectUnknownMembers(true))
	var body []byte
	if err == nil {
		body, err = resources.read(ctx, reference.Declaration)
	}
	if err == nil {
		err = json.Unmarshal(body, &members)
	}
	if err != nil || members == nil || json.Unmarshal(members[protocolMember], &requiredProtocol) != nil || requiredProtocol != plugincontract.Protocol || !onlyMembers(members, contributions, protocolMember, managedBackendsMember, contributesMember) {
		report(release, plugin.Component{Kind: plugin.ComponentFlameExtension}, plugin.DiagnosticInvalidDeclaration)
		return nil, false
	}
	return members, true
}

// An absent contributes member is valid; a present one must be an object of declared members.
func decodePresentation(raw jsontext.Value, presentation []declarationMember) (map[string]jsontext.Value, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	var contributes map[string]jsontext.Value
	if err := json.Unmarshal(raw, &contributes); err != nil || contributes == nil {
		return nil, false
	}
	return contributes, onlyMembers(contributes, presentation)
}

func onlyMembers(members map[string]jsontext.Value, declared []declarationMember, fixed ...string) bool {
	for name := range members {
		if !slices.Contains(fixed, name) && !slices.ContainsFunc(declared, func(member declarationMember) bool { return member.name == name }) {
			return false
		}
	}
	return true
}

type declarationResources struct{ root *os.Root }

func (r declarationResources) readWithin(ctx context.Context, resource string, limit int64) ([]byte, error) {
	if !plugin.ValidResourcePath(resource) || !strings.HasPrefix(resource, plugin.Namespace+"/") {
		return nil, plugin.ErrInvalid
	}
	return read(ctx, r.root, resource, limit)
}

func (r declarationResources) read(ctx context.Context, resource string) ([]byte, error) {
	return r.readWithin(ctx, resource, maxManifestBytes)
}

func (r declarationResources) component(ctx context.Context, w wireComponent) (plugin.ClientComponent, error) {
	var contribution plugin.ContributionID
	if w.Contribution != nil {
		if err := w.Contribution.Validate(); err != nil {
			return plugin.ClientComponent{}, err
		}
		contribution = *w.Contribution
	}
	material, err := presentText(w.Material)
	if err != nil {
		return plugin.ClientComponent{}, err
	}
	settingsIntent, err := presentText(w.SettingsIntent)
	if err != nil {
		return plugin.ClientComponent{}, err
	}
	if _, err := r.readWithin(ctx, w.Entry, maxPackageBytes); err != nil {
		return plugin.ClientComponent{}, err
	}
	var selector *plugin.RendererSelector
	if w.Selector != nil {
		selector = &plugin.RendererSelector{Kind: w.Selector.Type, Name: w.Selector.Name, Shape: w.Selector.Shape}
	}
	return plugin.ClientComponent{Selector: selector, ID: w.ID, Title: w.Title, LocalizedTitles: w.LocalizedTitles, ToolCategory: w.ToolCategory, TargetView: w.TargetView, Scope: w.Scope, Placement: w.Placement, Entry: w.Entry, Export: w.Export, Contribution: contribution, Material: material, SettingsIntent: settingsIntent}, nil
}

func (r declarationResources) locale(ctx context.Context, w wireLocale) (plugin.Locale, error) {
	body, err := r.read(ctx, w.Entry)
	if err != nil {
		return plugin.Locale{}, err
	}
	if _, err := decodeLocale(body); err != nil {
		return plugin.Locale{}, err
	}
	return plugin.Locale{Tag: w.Tag, Title: w.Title, Entry: w.Entry}, nil
}

func presentText(value *string) (string, error) {
	if value == nil {
		return "", nil
	}
	if *value == "" {
		return "", plugin.ErrInvalid
	}
	return *value, nil
}

func (r declarationResources) point(ctx context.Context, resource string) (*plugincontract.Point, error) {
	body, err := r.read(ctx, resource)
	if err != nil {
		return nil, err
	}
	return plugincontract.ParsePoint(body)
}

func (r declarationResources) capability(ctx context.Context, resource string) (*plugincontract.Capability, error) {
	body, err := r.read(ctx, resource)
	if err != nil {
		return nil, err
	}
	return plugincontract.ParseCapability(body)
}

func (r declarationResources) contribution(ctx context.Context, w wireContribution) (plugin.Contribution, error) {
	contract, err := r.capability(ctx, w.Contract)
	if err != nil {
		return plugin.Contribution{}, err
	}
	value := plugin.Contribution{Background: w.Background, ID: w.ID, Server: w.Server, Export: w.Export, Contract: contract.Identity()}
	if w.ModelProviders != "" {
		body, err := r.read(ctx, w.ModelProviders)
		if err != nil {
			return plugin.Contribution{}, err
		}
		if err := json.Unmarshal(body, &value.ModelProviders); err != nil {
			return plugin.Contribution{}, fmt.Errorf("read model adapter provider scopes: %w", err)
		}
	}
	for _, requirement := range w.Requires {
		dependency, err := r.capability(ctx, requirement.Contract)
		if err != nil {
			return plugin.Contribution{}, err
		}
		value.Requires = append(value.Requires, plugin.Requirement{ID: requirement.ID, Contract: dependency.Identity(), Optional: requirement.Optional})
	}
	return value, nil
}

func report(release *plugin.Builder, component plugin.Component, code plugin.DiagnosticCode) {
	release.Report(plugin.Diagnostic{Component: component, Code: code})
}

func admitContributions[T any](raw jsontext.Value, release *plugin.Builder, component plugin.Component, admit func(T) error) {
	if len(raw) == 0 {
		return
	}
	var values []jsontext.Value
	if err := json.Unmarshal(raw, &values); err != nil || values == nil {
		report(release, component, plugin.DiagnosticInvalidDeclaration)
		return
	}
	for _, raw := range values {
		var wire T
		if err := framevalue.DecodeValue(raw, &wire, "plugin declaration"); err != nil {
			report(release, component, plugin.DiagnosticInvalidDeclaration)
			continue
		}
		code, admitted := admission(admit(wire))
		if admitted {
			continue
		}
		report(release, component, code)
		if code == plugin.DiagnosticComponentLimit {
			return
		}
	}
}

func admission(err error) (plugin.DiagnosticCode, bool) {
	if err == nil {
		return "", true
	}
	if refusal, ok := errors.AsType[*plugin.Refusal](err); ok {
		return refusal.Code, false
	}
	return plugin.DiagnosticInvalidDeclaration, false
}
