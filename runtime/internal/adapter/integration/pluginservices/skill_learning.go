package pluginservices

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"

	sdk "github.com/Tangerg/go-sdk/mcp"
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
	Utility      modeladapter.AuxiliaryResolver
	Conversation conversationReader
	Source       learningSkillSource
	Proposals    *SkillGovernance
	Archives     learningArchives
}

type SkillLearning struct {
	contract  *plugincontract.Capability
	commands  *CommandCalls
	callbacks *pluginmcp.Callbacks
	utility   modeladapter.AuxiliaryResolver
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

func (s *SkillLearning) MaintainSkills(ctx context.Context, input execution.RunMaintenanceInput, completedToolCalls int64) (returned error) {
	c := s.commands
	binding, borrowed, err := contributionSession(ctx, c.framework, c.installations, c.selections, c.pool, s.contract)
	if errors.Is(err, errInactive) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, borrowed.Close()) }()
	ctx = borrowed.Context()
	mineErr := s.mine(ctx, binding, input, completedToolCalls)
	archiveErr := s.archive(ctx, binding, time.Now())
	return errors.Join(mineErr, archiveErr)
}

func (s *SkillLearning) mine(ctx context.Context, binding plugins.ContributionBinding, input execution.RunMaintenanceInput, completedToolCalls int64) error {
	if input.SessionID.IsZero() || input.WorkspaceCWD == "" || input.RootMemberID == "" {
		return nil
	}
	method, err := skilllearning.MineMethod()
	if err != nil {
		return err
	}
	mining := operation.Start(ctx, extension.Automatic, method.Name(), input.RootMemberID)
	binding, err = s.miningBinding(ctx, mining, binding)
	if err != nil {
		return err
	}
	library, err := s.proposals.libraryID(ctx, input.WorkspaceCWD, skills.ScopeUser, binding)
	if err != nil {
		return err
	}
	request := skilllearning.Run{SessionID: input.SessionID.String(), ToolCalls: completedToolCalls, Library: library}
	if err := request.Validate(); err != nil {
		return err
	}
	state := &learningAdmission{models: newExternalCallLedger("skill learning", skilllearning.MaxModelCalls)}
	reply, err := invokeResolvedCommand[skilllearning.Run, skilllearning.MineResult](mining, s.commands, method.Name(), request, nil, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, func(ctx context.Context, _ plugins.ContributionBinding, session *sdk.ClientSession) (sdk.ParamsBase, func() error, error) {
		return s.grantMetadata(ctx, session, input.SessionID, state)
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
	recordMinedSkillProposal(ctx, advice.Revises)
	return nil
}

func (s *SkillLearning) miningBinding(ctx, mining context.Context, selected plugins.ContributionBinding) (plugins.ContributionBinding, error) {
	record, err := s.commands.routing.Get(mining, operation.Identity(mining))
	if errors.Is(err, plugin.ErrNotFound) || errors.Is(err, plugin.ErrCommandUnconfirmed) {
		return selected, nil
	}
	if err != nil {
		return plugins.ContributionBinding{}, err
	}
	return s.commands.original(ctx, record.Snapshot())
}

func (s *SkillLearning) grantMetadata(ctx context.Context, session *sdk.ClientSession, sessionID identity.SessionID, state *learningAdmission) (sdk.ParamsBase, func() error, error) {
	grant, release, err := s.grant(ctx, session, sessionID, state)
	if err != nil {
		return sdk.ParamsBase{}, nil, err
	}
	metadata, err := pluginmcp.CallbackMetadata(grant)
	if err != nil {
		release()
		return sdk.ParamsBase{}, nil, err
	}
	return metadata, func() error { release(); return nil }, nil
}

func (s *SkillLearning) archive(ctx context.Context, binding plugins.ContributionBinding, now time.Time) error {
	method, err := skilllearning.ArchiveMethod()
	if err != nil {
		return err
	}
	reply, err := invokeResolvedCommand[skilllearning.ArchiveRequest, skilllearning.ArchiveAdvice](operation.Start(ctx, extension.Automatic, automaticOperationNamespace, uuid.NewString()), s.commands, method.Name(), skilllearning.ArchiveRequest{Now: now}, func(advice skilllearning.ArchiveAdvice) error { return advice.Validate(now) }, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, nil)
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
