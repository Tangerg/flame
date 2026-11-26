package skillauthoring

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	skillspec "github.com/Tangerg/scope/skills"
	"go.yaml.in/yaml/v3"
)

func renderDocument(header skillspec.Frontmatter, instructions string) ([]byte, error) {
	frontmatter, err := yaml.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: render frontmatter: %w", err)
	}
	document := []byte("---\n" + string(frontmatter) + "---\n\n" + strings.TrimSpace(instructions) + "\n")
	if len(document) > skills.MaxAuthoredSkillDocumentBytes {
		return nil, fmt.Errorf(
			"skillauthoring: render proposal %q: %w: %d bytes exceeds %d",
			header.Name,
			skills.ErrDocumentTooLarge,
			len(document),
			skills.MaxAuthoredSkillDocumentBytes,
		)
	}
	return document, nil
}

func proposalFrontmatter(proposal skills.Proposal) skillspec.Frontmatter {
	metadata := map[string]string{metadataOrigin: string(proposal.Origin)}
	if proposal.SourceSession != "" {
		metadata[metadataSourceSession] = proposal.SourceSession
	}
	return skillspec.Frontmatter{Name: proposal.Name, Description: proposal.Description, Metadata: metadata}
}
