package httpresponse

import "net/http"

// NewClient returns an independent client that admits response bodies through
// one bounded document-or-event stream before the caller's protocol decoder.
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
		LimitBody(response, r.maxFrameBytes, r.tooLarge)
	}
	return response, err
}
