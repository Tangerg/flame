import { createPublicationSlot } from "@/lib/publicationSlot";
import { tupleKey } from "@/lib/tupleKey";
import { type InterruptRef, type InterruptResumeInput } from "@/features/agent/domain/hitl";
import {
  type AgentSessionViewEntry,
  type AgentSessionViewPort,
} from "@/features/agent/application/ports/sessionView";
import { agentSessionView } from "@/features/agent/application/ports/sessionView";
import { selectAwaitingGroups } from "@/features/agent/application/view/awaitingInterrupts";
interface StagedResponse {
  input: InterruptResumeInput;
  onSettled?: () => void;
  onError?: () => void;
}

class InterruptResponseBatch {
  readonly #responses = new Map<string, StagedResponse>();
  #submitted = false;
  #settled = false;

  constructor(
    readonly sessionId: string,
    readonly rootRunId: string,
  ) {}

  get submitting(): boolean {
    return this.#submitted;
  }

  get settled(): boolean {
    return this.#settled;
  }

  stage(itemId: string, response: StagedResponse): void {
    this.#responses.get(itemId)?.onError?.();
    this.#responses.set(itemId, response);
  }

  contains(itemId: string): boolean {
    return this.#responses.has(itemId);
  }

  items(): string[] {
    return [...this.#responses.keys()];
  }

  covers(itemIds: readonly string[]): boolean {
    return itemIds.every((itemId) => this.#responses.has(itemId));
  }

  ordered(itemIds: readonly string[]): StagedResponse[] {
    return itemIds.map((itemId) => this.#responses.get(itemId)!);
  }

  submit(): void {
    this.#submitted = true;
  }

  settle(): void {
    this.#settled = true;
  }

  responses(): IterableIterator<StagedResponse> {
    return this.#responses.values();
  }
}

const batchKey = (sessionId: string, rootRunId: string) => tupleKey(sessionId, rootRunId);

function openResponseIds(entry: AgentSessionViewEntry, rootRunId: string): string[] {
  return selectAwaitingGroups(entry.view)
    .filter((group) => group.rootRunId === rootRunId)
    .flatMap((group) => group.interrupts.map((interrupt) => interrupt.itemId));
}

class InterruptResponseCoordinator {
  readonly #view: AgentSessionViewPort;
  readonly #batches = new Map<string, InterruptResponseBatch>();
  readonly #unsubscribe: () => void;
  #retired = false;

  constructor(view: AgentSessionViewPort) {
    this.#view = view;
    this.#unsubscribe = view.subscribeSessions((sessions) => this.#reconcile(sessions));
  }

  stage(
    { sessionId, rootRunId, itemId }: InterruptRef,
    response: InterruptResumeInput["response"],
    hooks?: { onSettled?: () => void; onError?: () => void },
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
    if (batch.submitting) return false;

    batch.stage(itemId, {
      input: { itemId, response },
      ...hooks,
    });

    if (!batch.covers(expected)) return true;
    batch.submit();
    const ordered = batch.ordered(expected);
    const accepted = entry.resume(
      rootRunId,
      ordered.map(({ input }) => input),
      () => {
        if (this.#batches.get(key) !== batch) return;
        batch.settle();
        for (const response of batch.responses()) response.onSettled?.();
      },
      () => this.#reject(batch),
    );
    if (!accepted && this.#batches.get(key) === batch) this.#reject(batch);
    return true;
  }

  isStaged({ sessionId, rootRunId, itemId }: InterruptRef): boolean {
    return this.#batches.get(batchKey(sessionId, rootRunId))?.contains(itemId) ?? false;
  }

  discard(): void {
    for (const batch of [...this.#batches.values()]) {
      if (batch.settled) this.#drop(batch);
      else this.#reject(batch);
    }
  }

  retire(): void {
    if (this.#retired) return;
    this.#retired = true;
    this.#unsubscribe();
    this.discard();
  }

  #reject(batch: InterruptResponseBatch): void {
    const key = batchKey(batch.sessionId, batch.rootRunId);
    if (this.#batches.get(key) !== batch) return;
    // A settled response is final; a reject arriving afterwards is stale and must
    // not roll the card back as if the answer never reached the Runtime.
    if (batch.settled) return;
    this.#batches.delete(key);
    for (const response of batch.responses()) {
      response.onError?.();
    }
  }

  #drop(batch: InterruptResponseBatch): void {
    const key = batchKey(batch.sessionId, batch.rootRunId);
    if (this.#batches.get(key) === batch) this.#batches.delete(key);
  }

  #reconcile(sessions: Record<string, AgentSessionViewEntry>): void {
    if (this.#retired) return;
    for (const batch of [...this.#batches.values()]) {
      const entry = sessions[batch.sessionId];
      const open = entry ? new Set(openResponseIds(entry, batch.rootRunId)) : new Set<string>();
      if (batch.settled) {
        // Retain the accepted response until the view stops listing its items, so
        // the window between acceptance and refresh cannot admit a duplicate resume.
        if (!batch.items().some((itemId) => open.has(itemId))) this.#drop(batch);
      } else if (!batch.submitting && batch.items().some((itemId) => !open.has(itemId))) {
        this.#reject(batch);
      }
    }
  }
}

const interruptResponsePublication = createPublicationSlot<InterruptResponseCoordinator>();

function coordinator(): InterruptResponseCoordinator {
  const current = interruptResponsePublication.current();
  if (!current) throw new Error("Interrupt response coordinator is not installed");
  return current;
}

export function stageInterruptResponse(
  ref: InterruptRef,
  response: InterruptResumeInput["response"],
  hooks?: { onSettled?: () => void; onError?: () => void },
): boolean {
  return coordinator().stage(ref, response, hooks);
}

export function interruptResponseIsStaged(ref: InterruptRef): boolean {
  // A read with no coordinator installed means nothing is staged; only the mutation
  // path requires the owner to exist.
  return interruptResponsePublication.current()?.isStaged(ref) ?? false;
}

export function installInterruptResponseCoordinator(): () => void {
  const next = new InterruptResponseCoordinator(agentSessionView());
  interruptResponsePublication.publish(next, (predecessor) => predecessor.retire());
  return () => {
    interruptResponsePublication.withdraw(next);
    next.retire();
  };
}
