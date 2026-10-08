package pluginpackage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func validateViewHTML(body []byte) error {
	if !utf8.Valid(body) || !strings.HasPrefix(http.DetectContentType(body), "text/html") {
		return fmt.Errorf("%w: view must be UTF-8 HTML", plugin.ErrInvalid)
	}
	return nil
}

func (r *Releases) ReadView(ctx context.Context, installation *plugin.Installation, view plugin.ViewDeclaration) (string, error) {
	body, err := r.readResource(ctx, installation, view.Entry, plugin.MaxViewBytes)
	if err != nil {
		return "", errors.Join(plugin.ErrUnavailable, err)
	}
	if err := validateViewHTML(body); err != nil {
		return "", err
	}
	return string(body), nil
}
