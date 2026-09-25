package tool

import (
	"github.com/Tangerg/flame/runtime/internal/domain/destructive"
)

// FileMutationScope describes what a filesystem-capable tool will affect
// relative to the active workspace. It is derived from the concrete tool's
// mutation-reporting capability after hook argument rewrites and symlink
// resolution; policy never guesses paths from a tool name or JSON key.
type FileMutationScope string

const (
	FileMutationNone             FileMutationScope = "none"
	FileMutationWithinWorkspace  FileMutationScope = "withinWorkspace"
	FileMutationOutsideWorkspace FileMutationScope = "outsideWorkspace"
	FileMutationUnknown          FileMutationScope = "unknown"
)

// Valid reports whether f is one supported filesystem mutation relation.
func (f FileMutationScope) Valid() bool {
	return f == FileMutationNone || f == FileMutationWithinWorkspace ||
		f == FileMutationOutsideWorkspace || f == FileMutationUnknown
}

// BypassImmunity identifies a call that must still be confirmed under an
// auto-approve mode. It carries policy identity without presentation text.
type BypassImmunity string

const (
	BypassAllowed                   BypassImmunity = "allowed"
	BypassImmuneOutsideWorkspace    BypassImmunity = "outsideWorkspace"
	BypassImmuneUnknownMutation     BypassImmunity = "unknownMutation"
	BypassImmuneCatastrophicCommand BypassImmunity = "catastrophicCommand"
)

// BypassImmunityFor reports whether a tool call is dangerous enough to confirm
// with a human EVEN under an auto-approve mode (Yolo, or Balanced for
// file mutations).
//
// Two independent, deliberately-conservative checks, both DEFENSE-IN-DEPTH
// CONFIRMS — not security jails (real confinement is a sandbox executor, the
// deferred C7); they only insist a human sees the most obviously-catastrophic
// actions before an auto-approve mode runs them, and a remembered approval still
// lets a repeat through:
//   - a file mutation whose target escapes the workspace directory (a PRECISE
//     path property);
//   - a shell command matching a high-confidence catastrophic pattern
//     (rm -rf of / or $HOME, --no-preserve-root, a fork bomb, mkfs/dd to a
//     device). Tight by design so an ordinary command never trips it.
func BypassImmunityFor(mutation FileMutationScope, shellCommand string) BypassImmunity {
	switch mutation {
	case FileMutationNone, FileMutationWithinWorkspace:
	case FileMutationOutsideWorkspace:
		return BypassImmuneOutsideWorkspace
	case FileMutationUnknown:
		return BypassImmuneUnknownMutation
	default:
		return BypassImmuneUnknownMutation
	}
	if destructive.Command(shellCommand) {
		return BypassImmuneCatastrophicCommand
	}
	return BypassAllowed
}
