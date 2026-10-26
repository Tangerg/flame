package plugin

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/fingerprint"
)

func TestRendererComponentAdmissionAndReleaseCopies(t *testing.T) {
	valid := ClientComponent{ID: "search", Title: "Search", Scope: "toolResult", Placement: "toolResultRenderer", Entry: Namespace + "/components/component.js", Export: "Result", Selector: &RendererSelector{Kind: RendererResultShape, Name: "search"}}
	for _, test := range []struct {
		name   string
		change func(*ClientComponent)
	}{
		{"unknown selector", func(v *ClientComponent) { v.Selector.Kind = "execute" }},
		{"unknown shape", func(v *ClientComponent) { v.Selector.Name = "arbitrary" }},
		{"blank tool name", func(v *ClientComponent) { v.Selector = &RendererSelector{Kind: RendererToolName, Name: " "} }},
		{"file selector", func(v *ClientComponent) { v.Selector = &RendererSelector{Kind: RendererFileExtension, Name: "md"} }},
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
			if _, err := NewRelease(fingerprint.Strings(test.name), Declaration{Name: "renderer", Components: []ClientComponent{invalid}}); err == nil {
				t.Fatal("invalid renderer component admitted")
			}
		})
	}
	release, err := NewRelease(fingerprint.Strings("renderer"), Declaration{Name: "renderer", Components: []ClientComponent{valid}})
	if err != nil {
		t.Fatal(err)
	}
	valid.Selector.Name = "changed"
	projected := release.Declaration()
	if projected.Components[0].Selector.Name != "search" {
		t.Fatal("author changed admitted selector")
	}
	projected.Components[0].Selector.Name = "changed"
	if release.Declaration().Components[0].Selector.Name != "search" {
		t.Fatal("projection changed admitted selector")
	}
}

func TestFileComponentReleaseCopies(t *testing.T) {
	selector := RendererSelector{Kind: RendererFileExtension, Name: "md"}
	component := ClientComponent{ID: "markdown", Title: "Markdown", Scope: "file", Placement: "fileRenderer", Entry: Namespace + "/components/component.js", Export: "Markdown", Selector: &selector}
	release, err := NewRelease(fingerprint.Strings("file-component"), Declaration{Name: "file-component", Components: []ClientComponent{component}})
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
