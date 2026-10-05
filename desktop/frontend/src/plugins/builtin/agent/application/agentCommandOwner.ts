import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import { tupleKey } from "@/lib/tupleKey";

export interface SessionRollbackLease {
  isCurrent(): boolean;
  release(): void;
}

export interface AgentCommandEffect {
  settle(): void;
  rollback(): void;
}

export class AgentCommandOwner {
  readonly #creates = new Map<string, Promise<unknown>>();
  readonly #forks = new Map<string, Promise<unknown>>();
  readonly #rollbackSessions = new Set<string>();
  readonly #effects = new Set<AgentCommandEffect>();
  readonly #retiredError = new GenerationRetiredError("agent_command_owner");
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);

  private constructor() {}

  static install(): AgentCommandOwner {
    const owner = new AgentCommandOwner();
    agentCommandPublication.publish(owner, (predecessor) => predecessor.#retire());
    return owner;
  }

  static current(): AgentCommandOwner {
    const owner = agentCommandPublication.current();
    if (!owner) throw new GenerationRetiredError("agent_command_owner");
    return owner;
  }

  isCurrent(): boolean {
    return !this.#cohort.retired && agentCommandPublication.owns(this);
  }

  assertCurrent(): void {
    if (!agentCommandPublication.owns(this)) throw this.#retiredError;
    this.#cohort.assertCurrent();
  }

  settle<T>(operation: Promise<T>): Promise<T> {
    this.assertCurrent();
    return this.#cohort.settle(operation);
  }

  runSessionCreate<T>(key: string | null, execute: () => Promise<T>): Promise<T> {
    this.assertCurrent();
    if (key === null) return this.settle(execute());
    return this.#runSingleFlight(this.#creates, key, execute);
  }

  runSessionFork<T>(key: string, execute: () => Promise<T>): Promise<T> {
    this.assertCurrent();
    return this.#runSingleFlight(this.#forks, key, execute);
  }

  beginSessionRollback(sessionId: string): SessionRollbackLease | null {
    this.assertCurrent();
    if (this.#rollbackSessions.has(sessionId)) return null;
    this.#rollbackSessions.add(sessionId);
    let released = false;
    return {
      isCurrent: () => this.isCurrent(),
      release: () => {
        if (released) return;
        released = true;
        this.#rollbackSessions.delete(sessionId);
      },
    };
  }

  serializeSessionSummary<T>(sessionId: string, execute: () => Promise<T>): Promise<T> {
    this.assertCurrent();
    return this.#cohort.runSerial(tupleKey("session-summary", sessionId), execute);
  }

  serializeApprovalMode<T>(execute: () => Promise<T>): Promise<T> {
    this.assertCurrent();
    return this.#cohort.runSerial("approval-mode", execute);
  }

  serializeApprovalRules<T>(execute: () => Promise<T>): Promise<T> {
    this.assertCurrent();
    return this.#cohort.runSerial("approval-rules", execute);
  }

  trackEffect(rollback: () => void): AgentCommandEffect {
    this.assertCurrent();
    let pending = true;
    const effect: AgentCommandEffect = {
      settle: () => {
        if (!pending) return;
        pending = false;
        this.#effects.delete(effect);
      },
      rollback: () => {
        if (!pending) return;
        pending = false;
        this.#effects.delete(effect);
        rollback();
      },
    };
    this.#effects.add(effect);
    return effect;
  }

  dispose(): void {
    if (this.#cohort.retired) return;
    agentCommandPublication.withdraw(this);
    this.#retire();
  }

  #runSingleFlight<T>(
    flights: Map<string, Promise<unknown>>,
    key: string,
    execute: () => Promise<T>,
  ): Promise<T> {
    const existing = flights.get(key) as Promise<T> | undefined;
    if (existing) return existing;
    const operation = this.settle(execute());
    const tracked = operation.finally(() => {
      if (flights.get(key) === tracked) flights.delete(key);
    });
    flights.set(key, tracked);
    return tracked;
  }

  #retire(): void {
    if (this.#cohort.retired) return;
    this.#cohort.retire();
    this.#creates.clear();
    this.#forks.clear();
    this.#rollbackSessions.clear();
    for (const effect of [...this.#effects]) effect.rollback();
  }
}

const agentCommandPublication = createPublicationSlot<AgentCommandOwner>();

export function agentCommandOwner(): AgentCommandOwner {
  return AgentCommandOwner.current();
}
