package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"iter"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/scope/core/chat"
)

func execute[P, R any](ctx context.Context, s *store, method string, command qimumcp.Command[P], validate func(P) error, call func(context.Context, P) (R, func(), error)) (R, error) {
	var zero R
	input, err := command.RequestInput()
	if err != nil {
		return zero, err
	}
	var request P
	prepared, reply, err := pluginstorage.AdmitCommand(ctx, s.db, method, command.ID, input, func(context.Context) error {
		if err := json.Unmarshal(input.JSON(), &request, json.RejectUnknownMembers(true)); err != nil {
			return err
		}
		if err := validate(request); err != nil {
			return &portable.Refusal{Code: "invalid_input", Message: "model provider: invalid request"}
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
	result, release, err := call(ctx, request)
	if release != nil {
		defer release()
	}
	if err != nil {
		return zero, err
	}
	encoded, err := qimumcp.EncodeReply(result, nil, nil)
	if err != nil {
		return zero, err
	}
	if err := prepared.Complete(context.WithoutCancel(ctx), encoded.JSON()); err != nil {
		return zero, err
	}
	return result, nil
}

func (s *store) call(ctx context.Context, request modelprovider.ChatRequest) (modelprovider.ChatResult, func(), error) {
	entry, leave, err := s.acquire(ctx, request.Handle, "chat")
	if err != nil {
		return modelprovider.ChatResult{Failure: notExecuted(err)}, nil, nil
	}
	if request.Request.Options.Model != "" && request.Request.Options.Model != entry.modelID {
		return modelprovider.ChatResult{Failure: notExecuted(errors.New("model provider: captured model cannot change"))}, leave, nil
	}
	response, err := entry.chat.Call(ctx, request.Request.Clone())
	if err == nil && response == nil {
		err = errors.New("model provider: missing response")
	}
	return modelprovider.ChatResult{Response: response, Failure: failure(err)}, leave, nil
}

func (s *store) count(ctx context.Context, request modelprovider.ChatRequest) (modelprovider.TokenCount, func(), error) {
	entry, leave, err := s.acquire(ctx, request.Handle, "chat")
	if err != nil {
		return modelprovider.TokenCount{Failure: notExecuted(err)}, nil, nil
	}
	if entry.count == nil || (request.Request.Options.Model != "" && request.Request.Options.Model != entry.modelID) {
		return modelprovider.TokenCount{Failure: notExecuted(errors.New("model provider: captured counter is unavailable"))}, leave, nil
	}
	count, err := entry.count.CountInputTokens(ctx, request.Request.Clone())
	if err == nil && count < 0 {
		err = errors.New("model provider: negative input count")
	}
	if err != nil {
		return modelprovider.TokenCount{Failure: failure(err)}, leave, nil
	}
	return modelprovider.TokenCount{Count: &count}, leave, nil
}

func (s *store) embed(ctx context.Context, request modelprovider.EmbeddingRequest) (modelprovider.EmbeddingResult, func(), error) {
	input, err := request.Native()
	if err != nil {
		return modelprovider.EmbeddingResult{Failure: notExecuted(err)}, nil, nil
	}
	entry, leave, err := s.acquire(ctx, request.Handle, "embedding")
	if err != nil {
		return modelprovider.EmbeddingResult{Failure: notExecuted(err)}, nil, nil
	}
	if input.Options.Model != entry.modelID {
		return modelprovider.EmbeddingResult{Failure: notExecuted(errors.New("model provider: captured model cannot change"))}, leave, nil
	}
	response, err := entry.embed.Call(ctx, input)
	result := modelprovider.EmbeddingResult{Failure: failure(err)}
	if response != nil {
		result.Response, err = json.Marshal(response)
		if err != nil {
			return modelprovider.EmbeddingResult{}, leave, err
		}
	} else if err == nil {
		result.Failure = failure(errors.New("model provider: missing embedding response"))
	}
	return result, leave, nil
}

func (s *store) stream(ctx context.Context, method string, command qimumcp.Command[modelprovider.ChatRequest]) iter.Seq2[qimumcp.StreamValue[chat.ResponseDelta, modelprovider.ChatResult], error] {
	return func(yield func(qimumcp.StreamValue[chat.ResponseDelta, modelprovider.ChatResult], error) bool) {
		continuing := true
		emit := func(delta chat.ResponseDelta) bool {
			continuing = yield(qimumcp.StreamValue[chat.ResponseDelta, modelprovider.ChatResult]{Chunk: &delta}, nil)
			return continuing
		}
		result, err := execute(ctx, s, method, command, modelprovider.ChatRequest.Validate, func(ctx context.Context, request modelprovider.ChatRequest) (modelprovider.ChatResult, func(), error) {
			entry, leave, err := s.acquire(ctx, request.Handle, "chat")
			if err != nil {
				return modelprovider.ChatResult{Failure: notExecuted(err)}, nil, nil
			}
			if entry.stream == nil || (request.Request.Options.Model != "" && request.Request.Options.Model != entry.modelID) {
				return modelprovider.ChatResult{Failure: notExecuted(errors.New("model provider: captured stream is unavailable"))}, leave, nil
			}
			var accumulated chat.ResponseAccumulator
			sequence := entry.stream.Stream(ctx, request.Request.Clone())
			if sequence == nil {
				err = errors.New("model provider: missing stream")
			} else {
				for delta, cause := range sequence {
					if cause != nil {
						err = cause
						break
					}
					if cause := accumulated.Add(delta); cause != nil {
						err = cause
						break
					}
					if !emit(*delta.Clone()) {
						err = context.Canceled
						break
					}
				}
			}
			response, responseErr := accumulated.Response()
			if responseErr != nil {
				response = nil
				err = errors.Join(err, responseErr)
			}
			return modelprovider.ChatResult{Response: response, Failure: failure(err)}, leave, nil
		})
		if !continuing {
			return
		}
		if err != nil {
			yield(qimumcp.StreamValue[chat.ResponseDelta, modelprovider.ChatResult]{}, err)
			return
		}
		yield(qimumcp.StreamValue[chat.ResponseDelta, modelprovider.ChatResult]{Result: &result}, nil)
	}
}

func notExecuted(err error) *modelprovider.Failure {
	result := failure(err)
	result.Execution = chat.CallFailureNotExecuted
	return result
}

func failure(err error) *modelprovider.Failure {
	if err == nil {
		return nil
	}
	result := &modelprovider.Failure{Kind: modelprovider.Internal, Execution: chat.ClassifyFailure(err)}
	switch {
	case errors.Is(err, context.Canceled):
		result.Kind = modelprovider.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		result.Kind = modelprovider.Timeout
	default:
		type httpError interface {
			error
			HTTPStatus() int
			HTTPHeader() http.Header
		}
		if matched, ok := errors.AsType[httpError](err); ok {
			switch status := matched.HTTPStatus(); {
			case status == http.StatusUnauthorized || status == http.StatusForbidden:
				result.Kind = modelprovider.InvalidCredentials
			case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
				result.Kind = modelprovider.Timeout
			case status == http.StatusTooManyRequests:
				result.Kind = modelprovider.RateLimited
			case status >= 500:
				result.Kind = modelprovider.ProviderUnavailable
			case status >= 400:
				result.Kind = modelprovider.ProviderRejected
			}
			if result.Kind == modelprovider.RateLimited || result.Kind == modelprovider.ProviderUnavailable {
				result.RetryAfterMillis = retryAfterMillis(matched.HTTPHeader(), time.Now())
			}
		} else if network, ok := errors.AsType[net.Error](err); ok {
			result.Kind = modelprovider.ProviderUnavailable
			if network.Timeout() {
				result.Kind = modelprovider.Timeout
			}
		}
	}
	return result
}

func retryAfterMillis(header http.Header, now time.Time) int64 {
	for _, value := range []struct {
		name string
		unit float64
	}{{"Retry-After-Ms", 1}, {"Retry-After", 1000}} {
		text := header.Get(value.name)
		if quantity, err := strconv.ParseFloat(text, 64); err == nil && quantity >= 0 && quantity*value.unit < 1<<63 {
			return int64(quantity * value.unit)
		}
		if value.name == "Retry-After" {
			if when, err := http.ParseTime(text); err == nil {
				return max(0, when.Sub(now).Milliseconds())
			}
		}
	}
	return 0
}
