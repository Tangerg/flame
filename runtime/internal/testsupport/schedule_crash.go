package testsupport

// ScheduleCrashPhase is one step of the schedule SIGKILL recovery scenario: the
// parent test passes it to each re-executed Runtime and to the message-loss
// probe, which decides where to cut from it.
type ScheduleCrashPhase string

const (
	ScheduleCrashCommit     ScheduleCrashPhase = "commit"
	ScheduleCrashEdited     ScheduleCrashPhase = "edited"
	ScheduleCrashQuery      ScheduleCrashPhase = "query"
	ScheduleCrashResolution ScheduleCrashPhase = "resolution"
	ScheduleCrashDisable    ScheduleCrashPhase = "disable"
	ScheduleCrashFinish     ScheduleCrashPhase = "finish"
	ScheduleCrashAbsent     ScheduleCrashPhase = "absent"
	ScheduleCrashConflict   ScheduleCrashPhase = "conflict"
	ScheduleCrashMalformed  ScheduleCrashPhase = "malformed"
	ScheduleCrashDisabled   ScheduleCrashPhase = "disabled"
)

// Inconclusive reports whether recovery at this phase cannot prove the original
// create, so the create stays Unknown and its Run ends Lost.
func (p ScheduleCrashPhase) Inconclusive() bool {
	switch p {
	case ScheduleCrashAbsent, ScheduleCrashConflict, ScheduleCrashMalformed, ScheduleCrashDisabled:
		return true
	default:
		return false
	}
}
