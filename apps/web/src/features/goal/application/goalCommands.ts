import type { GoalTarget, StartGoalRequest, UpdateGoalRequest } from "@flame/runtime-client/wire";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import { type GoalCommandsGateway } from "@/features/goal/application/ports/goalCommandsGateway";

export const GOAL_COMMAND_SCOPE = "goal_command_scope";
export const GOAL_RUNTIME_FEATURE = "goals";

class GoalCommandScope {
  readonly #gateway: GoalCommandsGateway;
  readonly #cohort = new RetirableTaskCohort(new GenerationRetiredError(GOAL_COMMAND_SCOPE));

  constructor(gateway: GoalCommandsGateway) {
    this.#gateway = gateway;
  }

  start(input: StartGoalRequest): Promise<void> {
    return this.#cohort.runSerial(input.sessionId, () => this.#gateway.start(input));
  }

  update(input: UpdateGoalRequest): Promise<void> {
    return this.#cohort.runSerial(input.sessionId, () => this.#gateway.update(input));
  }

  clear(target: GoalTarget): Promise<void> {
    return this.#cohort.runSerial(target.sessionId, () => this.#gateway.clear(target));
  }

  stop(target: GoalTarget): Promise<void> {
    return this.#cohort.runSerial(target.sessionId, () => this.#gateway.stop(target));
  }

  resume(target: GoalTarget): Promise<void> {
    return this.#cohort.runSerial(target.sessionId, () => this.#gateway.resume(target));
  }

  retire(): void {
    this.#cohort.retire();
    this.#gateway.dispose();
  }
}

export type GoalCommands = Pick<GoalCommandScope, "start" | "update" | "clear" | "stop" | "resume">;

export type GoalCommandAccess = Pick<GoalCommandOwner, "capture" | "subscribe">;

export class GoalCommandOwner {
  #scope: GoalCommandScope | null = null;
  readonly #listeners = new Set<() => void>();
  #disposed = false;

  readonly capture = (): GoalCommands | null => this.#scope;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  replaceTarget(createGateway: () => GoalCommandsGateway): void {
    if (this.#disposed) return;
    const predecessor = this.#scope;
    this.#scope = new GoalCommandScope(createGateway());
    predecessor?.retire();
    this.#publish();
  }

  retireTarget(): void {
    const predecessor = this.#scope;
    if (!predecessor) return;
    this.#scope = null;
    predecessor.retire();
    this.#publish();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.retireTarget();
    this.#listeners.clear();
  }

  #publish(): void {
    for (const listener of this.#listeners) listener();
  }
}
