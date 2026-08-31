package arch

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestRingPackagesUseContextHierarchy keeps architecture rings from becoming
// flat package catalogs. Direct packages are reserved for a ring-wide mechanism
// or an aggregate that also names its context; cohesive capability families use
// one namespace directory without adding facade Go packages.
func TestRingPackagesUseContextHierarchy(t *testing.T) {
	topologies := []struct {
		ring       string
		direct     []string
		contexts   []string
		exceptions []string
	}{
		{
			ring:     "domain",
			direct:   []string{"modelref", "resourceid", "run", "session"},
			contexts: []string{"automation", "integration", "run", "session", "workspace"},
		},
		{
			ring:     "application",
			direct:   []string{"invalidation", "opaquetoken", "ownership", "pagination", "taskgroup", "workspace"},
			contexts: []string{"agent", "automation", "integration", "workspace"},
		},
		{
			ring:       "adapter",
			direct:     []string{"agentexec", "executionctx", "model", "persistence", "runtimeownership", "scheduleidentity", "toolset", "workspace"},
			contexts:   []string{"agentexec", "integration", "model", "run", "toolset", "workspace"},
			exceptions: []string{"toolset/internal/toolarg"},
		},
		{
			ring:     "infra",
			direct:   []string{"advisorylock", "git", "telemetry"},
			contexts: []string{"filesystem", "git", "integration", "process", "storage"},
		},
	}

	root := filepath.Join(moduleRoot(t), "internal")
	for _, topology := range topologies {
		t.Run(topology.ring, func(t *testing.T) {
			ringRoot := filepath.Join(root, topology.ring)
			err := filepath.WalkDir(ringRoot, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !entry.IsDir() || path == ringRoot {
					return nil
				}
				if entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				hasPackage, err := directoryHasDirectProductionGo(path)
				if err != nil {
					return err
				}
				if !hasPackage {
					return nil
				}
				relative, err := filepath.Rel(ringRoot, path)
				if err != nil {
					return err
				}
				relative = filepath.ToSlash(relative)
				if slices.Contains(topology.exceptions, relative) {
					return nil
				}
				parts := strings.Split(relative, "/")
				switch len(parts) {
				case 1:
					if !slices.Contains(topology.direct, relative) {
						t.Errorf("%s is a flat package without ring-wide ownership; place it under a proven context namespace", relative)
					}
				case 2:
					if !slices.Contains(topology.contexts, parts[0]) {
						t.Errorf("%s uses unreviewed context namespace %s", relative, parts[0])
					}
				default:
					t.Errorf("%s nests beyond ring/context/package", relative)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("scan %s package topology: %v", topology.ring, err)
			}
		})
	}
}
