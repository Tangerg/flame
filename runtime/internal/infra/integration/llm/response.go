package llm

import (
	"errors"
	"net/http"

	"github.com/Tangerg/flame/runtime/transport/httpredirect"
	"github.com/Tangerg/flame/runtime/transport/httpresponse"
)

const maxModelResponseFrameBytes int64 = 64 << 20

var errModelResponseFrameTooLarge = errors.New("llm: response frame too large")

func newModelHTTPClient() *http.Client {
	client := httpresponse.NewClient(maxModelResponseFrameBytes, errModelResponseFrameTooLarge)
	client.CheckRedirect = httpredirect.CheckRedirect
	return client
}
