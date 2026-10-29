package main

import (
	"errors"
	"net/http"

	"github.com/Tangerg/flame/packages/go-sdk/httpresponse"
)

const maxModelResponseFrameBytes int64 = 64 << 20

var errModelResponseFrameTooLarge = errors.New("model: response frame too large")

func newModelHTTPClient() *http.Client {
	return httpresponse.NewClient(maxModelResponseFrameBytes, errModelResponseFrameTooLarge)
}
