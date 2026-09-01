package mcpserver

import (
	"errors"
	"fmt"
	"strings"
)

// injectionEnvKeys are environment variables that hijack the dynamic linker or a
// language runtime of a spawned process. They have no legitimate use in an MCP
// server's stdio env and are a code-injection vector: a workspace-supplied
// config that set one of these would load attacker code into the server
// subprocess. Matched case-insensitively — the canonical form is upper-case, but
// a case variant is still honored by some loaders / on case-insensitive OSes.
//
// Deliberately narrow: only the no-legitimate-use linker/loader keys. PATH,
// NODE_OPTIONS, PYTHONPATH and friends are NOT here — a server config may set
// them for benign reasons, and dropping them would break legitimate servers.
var injectionEnvKeys = map[string]struct{}{
	"LD_PRELOAD":            {},
	"LD_LIBRARY_PATH":       {},
	"LD_AUDIT":              {},
	"DYLD_INSERT_LIBRARIES": {},
	"DYLD_LIBRARY_PATH":     {},
	"DYLD_FRAMEWORK_PATH":   {},
}

func validateProcessConfiguration(command string, args []string, environment map[string]string, dir string) error {
	if strings.ContainsRune(command, 0) {
		return errors.New("command contains NUL")
	}
	for index, argument := range args {
		if strings.ContainsRune(argument, 0) {
			return fmt.Errorf("args[%d] contains NUL", index)
		}
	}
	if strings.ContainsRune(dir, 0) {
		return errors.New("dir contains NUL")
	}
	for key, value := range environment {
		if key == "" || strings.ContainsAny(key, "=\x00") {
			return fmt.Errorf("env key %q is invalid", key)
		}
		if strings.ContainsRune(value, 0) {
			return fmt.Errorf("env value for %q contains NUL", key)
		}
		if _, blocked := injectionEnvKeys[strings.ToUpper(key)]; blocked {
			return fmt.Errorf("env key %q is not allowed", key)
		}
	}
	return nil
}
