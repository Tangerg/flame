import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { SCHEDULES_KEY } from "./scheduleQueries";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { queryClient, repairCachedProjection } from "@/lib/queryClient";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import type { ScheduleConfig, ScheduleConfigInput, ScheduledRunIdentity } from "./scheduleConfig";
import { selectAgentSession } from "@/plugins/builtin/agent/public/session";
export type { ScheduleConfig, ScheduleConfigInput } from "./scheduleConfig";

export interface ScheduleUpdateInput extends ScheduleConfigInput {
  id: string;
  revision: number;
}

export interface ScheduleGateway {
  create(input: ScheduleConfigInput): Promise<ScheduleConfig>;
  update(input: ScheduleUpdateInput): Promise<ScheduleConfig>;
  setEnabled(id: string, expectedRevision: number, enabled: boolean): Promise<ScheduleConfig>;
  remove(id: string): Promise<void>;
  runNow(id: string): Promise<ScheduledRunIdentity>;
}

class ScheduleMutationGeneration {
  readonly #gateway: ScheduleGateway;
  readonly #retiredError = new GenerationRetiredError("schedule_mutation_generation");
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);

  constructor(gateway: ScheduleGateway) {
    this.#gateway = gateway;
  }

  create(input: ScheduleConfigInput): Promise<ScheduleConfig> {
    return this.#executeMutation(() => this.#gateway.create(input), commitScheduleSaved);
  }

  update(input: ScheduleUpdateInput): Promise<ScheduleConfig> {
    return this.#run(input.id, () => this.#gateway.update(input), commitScheduleSaved);
  }

  setEnabled(schedule: ScheduleConfig, enabled: boolean): Promise<ScheduleConfig> {
    return this.#run(
      schedule.id,
      () => {
        // A queued toggle runs after the same schedule's earlier writes have
        // been committed to the read projection, so it states the revision
        // the Runtime last returned rather than the one its caller rendered.
        const current = cachedSchedule(schedule.id) ?? schedule;
        return this.#gateway.setEnabled(current.id, current.revision, enabled);
      },
      commitScheduleSaved,
    );
  }

  remove(id: string): Promise<void> {
    return this.#run(
      id,
      () => this.#gateway.remove(id),
      () => removeSchedule(id),
    );
  }

  runNow(id: string): Promise<ScheduledRunIdentity> {
    return this.#run(
      id,
      () => this.#gateway.runNow(id),
      () => undefined,
      (run) => selectAgentSession(run.sessionId),
    );
  }

  retire(): void {
    this.#cohort.retire();
  }

  #run<T>(
    identity: string,
    execute: () => Promise<T>,
    commit: (value: T) => void,
    afterRepair?: (value: T) => void,
  ): Promise<T> {
    return this.#cohort.runSerial(identity, () =>
      this.#executeMutation(execute, commit, afterRepair),
    );
  }

  async #executeMutation<T>(
    execute: () => Promise<T>,
    commit: (value: T) => void,
    afterRepair?: (value: T) => void,
  ): Promise<T> {
    this.#cohort.assertCurrent();
    let value: T;
    try {
      value = await this.#cohort.settle(execute());
    } catch (error) {
      if (error === this.#retiredError) throw error;
      await repairCachedProjection(this.#cohort, [SCHEDULES_KEY]);
      this.#cohort.assertCurrent();
      throw error;
    }
    this.#cohort.assertCurrent();
    commit(value);
    await repairCachedProjection(this.#cohort, [SCHEDULES_KEY]);
    this.#cohort.assertCurrent();
    afterRepair?.(value);
    return value;
  }
}

export class ScheduleMutationOwner {
  #generation: ScheduleMutationGeneration;
  #disposed = false;

  private constructor(gateway: ScheduleGateway) {
    this.#generation = new ScheduleMutationGeneration(gateway);
  }

  static install(gateway: ScheduleGateway): ScheduleMutationOwner {
    const owner = new ScheduleMutationOwner(gateway);
    scheduleMutationPublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  static current(): ScheduleMutationOwner {
    const owner = scheduleMutationPublication.current();
    if (!owner || owner.#disposed) throw new Error("Schedule mutation owner is not installed");
    return owner;
  }

  create(input: ScheduleConfigInput): Promise<ScheduleConfig> {
    return this.#generation.create(input);
  }

  update(input: ScheduleUpdateInput): Promise<ScheduleConfig> {
    return this.#generation.update(input);
  }

  setEnabled(schedule: ScheduleConfig, enabled: boolean): Promise<ScheduleConfig> {
    return this.#generation.setEnabled(schedule, enabled);
  }

  remove(id: string): Promise<void> {
    return this.#generation.remove(id);
  }

  runNow(id: string): Promise<ScheduledRunIdentity> {
    return this.#generation.runNow(id);
  }

  replaceRuntimeGeneration(createGateway: () => ScheduleGateway): void {
    if (this.#disposed || !scheduleMutationPublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new ScheduleMutationGeneration(createGateway());
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    scheduleMutationPublication.withdraw(this);
  }
}

const scheduleMutationPublication = createPublicationSlot<ScheduleMutationOwner>();

export async function createSchedule(input: ScheduleConfigInput): Promise<ScheduleConfig> {
  return ScheduleMutationOwner.current().create(input);
}

export async function updateSchedule(input: ScheduleUpdateInput): Promise<ScheduleConfig> {
  return ScheduleMutationOwner.current().update(input);
}

export async function setScheduleEnabled(
  s: ScheduleConfig,
  enabled: boolean,
): Promise<ScheduleConfig> {
  return ScheduleMutationOwner.current().setEnabled(s, enabled);
}

export async function deleteSchedule(id: string): Promise<void> {
  return ScheduleMutationOwner.current().remove(id);
}

export async function runScheduleNow(id: string): Promise<ScheduledRunIdentity> {
  return ScheduleMutationOwner.current().runNow(id);
}

function cachedSchedule(id: string): ScheduleConfig | undefined {
  return queryClient
    .getQueryData<ScheduleConfig[]>([SCHEDULES_KEY])
    ?.find((schedule) => schedule.id === id);
}

function commitScheduleSaved(saved: ScheduleConfig): void {
  queryClient.setQueryData<ScheduleConfig[]>([SCHEDULES_KEY], (current) => {
    if (!current) return current;
    const index = current.findIndex((schedule) => schedule.id === saved.id);
    if (index < 0) return [...current, saved];
    return current.map((schedule) => (schedule.id === saved.id ? saved : schedule));
  });
}

function removeSchedule(id: string): void {
  queryClient.setQueryData<ScheduleConfig[]>([SCHEDULES_KEY], (current) =>
    current?.filter((schedule) => schedule.id !== id),
  );
}
