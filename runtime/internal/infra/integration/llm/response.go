package llm

import (
	"errors"
	"net/http"

	"github.com/Tangerg/flame/runtime/internal/infra/integration/httpresponse"
)

const maxModelResponseFrameBytes int64 = 64 << 20

var errModelResponseFrameTooLarge = errors.New("llm: response frame too large")

func newModelHTTPClient() *http.Client {
	client := *http.DefaultClient
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = modelResponseRoundTripper{base: base, maxFrameBytes: maxModelResponseFrameBytes}
	return &client
}

type modelResponseRoundTripper struct {
	base          http.RoundTripper
	maxFrameBytes int64
}

func (m modelResponseRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := m.base.RoundTrip(request)
	if response != nil {
		httpresponse.LimitBody(response, m.maxFrameBytes, errModelResponseFrameTooLarge)
	}
	return response, err
}
