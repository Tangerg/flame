import type { GoalTarget, StartGoalRequest, UpdateGoalRequest } from "@flame/runtime-client/wire";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import { type GoalCommandsGateway } from "@/features/goal/application/ports/goalCommandsGateway";
export type GoalProjectionRepair = (sessionId: string) => Promise<unknown>;

export const GOAL_COMMAND_SCOPE = "goal_command_scope";

class GoalCommandScope {
  readonly #gateway: GoalCommandsGateway;
  readonly #repairProjection: GoalProjectionRepair;
  readonly #cohort = new RetirableTaskCohort(new GenerationRetiredError(GOAL_COMMAND_SCOPE));

  constructor(gateway: GoalCommandsGateway, repairProjection: GoalProjectionRepair) {
    this.#gateway = gateway;
    this.#repairProjection = repairProjection;
  }

  start(input: StartGoalRequest): Promise<void> {
    return this.#run(input.sessionId, () => this.#gateway.start(input));
  }

  update(input: UpdateGoalRequest): Promise<void> {
    return this.#run(input.sessionId, () => this.#gateway.update(input));
  }

  clear(target: GoalTarget): Promise<void> {
    return this.#run(target.sessionId, () => this.#gateway.clear(target));
  }

  stop(target: GoalTarget): Promise<void> {
    return this.#run(target.sessionId, () => this.#gateway.stop(target));
  }

  resume(target: GoalTarget): Promise<void> {
    return this.#run(target.sessionId, () => this.#gateway.resume(target));
  }

  retire(): void {
    this.#cohort.retire();
    this.#gateway.dispose();
  }

  #run(sessionId: string, command: () => Promise<void>): Promise<void> {
    return this.#cohort.runSerial(sessionId, () => this.#mutate(sessionId, command));
  }

  async #mutate(sessionId: string, command: () => Promise<void>): Promise<void> {
    try {
      await this.#cohort.run(command);
    } catch (error) {
      if (this.#cohort.retired) throw error;
      await this.#repairStandingProjection(sessionId);
      throw error;
    }
    await this.#repairStandingProjection(sessionId);
    this.#cohort.assertCurrent();
  }

  async #repairStandingProjection(sessionId: string): Promise<void> {
    this.#cohort.assertCurrent();
    try {
      await this.#cohort.settle(this.#repairProjection(sessionId));
    } catch (error) {
      if (this.#cohort.retired) throw error;
    }
  }
}

export type GoalCommands = Pick<GoalCommandScope, "start" | "update" | "clear" | "stop" | "resume">;

export type GoalCommandAccess = Pick<GoalCommandOwner, "capture" | "subscribe">;

export class GoalCommandOwner {
  #scope: GoalCommandScope | null = null;
  readonly #repairProjection: GoalProjectionRepair;
  readonly #listeners = new Set<() => void>();
  #disposed = false;

  constructor(repairProjection: GoalProjectionRepair) {
    this.#repairProjection = repairProjection;
  }

  readonly capture = (): GoalCommands | null => this.#scope;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  replaceTarget(createGateway: () => GoalCommandsGateway): void {
    if (this.#disposed) return;
    const predecessor = this.#scope;
    this.#scope = new GoalCommandScope(createGateway(), this.#repairProjection);
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
