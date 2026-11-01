package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// skillScreens shows discovered and managed Skills and decides proposals
// through the Runtime's Skills port.
type skillScreens struct {
	screenHost
	service Skills
}

func (s *skillScreens) ShowDiscoveredSkills(name string) {
	if name = strings.TrimSpace(name); name != "" {
		s.reader.show(s.skillDetailReaderQuery(name))
		return
	}
	s.reader.show(s.discoveredSkillsReaderQuery())
}

func (s *skillScreens) skillDetailReaderQuery(name string) *readerQuery {
	workspace := s.session.current.Workspace.Path
	return &readerQuery{status: "loading skill detail", follows: followsTopic(protocol.TopicSkillsChanged), read: func(ctx context.Context) (readerDocument, error) {
		detail, err := s.service.InspectSkill(ctx, workspace, name)
		if err != nil {
			return readerDocument{}, err
		}
		return readerDocument{Title: detail.Name, Detail: string(detail.Scope) + " · " + detail.Revision, Sections: []toolSection{
			{Title: "Source", Style: toolSectionParagraph, Text: detail.Path},
			{Title: "Instructions", Style: toolSectionParagraph, Text: detail.Instructions},
		}}, nil
	}}
}

func (s *skillScreens) discoveredSkillsReaderQuery() *readerQuery {
	workspace := s.session.current.Workspace.Path
	return &readerQuery{
		status:  "loading discovered skills",
		follows: followsTopic(protocol.TopicSkillsChanged),
		read: func(ctx context.Context) (readerDocument, error) {
			discovered, err := s.service.Discover(ctx, workspace)
			if err != nil {
				return readerDocument{}, err
			}
			return discoveredSkillsDocument(workspace, discovered), nil
		},
	}
}

func discoveredSkillsDocument(workspacePath string, catalog protocol.SkillDiscovery) readerDocument {
	discovered := catalog.Skills
	lines := make([]string, 0, len(discovered))
	for _, skill := range discovered {
		line := workspace.DiscoveredSkillKey(skill)
		if skill.Description != "" {
			line += "  " + skill.Description
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, "No skills are discoverable for this workspace.")
	}
	for _, diagnostic := range catalog.Diagnostics {
		lines = append(lines, diagnostic.Name+": "+diagnostic.Detail)
	}
	return paragraphDocument("Discovered skills", fmt.Sprintf("%d available · %s", len(discovered), workspacePath), lines)
}

func (s *skillScreens) ShowManagedSkills() {
	s.reader.show(s.managedSkillsReaderQuery())
}

func (s *skillScreens) managedSkillsReaderQuery() *readerQuery {
	return &readerQuery{
		status:  "loading managed skills",
		follows: followsTopic(protocol.TopicSkillsChanged),
		read: func(ctx context.Context) (readerDocument, error) {
			managed, err := s.service.Managed(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return managedSkillsDocument(managed), nil
		},
	}
}

func managedSkillsDocument(managed []protocol.ManagedSkill) readerDocument {
	lines := make([]string, 0, len(managed))
	for _, skill := range managed {
		line := skill.Name + "  " + string(skill.Lifecycle)
		if skill.Description != "" {
			line += "  · " + skill.Description
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, "The managed user Skill library is empty.")
	}
	return paragraphDocument("Managed Skill library", fmt.Sprintf("%d entries", len(managed)), lines)
}

func (s *skillScreens) ShowSkillProposals() {
	s.reader.show(s.skillProposalsReaderQuery())
}

func (s *skillScreens) skillProposalsReaderQuery() *readerQuery {
	workspace := s.session.current.Workspace.Path
	return &readerQuery{
		status:  "loading skill proposals",
		follows: followsTopic(protocol.TopicSkillsChanged),
		read: func(ctx context.Context) (readerDocument, error) {
			proposals, err := s.service.Proposals(ctx, workspace)
			if err != nil {
				return readerDocument{}, err
			}
			return skillProposalsDocument(proposals), nil
		},
	}
}

func skillProposalsDocument(proposals []workspace.SkillProposal) readerDocument {
	if len(proposals) == 0 {
		return paragraphDocument("Skill proposals", "none pending", []string{"No Skill proposals are awaiting review."})
	}
	sections := make([]toolSection, 0, len(proposals)*2)
	for _, proposal := range proposals {
		provenance := string(proposal.Origin)
		if proposal.Revises {
			provenance += " · revises existing skill"
		}
		if proposal.SourceSession != "" {
			provenance += " · session " + shortIdentity(proposal.SourceSession)
		}
		sections = append(sections,
			toolSection{Title: proposal.Key(), Style: toolSectionParagraph, Text: proposal.Description + "\n" + provenance},
			toolSection{Title: "Instructions", Style: toolSectionParagraph, Text: proposal.Instructions, Links: true},
		)
	}
	return readerDocument{Title: "Skill proposals", Detail: fmt.Sprintf("%d awaiting review", len(proposals)), Sections: sections}
}

func (s *skillScreens) ArchiveSkill(name string) error {
	return s.changeSkillLifecycle("archiving skill", name, s.service.Archive)
}

func (s *skillScreens) RestoreSkill(name string) error {
	return s.changeSkillLifecycle("restoring skill", name, s.service.Restore)
}

func (s *skillScreens) changeSkillLifecycle(
	status, name string,
	change func(context.Context, string) error,
) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("a skill name is required")
	}
	s.status.note(status + " " + name)
	started := s.tasks.admissionMutation(skillOperation, false,
		func(ctx context.Context) (string, error) {
			return name, change(ctx, name)
		},
		func(changed string, err error) {
			if err != nil {
				s.message(status + " failed: " + err.Error())
				return
			}
			s.message(status + " complete · " + changed)
		},
	)
	if !started {
		return errors.New("another skill operation is running")
	}
	return nil
}

type skillProposalDecision struct {
	proposal  workspace.SkillProposal
	reference workspace.SkillProposalReference
}

func (s *skillScreens) PrepareSkillProposalDecision(identity string, approve bool) error {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return errors.New("a proposal name, scope/name or scope/name@revision is required")
	}
	workspace := s.session.current.Workspace.Path
	s.status.note("loading skill proposal " + identity)
	started := s.tasks.session(skillOperation, false,
		func(ctx context.Context) (skillProposalDecision, error) {
			proposals, err := s.service.Proposals(ctx, workspace)
			if err != nil {
				return skillProposalDecision{}, err
			}
			proposal, err := resolveSkillProposal(proposals, identity)
			if err != nil {
				return skillProposalDecision{}, err
			}
			reference, err := proposal.Reference(workspace)
			return skillProposalDecision{proposal: proposal, reference: reference}, err
		},
		func(decision skillProposalDecision, err error) {
			if err != nil {
				s.message("load skill proposal failed: " + err.Error())
				return
			}
			s.confirmSkillProposalDecision(decision, approve)
		},
	)
	if !started {
		return errors.New("another skill operation is running")
	}
	return nil
}

func resolveSkillProposal(proposals []workspace.SkillProposal, identity string) (workspace.SkillProposal, error) {
	matches := make([]workspace.SkillProposal, 0, 1)
	for _, proposal := range proposals {
		if proposal.Key() == identity || proposal.QualifiedName() == identity || proposal.Name == identity {
			matches = append(matches, proposal)
		}
	}
	switch len(matches) {
	case 0:
		return workspace.SkillProposal{}, errors.New("skill proposal not found: " + identity)
	case 1:
		return matches[0], nil
	default:
		return workspace.SkillProposal{}, errors.New("skill proposal name is ambiguous; use scope/name@revision from /skill-proposals")
	}
}

func (s *skillScreens) confirmSkillProposalDecision(decision skillProposalDecision, approve bool) {
	verb, action := "Reject", "Reject permanently"
	if approve {
		verb, action = "Approve", "Approve and publish"
	}
	var content strings.Builder
	content.WriteString("Revision: " + decision.reference.Revision + "\n\n")
	for _, section := range skillProposalsDocument([]workspace.SkillProposal{decision.proposal}).Sections {
		content.WriteString(section.Title + "\n" + section.Text + "\n\n")
	}
	question := "Decide this exact revision · PgUp/PgDn scroll"
	s.confirm(verb+" Skill proposal", question, action, content.String(), func() {
		s.decideSkillProposal(decision.reference, approve)
	})
}

func (s *skillScreens) decideSkillProposal(reference workspace.SkillProposalReference, approve bool) {
	verb := "rejecting"
	decide := s.service.Reject
	if approve {
		verb = "approving"
		decide = s.service.Approve
	}
	s.status.note(verb + " skill proposal " + reference.Name)
	started := s.tasks.admissionMutation(skillOperation, false,
		func(ctx context.Context) (workspace.SkillProposalReference, error) {
			return reference, decide(ctx, reference)
		},
		func(reviewed workspace.SkillProposalReference, err error) {
			if err != nil {
				s.message(verb + " skill proposal failed: " + err.Error())
				return
			}
			s.message(verb + " skill proposal complete · " + string(reviewed.Scope) + "/" + reviewed.Name)
		},
	)
	if !started {
		s.message("another skill operation is running")
	}
}
