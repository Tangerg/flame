package pluginservices

import (
	"context"
	"errors"
	"fmt"
	"time"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	execution "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"

	skillspec "github.com/Tangerg/scope/skills"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

type learningSkillSource interface {
	CurrentDocument(context.Context, string) (*skillspec.Skill, string, error)
}

type learningArchives interface {
	ArchiveIdle(context.Context, time.Time, time.Time) ([]string, error)
}

type SkillLearningConfig struct {
	Commands     *CommandCalls
	Callbacks    *pluginmcp.Callbacks
	Utility      models.AuxiliaryResolver
	Conversation conversationReader
	Source       learningSkillSource
	Proposals    *SkillGovernance
	Archives     learningArchives
}

type SkillLearning struct {
	contract  *plugincontract.Capability
	commands  *CommandCalls
	callbacks *pluginmcp.Callbacks
	utility   models.AuxiliaryResolver
	history   conversationReader
	source    learningSkillSource
	proposals *SkillGovernance
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
	if input.SessionID == "" || input.WorkspaceCWD == "" || input.RootMemberID == "" {
		return nil
	}
	method, err := skilllearning.MineMethod()
	if err != nil {
		return err
	}
	mining := operation.Start(ctx, extension.Automatic, method.Name(), input.RootMemberID)
	record, getErr := s.commands.routing.Get(mining, operation.Identity(mining))
	if getErr == nil {
		binding, err = s.commands.original(ctx, record.Snapshot())
		if err != nil {
			return err
		}
	} else if !errors.Is(getErr, plugin.ErrNotFound) && !errors.Is(getErr, plugin.ErrCommandUnconfirmed) {
		return getErr
	}
	library, err := s.proposals.libraryID(ctx, input.WorkspaceCWD, skills.ScopeUser, binding)
	if err != nil {
		return err
	}
	request := skilllearning.Run{SessionID: input.SessionID, ToolCalls: int64(input.ToolCalls), Library: library}
	if err := request.Validate(); err != nil {
		return err
	}
	state := &learningAdmission{modelCalls: make(map[string]framevalue.JSON)}
	reply, err := invokeResolvedCommand[skilllearning.Run, skilllearning.MineResult](mining, s.commands, method.Name(), request, nil, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, func(ctx context.Context, _ plugins.ContributionBinding, session *sdk.ClientSession) (sdk.ParamsBase, func() error, error) {
		grant, release, err := s.grant(ctx, session, input.SessionID, state)
		if err != nil {
			return sdk.ParamsBase{}, nil, err
		}
		metadata, err := pluginmcp.CallbackMetadata(grant)
		if err != nil {
			release()
			return sdk.ParamsBase{}, nil, err
		}
		return metadata, func() error { release(); return nil }, nil
	})
	_, callbackErr := state.outcome()
	if err := errors.Join(err, callbackErr); err != nil {
		return fmt.Errorf("skill learning: mine: %w", err)
	}
	advice := reply.value.Proposal
	if advice == nil {
		return nil
	}
	s.proposals.changed()
	kind := "new"
	if advice.Revises {
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
	reply, err := invokeResolvedCommand[skilllearning.ArchiveRequest, skilllearning.ArchiveAdvice](operation.Start(ctx, extension.Automatic, "plugins.automatic", uuid.NewString()), s.commands, method.Name(), skilllearning.ArchiveRequest{Now: now}, func(advice skilllearning.ArchiveAdvice) error { return advice.Validate(now) }, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, nil)
	if err != nil {
		return fmt.Errorf("skill learning: archive advice: %w", err)
	}
	if reply.value.InactiveSince == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, context.Cause(ctx))
	}
	archived, err := s.archives.ArchiveIdle(ctx, now, *reply.value.InactiveSince)
	recordArchivedIdleSkills(ctx, len(archived))
	return err
}
