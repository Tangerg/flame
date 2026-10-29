package pluginservices

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	execution "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	sdk "github.com/Tangerg/go-sdk/mcp"
	skillspec "github.com/Tangerg/scope/skills"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

type learningSkillSource interface {
	Load(context.Context, string) (*skillspec.Skill, error)
}

type learningProposals interface {
	SubmitProposal(context.Context, string, skills.Proposal) (skills.ProposalRef, error)
}

type learningArchives interface {
	ArchiveIdle(context.Context, time.Time, time.Duration) ([]string, error)
}

type SkillLearningConfig struct {
	Commands     *CommandCalls
	Callbacks    *qimumcp.Callbacks
	Utility      models.AuxiliaryResolver
	Conversation conversationReader
	Source       learningSkillSource
	Proposals    learningProposals
	Archives     learningArchives
}

type SkillLearning struct {
	contract  *portable.Capability
	commands  *CommandCalls
	callbacks *qimumcp.Callbacks
	utility   models.AuxiliaryResolver
	history   conversationReader
	source    learningSkillSource
	proposals learningProposals
	archives  learningArchives
}

func NewSkillLearning(config SkillLearningConfig) (*SkillLearning, error) {
	for _, dependency := range []struct {
		name  string
		value any
	}{
		{"commands", config.Commands}, {"callbacks", config.Callbacks}, {"utility resolver", config.Utility},
		{"conversation", config.Conversation}, {"skill source", config.Source}, {"proposals", config.Proposals}, {"archives", config.Archives},
	} {
		if lo.IsNil(dependency.value) {
			return nil, fmt.Errorf("skill learning: %s is required", dependency.name)
		}
	}
	contract, err := skilllearning.Contract()
	if err != nil {
		return nil, err
	}
	return &SkillLearning{contract: contract, commands: config.Commands, callbacks: config.Callbacks, utility: config.Utility, history: config.Conversation, source: config.Source, proposals: config.Proposals, archives: config.Archives}, nil
}

func (s *SkillLearning) MaintainSkills(ctx context.Context, input execution.RunMaintenanceInput) error {
	c := s.commands
	binding, borrowed, err := contributionSession(ctx, c.framework, c.installations, c.selections, c.pool, s.contract)
	if errors.Is(err, errInactive) {
		return nil
	}
	if err != nil {
		return err
	}
	defer borrowed.Close()
	ctx = borrowed.Context()
	mineErr := s.mine(ctx, binding, input)
	archiveErr := s.archive(ctx, binding, time.Now())
	return errors.Join(mineErr, archiveErr)
}

func (s *SkillLearning) mine(ctx context.Context, binding plugins.ContributionBinding, input execution.RunMaintenanceInput) error {
	if input.SessionID == "" || input.WorkspaceCWD == "" {
		return nil
	}
	request := skilllearning.Run{SessionID: input.SessionID, ToolCalls: int64(input.ToolCalls)}
	if err := request.Validate(); err != nil {
		return err
	}
	method, err := skilllearning.MineMethod()
	if err != nil {
		return err
	}
	state := &learningAdmission{modelCalls: make(map[string]jsonvalue.Value)}
	reply, err := invokeResolvedCommand[skilllearning.Run, skilllearning.MineResult](operation.Start(ctx, extension.Automatic, "plugins.automatic", uuid.NewString()), s.commands, method.Name(), request, nil, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, func(ctx context.Context, _ plugins.ContributionBinding, session *sdk.ClientSession) (sdk.ParamsBase, func(), error) {
		grant, release, err := s.grant(ctx, session, input.SessionID, state)
		if err != nil {
			return sdk.ParamsBase{}, nil, err
		}
		metadata, err := qimumcp.CallbackMetadata(grant)
		if err != nil {
			release()
			return sdk.ParamsBase{}, nil, err
		}
		return metadata, release, nil
	})
	name, callbackErr := state.outcome()
	if err := errors.Join(err, callbackErr); err != nil {
		return fmt.Errorf("skill learning: mine: %w", err)
	}
	advice := reply.value.Proposal
	if advice == nil {
		return nil
	}
	if advice.Revises && name != advice.Skill.Name {
		return errors.New("skill learning: revision has no admitted current source")
	}
	proposal := skills.Proposal{Scope: skills.ScopeUser, Name: advice.Skill.Name, Description: advice.Skill.Description, Instructions: advice.Skill.Instructions, Origin: skills.ProposalOriginMined, SourceSession: input.SessionID, Revises: advice.Revises}
	if err := proposal.Validate(); err != nil {
		return fmt.Errorf("skill learning: invalid proposal: %w", err)
	}
	if proposal.SafetyIssue() != skills.ProposalSafe {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, context.Cause(ctx))
	}
	if _, err := s.proposals.SubmitProposal(ctx, input.WorkspaceCWD, proposal); err != nil {
		return fmt.Errorf("skill learning: submit proposal: %w", err)
	}
	kind := "new"
	if proposal.Revises {
		kind = "revise"
	}
	recordMinedSkillProposal(ctx, kind)
	return nil
}

func (s *SkillLearning) archive(ctx context.Context, binding plugins.ContributionBinding, now time.Time) error {
	method, err := skilllearning.ArchiveMethod()
	if err != nil {
		return err
	}
	reply, err := invokeResolvedCommand[skilllearning.ArchiveRequest, skilllearning.ArchiveAdvice](operation.Start(ctx, extension.Automatic, "plugins.automatic", uuid.NewString()), s.commands, method.Name(), skilllearning.ArchiveRequest{Now: now}, func(advice skilllearning.ArchiveAdvice) error { _, err := advice.Duration(); return err }, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, nil)
	if err != nil {
		return fmt.Errorf("skill learning: archive advice: %w", err)
	}
	after, err := reply.value.Duration()
	if err != nil || after == 0 {
		return err
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, context.Cause(ctx))
	}
	archived, err := s.archives.ArchiveIdle(ctx, now, after)
	recordArchivedIdleSkills(ctx, len(archived))
	return err
}
