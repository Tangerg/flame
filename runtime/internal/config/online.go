package config

import (
	"cmp"
	"strings"
)

// loadOnline reads the optional provider-tool credentials. yaml under
// `online:`; the FLAME_* env vars take precedence over yaml, matching
// the overall source ordering (env over file).
func loadOnline(source Online) Online {
	jina := cmp.Or(jinaAPIKeyEnvironment.Value(), source.JinaAPIKey)
	tavily := cmp.Or(tavilyAPIKeyEnvironment.Value(), source.TavilyAPIKey)
	hosts := source.HTTPAllowedHosts
	if env := httpHostsEnvironment.Value(); env != "" {
		hosts = splitHosts(env)
	}
	return Online{
		JinaAPIKey:       jina,
		TavilyAPIKey:     tavily,
		HTTPAllowedHosts: hosts,
	}
}

// splitHosts parses the comma-separated FLAME_HTTP_ALLOWED_HOSTS value.
func splitHosts(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
