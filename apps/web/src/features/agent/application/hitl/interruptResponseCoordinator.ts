import { tupleKey } from "@/lib/tupleKey";
import { type InterruptRef, type InterruptResumeInput } from "@/features/agent/domain/hitl";
import {
  type AgentSessionViewEntry,
  type AgentSessionViewPort,
} from "@/features/agent/application/session/sessionAccess";
import { selectAwaitingGroups } from "@/features/agent/application/view/awaitingInterrupts";

type BatchPhase = "collecting" | "opening" | "accepted";
export type InterruptResponseState = "unsubmitted" | "pending" | "accepted";

class InterruptResponseBatch {
  readonly #responses = new Map<string, InterruptResumeInput>();
  #phase: BatchPhase = "collecting";

  constructor(
    readonly sessionId: string,
    readonly rootRunId: string,
  ) {}

  stage(input: InterruptResumeInput): boolean {
    if (this.#phase !== "collecting" || this.#responses.has(input.itemId)) return false;
    this.#responses.set(input.itemId, input);
    return true;
  }

  responseState(itemId: string): InterruptResponseState {
    if (!this.#responses.has(itemId)) return "unsubmitted";
    return this.#phase === "accepted" ? "accepted" : "pending";
  }

  beginOpening(itemIds: readonly string[]): InterruptResumeInput[] | undefined {
    const responses: InterruptResumeInput[] = [];
    for (const itemId of itemIds) {
      const response = this.#responses.get(itemId);
      if (!response) return;
      responses.push(response);
    }
    this.#phase = "opening";
    return responses;
  }

  accept(): boolean {
    if (this.#phase !== "opening") return false;
    this.#phase = "accepted";
    return true;
  }

  isOpening(): boolean {
    return this.#phase === "opening";
  }

  retiredBy(open: ReadonlySet<string>): boolean {
    if (this.#phase === "opening") return false;
    const staged = [...this.#responses.keys()];
    // Acceptance precedes projection of the closed interrupts.
    if (this.#phase === "accepted") return !staged.some((itemId) => open.has(itemId));
    return staged.some((itemId) => !open.has(itemId));
  }
}

const batchKey = (sessionId: string, rootRunId: string) => tupleKey(sessionId, rootRunId);

function openResponseIds(entry: AgentSessionViewEntry | undefined, rootRunId: string): string[] {
  if (!entry) return [];
  return selectAwaitingGroups(entry.view)
    .filter((group) => group.rootRunId === rootRunId)
    .flatMap((group) => group.interrupts.map((interrupt) => interrupt.itemId));
}

export class InterruptResponseCoordinator {
  readonly #view: AgentSessionViewPort;
  readonly #batches = new Map<string, InterruptResponseBatch>();
  readonly #listeners = new Set<() => void>();
  readonly #unsubscribe: () => void;
  #retired = false;

  constructor(view: AgentSessionViewPort) {
    this.#view = view;
    this.#unsubscribe = view.subscribeSessions((sessions) => this.#reconcile(sessions));
  }

  stage(
    { sessionId, rootRunId, itemId }: InterruptRef,
    response: InterruptResumeInput["response"],
  ): boolean {
    if (this.#retired) return false;
    const entry = this.#view.getSession(sessionId);
    if (!entry?.resume) return false;
    const expected = openResponseIds(entry, rootRunId);
    if (!expected.includes(itemId)) return false;

    const key = batchKey(sessionId, rootRunId);
    let batch = this.#batches.get(key);
    if (!batch) {
      batch = new InterruptResponseBatch(sessionId, rootRunId);
      this.#batches.set(key, batch);
    }
    if (!batch.stage({ itemId, response })) return false;
    const responses = batch.beginOpening(expected);
    this.#changed();
    if (!responses) return true;
    const opening = batch;
    const refuse = () => {
      if (this.#batches.get(key) !== opening || !opening.isOpening()) return;
      this.#batches.delete(key);
      this.#changed();
    };
    const accepted = entry.resume(
      rootRunId,
      responses,
      () => {
        if (this.#batches.get(key) !== opening || !opening.accept()) return;
        const open = new Set(openResponseIds(this.#view.getSession(sessionId), rootRunId));
        if (opening.retiredBy(open)) this.#batches.delete(key);
        this.#changed();
      },
      refuse,
    );
    if (!accepted) refuse();
    return accepted;
  }

  responseState(sessionId: string, itemId: string): InterruptResponseState {
    // A view can lose its resume target before the original Item's receipt arrives.
    for (const batch of this.#batches.values()) {
      if (batch.sessionId !== sessionId) continue;
      const state = batch.responseState(itemId);
      if (state !== "unsubmitted") return state;
    }
    return "unsubmitted";
  }

  subscribe = (listener: () => void): (() => void) => {
    if (this.#retired) return () => {};
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  discard(): void {
    if (!this.#batches.size) return;
    this.#batches.clear();
    this.#changed();
  }

  retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    this.#unsubscribe();
    this.discard();
    this.#listeners.clear();
  }

  #changed(): void {
    for (const listener of this.#listeners) listener();
  }

  #reconcile(sessions: Record<string, AgentSessionViewEntry>): void {
    if (this.#retired) return;
    let changed = false;
    for (const [key, batch] of this.#batches) {
      const open = new Set(openResponseIds(sessions[batch.sessionId], batch.rootRunId));
      if (!batch.retiredBy(open)) continue;
      this.#batches.delete(key);
      changed = true;
    }
    if (changed) this.#changed();
  }
}
