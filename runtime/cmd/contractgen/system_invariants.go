package main

const (
	boundaryGoalsLifecycle                 = "goals.lifecycle"
	boundaryRunsAdmission                  = "runs.admission"
	boundaryRunsRecovery                   = "runs.recovery"
	boundaryRunsegmentEvent                = "runsegment.event"
	boundaryRunsegmentOpening              = "runsegment.opening"
	boundaryRunsegmentWaitingSubtreeCancel = "runsegment.waiting_subtree_cancel"
	boundarySessionsDelete                 = "sessions.delete"
	boundarySessionsImport                 = "sessions.import"
	boundarySessionsParkedTerminal         = "sessions.parked_terminal"
	boundarySessionsRollback               = "sessions.rollback"
)

func systemInvariants() []invariantEntry {
	return []invariantEntry{{
		Key: "session_has_at_most_one_open_run",
		Why: "Two concurrently-appending Runs would interleave one session's history, " +
			"and the second would read a transcript the first had not finished writing.",
		Boundaries: []string{boundaryRunsAdmission, boundaryRunsegmentOpening},
	}, {
		Key: "terminal_run_explains_how_it_ended",
		Why: "A Run row that claims a terminal state without the outcome — and, when " +
			"that outcome is a failure, the failure itself — cannot answer why the run " +
			"ended, and no later write will supply it.",
		Boundaries: []string{boundaryRunsegmentEvent, boundaryRunsRecovery, boundarySessionsImport},
	}, {
		Key: "run_capabilities_are_immutable",
		Why: "A Run may exercise only the optional behavior enabled at admission. If " +
			"a later segment could restate it, continuation and subscription checks would " +
			"no longer describe the same Run.",
		Boundaries: []string{boundaryRunsAdmission},
	}, {
		Key: "parked_tree_has_exactly_one_open_interrupt_set",
		Why: "A Run tree parked without one complete pending set cannot be resumed " +
			"atomically; clients would observe only part of a barrier that can never move.",
		Boundaries: []string{boundaryRunsegmentEvent, boundaryRunsRecovery},
	}, {
		Key: "parked_continuation_restates_no_run_fact",
		Why: "A continuation names the parked Runs and the Items it awaits, and nothing " +
			"they own. Run model, accounting, lineage, Goal incarnation and capabilities, " +
			"and each Item's occurrence, Tool invocation and Question, are read from their " +
			"owners, so a resume or teardown cannot write a second version.",
		Boundaries: []string{
			boundaryRunsegmentOpening,
			boundaryRunsegmentEvent,
			boundaryRunsegmentWaitingSubtreeCancel,
			boundaryRunsRecovery,
			boundarySessionsParkedTerminal,
		},
	}, {
		Key: "dropped_run_leaves_nothing_behind",
		Why: "A dropped Run's items, interrupts, checkpoints and admission slot must " +
			"go with it, or the session keeps an invisible run holding its only slot.",
		Boundaries: []string{boundarySessionsRollback, boundarySessionsDelete},
	}, {
		Key: "imported_session_keeps_its_identity",
		Why: "Import is restore, not copy: an artifact must come back under the id it " +
			"was exported with, or its runs and items reference a session that is gone.",
		Boundaries: []string{boundarySessionsImport},
	}, {
		Key: "goal_never_outlives_its_session",
		Why: "A goal is session-owned. One that survives its session's deletion or " +
			"rollback would keep driving runs toward an objective nobody can see or stop.",
		Boundaries: []string{boundaryGoalsLifecycle, boundarySessionsDelete, boundarySessionsRollback},
	}}
}
