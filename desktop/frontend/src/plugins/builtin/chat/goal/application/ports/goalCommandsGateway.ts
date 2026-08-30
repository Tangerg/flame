import type { GoalBudget } from "../goalBudget";

export interface StartGoalInput {
  sessionId: string;
  objective: string;
  provider?: string;
  model?: string;
  reasoningEffort?: string;
  /** Omitted means unlimited; a present budget has at least one finite limit. */
  budget?: GoalBudget;
}

export interface UpdateGoalInput {
  sessionId: string;
  objective: string;
}

/** Correlates a committed Goal lifecycle command with the Session it addressed.
 * The standing Goal projection is deliberately absent: only the mounted
 * sessions.snapshot material boundary owns that state. */
export interface GoalCommandReceipt {
  sessionId: string;
}

export interface GoalCommandsGateway {
  start(input: StartGoalInput): Promise<GoalCommandReceipt>;
  update(input: UpdateGoalInput): Promise<GoalCommandReceipt>;
  clear(sessionId: string): Promise<GoalCommandReceipt>;
  stop(sessionId: string): Promise<GoalCommandReceipt>;
  resume(sessionId: string): Promise<GoalCommandReceipt>;
}
