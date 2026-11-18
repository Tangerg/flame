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
