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

func TestViewAdmissionIsBoundedAndIsolatesInvalidSiblings(t *testing.T) {
	releases := testReleases(t)
	manifest := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"test.views","extensions":{"io.github.tangerg.flame":{"apiVersion":1,"contributes":{"views":[{"id":"good","title":"Good","type":"sessionTrajectory","entry":"good.html"},{"id":"escape","title":"Escape","type":"sessionTrajectory","entry":"../escape.html"},{"id":"wrong","title":"Wrong MIME","type":"sessionTrajectory","entry":"wrong.html"},{"id":"large","title":"Large","type":"sessionTrajectory","entry":"large.html"},{"id":"missing","title":"Missing","type":"sessionTrajectory","entry":"missing.html"},{"id":"write","title":"Write","type":"tools.invoke","entry":"good.html"}]}}}}`
	release, err := publishPackage(t.Context(), releases, writePackage(t, map[string]string{"plugin.json": manifest, "good.html": "<!doctype html><p>Good</p>", "wrong.html": "plain text", "large.html": "<!doctype html>" + strings.Repeat("x", plugin.MaxViewBytes)}))
	if err != nil {
		t.Fatal(err)
	}
	declaration := release.Declaration()
	if len(declaration.Views) != 1 || declaration.Views[0].ID != "good" || len(declaration.Diagnostics) != 5 {
		t.Fatalf("view admission: %+v", declaration)
	}
}

func TestViewReadRejectsChangedBytesAndFilesystemEscape(t *testing.T) {
	for _, attack := range []string{"rewrite", "symlink", "missing"} {
		t.Run(attack, func(t *testing.T) {
			r := testReleases(t)
			release, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{
				"plugin.json": `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"test.views","extensions":{"io.github.tangerg.flame":{"apiVersion":1,"contributes":{"views":[{"id":"view","title":"View","type":"sessionTrajectory","entry":"view.html"}]}}}}`,
				"view.html":   "<!doctype html><p>Original</p>",
			}))
			if err != nil {
				t.Fatal(err)
			}
			i, err := plugin.New(testsupport.InstallationID(t), "/package", release)
			if err != nil {
				t.Fatal(err)
			}
			view := release.Declaration().Views[0]
			if _, err := r.ReadView(t.Context(), i, view); err != nil {
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
			body, err := r.ReadView(t.Context(), i, view)
			if body != "" || !errors.Is(err, plugin.ErrUnavailable) {
				t.Fatalf("changed resource leaked: %q, %v", body, err)
			}
		})
	}
}
