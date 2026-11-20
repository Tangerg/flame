// Package destructive owns which shell command shapes are irreversibly
// destructive. Two policies ask that question — a Run tool call that may not be
// approved by a blanket bypass, and a Skill proposal that must not teach one —
// and each keeps its own threshold and its own subject. What they must not keep
// is a separate answer: a pattern added to one list protected only one path,
// which is how wipefs and a redirect into a raw device came to be caught before
// a tool call and not before a published Skill.
//
// It is not a security boundary. Quoting and variables defeat it trivially. It
// is the courtesy confirmation before an obvious disaster.
package destructive

import (
	"regexp"
	"slices"
	"strings"
)

var (
	// forkBomb matches the classic :(){ :|:& };: (whitespace-tolerant).
	shellSeparators = regexp.MustCompile(`&&|\|\||[;|&\n]`)
	forkBomb        = regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`)
	// deviceDestroyers match filesystem/disk-wiping commands that are essentially
	// never run against a real target by accident.
	deviceDestroyers = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bmkfs(\.\w+)?\b`),
		regexp.MustCompile(`(?i)\bwipefs\b`),
		regexp.MustCompile(`(?i)\bdd\b[^|;&\n]*\bof=/dev/`),
		regexp.MustCompile(`(?i)>\s*/dev/(sd|nvme|hd|disk|vd)`),
	}
)

// Command reports whether a shell command has a high-confidence destructive
// shape: rm -rf of a root or home path, --no-preserve-root, a fork bomb or a
// device wipe. It is a courtesy confirmation before an obvious disaster, not a
// security boundary; quoting or variables bypass it. The command must already
// be extracted from its concrete tool input.
func Command(command string) bool {
	if command == "" {
		return false
	}
	// The explicit "yes, allow removing /" flag is catastrophic on its own.
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
	// Check each pipeline/sequence segment for a recursive-force rm of a
	// catastrophic target, so `cd x && rm -rf ~` is caught in its own segment.
	return slices.ContainsFunc(shellSegments(command), recursiveForceRemoveOfRootOrHome)
}

// shellSegments splits a command line on the shell operators that separate
// commands (; && || | & newline) so each is inspected on its own.
func shellSegments(command string) []string {
	return shellSeparators.Split(command, -1)
}

// catastrophicRemoveTarget is the set of rm targets that a recursive-force
// delete should never hit unintentionally.
var catastrophicRemoveTarget = map[string]bool{
	"/": true, "/*": true,
	"~": true, "~/": true, "~/*": true,
	"$HOME": true, "${HOME}": true, "$HOME/": true, "$HOME/*": true,
	"*": true, ".": true, "..": true,
}

// recursiveForceRemoveOfRootOrHome reports whether one command segment is an
// `rm` invocation with BOTH recursive and force flags (any spelling / order)
// aimed at a catastrophic target.
func recursiveForceRemoveOfRootOrHome(segment string) bool {
	removal := parseRemoval(segment)
	if !removal.rm || !removal.recursive || !removal.force {
		return false
	}
	return slices.ContainsFunc(removal.targets, func(target string) bool { return catastrophicRemoveTarget[target] })
}

// removal is what one command segment asks rm to do.
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
			// prefixes that still front an rm — keep scanning.
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
