package httpresponse

import (
	"context"
	"net/http"
)

type eventStreamKey struct{}

func WithEventStream(ctx context.Context) context.Context {
	return context.WithValue(ctx, eventStreamKey{}, DocumentOrEvents)
}

func NewClient(maxFrameBytes int64, tooLarge error) *http.Client {
	validateFrameLimit(maxFrameBytes, tooLarge)
	client := *http.DefaultClient
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = responseRoundTripper{
		base:          base,
		maxFrameBytes: maxFrameBytes,
		tooLarge:      tooLarge,
	}
	return &client
}

type responseRoundTripper struct {
	base          http.RoundTripper
	maxFrameBytes int64
	tooLarge      error
}

func (r responseRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := r.base.RoundTrip(request)
	if response != nil {
		mode, _ := request.Context().Value(eventStreamKey{}).(BodyMode)
		LimitBody(response, mode, r.maxFrameBytes, r.tooLarge)
	}
	return response, err
}
