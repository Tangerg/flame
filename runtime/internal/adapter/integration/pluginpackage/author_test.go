package pluginpackage

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/distribution"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestNeutralDeclarationCannotSubstituteManagedRuntimeOrCapabilities(t *testing.T) {
	for _, change := range []string{"interpreter", "capability"} {
		t.Run(change, func(t *testing.T) {
			source := writePackage(t, managedFiles(`[{"id":"backend","runtime":"node","entry":"backend/start.mjs"}]`))
			built, err := BuildRelease(t.Context(), source, BuildTarget{}, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			manifest := built.Manifest
			switch change {
			case "interpreter":
				manifest.Units[0].Variants[0].Runtime.Digest = qimu.Hash([]byte("different interpreter"))
			case "capability":
				manifest.Units[0].RequestedCapabilities = []string{"undeclared-host-access"}
			}
			var output bytes.Buffer
			if _, err := distribution.Build(t.Context(), source, manifest, nil, &output); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(t.TempDir(), "changed.zip")
			if err := os.WriteFile(archive, output.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := testReleases(t).Materialize(t.Context(), archive); !errors.Is(err, plugin.ErrInvalid) {
				t.Fatalf("neutral metadata silently changed Flame's actual process contract: %v", err)
			}
		})
	}
}

func TestNativeReleaseWithoutCurrentPlatformNeverProducesALaunch(t *testing.T) {
	releases := testReleases(t)
	source := writePackage(t, map[string]string{"plugin.json": portableManifest, "mcp.json": `{"$schema":"` + MCPSchema + `","mcpServers":{"backend":{"type":"stdio","command":"./bin/backend"}}}`, "bin/backend": "foreign native fixture"})
	other := "darwin"
	if runtime.GOOS == other {
		other = "linux"
	}
	if _, err := BuildRelease(t.Context(), source, BuildTarget{OS: other, Arch: runtime.GOARCH}, io.Discard); err != nil {
		t.Fatal(err)
	}
	candidate, err := releases.Materialize(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Discard()
	release, err := candidate.Publish(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	launch, found, err := releases.Connection(t.Context(), installation, release, release.Declaration().Servers[0].Name)
	if !errors.Is(err, plugin.ErrUnavailable) || found || launch.Stdio != nil || launch.Retire != nil {
		t.Fatalf("foreign package target became an executable launch: %+v, %v, %v", launch, found, err)
	}
}
