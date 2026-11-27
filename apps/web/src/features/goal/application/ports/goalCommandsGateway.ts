import type { GoalTarget, StartGoalRequest, UpdateGoalRequest } from "@flame/runtime-client/wire";

export interface GoalCommandReceipt {
  sessionId: string;
}

export interface GoalCommandsGateway {
  start(input: StartGoalRequest): Promise<GoalCommandReceipt>;
  update(input: UpdateGoalRequest): Promise<GoalCommandReceipt>;
  clear(target: GoalTarget): Promise<GoalCommandReceipt>;
  stop(target: GoalTarget): Promise<GoalCommandReceipt>;
  resume(target: GoalTarget): Promise<GoalCommandReceipt>;
}
