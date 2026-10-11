package plugin

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

func TestRendererAdmissionAndReleaseCopies(t *testing.T) {
	valid := Renderer{ID: "search", Title: "Search result", Selector: RendererSelector{Kind: RendererResultShape, Name: "search"}, Entry: "renderers/search.html"}
	for _, test := range []struct {
		name   string
		change func(*Renderer)
	}{
		{"unknown selector", func(r *Renderer) { r.Selector.Kind = "execute" }},
		{"unknown shape", func(r *Renderer) { r.Selector.Name = "arbitrary" }},
		{"empty tool name", func(r *Renderer) { r.Selector = RendererSelector{Kind: RendererToolName} }},
		{"escaped entry", func(r *Renderer) { r.Entry = "../search.html" }},
		{"script entry", func(r *Renderer) { r.Entry = "search.js" }},
		{"long title", func(r *Renderer) { r.Title = strings.Repeat("x", MaxViewTitleBytes+1) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, err := NewBuilder("renderer-test", "1", "")
			if err != nil {
				t.Fatal(err)
			}
			invalid := valid
			test.change(&invalid)
			if err := b.AdmitRenderer(invalid); err == nil {
				t.Fatal("invalid renderer admitted")
			}
			if err := b.AdmitRenderer(valid); err != nil {
				t.Fatal(err)
			}
		})
	}
	release, err := NewRelease(fingerprint.Strings("renderer"), Declaration{Name: "renderer-test", Renderers: []Renderer{valid}})
	if err != nil {
		t.Fatal(err)
	}
	copy := release.Declaration()
	copy.Renderers[0].Entry = "replacement.html"
	if release.Declaration().Renderers[0] != valid {
		t.Fatal("projection changed the admitted release")
	}
	duplicate := Declaration{Name: "renderer-test", Renderers: []Renderer{valid, valid}}
	if _, err := NewRelease(release.Digest(), duplicate); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate: %v", err)
	}
}
