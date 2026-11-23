package mcpserver

import (
	"errors"
	"fmt"
	"strings"
)

// Linker/loader hijack keys are a code-injection vector with no legitimate MCP
// use, matched case-insensitively because some loaders honor variants. PATH,
// NODE_OPTIONS and PYTHONPATH stay allowed for legitimate servers.
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
	return ValidateEnvironment(environment)
}

func ValidateEnvironment(environment map[string]string) error {
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
