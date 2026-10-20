export interface StartGoalInput {
  sessionId: string;
  objective: string;
  provider?: string;
  model?: string;
  reasoningEffort?: string;
}

export interface UpdateGoalInput {
  sessionId: string;
  objective: string;
}

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
