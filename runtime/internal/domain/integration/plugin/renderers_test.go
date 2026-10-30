package plugin

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

func TestRendererComponentAdmissionAndReleaseCopies(t *testing.T) {
	valid := ClientComponent{ID: "search", Title: "Search", Scope: "toolResult", Placement: "toolResultRenderer", Entry: Namespace + "/components/component.js", Export: "Result", Selector: &RendererSelector{Kind: extension.RendererResultShape, Shape: extension.ResultShapeSearch}}
	for _, test := range []struct {
		name   string
		change func(*ClientComponent)
	}{
		{"unknown selector", func(v *ClientComponent) { v.Selector.Kind = "execute" }},
		{"legacy shape name", func(v *ClientComponent) {
			v.Selector = &RendererSelector{Kind: extension.RendererResultShape, Name: "search"}
		}},
		{"shape with free name", func(v *ClientComponent) { v.Selector.Name = "search" }},
		{"named tool with shape", func(v *ClientComponent) {
			v.Selector = &RendererSelector{Kind: extension.RendererToolName, Name: "custom.scan", Shape: extension.ResultShapeSearch}
		}},
		{"unknown shape", func(v *ClientComponent) { v.Selector.Shape = "arbitrary" }},
		{"blank tool name", func(v *ClientComponent) { v.Selector = &RendererSelector{Kind: extension.RendererToolName, Name: " "} }},
		{"file selector", func(v *ClientComponent) {
			v.Selector = &RendererSelector{Kind: extension.RendererFileExtension, Name: "md"}
		}},
		{"wrong scope", func(v *ClientComponent) { v.Scope = "workspace" }},
		{"missing selector", func(v *ClientComponent) { v.Selector = nil }},
		{"escaped entry", func(v *ClientComponent) { v.Entry = "../component.js" }},
		{"HTML entry", func(v *ClientComponent) { v.Entry = Namespace + "/result.html" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := valid
			selector := *valid.Selector
			invalid.Selector = &selector
			test.change(&invalid)
			if _, err := NewRelease(identity.DigestStrings(test.name), Declaration{Name: "renderer", Components: []ClientComponent{invalid}}); err == nil {
				t.Fatal("invalid renderer component admitted")
			}
		})
	}
	release, err := NewRelease(identity.DigestStrings("renderer"), Declaration{Name: "renderer", Components: []ClientComponent{valid}})
	if err != nil {
		t.Fatal(err)
	}
	valid.Selector.Shape = "changed"
	projected := release.Declaration()
	if projected.Components[0].Selector.Shape != extension.ResultShapeSearch {
		t.Fatal("author changed admitted selector")
	}
	projected.Components[0].Selector.Shape = "changed"
	if release.Declaration().Components[0].Selector.Shape != extension.ResultShapeSearch {
		t.Fatal("projection changed admitted selector")
	}
}

func TestFileComponentReleaseCopies(t *testing.T) {
	selector := RendererSelector{Kind: extension.RendererFileExtension, Name: "md"}
	component := ClientComponent{ID: "markdown", Title: "Markdown", Scope: "file", Placement: "fileRenderer", Entry: Namespace + "/components/component.js", Export: "Markdown", Selector: &selector}
	release, err := NewRelease(identity.DigestStrings("file-component"), Declaration{Name: "file-component", Components: []ClientComponent{component}})
	if err != nil {
		t.Fatal(err)
	}
	selector.Name = "changed"
	projected := release.Declaration()
	if projected.Components[0].Selector.Name != "md" {
		t.Fatal("author changed admitted selector")
	}
	projected.Components[0].Selector.Name = "changed"
	if release.Declaration().Components[0].Selector.Name != "md" {
		t.Fatal("projection changed admitted selector")
	}
}

func TestRendererAdmissionUsesTheExtensionResultVocabulary(t *testing.T) {
	for _, word := range extension.ResultShape("").Values() {
		shape := extension.ResultShape(word)
		component := ClientComponent{ID: "result", Title: "Result", Scope: extension.ComponentScopeToolResult, Placement: extension.ComponentPlacementToolResultRenderer, Entry: Namespace + "/result.js", Export: "Result", Selector: &RendererSelector{Kind: extension.RendererResultShape, Shape: shape}}
		release, err := NewRelease(identity.DigestStrings(word), Declaration{Name: "renderer", Components: []ClientComponent{component}})
		if err != nil {
			t.Fatalf("owner shape %q refused: %v", word, err)
		}
		if release.Declaration().Components[0].Selector.Shape != shape {
			t.Fatal("result shape changed at admission")
		}
	}
	for _, word := range []string{"custom.scan_results", "project.custom-tool"} {
		component := ClientComponent{ID: "result", Title: "Result", Scope: extension.ComponentScopeToolResult, Placement: extension.ComponentPlacementToolResultRenderer, Entry: Namespace + "/result.js", Export: "Result", Selector: &RendererSelector{Kind: extension.RendererToolName, Name: word}}
		if _, err := NewRelease(identity.DigestStrings(word), Declaration{Name: "renderer", Components: []ClientComponent{component}}); err != nil {
			t.Fatalf("free tool name %q refused: %v", word, err)
		}
	}
}
