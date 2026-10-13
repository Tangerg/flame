package pluginpackage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRendererAdmissionIsBoundedAndIsolatesInvalidSiblings(t *testing.T) {
	releases := testReleases(t)
	manifest := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"test.renderers","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`
	release, err := publishPackage(t.Context(), releases, writePackage(t, map[string]string{"plugin.json": manifest, "io.github.tangerg.flame/declaration.json": `{"protocol":"io.github.tangerg.flame/backend/2026-10-14.2","contributes":{"renderers":[{"id":"good","title":"Good","selector":{"type":"toolName","name":"read_file"},"entry":"good.html"},{"id":"escape","title":"Escape","selector":{"type":"toolName","name":"read_file"},"entry":"../escape.html"},{"id":"wrong","title":"Wrong MIME","selector":{"type":"toolName","name":"read_file"},"entry":"wrong.html"},{"id":"large","title":"Large","selector":{"type":"toolName","name":"read_file"},"entry":"large.html"},{"id":"missing","title":"Missing","selector":{"type":"toolName","name":"read_file"},"entry":"missing.html"},{"id":"write","title":"Write","selector":{"type":"invalid","name":"read_file"},"entry":"good.html"}]}}`, "good.html": "<!doctype html><p>Good</p>", "wrong.html": "plain text", "large.html": "<!doctype html>" + strings.Repeat("x", plugin.MaxPresentationBytes)}))
	if err != nil {
		t.Fatal(err)
	}
	declaration := release.Declaration()
	if len(declaration.Renderers) != 1 || declaration.Renderers[0].ID != "good" || len(declaration.Diagnostics) != 5 {
		t.Fatalf("renderer admission: %+v", declaration)
	}
}

func TestRendererReadRejectsChangedBytesAndFilesystemEscape(t *testing.T) {
	for _, attack := range []string{"rewrite", "symlink", "missing"} {
		t.Run(attack, func(t *testing.T) {
			r := testReleases(t)
			release, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{
				"plugin.json": `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"test.renderers","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
				"io.github.tangerg.flame/declaration.json": `{"protocol":"io.github.tangerg.flame/backend/2026-10-14.2","contributes":{"renderers":[{"id":"view","title":"View","selector":{"type":"toolName","name":"read_file"},"entry":"view.html"}]}}`,
				"view.html": "<!doctype html><p>Original</p>",
			}))
			if err != nil {
				t.Fatal(err)
			}
			i, err := plugin.New(testsupport.InstallationID(t), "/package", release)
			if err != nil {
				t.Fatal(err)
			}
			view := release.Declaration().Renderers[0]
			if _, err := r.ReadPage(t.Context(), i, view.Entry); err != nil {
				t.Fatal(err)
			}
			root, err := r.Root(release.Digest())
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(root, "view.html")
			if err := os.Chmod(root, 0700); err != nil {
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
			body, err := r.ReadPage(t.Context(), i, view.Entry)
			if body != "" || !errors.Is(err, plugin.ErrUnavailable) {
				t.Fatalf("changed resource leaked: %q, %v", body, err)
			}
		})
	}
}
