package delivery

import (
	"context"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
	framecall "github.com/Tangerg/frame/call"
)

type skillReviewAction func(*workspace.Skills, context.Context, workspace.SkillReviewTarget) error

func registerSkillReview(registry *Registry, method Name, goName string, apply skillReviewAction) {
	registry.commandAck(MethodMeta{
		Name: method, GoName: goName,
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error(), protocol.ErrRevisionConflict.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
	}, func(s *Handler, ctx context.Context, in protocol.SkillProposalRef) error {
		review, err := s.prepareSkillReview(in)
		if err != nil {
			return mapSkillProposalErr(err)
		}
		return mapSkillProposalErr(apply(s.workspaceSkills, ctx, review))
	}).withOwnedReplay(func(s *Handler, ctx context.Context, in protocol.SkillProposalRef, call keyedCall) protocol.OperationResult {
		payload, err := call.replay.InvokeDeferred(ctx, call.key, call.fingerprint, func(complete *operation.DeferredCompletion) ([]byte, error) {
			review, err := s.prepareSkillReview(in)
			if err != nil {
				return encodeAcknowledgement(mapSkillProposalErr(err))
			}
			data, err := json.Marshal(review)
			if err != nil {
				return nil, err
			}
			return nil, complete.Stage(ctx, func(context.Context) ([]byte, error) { return data, nil })
		}, func(data []byte, recovering bool) ([]byte, error) {
			var review workspace.SkillReviewTarget
			if err := json.Unmarshal(data, &review, json.RejectUnknownMembers(true)); err != nil {
				return nil, err
			}
			if err := review.Validate(); err != nil {
				return nil, err
			}
			return settleSkillReview(apply(s.workspaceSkills, ctx, review), recovering)
		})
		return call.result(ctx, payload, err)
	})
}

func (s *Handler) prepareSkillReview(in protocol.SkillProposalRef) (workspace.SkillReviewTarget, error) {
	ref, err := skillProposalRef(in)
	if err != nil {
		return workspace.SkillReviewTarget{}, err
	}
	return s.workspaceSkills.ResolveReviewTarget(in.Workspace.Path, ref)
}

func settleSkillReview(err error, recovering bool) ([]byte, error) {
	if err == nil {
		return encodeAcknowledgement(nil)
	}
	if errors.Is(err, framecall.ErrConflict) {
		return nil, err
	}
	if errors.Is(err, plugin.ErrCommandUnconfirmed) || errors.Is(err, skills.ErrPublicationUnconfirmed) {
		return nil, errors.Join(operation.ErrOutcomePending, err)
	}
	if recovering && (errors.Is(err, plugin.ErrUnavailable) || errors.Is(err, plugin.ErrDisabled) || errors.Is(err, plugin.ErrStale)) {
		return nil, errors.Join(operation.ErrOutcomePending, err)
	}
	failure := projectError(mapSkillProposalErr(err))
	if errors.Is(failure, protocol.ErrInternalError) {
		if _, refused := errors.AsType[*framecall.Refusal](err); !refused {
			return nil, errors.Join(operation.ErrOutcomePending, err)
		}
	}
	return encodeStoredOutcome(failed(failure))
}
