// Package httpredirect enforces credential authority across HTTP request chains.
package httpredirect

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/httporigin"
)

const maximumRedirects = 10

var ErrCrossOriginRedirect = errors.New("httpredirect: cross-origin redirect blocked")

// CheckRedirect permits redirects only within the initial request's origin.
// It is suitable for clients whose credential and endpoint authority must stay
// inseparable; callers with an explicit multi-origin policy need their own
// callback.
func CheckRedirect(request *http.Request, via []*http.Request) error {
	if request == nil {
		return errors.New("httpredirect: redirect request is nil")
	}
	if len(via) == 0 {
		return errors.New("httpredirect: redirect chain is empty")
	}
	if via[0] == nil {
		return errors.New("httpredirect: initial redirect request is nil")
	}
	if len(via) >= maximumRedirects {
		return fmt.Errorf("httpredirect: stopped after %d redirects", maximumRedirects)
	}
	source, err := httporigin.FromURL(via[0].URL)
	if err != nil {
		return fmt.Errorf("httpredirect: initial redirect request: %w", err)
	}
	target, err := httporigin.FromURL(request.URL)
	if err != nil {
		return fmt.Errorf("httpredirect: redirect target: %w", err)
	}
	if source != target {
		return fmt.Errorf("%w: %s to %s", ErrCrossOriginRedirect, source, target)
	}
	return nil
}
