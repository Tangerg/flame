import type { GoalTarget, StartGoalRequest, UpdateGoalRequest } from "@flame/runtime-client/wire";

export interface GoalCommandsGateway {
  start(input: StartGoalRequest): Promise<void>;
  update(input: UpdateGoalRequest): Promise<void>;
  clear(target: GoalTarget): Promise<void>;
  stop(target: GoalTarget): Promise<void>;
  resume(target: GoalTarget): Promise<void>;
  dispose(): void;
}
