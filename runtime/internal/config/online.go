package config

import (
	"strings"
)

// The host allowlist belongs to the method-selectable HTTP tool. Provider-backed
// web credentials belong to the installed web backend.
func loadOnline(source Online) Online {
	hosts := source.HTTPAllowedHosts
	if env := httpHostsEnvironment.Value(); env != "" {
		hosts = splitHosts(env)
	}
	return Online{
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
