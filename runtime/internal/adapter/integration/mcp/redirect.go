package mcp

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
)

const maximumRedirects = 10

var errCrossOriginRedirect = errors.New("mcp: cross-origin redirect blocked")

func checkRedirect(request *http.Request, via []*http.Request) error {
	if request == nil {
		return errors.New("mcp: redirect request is nil")
	}
	if len(via) == 0 {
		return errors.New("mcp: redirect chain is empty")
	}
	if via[0] == nil {
		return errors.New("mcp: initial redirect request is nil")
	}
	if len(via) >= maximumRedirects {
		return fmt.Errorf("mcp: stopped after %d redirects", maximumRedirects)
	}
	source, err := httporigin.FromURL(via[0].URL)
	if err != nil {
		return fmt.Errorf("mcp: initial redirect request: %w", err)
	}
	target, err := httporigin.FromURL(request.URL)
	if err != nil {
		return fmt.Errorf("mcp: redirect target: %w", err)
	}
	if source != target {
		return fmt.Errorf("%w: %s to %s", errCrossOriginRedirect, source, target)
	}
	return nil
}
