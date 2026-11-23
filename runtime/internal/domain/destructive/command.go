// Not a security boundary: quoting and variables defeat it. It is a courtesy
// confirmation before an obvious disaster.
package destructive

import (
	"regexp"
	"slices"
	"strings"
)

var (
	shellSeparators  = regexp.MustCompile(`&&|\|\||[;|&\n]`)
	forkBomb         = regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`)
	deviceDestroyers = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bmkfs(\.\w+)?\b`),
		regexp.MustCompile(`(?i)\bwipefs\b`),
		regexp.MustCompile(`(?i)\bdd\b[^|;&\n]*\bof=/dev/`),
		regexp.MustCompile(`(?i)>\s*/dev/(sd|nvme|hd|disk|vd)`),
	}
)

func Command(command string) bool {
	if command == "" {
		return false
	}
	if strings.Contains(command, "--no-preserve-root") {
		return true
	}
	if forkBomb.MatchString(command) {
		return true
	}
	for _, re := range deviceDestroyers {
		if re.MatchString(command) {
			return true
		}
	}
	return slices.ContainsFunc(shellSegments(command), recursiveForceRemoveOfRootOrHome)
}

func shellSegments(command string) []string {
	return shellSeparators.Split(command, -1)
}

var catastrophicRemoveTarget = map[string]bool{
	"/": true, "/*": true,
	"~": true, "~/": true, "~/*": true,
	"$HOME": true, "${HOME}": true, "$HOME/": true, "$HOME/*": true,
	"*": true, ".": true, "..": true,
}

func recursiveForceRemoveOfRootOrHome(segment string) bool {
	removal := parseRemoval(segment)
	if !removal.rm || !removal.recursive || !removal.force {
		return false
	}
	return slices.ContainsFunc(removal.targets, func(target string) bool { return catastrophicRemoveTarget[target] })
}

type removal struct {
	rm, recursive, force bool
	targets              []string
}

func parseRemoval(segment string) removal {
	var parsed removal
	for _, field := range strings.Fields(segment) {
		switch {
		case field == "rm" || strings.HasSuffix(field, "/rm"):
			parsed.rm = true
		case field == "sudo" || field == "command" || field == "env":
		case strings.HasPrefix(field, "-"):
			parsed.applyFlag(field)
		default:
			parsed.targets = append(parsed.targets, strings.Trim(field, `"'`))
		}
	}
	return parsed
}

func (r *removal) applyFlag(flag string) {
	switch {
	case flag == "--recursive":
		r.recursive = true
	case flag == "--force":
		r.force = true
	case strings.HasPrefix(flag, "--"):
	default:
		r.recursive = r.recursive || strings.ContainsAny(flag, "rR")
		r.force = r.force || strings.Contains(flag, "f")
	}
}
