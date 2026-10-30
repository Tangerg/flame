package pluginpackage

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func decodeLocale(body []byte) (map[string]string, error) {
	var messages map[string]string
	if err := jsonvalue.UnmarshalNonNull(body, &messages); err != nil {
		return nil, fmt.Errorf("%w: locale catalog: %w", plugin.ErrInvalid, err)
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("%w: empty locale catalog", plugin.ErrInvalid)
	}
	if _, exists := messages[""]; exists {
		return nil, fmt.Errorf("%w: empty locale message key", plugin.ErrInvalid)
	}
	return messages, nil
}

func (r *Releases) ReadLocale(ctx context.Context, installation *plugin.Installation, locale plugin.Locale) (map[string]string, error) {
	body, err := r.readResource(ctx, installation, locale.Entry, maxManifestBytes)
	if err != nil {
		if cancellation.Err(ctx) != nil {
			return nil, err
		}
		return nil, errors.Join(plugin.ErrUnavailable, err)
	}
	return decodeLocale(body)
}
