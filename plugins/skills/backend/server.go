package main

import (
	"context"
	json "encoding/json/v2"
	"errors"

	framemcp "github.com/Tangerg/frame/process/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func newServer(learning *learner) (*sdk.Server, error) {
	if learning == nil {
		return nil, errors.New("skill learning: learner is required")
	}
	contract, err := skilllearning.Contract()
	if err != nil {
		return nil, err
	}
	mine, err := skilllearning.MineMethod()
	if err != nil {
		return nil, err
	}
	archive, err := skilllearning.ArchiveMethod()
	if err != nil {
		return nil, err
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.skills", Version: "5.0.0"}, nil)
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, learning.db),
		registerReviews(backend, learning.db),
		registerDiscovery(backend, learning.db),
		mine.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[skilllearning.Run]) (skilllearning.MineResult, error) {
			return learningCommand(ctx, learning, mine.Name(), command, skilllearning.Run.Validate, func(ctx context.Context, run skilllearning.Run) (skilllearning.MineResult, error) {
				result, err := learning.mine(ctx, run, remoteInputs{})
				if err != nil || result.Proposal == nil {
					return result, err
				}
				allowed, err := admitMined(ctx, *result.Proposal)
				if err != nil || !allowed {
					return skilllearning.MineResult{}, err
				}
				return result, nil
			}, func(ctx context.Context, run skilllearning.Run, result skilllearning.MineResult) error {
				return learning.recordMined(ctx, string(command.ID), run, result)
			})

		}),
		archive.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[skilllearning.ArchiveRequest]) (skilllearning.ArchiveAdvice, error) {
			return learningCommand(ctx, learning, archive.Name(), command, func(request skilllearning.ArchiveRequest) error {
				if request.Now.IsZero() {
					return errors.New("skill learning: archive time is required")
				}
				return nil
			}, func(_ context.Context, request skilllearning.ArchiveRequest) (skilllearning.ArchiveAdvice, error) {
				return learning.archive(request.Now), nil
			}, nil)
		}),
		backend.BindCapability("skill-learning", contract),
	); err != nil {
		return nil, err
	}
	if err := backend.Publish(); err != nil {
		return nil, err
	}
	return server, nil
}

func learningCommand[P, R any](ctx context.Context, l *learner, method string, command pluginmcp.Command[P], validate func(P) error, execute func(context.Context, P) (R, error), record func(context.Context, P, R) error) (R, error) {
	var zero R
	input, err := command.RequestInput()
	if err != nil {
		return zero, err
	}
	var request P
	prepared, reply, err := pluginstorage.AdmitCommand(ctx, l.db, method, string(command.ID), input, func(context.Context) error {
		if err := json.Unmarshal(input.Bytes(), &request, json.RejectUnknownMembers(true)); err != nil {
			return err
		}
		if err := validate(request); err != nil {
			return &framemcp.Refusal{Code: "invalid_input", Message: err.Error()}
		}
		return nil
	})
	if err != nil {
		return zero, err
	}
	if len(reply) != 0 {
		return pluginmcp.DecodeReply[R](reply)
	}
	if !prepared.NewlyAdmitted() {
		return zero, pluginstorage.ErrCommandUnconfirmed
	}
	value, err := execute(ctx, request)
	if errors.Is(err, errInvalidSkillDraft) {
		refusal := &framemcp.Refusal{Code: "invalid_skill_draft", Message: errInvalidSkillDraft.Error()}
		reply, encodeErr := pluginmcp.EncodeReply(zero, refusal, nil)
		if encodeErr != nil {
			return zero, encodeErr
		}
		if settleErr := prepared.SettleExternal(ctx, reply.Bytes(), nil); settleErr != nil {
			return zero, settleErr
		}
		return zero, refusal
	}
	if err != nil {
		return zero, err
	}
	outcome, err := pluginmcp.EncodeReply(value, nil, nil)
	if err != nil {
		return zero, err
	}
	var commit func(context.Context) error
	if record != nil {
		commit = func(tx context.Context) error { return record(tx, request, value) }
	}
	if err := prepared.SettleExternal(ctx, outcome.Bytes(), commit); err != nil {
		if _, known := errors.AsType[*framemcp.Refusal](err); known {
			refusal, encodeErr := pluginmcp.EncodeReply(zero, err, nil)
			if encodeErr != nil {
				return zero, encodeErr
			}
			if settleErr := prepared.SettleExternal(ctx, refusal.Bytes(), nil); settleErr != nil {
				return zero, settleErr
			}
		}
		return zero, err
	}
	return value, nil
}

func (l *learner) recordMined(ctx context.Context, id string, run skilllearning.Run, result skilllearning.MineResult) error {
	if result.Proposal == nil {
		return nil
	}
	_, err := (reviewQueue{db: l.db}).stageCommand(ctx, id, result.Proposal.Draft(run))
	return err
}
