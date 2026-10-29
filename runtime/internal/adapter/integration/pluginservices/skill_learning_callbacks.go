package pluginservices

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"sync"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	skillspec "github.com/Tangerg/scope/skills"
	"github.com/samber/lo"
)

type learningAdmission struct {
	mu                     sync.Mutex
	modelCalls             map[string]jsonvalue.Value
	historyRead, skillRead bool
	sourceName             string
	failure                error
}

func (a *learningAdmission) model(id string, input jsonvalue.Value) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if original, found := a.modelCalls[id]; found {
		if original != input {
			return &portable.Refusal{Code: "command_conflict", Message: portable.ErrInvocationConflict.Error()}
		}
		return &portable.Refusal{Code: "model_redelivery", Message: "skill learning: original model invocation cannot be redelivered"}
	}
	if len(a.modelCalls) >= skilllearning.MaxModelCalls {
		return &portable.Refusal{Code: "model_budget", Message: "skill learning: model budget exhausted"}
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
		return &portable.Refusal{Code: "read_budget", Message: "skill learning: source read budget exhausted"}
	}
	*used = true
	return nil
}

func (a *learningAdmission) remember(name string, failure error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if name != "" {
		a.sourceName = name
	}
	a.failure = errors.Join(a.failure, failure)
}

func (a *learningAdmission) outcome() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sourceName, a.failure
}

func (s *SkillLearning) grant(ctx context.Context, session *sdk.ClientSession, sessionID string, state *learningAdmission) (qimumcp.CallbackRef, func(), error) {
	conversationMethod, err := skilllearning.ConversationMethod()
	if err != nil {
		return qimumcp.CallbackRef{}, nil, err
	}
	conversation, err := qimumcp.NewCallback(conversationMethod, nil, func(ctx context.Context, _ qimumcp.Command[struct{}]) (result skilllearning.Conversation, err error) {
		defer func() { state.remember("", err) }()
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
		return qimumcp.CallbackRef{}, nil, err
	}
	readMethod, err := skilllearning.ReadSkillMethod()
	if err != nil {
		return qimumcp.CallbackRef{}, nil, err
	}
	read, err := qimumcp.NewCallback(readMethod, nil, func(ctx context.Context, command qimumcp.Command[skilllearning.ReadSkill]) (result skilllearning.ReadResult, err error) {
		defer func() { state.remember("", err) }()
		if err := state.read(true); err != nil {
			return result, err
		}
		current, err := s.source.Load(ctx, command.Input.Name)
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
		state.remember(current.Name, nil)
		return skilllearning.ReadResult{Skill: &skilllearning.Skill{Name: current.Name, Description: current.Description, Instructions: current.Instructions}}, nil
	})
	if err != nil {
		return qimumcp.CallbackRef{}, nil, err
	}
	model, err := pluginsdk.NewModelCallback(func(ctx context.Context, command qimumcp.Command[chat.Request]) (response *chat.Response, err error) {
		defer func() { state.remember("", err) }()
		if !learningModelRequest(command.Input) {
			return nil, &portable.Refusal{Code: "model_envelope", Message: "skill learning: model envelope refused"}
		}
		input, err := command.RequestInput()
		if err != nil {
			return nil, err
		}
		if err := state.model(command.ID, input); err != nil {
			return nil, err
		}
		captured, release, err := s.utility(ctx, models.AuxiliaryBudget{MaxInputBytes: skilllearning.MaxModelInputBytes, MaxOutputTokens: skilllearning.MaxModelOutputTokens})
		if err != nil {
			return nil, err
		}
		if release == nil {
			return nil, errors.New("skill learning: model release is required")
		}
		defer release()
		if lo.IsNil(captured.Model()) {
			return nil, errors.New("skill learning: model is required")
		}
		return captured.Model().Call(ctx, command.Input.Clone())
	})
	if err != nil {
		return qimumcp.CallbackRef{}, nil, err
	}
	return s.callbacks.Open(ctx, session, qimumcp.CallbackLimits{MaxRequestBytes: skilllearning.MaxCallbackBytes, MaxResponseBytes: skilllearning.MaxCallbackBytes}, conversation, read, model)
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
