import {
  openDurableMutationJournal,
  type DurableMutationJournal,
  type DurableMutationJournalOptions,
  type DispatchPhase,
} from "./durableMutationJournal";
import type { WireMethodName } from "@flame/runtime-client/methods";
import {
  mutationIdentity,
  type DurableMutationIdentity,
  type PendingRuntimeCommand,
  type RunCommand,
  type PluginCommandInput,
  type PluginCommandBinding,
} from "./mutationJournalEntry";
import {
  MutationJournalCleanupError,
  MutationJournalError,
  MutationJournalOwnershipError,
  type MutationConfirmation,
} from "./mutationJournalErrors";
import {
  MutationLookupError,
  mutationHandle,
  mutationSettlementIsUnknown,
  type MutationPromise,
} from "./mutation";
import type { CallPluginRequest, PluginCallRoute } from "@flame/runtime-client/wire";
import { RpcCancellationError } from "./errors";
import { captureRequestMeta } from "./request";
import { cloneJSON } from "@frame/sdk/json";

export type { PluginCommandInput, PluginCommandBinding } from "./mutationJournalEntry";
import type { RequestMeta } from "./generated/wire.generated";

export {
  MutationJournalCapacityError,
  MutationJournalOwnershipError,
  MutationJournalScopeUnavailableError,
  MutationJournalStorageError,
} from "./mutationJournalErrors";
export type { MutationJournalScope, MutationJournalStorage } from "./durableMutationJournal";

export interface MutationReservation {
  readonly idempotencyKey: string;
  readonly steerItemId: string | null;
  bindSteerItem(userItemId: string): string;
  readonly requestMeta: RequestMeta | null;
  lookupNamespace(): string;
  authorizeDispatch(): string;
  markDispatched(): DispatchPhase;
  track<T>(create: () => MutationPromise<T>): MutationPromise<T>;
}

export interface PendingPluginCommand {
  readonly idempotencyKey: string;
  readonly namespace: string;
  readonly binding: PluginCallRoute;
  matchesIntent(intent: string): boolean;
  reserve(): MutationReservation;
  validate(request: CallPluginRequest): void;
}

export type PendingRunCommand = RunCommand & {
  readonly idempotencyKey: string;
  readonly namespace: string;
  readonly steerItemId: string | null;
  acknowledge(): void;
  reserve(): MutationReservation;
};

type AttemptOutcome<T> =
  | { type: "confirmed"; confirmation: MutationConfirmation<T> }
  | { type: "unresolved" }
  | { type: "canceled" };

export type MutationJournalCleanupObserver = (failure: MutationJournalCleanupError) => void;

class ConfirmedMutation<T> {
  #cleanup: (() => void) | undefined;

  constructor(
    private readonly confirmation: MutationConfirmation<T>,
    cleanup: () => void,
  ) {
    this.#cleanup = cleanup;
  }

  tryCleanup(): MutationJournalCleanupError<T> | undefined {
    if (!this.#cleanup) return;
    try {
      this.#cleanup();
    } catch (cause) {
      return new MutationJournalCleanupError(
        this.confirmation,
        () => {
          const failure = this.tryCleanup();
          if (failure) throw failure;
        },
        cause,
      );
    }
    this.#cleanup = undefined;
  }

  result(): T {
    if (this.confirmation.type === "refused") throw this.confirmation.error;
    return this.confirmation.result;
  }
}

// Caller-owned copies of admitted journal data must retain exact JSON numbers.
function copyJournalProjection<T>(value: T): T {
  const copy: unknown = cloneJSON(value);
  return copy as T;
}

export class MutationJournal {
  readonly #journal: DurableMutationJournal;
  readonly #claims = new Map<string, DurableMutationIdentity>();
  readonly #preparing = new Set<string>();
  #retired = false;

  constructor(
    journal: DurableMutationJournal,
    private readonly onCleanupFailure: MutationJournalCleanupObserver = (failure) =>
      console.error(failure),
  ) {
    this.#journal = journal;
  }

  pendingRuntimeCommands(): PendingRuntimeCommand[] {
    this.#assertCurrent();
    return this.#journal.pendingEntries().flatMap(({ command, idempotencyKey, namespace }) => {
      if (command.kind !== "method" || this.#claims.has(idempotencyKey)) return [];
      return [{ method: command.method, idempotencyKey, namespace }];
    });
  }

  recoverRuntimeCommand(command: PendingRuntimeCommand, params: unknown): MutationReservation {
    this.#assertCurrent();
    if (this.#claims.has(command.idempotencyKey)) {
      throw new MutationJournalOwnershipError(
        "Runtime mutation identity is already owned by an active command",
      );
    }
    return this.#claim(this.#journal.recoverRuntimeCommand(command, params));
  }

  pendingPluginCommands(): PendingPluginCommand[] {
    this.#assertCurrent();
    return this.#journal.pendingEntries().flatMap((entry) => {
      const { command } = entry;
      if (command.kind !== "plugin" || this.#claims.has(entry.idempotencyKey)) return [];
      const identity = mutationIdentity(entry);
      const pending: PendingPluginCommand = {
        idempotencyKey: identity.idempotencyKey,
        namespace: identity.namespace,
        binding: copyJournalProjection(command.route),
        matchesIntent: (intent) => {
          this.#assertCurrent();
          return this.#journal.matchesPluginCommandIntent(identity, intent);
        },
        reserve: () => {
          this.#assertCurrent();
          if (this.#claims.has(identity.idempotencyKey))
            throw new MutationJournalOwnershipError(
              "plugin command is already owned by an active attempt",
            );
          return this.#claim(identity);
        },
        validate: (request) => {
          this.#assertClaim(identity);
          this.#journal.validatePluginCommand(identity, request);
        },
      };
      return [pending];
    });
  }

  async capturePluginCommand(
    intent: string,
    input: PluginCommandInput,
    prepare: () => Promise<PluginCommandBinding>,
    requestMeta?: () => RequestMeta | undefined,
    signal?: AbortSignal,
  ): Promise<{ request: CallPluginRequest; reservation: MutationReservation }> {
    this.#assertCurrent();
    if (signal?.aborted) throw new RpcCancellationError(signal.reason);
    if (this.#preparing.has(intent))
      throw new MutationJournalOwnershipError("plugin command intent is already being prepared");
    this.#preparing.add(intent);
    try {
      const preparedInput: PluginCommandInput = {
        contract: input.contract,
        method: input.method,
        input: input.input,
      };
      let captured = this.#journal.findPluginCommand(intent, preparedInput, (key) =>
        this.#claims.has(key),
      );
      if (!captured) {
        const metadata = captureRequestMeta(requestMeta?.());
        const binding = await prepare();
        this.#assertCurrent();
        if (signal?.aborted) throw new RpcCancellationError(signal.reason);
        captured = this.#journal.stagePluginCommand(
          intent,
          {
            route: { ...binding, contract: preparedInput.contract, method: preparedInput.method },
            input: preparedInput.input,
          },
          (key) => this.#claims.has(key),
          metadata,
        );
      }
      return { request: captured.request, reservation: this.#claim(captured.identity) };
    } finally {
      this.#preparing.delete(intent);
    }
  }
  pendingCommands(): PendingRunCommand[] {
    this.#assertCurrent();
    return this.#journal.pendingEntries().flatMap((entry) => {
      const { command } = entry;
      if (command.kind !== "run" || this.#claims.has(entry.idempotencyKey)) return [];
      const identity = mutationIdentity(entry);
      const pending: PendingRunCommand = {
        ...copyJournalProjection(command.request),
        idempotencyKey: identity.idempotencyKey,
        namespace: identity.namespace,
        steerItemId: entry.steerItemId,
        acknowledge: () => this.#journal.settle(identity),
        reserve: () => {
          this.#assertCurrent();
          if (this.#claims.has(identity.idempotencyKey))
            throw new MutationJournalOwnershipError(
              "Runtime mutation identity is already owned by an active command",
            );
          return this.#claim(identity);
        },
      };
      return [pending];
    });
  }

  reserve(
    method: WireMethodName,
    params: unknown,
    requestedKey?: string,
    requestMeta: RequestMeta | null = null,
  ): MutationReservation {
    this.#assertCurrent();
    const identity = this.#journal.reserve(
      method,
      params,
      requestedKey,
      (idempotencyKey) => this.#claims.has(idempotencyKey),
      requestMeta,
    );
    return this.#claim(identity);
  }

  #claim(identity: DurableMutationIdentity): MutationReservation {
    this.#claims.set(identity.idempotencyKey, identity);
    const journal = this.#journal;
    return {
      idempotencyKey: identity.idempotencyKey,
      get steerItemId() {
        return journal.steerItemId(identity);
      },
      bindSteerItem: (userItemId) => {
        this.#assertClaim(identity);
        return journal.bindSteerItem(identity, userItemId);
      },
      get requestMeta() {
        return copyJournalProjection(identity.requestMeta);
      },
      lookupNamespace: () => {
        this.#assertClaim(identity);
        return this.#journal.lookupNamespace(identity);
      },
      authorizeDispatch: () => {
        this.#assertClaim(identity);
        return this.#journal.authorizeDispatch(identity);
      },
      markDispatched: () => {
        this.#assertClaim(identity);
        return this.#journal.markDispatched(identity);
      },
      track: (create) => this.#track(identity, create),
    };
  }

  dispose(): void {
    if (this.#retired) return;
    this.#retired = true;
    this.#claims.clear();
  }

  #track<T>(
    identity: DurableMutationIdentity,
    create: () => MutationPromise<T>,
  ): MutationPromise<T> {
    const idempotencyKey = identity.idempotencyKey;
    let activeAttempts = 0;
    let confirmed: ConfirmedMutation<T> | undefined;
    let reopen: (() => void) | undefined;
    const releaseClaim = () => {
      if (this.#claims.get(idempotencyKey) === identity) this.#claims.delete(idempotencyKey);
    };
    const finish = (outcome: AttemptOutcome<T>) => {
      if (outcome.type === "confirmed") {
        confirmed ??= new ConfirmedMutation(outcome.confirmation, () => {
          this.#journal.settle(identity);
          releaseClaim();
        });
      }
      activeAttempts--;
      if (activeAttempts !== 0) return;
      if (confirmed) {
        this.#cleanConfirmed(confirmed);
        return;
      }
      try {
        if (outcome.type === "canceled") reopen = this.#journal.withdrawBeforeDispatch(identity);
      } finally {
        releaseClaim();
      }
    };
    const observe = (start: () => MutationPromise<T>): MutationPromise<T> => {
      this.#assertClaim(identity);
      activeAttempts += 1;
      let mutation: MutationPromise<T>;
      try {
        mutation = start();
      } catch (cause) {
        finish({ type: "unresolved" });
        throw new MutationLookupError(cause);
      }
      const tracked = mutation.then(
        (value) => {
          finish({ type: "confirmed", confirmation: { type: "accepted", result: value } });
          return value;
        },
        (error: unknown) => {
          if (error instanceof RpcCancellationError) {
            finish({ type: "canceled" });
          } else if (mutationSettlementIsUnknown(error) || error instanceof MutationJournalError) {
            finish({ type: "unresolved" });
          } else {
            finish({ type: "confirmed", confirmation: { type: "refused", error } });
          }
          throw error;
        },
      );
      const retry: MutationPromise<T>["retry"] = (options) => {
        const observed = confirmed;
        if (observed) {
          const result = Promise.resolve().then(() => {
            // The last active attempt retires the entry that the other attempts still use.
            if (activeAttempts === 0) this.#cleanConfirmed(observed);
            return observed.result();
          });
          return mutationHandle(result, idempotencyKey, retry);
        }
        try {
          this.#assertCurrent();
          const claimed = this.#claims.get(idempotencyKey);
          if (claimed && claimed !== identity)
            throw new MutationJournalOwnershipError(
              "Runtime mutation identity is already owned by another attempt",
            );
          if (!claimed) {
            reopen?.();
            reopen = undefined;
            this.#claims.set(idempotencyKey, identity);
          }
          return observe(() => mutation.retry(options));
        } catch (error) {
          return mutationHandle(Promise.reject(error), idempotencyKey, retry);
        }
      };
      return mutationHandle(tracked, idempotencyKey, retry);
    };
    return observe(create);
  }

  #cleanConfirmed<T>(confirmed: ConfirmedMutation<T>): void {
    const failure = confirmed.tryCleanup();
    if (!failure) return;
    try {
      this.onCleanupFailure(failure);
    } catch (observerFailure) {
      console.error(
        new AggregateError([failure, observerFailure], "mutation journal cleanup observer failed"),
      );
    }
  }

  #assertClaim(identity: DurableMutationIdentity): void {
    this.#assertCurrent();
    if (this.#claims.get(identity.idempotencyKey) !== identity) {
      throw new MutationJournalOwnershipError(
        "Runtime mutation identity is no longer owned by this client",
      );
    }
  }

  #assertCurrent(): void {
    if (this.#retired) {
      throw new MutationJournalOwnershipError("Runtime mutation client was closed");
    }
  }
}

export function createMutationJournal({
  onCleanupFailure,
  ...options
}: Omit<DurableMutationJournalOptions, "storage" | "now"> &
  Partial<Pick<DurableMutationJournalOptions, "storage" | "now">> & {
    onCleanupFailure?: MutationJournalCleanupObserver;
  }): MutationJournal {
  const values = new Map<string, unknown>();
  const storage = options.storage ?? {
    get: (key: string) => values.get(key),
    set: (key: string, value: unknown) => void values.set(key, value),
    remove: (key: string) => void values.delete(key),
    keys: () => [...values.keys()],
  };
  return new MutationJournal(
    openDurableMutationJournal({ ...options, storage, now: options.now ?? Date.now }),
    onCleanupFailure,
  );
}
