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

func (r *Releases) ReadPage(ctx context.Context, installation *plugin.Installation, entry string) (string, error) {
	body, err := r.readResource(ctx, installation, entry, plugin.MaxViewBytes)
	if err != nil {
		return "", errors.Join(plugin.ErrUnavailable, err)
	}
	if err := validateViewHTML(body); err != nil {
		return "", err
	}
	return string(body), nil
}
