package pluginservices

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"sync"

	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	skillspec "github.com/Tangerg/scope/skills"
	"github.com/samber/lo"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/packages/go-sdk/scopewire"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
)

type learningSourceRevision struct{ name, revision string }

type learningAdmission struct {
	mu                     sync.Mutex
	modelCalls             map[string]framevalue.JSON
	historyRead, skillRead bool
	source                 learningSourceRevision
	failure                error
}

func (a *learningAdmission) model(id string, input framevalue.JSON) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if original, found := a.modelCalls[id]; found {
		if original != input {
			return &framemcp.Refusal{Code: framemcp.InvocationConflictCode, Message: framemcp.ErrInvocationConflict.Error()}
		}
		return &framemcp.Refusal{Code: "model_redelivery", Message: "skill learning: original model invocation cannot be redelivered"}
	}
	if len(a.modelCalls) >= skilllearning.MaxModelCalls {
		return &framemcp.Refusal{Code: "model_budget", Message: "skill learning: model budget exhausted"}
	}
	// External models have no receipt: admission must precede their first I/O.
	a.modelCalls[id] = input
	return nil
}

func (a *learningAdmission) read(skill bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	used := &a.historyRead
	if skill {
		used = &a.skillRead
	}
	if *used {
		return &framemcp.Refusal{Code: "read_budget", Message: "skill learning: source read budget exhausted"}
	}
	*used = true
	return nil
}

func (a *learningAdmission) remember(source learningSourceRevision, failure error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if source.name != "" {
		a.source = source
	}
	a.failure = errors.Join(a.failure, failure)
}

func (a *learningAdmission) outcome() (learningSourceRevision, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.source, a.failure
}

func (s *SkillLearning) grant(ctx context.Context, session *sdk.ClientSession, sessionID string, state *learningAdmission) (pluginmcp.CallbackRef, func(), error) {
	conversationMethod, err := skilllearning.ConversationMethod()
	if err != nil {
		return pluginmcp.CallbackRef{}, nil, err
	}
	conversation, err := pluginmcp.NewCallback(conversationMethod, nil, func(ctx context.Context, _ pluginmcp.Command[struct{}]) (result skilllearning.Conversation, err error) {
		defer func() { state.remember(learningSourceRevision{}, err) }()
		if err := state.read(false); err != nil {
			return result, err
		}
		messages, err := s.history.Read(ctx, sessionID)
		if err != nil {
			return result, err
		}
		result = skilllearning.Conversation{MessageCount: int64(len(messages)), Transcript: models.RenderTranscript(messages)}
		if len(result.Transcript) > skilllearning.MaxTranscriptBytes {
			return skilllearning.Conversation{}, errors.New("skill learning: conversation exceeds transcript limit")
		}
		return result, nil
	})
	if err != nil {
		return pluginmcp.CallbackRef{}, nil, err
	}
	read, err := s.readRevisionCallback(state)
	if err != nil {
		return pluginmcp.CallbackRef{}, nil, err
	}
	model, err := pluginsdk.NewModelCallback(func(ctx context.Context, command pluginmcp.Command[scopewire.Request]) (response *chat.Response, err error) {
		defer func() { state.remember(learningSourceRevision{}, err) }()
		if !learningModelRequest(chat.Request(command.Input)) {
			return nil, &framemcp.Refusal{Code: "model_envelope", Message: "skill learning: model envelope refused"}
		}
		input, err := command.RequestInput()
		if err != nil {
			return nil, err
		}
		if err := state.model(string(command.ID), input); err != nil {
			return nil, err
		}
		captured, release, err := s.utility(ctx, models.AuxiliaryBudget{MaxInputBytes: skilllearning.MaxModelInputBytes, MaxOutputTokens: skilllearning.MaxModelOutputTokens})
		if err != nil {
			return nil, err
		}
		if release == nil {
			return nil, errors.New("skill learning: model release is required")
		}
		defer func() { err = errors.Join(err, release()) }()
		if lo.IsNil(captured.Model()) {
			return nil, errors.New("skill learning: model is required")
		}
		return captured.Model().Call(ctx, (*chat.Request)(&command.Input).Clone())
	})
	if err != nil {
		return pluginmcp.CallbackRef{}, nil, err
	}
	admissionMethod, err := skilllearning.AdmitMethod()
	if err != nil {
		return pluginmcp.CallbackRef{}, nil, err
	}
	admit, err := pluginmcp.NewCallback(admissionMethod, nil, func(_ context.Context, cmd pluginmcp.Command[skilllearning.Proposal]) (skilllearning.Admission, error) {
		return state.admitDraft(cmd.Input, sessionID)
	})
	if err != nil {
		return pluginmcp.CallbackRef{}, nil, err
	}
	return s.callbacks.Open(ctx, session, pluginmcp.CallbackLimits{MaxRequestBytes: skilllearning.MaxCallbackBytes, MaxResponseBytes: skilllearning.MaxCallbackBytes}, conversation, read, model, admit)
}

func learningModelRequest(request chat.Request) bool {
	options := request.Options
	if options.MaxOutputTokens == nil || *options.MaxOutputTokens <= 0 || *options.MaxOutputTokens > skilllearning.MaxModelOutputTokens || len(request.Messages) != 2 || request.Messages[0].Role != chat.RoleSystem || request.Messages[1].Role != chat.RoleUser || len(request.Tools) != 0 || request.ToolChoice != nil {
		return false
	}
	if options.OutputFormat == nil || options.OutputFormat.Type != chat.OutputFormatText {
		return false
	}
	options.MaxOutputTokens, options.OutputFormat = nil, nil
	if !reflect.DeepEqual(options, chat.Options{}) {
		return false
	}
	for _, message := range request.Messages {
		if len(message.Parts) != 1 || !reflect.DeepEqual(message, chat.Message{Role: message.Role, Parts: []chat.Part{chat.NewTextPart(message.Text())}}) {
			return false
		}
	}
	return len(request.Messages[0].Text())+len(request.Messages[1].Text()) <= skilllearning.MaxModelInputBytes
}

func (a *learningAdmission) admitDraft(candidate skilllearning.Proposal, sessionID string) (skilllearning.Admission, error) {
	read, err := a.outcome()
	if err != nil {
		return skilllearning.Admission{}, err
	}
	if candidate.Revises && (read.name != candidate.Skill.Name || read.revision == "" || read.revision != candidate.Previous) {
		return skilllearning.Admission{}, errors.New("skill learning: revision does not match the admitted current source")
	}
	if !candidate.Revises && candidate.Previous != "" {
		return skilllearning.Admission{}, errors.New("skill learning: new proposal cannot replace current bytes")
	}
	p := skills.Proposal{Scope: skills.ScopeUser, Origin: skills.ProposalOriginMined, SourceSession: sessionID, Name: candidate.Skill.Name, Description: candidate.Skill.Description, Instructions: candidate.Skill.Instructions, Revises: candidate.Revises}
	if err := p.Validate(); err != nil {
		if errors.Is(err, skills.ErrUnsafeProposal) {
			return skilllearning.Admission{}, nil
		}
		return skilllearning.Admission{}, err
	}
	return skilllearning.Admission{Accepted: true}, nil
}

func (s *SkillLearning) readRevisionCallback(state *learningAdmission) (pluginmcp.Callback, error) {
	readMethod, err := skilllearning.ReadSkillMethod()
	if err != nil {
		return pluginmcp.Callback{}, err
	}
	read, err := pluginmcp.NewCallback(readMethod, nil, func(ctx context.Context, command pluginmcp.Command[skilllearning.ReadSkill]) (result skilllearning.ReadResult, err error) {
		defer func() { state.remember(learningSourceRevision{}, err) }()
		if err := state.read(true); err != nil {
			return result, err
		}
		current, revision, err := s.source.CurrentDocument(ctx, command.Input.Name)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, skillspec.ErrInvalidSkill) {
			return result, nil
		}
		if err != nil {
			return result, err
		}
		if current == nil {
			return result, skillspec.ErrNilSkill
		}
		if current.Name != command.Input.Name {
			return result, errors.New("skill learning: loaded skill has a different name")
		}
		state.remember(learningSourceRevision{name: current.Name, revision: revision}, nil)
		return skilllearning.ReadResult{Revision: revision, Skill: &skilllearning.Skill{Name: current.Name, Description: current.Description, Instructions: current.Instructions}}, nil
	})
	return read, err
}
