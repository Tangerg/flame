package tool

import (
	"github.com/Tangerg/flame/runtime/internal/domain/destructive"
)

type FileMutationScope string

const (
	FileMutationNone             FileMutationScope = "none"
	FileMutationWithinWorkspace  FileMutationScope = "withinWorkspace"
	FileMutationOutsideWorkspace FileMutationScope = "outsideWorkspace"
	FileMutationUnknown          FileMutationScope = "unknown"
)

func (f FileMutationScope) Valid() bool {
	return f == FileMutationNone || f == FileMutationWithinWorkspace ||
		f == FileMutationOutsideWorkspace || f == FileMutationUnknown
}

type BypassImmunity string

const (
	BypassAllowed                   BypassImmunity = "allowed"
	BypassImmuneOutsideWorkspace    BypassImmunity = "outsideWorkspace"
	BypassImmuneUnknownMutation     BypassImmunity = "unknownMutation"
	BypassImmuneCatastrophicCommand BypassImmunity = "catastrophicCommand"
)

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
