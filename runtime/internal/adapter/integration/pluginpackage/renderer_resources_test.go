package pluginpackage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRetiredHTMLRendererIsRefused(t *testing.T) {
	release, err := publishPackage(t.Context(), testReleases(t), writePackage(t, map[string]string{
		"plugin.json": `{"$schema":"` + manifestSchema + `","name":"test.retired","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
		"io.github.tangerg.flame/declaration.json": flameDeclaration(`"contributes":{"renderers":[{"id":"old","title":"Old","selector":{"type":"toolName","name":"read"},"entry":"old.html"}]}`),
		"old.html": "<!doctype html><p>Old</p>",
	}))
	if err != nil {
		t.Fatal(err)
	}
	declaration := release.Declaration()
	if len(declaration.Components) != 0 || len(declaration.Diagnostics) != 1 || declaration.Diagnostics[0].Code != plugin.DiagnosticInvalidDeclaration {
		t.Fatalf("retired renderer admitted: %+v", declaration)
	}
}

func TestRendererComponentResourceRejectsChangedBytesAndFilesystemEscape(t *testing.T) {
	for _, attack := range []string{"rewrite", "symlink", "missing"} {
		t.Run(attack, func(t *testing.T) {
			r := testReleases(t)
			release, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{
				"plugin.json": `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"test.renderers","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
				"io.github.tangerg.flame/declaration.json":        flameDeclaration(`"components":[{"id":"view","title":"View","scope":"toolResult","placement":"toolResultRenderer","selector":{"type":"toolName","name":"read"},"entry":"io.github.tangerg.flame/components/component.js","export":"Result"}]`),
				"io.github.tangerg.flame/components/component.js": "export function Result(){}",
			}))
			if err != nil {
				t.Fatal(err)
			}
			i, err := plugin.New(testsupport.InstallationID(t), "/package", release)
			if err != nil {
				t.Fatal(err)
			}
			view := release.Declaration().Components[0]
			if _, err := r.ReadResource(t.Context(), i, view.Entry); err != nil {
				t.Fatal(err)
			}
			canceled, cancel := context.WithCancel(t.Context())
			cancel()
			if body, err := r.ReadResource(canceled, i, view.Entry); len(body) != 0 || !errors.Is(err, context.Canceled) || errors.Is(err, plugin.ErrUnavailable) {
				t.Fatalf("resource cancellation reclassified: %q, %v", body, err)
			}
			root, err := r.Root(release.Digest())
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(root, view.Entry)
			if err := os.Chmod(filepath.Dir(file), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
			switch attack {
			case "rewrite":
				err = os.WriteFile(file, []byte("<!doctype html><p>Changed</p>"), 0600)
			case "symlink":
				outside := filepath.Join(t.TempDir(), "outside.html")
				if err := os.WriteFile(outside, []byte("<!doctype html><p>Outside</p>"), 0600); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(outside, file)
			}
			if err != nil {
				t.Fatal(err)
			}
			body, err := r.ReadResource(t.Context(), i, view.Entry)
			if len(body) != 0 || !errors.Is(err, plugin.ErrUnavailable) {
				t.Fatalf("changed resource leaked: %q, %v", body, err)
			}
		})
	}
}
