import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import {
  type GoalCommandsGateway,
  type GoalCommandReceipt,
  type StartGoalInput,
  type UpdateGoalInput,
} from "@/features/goal/application/ports/goalCommandsGateway";
export type GoalProjectionRepair = (sessionId: string) => Promise<unknown>;

export class GoalCommandSessionMismatchError extends Error {
  constructor(
    readonly expectedSessionId: string,
    readonly actualSessionId: string,
  ) {
    super();
    this.name = "GoalCommandSessionMismatchError";
  }
}

class GoalCommandGeneration {
  readonly #gateway: GoalCommandsGateway;
  readonly #repairProjection: GoalProjectionRepair;
  readonly #retiredError = new GenerationRetiredError("goal_command_generation");
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);

  constructor(gateway: GoalCommandsGateway, repairProjection: GoalProjectionRepair) {
    this.#gateway = gateway;
    this.#repairProjection = repairProjection;
  }

  start(input: StartGoalInput): Promise<void> {
    return this.#run(input.sessionId, () => this.#gateway.start(input));
  }

  update(input: UpdateGoalInput): Promise<void> {
    return this.#run(input.sessionId, () => this.#gateway.update(input));
  }

  clear(sessionId: string): Promise<void> {
    return this.#run(sessionId, () => this.#gateway.clear(sessionId));
  }

  stop(sessionId: string): Promise<void> {
    return this.#run(sessionId, () => this.#gateway.stop(sessionId));
  }

  resume(sessionId: string): Promise<void> {
    return this.#run(sessionId, () => this.#gateway.resume(sessionId));
  }

  retire(): void {
    this.#cohort.retire();
  }

  #run(sessionId: string, command: () => Promise<GoalCommandReceipt>): Promise<void> {
    return this.#cohort.runSerial(sessionId, () => this.#mutate(sessionId, command));
  }

  async #mutate(sessionId: string, command: () => Promise<GoalCommandReceipt>): Promise<void> {
    try {
      const committed = await this.#cohort.run(command);
      if (committed.sessionId !== sessionId) {
        throw new GoalCommandSessionMismatchError(sessionId, committed.sessionId);
      }
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

export type GoalCommands = Pick<
  GoalCommandGeneration,
  "start" | "update" | "clear" | "stop" | "resume"
>;

export type GoalCommandAccess = Pick<GoalCommandOwner, "capture" | "subscribe">;

export class GoalCommandOwner {
  #generation: GoalCommandGeneration | null;
  readonly #repairProjection: GoalProjectionRepair;
  readonly #listeners = new Set<() => void>();
  #disposed = false;

  constructor(gateway: GoalCommandsGateway | null, repairProjection: GoalProjectionRepair) {
    this.#repairProjection = repairProjection;
    this.#generation = gateway ? new GoalCommandGeneration(gateway, repairProjection) : null;
  }

  readonly capture = (): GoalCommands | null => this.#generation;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  replaceRuntimeGeneration(createGateway: () => GoalCommandsGateway): boolean {
    if (this.#disposed) return false;
    const successor = new GoalCommandGeneration(createGateway(), this.#repairProjection);
    const predecessor = this.#generation;
    this.#generation = successor;
    predecessor?.retire();
    this.#publish();
    return true;
  }

  retireRuntimeGeneration(): boolean {
    if (this.#disposed) return false;
    const predecessor = this.#generation;
    this.#generation = null;
    predecessor?.retire();
    if (predecessor) this.#publish();
    return true;
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation?.retire();
    this.#generation = null;
    this.#publish();
    this.#listeners.clear();
  }

  #publish(): void {
    for (const listener of this.#listeners) listener();
  }
}
