package llm

import (
	"errors"
	"net/http"

	"github.com/Tangerg/flame/runtime/internal/infra/integration/httpresponse"
)

const maxModelResponseFrameBytes int64 = 64 << 20

var errModelResponseFrameTooLarge = errors.New("llm: response frame too large")

func newModelHTTPClient() *http.Client {
	return httpresponse.NewClient(maxModelResponseFrameBytes, errModelResponseFrameTooLarge)
}
