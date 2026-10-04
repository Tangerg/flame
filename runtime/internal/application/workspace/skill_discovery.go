package workspace

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
)

// Discovery and execution share these limits after source precedence is resolved.
// A diagnostic may describe an override of an otherwise available Skill.
func (d SkillDiscovery) Validate() error {
	if len(d.Skills) > (plugin.MaxInstallations+2)*skills.MaxSkillsPerSource {
		return fmt.Errorf("%w: discovered catalog exceeds source capacity", skills.ErrLibraryCapacity)
	}
	counts := make(map[SkillSource]int)
	seen := make(map[string]bool, len(d.Skills))
	for index, entry := range d.Skills {
		if err := validateSkillSummary(entry); err != nil {
			return fmt.Errorf("workspace: discovered Skill %d is invalid: %w", index+1, err)
		}
		if seen[entry.Name] {
			return fmt.Errorf("workspace: discovered Skill catalog repeats visible name %q", entry.Name)
		}
		seen[entry.Name] = true
		counts[entry.Source]++
		if counts[entry.Source] > skills.MaxSkillsPerSource {
			return fmt.Errorf("%w: selected source exceeds %d Skills", skills.ErrLibraryCapacity, skills.MaxSkillsPerSource)
		}
	}
	if len(d.Diagnostics) > plugin.MaxInstallations*skills.MaxSkillsPerSource+2*skills.MaxSkillDirectoryEntries {
		return fmt.Errorf("%w: too many discovery diagnostics", skills.ErrLibraryCapacity)
	}
	seen = make(map[string]bool, len(d.Diagnostics))
	for _, diagnostic := range d.Diagnostics {
		if err := skills.ValidateName(diagnostic.Name); err != nil {
			return fmt.Errorf("workspace: invalid Skill diagnostic: %w", err)
		}
		if strings.TrimSpace(diagnostic.Detail) == "" || len(diagnostic.Detail) > 512 || !utf8.ValidString(diagnostic.Detail) || seen[diagnostic.Name] {
			return fmt.Errorf("workspace: invalid or repeated Skill diagnostic %q", diagnostic.Name)
		}
		seen[diagnostic.Name] = true
	}
	return nil
}
