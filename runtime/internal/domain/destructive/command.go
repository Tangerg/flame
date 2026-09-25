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
	forkBomb = regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`)
	// deviceDestroyers match filesystem/disk-wiping commands that are essentially
	// never run against a real target by accident.
	deviceDestroyers = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bmkfs(\.\w+)?\b`),
		regexp.MustCompile(`(?i)\bwipefs\b`),
		regexp.MustCompile(`(?i)\bdd\b[^|;&\n]*\bof=/dev/`),
		regexp.MustCompile(`(?i)>\s*/dev/(sd|nvme|hd|disk|vd)`),
	}
)

// catastrophicCommand reports whether a shell command line matches a
// high-confidence, near-zero-false-positive catastrophic pattern. Conservative
// on purpose: it flags the handful of forms that are essentially never
// intentional (rm -rf of a root/home path, the explicit --no-preserve-root, a
// fork bomb, a device wipe), and leaves everything else — including ordinary
// rm -rf of a subdirectory — alone. It is NOT a security boundary (trivially
// bypassable via quoting/variables); it is a courtesy confirm before an obvious
// disaster.
// Command reports whether a shell command has a high-confidence destructive
// shape. The command must already be extracted from its concrete tool input.
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
	return regexp.MustCompile(`&&|\|\||[;|&\n]`).Split(command, -1)
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
	fields := strings.Fields(segment)
	rm := false
	recursive, force := false, false
	var targets []string
	for _, f := range fields {
		switch {
		case f == "rm" || strings.HasSuffix(f, "/rm"):
			rm = true
		case f == "sudo" || f == "command" || f == "env":
			// prefixes that still front an rm — keep scanning.
		case strings.HasPrefix(f, "--"):
			switch f {
			case "--recursive":
				recursive = true
			case "--force":
				force = true
			}
		case strings.HasPrefix(f, "-"):
			if strings.ContainsAny(f, "rR") {
				recursive = true
			}
			if strings.Contains(f, "f") {
				force = true
			}
		default:
			targets = append(targets, strings.Trim(f, `"'`))
		}
	}
	if !rm || !recursive || !force {
		return false
	}
	for _, t := range targets {
		if catastrophicRemoveTarget[t] {
			return true
		}
	}
	return false
}
