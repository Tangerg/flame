// Commands always name their repository explicitly, so inherited GIT_* controls
// would make target and semantics ambiguous.
package process

import (
	"os"
	"runtime"
	"strings"
)

// Clearing the GIT_ namespace as a whole avoids a version-sensitive denylist.
func environment(overrides ...string) []string {
	overridden := make(map[string]struct{}, len(overrides))
	for _, entry := range overrides {
		if key, _, ok := strings.Cut(entry, "="); ok {
			overridden[environmentKey(key)] = struct{}{}
		}
	}

	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || hasGitPrefix(key) {
			continue
		}
		if _, replaced := overridden[environmentKey(key)]; replaced {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, overrides...)
}

func hasGitPrefix(key string) bool {
	const gitVariablePrefix = "GIT_"
	return len(key) >= len(gitVariablePrefix) && strings.EqualFold(key[:len(gitVariablePrefix)], gitVariablePrefix)
}

func environmentKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}
