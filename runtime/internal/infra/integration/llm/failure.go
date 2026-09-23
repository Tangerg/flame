package llm

import (
	"context"
	"errors"
	"iter"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"
)

// InputTokenCounter is the optional complete-request counting capability of a
// provider model. It stays separate from chatclient, whose contract owns only
// complete and streaming model calls.
type InputTokenCounter interface {
	CountInputTokens(context.Context, *chat.Request) (int64, error)
}

// classifyModelFailures translates provider-specific errors at the
// infrastructure boundary. The rest of the runtime sees one typed execution
// failure taxonomy and never parses provider error strings.
func classifyModelFailures(model chat.Model) (chat.Model, error) {
	return decorateModel(model, modelDecoration{
		name:   "classify model failures",
		call:   func(inner chat.Model) chat.Model { return failureModel{model: inner} },
		stream: func(inner chat.Streamer) chat.Streamer { return failureStreamer{streamer: inner} },
		count:  func(inner InputTokenCounter) InputTokenCounter { return failureCounter{counter: inner} },
	})
}

type failureModel struct{ model chat.Model }

func (f failureModel) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	response, err := f.model.Call(ctx, request)
	return response, classifyModelError(err)
}

type failureStreamer struct{ streamer chat.Streamer }

func (f failureStreamer) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.ResponseDelta, error] {
	return classifyModelStream(f.streamer.Stream(ctx, request))
}

type failureCounter struct{ counter InputTokenCounter }

func (f failureCounter) CountInputTokens(ctx context.Context, request *chat.Request) (int64, error) {
	count, err := f.counter.CountInputTokens(ctx, request)
	return count, classifyModelError(err)
}

func classifyModelStream(sequence iter.Seq2[*chat.ResponseDelta, error]) iter.Seq2[*chat.ResponseDelta, error] {
	if sequence == nil {
		return nil
	}
	return func(yield func(*chat.ResponseDelta, error) bool) {
		for response, err := range sequence {
			if !yield(response, classifyModelError(err)) {
				return
			}
		}
	}
}

func classifyModelError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return err
	}
	if _, ok := errors.AsType[*run.FailureError](err); ok {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &run.FailureError{Kind: run.FailureTimeout, Err: err}
	}
	if status, header, ok := providerHTTPError(err); ok {
		return classifyHTTPFailure(status, header, err)
	}
	if netErr, ok := errors.AsType[net.Error](err); ok {
		kind := run.FailureProviderUnavailable
		if netErr.Timeout() {
			kind = run.FailureTimeout
		}
		return &run.FailureError{Kind: kind, Err: err}
	}
	return err
}

func classifyHTTPFailure(status int, header http.Header, err error) error {
	kind := failureKindForHTTPStatus(status)
	var delay time.Duration
	if kind.AllowsRetryAfter() {
		delay = retryAfter(header, time.Now())
	}
	return &run.FailureError{Kind: kind, RetryAfter: delay, Err: err}
}

func providerHTTPError(err error) (int, http.Header, bool) {
	type httpError interface {
		error
		HTTPStatus() int
		HTTPHeader() http.Header
	}
	matched, ok := errors.AsType[httpError](err)
	if !ok {
		return 0, nil, false
	}
	return matched.HTTPStatus(), matched.HTTPHeader(), true
}

func failureKindForHTTPStatus(status int) run.FailureKind {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return run.FailureInvalidCredentials
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		return run.FailureTimeout
	case status == http.StatusTooManyRequests:
		return run.FailureRateLimited
	case status >= http.StatusInternalServerError:
		return run.FailureProviderUnavailable
	case status >= http.StatusBadRequest:
		return run.FailureProviderRejected
	default:
		return run.FailureInternal
	}
}

func retryAfter(header http.Header, now time.Time) time.Duration {
	for _, candidate := range []struct {
		name string
		unit time.Duration
	}{
		{name: "Retry-After-Ms", unit: time.Millisecond},
		{name: "Retry-After", unit: time.Second},
	} {
		value := strings.TrimSpace(header.Get(candidate.name))
		if value == "" {
			continue
		}
		if delay, ok := numericRetryAfter(value, candidate.unit); ok {
			return delay
		}
		if candidate.name == "Retry-After" {
			when, err := http.ParseTime(value)
			if err == nil {
				return min(max(0, when.Sub(now)), run.MaximumRetryAfter)
			}
		}
	}
	return 0
}

func numericRetryAfter(value string, unit time.Duration) (time.Duration, bool) {
	quantity, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(quantity) || math.IsInf(quantity, 0) || quantity < 0 {
		return 0, false
	}
	scaled := quantity * float64(unit)
	if scaled >= float64(run.MaximumRetryAfter) {
		return run.MaximumRetryAfter, true
	}
	return time.Duration(scaled), true
}
