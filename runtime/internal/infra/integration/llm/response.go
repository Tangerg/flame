package llm

import (
	"errors"
	"net/http"

	"github.com/Tangerg/flame/runtime/internal/httporigin"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/httpresponse"
)

const maxModelResponseFrameBytes int64 = 64 << 20

var errModelResponseFrameTooLarge = errors.New("llm: response frame too large")

func newModelHTTPClient() *http.Client {
	client := httpresponse.NewClient(maxModelResponseFrameBytes, errModelResponseFrameTooLarge)
	client.CheckRedirect = httporigin.CheckRedirect
	return client
}
