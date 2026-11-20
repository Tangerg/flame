package pluginservices

import (
	"context"
	"errors"
	"io/fs"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/packages/go-sdk/scopewire"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"

	framecall "github.com/Tangerg/frame/call"

	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	skillspec "github.com/Tangerg/scope/skills"
	"github.com/samber/lo"
)

type learningSourceRevision struct{ name, revision string }

type learningAdmission struct {
	models                      *externalCallLedger
	mu                          sync.Mutex
	conversationRead, skillRead bool
	source                      learningSourceRevision
	failure                     error
}

// Each learning source may be read once per admission.
func (a *learningAdmission) readConversation() error { return a.consumeRead(&a.conversationRead) }

func (a *learningAdmission) readSkill() error { return a.consumeRead(&a.skillRead) }

func (a *learningAdmission) consumeRead(used *bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if *used {
		return &framecall.Refusal{Code: refusalReadBudget, Message: "skill learning: source read budget exhausted"}
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

func (s *SkillLearning) grant(ctx context.Context, session *sdk.ClientSession, sessionID identity.SessionID, state *learningAdmission) (pluginmcp.CallbackRef, func(), error) {
	conversation, err := s.conversationCallback(sessionID, state)
	if err != nil {
		return pluginmcp.CallbackRef{}, nil, err
	}
	read, err := s.readRevisionCallback(state)
	if err != nil {
		return pluginmcp.CallbackRef{}, nil, err
	}
	model, err := s.modelCallback(state)
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

var learningModelRequests = auxiliaryTextRequest{maxOutputTokens: skilllearning.MaxModelOutputTokens, maxInputBytes: skilllearning.MaxModelInputBytes}

func (a *learningAdmission) admitDraft(candidate skilllearning.Proposal, sessionID identity.SessionID) (skilllearning.Admission, error) {
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
	p := skills.Proposal{Scope: skills.ScopeUser, Origin: skills.ProposalOriginMined, SourceSession: sessionID.String(), Name: candidate.Skill.Name, Description: candidate.Skill.Description, Instructions: candidate.Skill.Instructions, Revises: candidate.Revises}
	if err := p.Validate(); err != nil {
		if errors.Is(err, skills.ErrUnsafeProposal) {
			return skilllearning.Admission{}, nil
		}
		return skilllearning.Admission{}, err
	}
	return skilllearning.Admission{Accepted: true}, nil
}

func (s *SkillLearning) conversationCallback(sessionID identity.SessionID, state *learningAdmission) (pluginmcp.Callback, error) {
	method, err := skilllearning.ConversationMethod()
	if err != nil {
		return pluginmcp.Callback{}, err
	}
	return pluginmcp.NewCallback(method, nil, func(ctx context.Context, _ pluginmcp.Command[struct{}]) (result skilllearning.Conversation, err error) {
		defer func() { state.remember(learningSourceRevision{}, err) }()
		if err := state.readConversation(); err != nil {
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
}

func (s *SkillLearning) modelCallback(state *learningAdmission) (pluginmcp.Callback, error) {
	return pluginsdk.NewModelCallback(func(ctx context.Context, command pluginmcp.Command[scopewire.Request]) (response *chat.Response, err error) {
		defer func() { state.remember(learningSourceRevision{}, err) }()
		if !learningModelRequests.admits(chat.Request(command.Input)) {
			return nil, &framecall.Refusal{Code: refusalModelEnvelope, Message: "skill learning: model envelope refused"}
		}
		input, err := command.RequestInput()
		if err != nil {
			return nil, err
		}
		if err := state.models.admit(string(command.ID), externalModelCall, input); err != nil {
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
}

func (s *SkillLearning) readRevisionCallback(state *learningAdmission) (pluginmcp.Callback, error) {
	readMethod, err := skilllearning.ReadSkillMethod()
	if err != nil {
		return pluginmcp.Callback{}, err
	}
	read, err := pluginmcp.NewCallback(readMethod, nil, func(ctx context.Context, command pluginmcp.Command[skilllearning.ReadSkill]) (result skilllearning.ReadResult, err error) {
		defer func() { state.remember(learningSourceRevision{}, err) }()
		if err := state.readSkill(); err != nil {
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
