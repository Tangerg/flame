package builtin

import (
	"context"
	"fmt"
	"strings"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"

	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/samber/lo"
)

const proposalDescription = `Propose a reusable Skill for future work.

Call this only when the user explicitly asks to save, preserve, or create a
reusable workflow. Do not call it merely because a workflow seems useful, and
do not use it for one-off facts, ordinary task progress, transient fixes, or
secrets.

This creates a pending proposal for review. It does not activate, publish, or
run the Skill.`

type proposalArgs struct {
	Name         string       `json:"name" jsonschema:"required,minLength=1,maxLength=64,pattern=^[a-z0-9]+(-[a-z0-9]+)*$" jsonschema_description:"Stable lowercase hyphenated Skill identifier of at most 64 characters, such as review-go-api. Describe the workflow, not the current task."`
	Description  string       `json:"description" jsonschema:"required,minLength=1,maxLength=1024" jsonschema_description:"One or two sentences stating what reusable workflow the Skill provides and when it should be used."`
	Instructions string       `json:"instructions" jsonschema:"required,minLength=1,maxLength=1048576" jsonschema_description:"Complete self-contained instructions an Agent should follow in future work. The value may not exceed 1 MiB and must exclude one-off progress, transient context, and secrets."`
	Scope        skills.Scope `json:"scope" jsonschema:"required,enum=repository,enum=user" jsonschema_description:"repository = available only in the current repository; user = available across all workspaces for this user."`
}

type proposalStatus string

const proposalStatusPendingReview proposalStatus = "pending_review"

type proposalResult struct {
	Status   proposalStatus `json:"status"`
	Name     string         `json:"name"`
	Revision string         `json:"revision"`
	Scope    skills.Scope   `json:"scope"`
}

type SkillProposalSubmitter interface {
	SubmitProposal(ctx context.Context, cwd string, proposal skills.Proposal) (skills.ProposalRef, error)
}

type proposer struct {
	proposals SkillProposalSubmitter
}

func NewProposal(proposals SkillProposalSubmitter) (toolcontract.Tool, error) {
	if lo.IsNil(proposals) {
		return nil, fmt.Errorf("%s: Skill proposal submitter is required", builtincontract.ProposeSkill)
	}
	return toolcontract.NewFunc[proposalArgs, proposalResult](
		toolcontract.FuncConfig{Name: string(builtincontract.ProposeSkill), Description: proposalDescription},
		(&proposer{proposals: proposals}).run,
	)
}

func (p *proposer) run(ctx context.Context, input proposalArgs) (proposalResult, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID.IsZero() {
		return proposalResult{}, fmt.Errorf("%s: no active session", builtincontract.ProposeSkill)
	}
	if err := sessionID.Validate(); err != nil {
		return proposalResult{}, fmt.Errorf("%s: active %w", builtincontract.ProposeSkill, err)
	}
	cwd, attached := executionctx.WorkspaceCWD(ctx)
	if !attached {
		return proposalResult{}, fmt.Errorf("%s: no attached Run workspace", builtincontract.ProposeSkill)
	}
	if err := input.Scope.Validate(); err != nil {
		return proposalResult{}, pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: scope must be repository or user: %w", builtincontract.ProposeSkill, err))
	}
	proposal := skills.Proposal{
		Scope:         input.Scope,
		Name:          strings.TrimSpace(input.Name),
		Description:   strings.TrimSpace(input.Description),
		Instructions:  strings.TrimSpace(input.Instructions),
		Origin:        skills.ProposalOriginRequested,
		SourceSession: sessionID.String(),
	}
	if validateErr := proposal.Validate(); validateErr != nil {
		return proposalResult{}, pluginsdk.DefiniteToolFailure(fmt.Errorf("%s: invalid proposal: %w", builtincontract.ProposeSkill, validateErr))
	}
	ref, err := p.proposals.SubmitProposal(ctx, cwd, proposal)
	if err != nil {
		return proposalResult{}, fmt.Errorf("%s: submit proposal: %w", builtincontract.ProposeSkill, err)
	}
	return proposalResult{
		Status:   proposalStatusPendingReview,
		Name:     ref.Name,
		Revision: ref.Revision,
		Scope:    ref.Scope,
	}, nil
}
