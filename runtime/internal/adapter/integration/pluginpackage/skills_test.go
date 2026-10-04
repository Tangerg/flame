package pluginpackage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/Tangerg/scope/skills"

	"github.com/Tangerg/flame/runtime/internal/adapter/workspace/promptsource"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/google/uuid"
)

func TestSkillResourceReadPreservesFormatOwnerFailure(t *testing.T) {
	releases, store, _ := testReleaseStore(t)
	content, err := NewSkills(releases, store).ReadSkillResource(t.Context(), promptsource.InstallationDependency{}, "Invalid", "SKILL.md")
	if len(content) != 0 || !errors.Is(err, plugin.ErrInvalid) || !errors.Is(err, sdk.ErrNameInvalid) {
		t.Fatalf("invalid Skill name = (%q, %v), want no content and format owner cause", content, err)
	}
}

func TestSkillAdmissionAndReadsShareDirectoryNameBinding(t *testing.T) {
	for _, origin := range []string{"project", "package"} {
		t.Run(origin, func(t *testing.T) {
			name := "ｒｅｖｉｅｗ"
			document := "---\nname: review\ndescription: Review a result\n---\nInspect the results.\n"
			releases, store, _ := testReleaseStore(t)
			source := writePackage(t, map[string]string{"plugin.json": portableManifest, "skills/" + name + "/SKILL.md": document})
			cwd := ""
			if origin == "project" {
				cwd = t.TempDir()
				directory := filepath.Join(promptsource.ProjectSkillDir(cwd), name)
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, sdk.SkillFile), []byte(document), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				release, err := releases.Materialize(t.Context(), source)
				if err != nil || len(release.Skills) != 1 || release.Skills[0].Name != name {
					t.Fatalf("Skill admission = %+v, %v", release, err)
				}
				installation, err := plugin.New(uuid.NewString(), source, release)
				if err != nil {
					t.Fatal(err)
				}
				if err := installation.Approve(release.Digest, nil); err != nil {
					t.Fatal(err)
				}
				if err := installation.Enable(true); err != nil {
					t.Fatal(err)
				}
				if err := store.Save(t.Context(), installation); err != nil {
					t.Fatal(err)
				}
			}
			skills := NewSkills(releases, store)
			catalog := promptsource.NewSkills("", skills)
			discovery, err := catalog.List(t.Context(), cwd)
			if err != nil || len(discovery.Skills) != 1 || discovery.Skills[0].Name != name || len(discovery.Diagnostics) != 0 {
				t.Fatalf("Skill discovery contradicted admission: %+v, %v", discovery, err)
			}
			detail, err := catalog.Get(t.Context(), cwd, name)
			if err != nil || detail.Instructions != "Inspect the results.\n" {
				t.Fatalf("Skill detail contradicted admission: %+v, %v", detail, err)
			}
			model, _, err := promptsource.OverlaySkillSource(t.Context(), cwd, "", skills, nil)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := model.Load(t.Context(), name)
			if err != nil || loaded.Name != name || loaded.Instructions != detail.Instructions {
				t.Fatalf("model Skill contradicted admission: %+v, %v", loaded, err)
			}
		})
	}
}

func TestSkillDiscoveryDistinguishesUnavailablePackagesFromNameConflicts(t *testing.T) {
	for _, failure := range []string{"tampered document", "missing document", "replaced release directory"} {
		t.Run(failure, func(t *testing.T) {
			releases, store, _ := testReleaseStore(t)
			var broken plugin.Release
			for _, name := range []string{"broken", "working"} {
				document := "---\nname: " + name + "\ndescription: Inspect the results\n---\nRead carefully.\n"
				source := writePackage(t, map[string]string{"plugin.json": portableManifest, "skills/" + name + "/SKILL.md": document})
				release, err := releases.Materialize(t.Context(), source)
				if err != nil {
					t.Fatal(err)
				}
				installation, err := plugin.New(uuid.NewString(), source, release)
				if err != nil {
					t.Fatal(err)
				}
				if err := installation.Approve(release.Digest, nil); err != nil {
					t.Fatal(err)
				}
				if err := installation.Enable(true); err != nil {
					t.Fatal(err)
				}
				if err := store.Save(t.Context(), installation); err != nil {
					t.Fatal(err)
				}
				if name == "broken" {
					broken = release
				}
			}
			root, err := releases.Root(broken.Digest)
			if err != nil {
				t.Fatal(err)
			}
			directory := filepath.Join(root, "skills", "broken")
			if err := os.Chmod(directory, 0700); err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(directory, "SKILL.md")
			switch failure {
			case "tampered document":
				if err := os.Chmod(filename, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing document":
				if err := os.Remove(filename); err != nil {
					t.Fatal(err)
				}
			case "replaced release directory":
				if err := os.Rename(root, root+"-retired"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
			}
			catalog, err := promptsource.ListSkills(t.Context(), "", "", NewSkills(releases, store))
			if err != nil || len(catalog.Skills) != 1 || catalog.Skills[0].Name != "working" || len(catalog.Diagnostics) != 1 || catalog.Diagnostics[0].Name != "broken" {
				t.Fatalf("partial discovery = %+v, %v", catalog, err)
			}
			if !strings.Contains(catalog.Diagnostics[0].Detail, "unavailable") || strings.Contains(catalog.Diagnostics[0].Detail, "Multiple installations") {
				t.Fatalf("unavailable package was misdiagnosed: %+v", catalog.Diagnostics[0])
			}
		})
	}
}

func TestSkillReadsVerifyConsumedBytesAndEnforceResourceLimit(t *testing.T) {
	releases, store, _ := testReleaseStore(t)
	document := "---\nname: review\ndescription: Review a result\n---\nInspect the results.\n"
	source := writePackage(t, map[string]string{
		"plugin.json":                 portableManifest,
		"skills/review/SKILL.md":      document,
		"skills/review/reference.txt": strings.Repeat("x", domainskills.MaxSkillResourceBytes+1),
	})
	release, err := releases.Materialize(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(uuid.NewString(), source, release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release.Digest, nil); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(true); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	skills := NewSkills(releases, store)
	bundles, err := skills.SkillBundles(t.Context())
	if err != nil || len(bundles) != 1 {
		t.Fatalf("skill bundles = %+v, %v", bundles, err)
	}
	dependency := bundles[0].Dependency
	content, err := skills.ReadSkillResource(t.Context(), dependency, "review", "SKILL.md")
	if err != nil || string(content) != document {
		t.Fatalf("skill document = %q, %v", content, err)
	}
	if content, err := skills.ReadSkillResource(t.Context(), dependency, "review", "reference.txt"); !errors.Is(err, domainskills.ErrResourceTooLarge) || len(content) != 0 {
		t.Fatalf("oversized resource = %d bytes, %v", len(content), err)
	}
	root, err := releases.Root(release.Digest)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "skills/review/SKILL.md")
	if err := os.Chmod(filename, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(strings.ReplaceAll(document, "Inspect", "Corrupt")), 0600); err != nil {
		t.Fatal(err)
	}
	if content, err := skills.ReadSkillResource(t.Context(), dependency, "review", "SKILL.md"); !errors.Is(err, workspaceapp.ErrSkillUnavailable) || len(content) != 0 {
		t.Fatalf("tampered document = %q, %v", content, err)
	}
}
