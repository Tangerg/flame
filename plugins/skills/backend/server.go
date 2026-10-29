package main

import (
	"context"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
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
	server := sdk.NewServer(&sdk.Implementation{Name: "flame.skills", Version: "3.0.0"}, nil)
	backend, err := qimumcp.NewBackend(server)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, learning.db),
		mine.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[skilllearning.Run]) (skilllearning.MineResult, error) {
			return learningCommand(ctx, learning, mine.Name(), command, skilllearning.Run.Validate, func(ctx context.Context, run skilllearning.Run) (skilllearning.MineResult, error) {
				return learning.mine(ctx, run, remoteInputs{})
			})
		}),
		archive.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[skilllearning.ArchiveRequest]) (skilllearning.ArchiveAdvice, error) {
			return learningCommand(ctx, learning, archive.Name(), command, func(request skilllearning.ArchiveRequest) error {
				if request.Now.IsZero() {
					return errors.New("skill learning: archive time is required")
				}
				return nil
			}, func(_ context.Context, request skilllearning.ArchiveRequest) (skilllearning.ArchiveAdvice, error) {
				return learning.archive(request.Now), nil
			})
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

func learningCommand[P, R any](ctx context.Context, l *learner, method string, command qimumcp.Command[P], validate func(P) error, execute func(context.Context, P) (R, error)) (R, error) {
	var zero R
	input, err := command.RequestInput()
	if err != nil {
		return zero, err
	}
	var request P
	prepared, reply, err := pluginstorage.AdmitCommand(ctx, l.db, method, command.ID, input, func(context.Context) error {
		if err := json.Unmarshal(input.JSON(), &request, json.RejectUnknownMembers(true)); err != nil {
			return err
		}
		if err := validate(request); err != nil {
			return &portable.Refusal{Code: "invalid_input", Message: err.Error()}
		}
		return nil
	})
	if err != nil {
		return zero, err
	}
	if len(reply) != 0 {
		return qimumcp.DecodeReply[R](reply)
	}
	if !prepared.NewlyAdmitted() {
		return zero, pluginstorage.ErrCommandUnconfirmed
	}
	value, err := execute(ctx, request)
	if err != nil {
		return zero, err
	}
	outcome, err := qimumcp.EncodeReply(value, nil, nil)
	if err != nil {
		return zero, err
	}
	if err := prepared.Complete(ctx, outcome.JSON()); err != nil {
		return zero, err
	}
	return value, nil
}
