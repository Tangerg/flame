package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
	"github.com/google/uuid"
)

func (p *ModelProviders) OpenEmbedding(ctx context.Context, selection modelchoice.Selection, connection modelprovider.Connection) (_ models.ResolvedEmbedding, returned error) {
	use, profile, err := p.openUse(ctx, connection)
	if err != nil {
		return models.ResolvedEmbedding{}, err
	}
	defer func() {
		if returned != nil {
			returned = errors.Join(returned, use.Close(ctx))
		}
	}()
	ctx, leave, err := use.enter(ctx)
	if err != nil {
		return models.ResolvedEmbedding{}, err
	}
	defer leave()
	if profile.Embedding == nil {
		return models.ResolvedEmbedding{}, modelapp.ErrEmbeddingUnsupported
	}
	method, err := modelprovider.OpenEmbeddingMethod()
	if err != nil {
		return models.ResolvedEmbedding{}, err
	}
	opened, err := providerMethod(ctx, use, method, uuid.NewString(), modelprovider.Open{Connection: connection, Model: selection.Model()})
	if err != nil {
		return models.ResolvedEmbedding{}, err
	}
	use.handle = opened.Handle
	if err := opened.Validate(); err != nil || opened.Provider != selection.Provider() || opened.Model != selection.Model() {
		return models.ResolvedEmbedding{}, errors.New("model providers: invalid embedding construction")
	}
	return models.NewResolvedEmbedding(&providerEmbedding{use: use}, opened.Space, models.Pricing(profile.ID, profile.Models), use.Close)
}

func (p *ModelProviders) RemoteModels(ctx context.Context, connection modelprovider.Connection) (_ []string, returned error) {
	use, profile, err := p.openUse(ctx, connection)
	if err != nil {
		return nil, err
	}
	defer func() { returned = errors.Join(returned, use.Close(ctx)) }()
	if profile.ModelSource != modelprovider.EndpointModels {
		return nil, errors.New("model providers: provider uses its published model catalog")
	}
	method, err := modelprovider.RemoteModelsMethod()
	if err != nil {
		return nil, err
	}
	return providerMethod(ctx, use, method, "", connection)
}

type providerChat struct{ use *providerUse }

func (p *providerChat) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	if err := p.use.admitRequest(ctx, request); err != nil {
		return nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err}
	}
	if err := request.Validate(); err != nil {
		return nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err}
	}
	caller, err := qimumcp.InvocationFromContext(ctx)
	if err != nil {
		return nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err}
	}
	method, err := modelprovider.CallMethod()
	if err != nil {
		return nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err}
	}
	result, err := providerMethod(ctx, p.use, method, caller.ID, modelprovider.ChatRequest{Handle: p.use.handle, Request: *request.Clone()})
	if err != nil {
		return nil, unconfirmedProvider(err)
	}
	if err := result.Validate(); err != nil {
		return result.Response, unconfirmedProvider(err)
	}
	return result.Response, providerFailure(result.Failure)
}

type streamingProviderChat struct{ *providerChat }

func (p *streamingProviderChat) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	request = request.Clone()
	return func(yield func(*chat.ResponseDelta, error) bool) {
		if err := p.use.admitRequest(ctx, request); err != nil {
			yield(nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err})
			return
		}
		if err := request.Validate(); err != nil {
			yield(nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err})
			return
		}
		caller, err := qimumcp.InvocationFromContext(ctx)
		if err != nil {
			yield(nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err})
			return
		}
		calling, leave, err := p.use.enter(ctx)
		if err != nil {
			yield(nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err})
			return
		}
		defer leave()
		calling, err = providerContext(calling, p.use.binding, caller.ID)
		if err != nil {
			yield(nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err})
			return
		}
		borrowed, err := borrowContribution(calling, p.use.owner.config.Connections, p.use.binding)
		if err != nil {
			yield(nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err})
			return
		}
		continuing := true
		defer func() {
			if err := borrowed.Close(); err != nil && continuing {
				yield(nil, err)
			}
		}()
		method, err := modelprovider.StreamMethod()
		if err != nil {
			yield(nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err})
			return
		}
		exported, err := qimumcp.NewCapabilityBinding(borrowed.Session(), p.use.binding.Export, p.use.owner.contract)
		if err != nil {
			yield(nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err})
			return
		}
		binding, err := exported.Method(method.Descriptor().Name)
		if err != nil {
			yield(nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err})
			return
		}
		input := modelprovider.ChatRequest{Handle: p.use.handle, Request: *request}
		limits := qimumcp.CallbackLimits{MaxRequestBytes: sdk.DefaultMaxLineLength, MaxResponseBytes: sdk.DefaultMaxLineLength}
		chunks, complete := 0, false
		for value, err := range method.StreamWithMetadata(borrowed.Context(), binding, p.use.owner.config.Callbacks, limits, caller.ID, input, sdk.ParamsBase{}) {
			if err != nil {
				continuing = yield(nil, unconfirmedProvider(err))
				return
			}
			if value.Chunk != nil {
				chunks++
				continuing = yield(value.Chunk.Clone(), nil)
				if !continuing {
					return
				}
				continue
			}
			if value.Result == nil {
				continuing = yield(nil, unconfirmedProvider(errors.New("model provider: missing stream outcome")))
				return
			}
			complete = true
			if err := errors.Join(value.Result.Validate(), providerFailure(value.Result.Failure)); err != nil {
				continuing = yield(nil, err)
				return
			}
		}
		if !complete || chunks == 0 {
			continuing = yield(nil, unconfirmedProvider(errors.New("model provider: original stream delivery is unconfirmed")))
		}
	}
}

type providerCounter struct{ use *providerUse }

func (p *providerCounter) CountInputTokens(ctx context.Context, request *chat.Request) (int64, error) {
	if err := p.use.admitRequest(ctx, request); err != nil {
		return 0, err
	}
	if err := request.Validate(); err != nil {
		return 0, err
	}
	method, err := modelprovider.CountMethod()
	if err != nil {
		return 0, err
	}
	result, err := providerMethod(ctx, p.use, method, uuid.NewString(), modelprovider.ChatRequest{Handle: p.use.handle, Request: *request.Clone()})
	if err != nil {
		return 0, err
	}
	if err := errors.Join(result.Validate(), providerFailure(result.Failure)); err != nil {
		return 0, err
	}
	return *result.Count, nil
}

type providerEmbedding struct{ use *providerUse }

func (p *providerEmbedding) Call(ctx context.Context, request *embedding.Request) (*embedding.Response, error) {
	if err := request.Validate(); err != nil {
		return nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err}
	}
	caller, err := qimumcp.InvocationFromContext(ctx)
	if err != nil {
		return nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err}
	}
	method, err := modelprovider.EmbedMethod()
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err}
	}
	input := modelprovider.EmbeddingRequest{Handle: p.use.handle, Request: encoded}
	admitted, err := jsonvalue.Encode(input)
	if err != nil {
		return nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err}
	}
	target, err := providerContext(ctx, p.use.binding, caller.ID)
	if err != nil {
		return nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: err}
	}
	fits, err := p.use.frameFits(target, method.Name(), caller.ID, admitted)
	if err != nil || !fits {
		return nil, &chat.CallFailure{Kind: chat.CallFailureNotExecuted, Cause: errors.Join(err, errors.New("model provider: embedding request exceeds transport capacity"))}
	}
	result, err := providerMethod(ctx, p.use, method, caller.ID, input)
	if err != nil {
		return nil, unconfirmedProvider(err)
	}
	if err := result.Validate(); err != nil {
		return nil, unconfirmedProvider(err)
	}
	response, err := result.Native()
	return response, errors.Join(err, providerFailure(result.Failure))
}

func providerFailure(failure *modelprovider.Failure) error {
	if failure == nil {
		return nil
	}
	if err := failure.Validate(); err != nil {
		return unconfirmedProvider(err)
	}
	kind := run.FailureInternal
	cause := errors.New("model provider: " + string(failure.Kind))
	switch failure.Kind {
	case modelprovider.Canceled:
		cause = context.Canceled
	case modelprovider.Timeout:
		kind, cause = run.FailureTimeout, context.DeadlineExceeded
	case modelprovider.InvalidCredentials:
		kind = run.FailureInvalidCredentials
	case modelprovider.RateLimited:
		kind = run.FailureRateLimited
	case modelprovider.ProviderRejected:
		kind = run.FailureProviderRejected
	case modelprovider.ProviderUnavailable:
		kind = run.FailureProviderUnavailable
	}
	delay := time.Duration(min(failure.RetryAfterMillis, run.MaximumRetryAfter.Milliseconds())) * time.Millisecond
	return &chat.CallFailure{Kind: failure.Execution, Cause: &run.FailureError{Kind: kind, RetryAfter: delay, Err: cause}}
}

func unconfirmedProvider(err error) error {
	return &chat.CallFailure{Kind: chat.CallFailureUnknown, Cause: &run.FailureError{Kind: run.FailureProviderUnavailable, Err: fmt.Errorf("model provider: exchange is unconfirmed: %w", err)}}
}

func (u *providerUse) futureRequestFits(ctx context.Context, request *chat.Request) (bool, error) {
	caller, err := qimumcp.InvocationFromContext(ctx)
	if err != nil {
		return false, err
	}
	id := "endpoint_00000000-0000-0000-0000-000000000000"
	if caller.ID == id {
		id = "endpoint_00000000-0000-0000-0000-000000000001"
	}
	ctx, err = qimumcp.WithInvocation(ctx, portable.Invocation{ID: id, Origin: caller.Origin, Parent: caller.ID, Target: portable.Target{Scope: extension.ScopeCall, Reference: id}})
	if err != nil {
		return false, err
	}
	return u.requestFits(ctx, request)
}
func (u *providerUse) requestFits(ctx context.Context, request *chat.Request) (bool, error) {
	if request == nil {
		return false, errors.New("model provider: request is required")
	}
	caller, err := qimumcp.InvocationFromContext(ctx)
	if err != nil {
		return false, err
	}
	input, err := jsonvalue.Encode(modelprovider.ChatRequest{Handle: u.handle, Request: *request.Clone()})
	if err != nil {
		return false, err
	}
	ctx, err = providerContext(ctx, u.binding, caller.ID)
	if err != nil {
		return false, err
	}
	for _, name := range u.requestMethods {
		fits, err := u.frameFits(ctx, name, caller.ID, input)
		if err != nil || !fits {
			return fits, err
		}
	}
	return true, nil
}
func (u *providerUse) admitRequest(ctx context.Context, request *chat.Request) error {
	fits, err := u.requestFits(ctx, request)
	if err != nil {
		return err
	}
	if !fits {
		return errors.New("model provider: complete request exceeds transport capacity")
	}
	return nil
}

func (u *providerUse) frameFits(ctx context.Context, name, id string, input jsonvalue.Value) (bool, error) {
	size, err := qimumcp.ProcessRequestFrameBytes(ctx, u.owner.contract, u.binding.Export, name, id, input, sdk.ParamsBase{})
	return size <= sdk.DefaultMaxLineLength, err
}
