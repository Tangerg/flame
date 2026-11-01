package main

func runInstructions(objective string, continuing bool) string {
	prefix := objective
	if continuing {
		prefix = "Continue toward the goal: " + objective
	}
	return prefix + "\n\n(You are running autonomously toward this Goal — you do not need to wait for the user. Take one concrete next step. Call report_goal_outcome(outcome=\"completed\") only when the entire objective is achieved and verified, or report_goal_outcome(outcome=\"blocked\", reason=\"...\") if progress genuinely requires the user or an external state change. Otherwise keep working; the Goal loop will provide the next Run.)"
}
