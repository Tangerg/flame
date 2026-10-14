import {
  MutationJournalError,
  MutationJournalOwnershipError,
  openDurableMutationJournal,
  type DurableMutationIdentity,
  type DurableMutationJournal,
  type MutationJournalScope,
  type MutationJournalStorage,
  type RunCommand,
  type PluginCommandInput,
  type PluginCommandBinding,
} from "./durableMutationJournal";
import { MutationLookupError, mutationSettlementIsUnknown, type MutationPromise } from "./mutation";
import type { CallPluginRequest } from "@flame/runtime-contract/wire";

export type { PluginCommandInput, PluginCommandBinding } from "./durableMutationJournal";
import type { RequestMeta } from "../wire.generated";

export {
  MutationJournalCapacityError,
  MutationJournalOwnershipError,
  MutationJournalScopeUnavailableError,
  MutationJournalStorageError,
} from "./durableMutationJournal";
export type { MutationJournalScope, MutationJournalStorage } from "./durableMutationJournal";

export interface MutationReservation {
  readonly idempotencyKey: string;
  readonly requestMeta: RequestMeta | null;
  lookupNamespace(): string;
  authorizeDispatch(): string;
  track<T>(create: () => MutationPromise<T>): MutationPromise<T>;
}

export interface MutationJournal {
  capturePluginCommand(
    intent: string,
    input: PluginCommandInput,
    prepare: () => Promise<PluginCommandBinding>,
  ): Promise<{ request: CallPluginRequest; reservation: MutationReservation }>;
  pendingCommands(): PendingRunCommand[];
  reserve(
    method: string,
    params: unknown,
    preferredKey?: string,
    requestMeta?: RequestMeta | null,
  ): MutationReservation | undefined;
  dispose(): void;
}

export type PendingRunCommand = RunCommand & {
  readonly idempotencyKey: string;
  readonly namespace: string;
  settle(): void;
  reserve(): MutationReservation;
};

export interface MutationJournalOptions {
  storage?: MutationJournalStorage;
  scope: () => MutationJournalScope | null | undefined;
  now?: () => number;
}

function rejectedMutation<T>(
  error: unknown,
  idempotencyKey: string,
  retry: (options?: { signal?: AbortSignal }) => MutationPromise<T>,
): MutationPromise<T> {
  const rejected = Promise.reject(error);
  return Object.defineProperties(rejected, {
    idempotencyKey: { enumerable: true, value: idempotencyKey },
    retry: { enumerable: true, value: retry },
  }) as unknown as MutationPromise<T>;
}

interface MutationLifecycle {
  begin(): void;
  claim(): void;
  resolve(): void;
  reject(error: unknown): void;
}

function trackedMutation<T>(
  create: () => MutationPromise<T>,
  lifecycle: MutationLifecycle,
): MutationPromise<T> {
  lifecycle.begin();
  let mutation: MutationPromise<T>;
  try {
    mutation = create();
  } catch (error) {
    const unknown = new MutationLookupError(error);
    lifecycle.reject(unknown);
    throw unknown;
  }
  const tracked = mutation.then(
    (value) => {
      lifecycle.resolve();
      return value;
    },
    (error: unknown) => {
      lifecycle.reject(error);
      throw error;
    },
  );
  const retry = (options?: { signal?: AbortSignal }): MutationPromise<T> => {
    try {
      lifecycle.claim();
      return trackedMutation(() => mutation.retry(options), lifecycle);
    } catch (error) {
      return rejectedMutation(error, mutation.idempotencyKey, retry);
    }
  };
  return Object.defineProperties(tracked, {
    idempotencyKey: { enumerable: true, get: () => mutation.idempotencyKey },
    retry: { enumerable: true, value: retry },
  }) as MutationPromise<T>;
}

class MutationAuthority implements MutationJournal {
  readonly #journal: DurableMutationJournal;
  readonly #claims = new Set<string>();
  readonly #preparing = new Set<string>();
  #retired = false;

  constructor(journal: DurableMutationJournal) {
    this.#journal = journal;
  }

  async capturePluginCommand(
    intent: string,
    input: PluginCommandInput,
    prepare: () => Promise<PluginCommandBinding>,
  ): Promise<{ request: CallPluginRequest; reservation: MutationReservation }> {
    this.#assertCurrent();
    if (this.#preparing.has(intent))
      throw new MutationJournalOwnershipError("plugin command intent is already being prepared");
    this.#preparing.add(intent);
    try {
      const preparedInput = structuredClone(input);
      let captured = this.#journal.findPluginCommand(intent, preparedInput, (key) =>
        this.#claims.has(key),
      );
      if (!captured) {
        const binding = await prepare();
        this.#assertCurrent();
        captured = this.#journal.stagePluginCommand(
          intent,
          { ...binding, ...preparedInput },
          (key) => this.#claims.has(key),
        );
      }
      return { request: captured.request, reservation: this.#claim(captured.identity) };
    } finally {
      this.#preparing.delete(intent);
    }
  }
  pendingCommands(): PendingRunCommand[] {
    this.#assertCurrent();
    return this.#journal
      .pendingCommands()
      .filter((identity) => !this.#claims.has(identity.entry.idempotencyKey))
      .map((identity) => ({
        ...structuredClone(identity.entry.command!),
        idempotencyKey: identity.entry.idempotencyKey,
        namespace: this.#journal.lookupNamespace(identity),
        settle: () => {
          this.#assertCurrent();
          this.#journal.settle(identity);
        },
        reserve: () => {
          this.#assertCurrent();
          if (this.#claims.has(identity.entry.idempotencyKey))
            throw new MutationJournalOwnershipError(
              "Runtime mutation identity is already owned by an active command",
            );
          this.#journal.retain(identity);
          return this.#claim(identity);
        },
      }));
  }

  reserve(
    method: string,
    params: unknown,
    preferredKey?: string,
    requestMeta: RequestMeta | null = null,
  ): MutationReservation | undefined {
    this.#assertCurrent();
    const identity = this.#journal.reserve(
      method,
      params,
      preferredKey,
      (idempotencyKey) => this.#claims.has(idempotencyKey),
      requestMeta,
    );
    if (!identity) return undefined;
    return this.#claim(identity);
  }

  #claim(identity: DurableMutationIdentity): MutationReservation {
    this.#claims.add(identity.entry.idempotencyKey);
    const lifecycle = this.#lifecycle(identity);
    return {
      idempotencyKey: identity.entry.idempotencyKey,
      get requestMeta() {
        return structuredClone(identity.entry.command?.requestMeta ?? null);
      },
      lookupNamespace: () => {
        this.#assertClaim(identity);
        return this.#journal.lookupNamespace(identity);
      },
      authorizeDispatch: () => {
        this.#assertClaim(identity);
        return this.#journal.authorizeDispatch(identity);
      },
      track: (create) => trackedMutation(create, lifecycle),
    };
  }

  dispose(): void {
    if (this.#retired) return;
    this.#retired = true;
    this.#claims.clear();
  }

  #lifecycle(identity: DurableMutationIdentity): MutationLifecycle {
    const idempotencyKey = identity.entry.idempotencyKey;
    let activeAttempts = 0;
    let definitiveOutcome = false;
    const finish = (definitive: boolean) => {
      // Retirement revokes delivery, not a result's authority over its original
      // identity. The durable owner checks the exact record before removing it.
      activeAttempts = Math.max(0, activeAttempts - 1);
      definitiveOutcome ||= definitive;
      if (activeAttempts > 0) return;
      try {
        if (definitiveOutcome) this.#journal.settle(identity);
      } finally {
        definitiveOutcome = false;
        this.#claims.delete(idempotencyKey);
      }
    };
    return {
      begin: () => {
        this.#assertClaim(identity);
        activeAttempts += 1;
      },
      claim: () => {
        this.#assertCurrent();
        if (activeAttempts === 0) definitiveOutcome = false;
        if (!this.#claims.has(idempotencyKey)) {
          this.#journal.retain(identity);
          this.#claims.add(idempotencyKey);
        }
      },
      resolve: () => finish(true),
      reject: (error) =>
        finish(!mutationSettlementIsUnknown(error) && !(error instanceof MutationJournalError)),
    };
  }

  #assertClaim(identity: DurableMutationIdentity): void {
    this.#assertCurrent();
    if (!this.#claims.has(identity.entry.idempotencyKey)) {
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

export function createMutationJournal(options: MutationJournalOptions): MutationJournal {
  const values = new Map<string, unknown>();
  const storage = options.storage ?? {
    get: (key: string) => structuredClone(values.get(key)),
    set: (key: string, value: unknown) => void values.set(key, structuredClone(value)),
    remove: (key: string) => void values.delete(key),
    keys: () => [...values.keys()],
  };
  return new MutationAuthority(
    openDurableMutationJournal({ ...options, storage, now: options.now ?? Date.now }),
  );
}
