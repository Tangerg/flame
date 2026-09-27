import {
  entryKey,
  MutationJournalError,
  MutationJournalOwnershipError,
  MutationJournalStorageError,
  openDurableMutationJournal,
  type DurableMutationIdentity,
  type DurableMutationJournal,
  type MutationJournalScope,
  type MutationJournalStorage,
} from "./durableMutationJournal";
import { mutationSettlementIsUnknown, type MutationPromise } from "./mutation";
import {
  WIRE_METHOD_POLICY,
  wireMethodRequiresIdempotency,
  type WireMutationMethodName,
  type WireParams,
} from "@flame/runtime-contract/methods";
import { validateMethodParams } from "@flame/runtime-contract/validate";

export {
  MutationJournalCapacityError,
  MutationJournalOwnershipError,
  MutationJournalScopeUnavailableError,
  MutationJournalStorageError,
} from "./durableMutationJournal";
export type { MutationJournalScope, MutationJournalStorage } from "./durableMutationJournal";

export interface MutationReservation {
  readonly idempotencyKey: string;
  authorizeAttempt(): string;
  track<T>(mutation: MutationPromise<T>): MutationPromise<T>;
}

export interface MutationJournal {
  reserve(method: string, params: unknown, preferredKey?: string): MutationReservation | undefined;
  dispose(): void;
}

export interface MutationJournalOptions {
  storage: MutationJournalStorage;
  scope: () => MutationJournalScope | null | undefined;
  now?: () => number;
}

export type MutationCommand = {
  [M in WireMutationMethodName]: { method: M; params: WireParams<M> };
}[WireMutationMethodName];

export type PreparedMutation = MutationCommand & { idempotencyKey: string };

export interface PreparedMutationJournal {
  prepare(command: MutationCommand): PreparedMutation;
  read(idempotencyKey: string): PreparedMutation | undefined;
  list(): PreparedMutation[];
  recover(idempotencyKey: string, method: string, params: unknown): MutationReservation;
  dispose(): void;
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
  mutation: MutationPromise<T>,
  lifecycle: MutationLifecycle,
): MutationPromise<T> {
  lifecycle.begin();
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
      return trackedMutation(mutation.retry(options), lifecycle);
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
  #retired = false;

  constructor(journal: DurableMutationJournal) {
    this.#journal = journal;
  }

  reserve(method: string, params: unknown, preferredKey?: string): MutationReservation | undefined {
    this.assertCurrent();
    const identity = this.#journal.reserve(method, params, preferredKey, (idempotencyKey) =>
      this.#claims.has(idempotencyKey),
    );
    if (!identity) return undefined;
    return this.#claim(identity);
  }

  recover(method: string, params: unknown, idempotencyKey: string): MutationReservation {
    this.assertCurrent();
    if (this.#claims.has(idempotencyKey)) {
      throw new MutationJournalOwnershipError(
        "Runtime mutation identity is already owned by an active command",
      );
    }
    return this.#claim(this.#journal.recover(method, params, idempotencyKey));
  }

  #claim(identity: DurableMutationIdentity): MutationReservation {
    this.#claims.add(identity.entry.idempotencyKey);
    const lifecycle = this.#lifecycle(identity);
    return {
      idempotencyKey: identity.entry.idempotencyKey,
      authorizeAttempt: () => {
        this.#assertClaim(identity);
        return this.#journal.authorize(identity);
      },
      track: (mutation) => trackedMutation(mutation, lifecycle),
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
      activeAttempts = Math.max(0, activeAttempts - 1);
      this.#assertClaim(identity);
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
        this.assertCurrent();
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
    this.assertCurrent();
    if (!this.#claims.has(identity.entry.idempotencyKey)) {
      throw new MutationJournalOwnershipError(
        "Runtime mutation identity is no longer owned by this client",
      );
    }
  }

  assertCurrent(): void {
    if (this.#retired) {
      throw new MutationJournalOwnershipError("Runtime mutation client was closed");
    }
  }
}

export function createMutationJournal(options: MutationJournalOptions): MutationJournal {
  return new MutationAuthority(
    openDurableMutationJournal({ ...options, now: options.now ?? Date.now }),
  );
}

interface PreparedRecord {
  method: WireMutationMethodName;
  params: unknown;
  identity: unknown;
}

function validateMutationCommand(command: { method?: unknown; params?: unknown }): void {
  if (
    typeof command.method !== "string" ||
    !Object.hasOwn(WIRE_METHOD_POLICY, command.method) ||
    !wireMethodRequiresIdempotency(command.method as WireMutationMethodName) ||
    validateMethodParams(command.method as WireMutationMethodName, command.params).length > 0
  ) {
    throw new MutationJournalStorageError("Runtime prepared mutation parameters are invalid");
  }
}

function preparedRecord(value: unknown): PreparedRecord {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new MutationJournalStorageError("Runtime prepared mutation record is invalid");
  }
  const record = value as Partial<PreparedRecord>;
  if (Object.keys(record).length !== 3 || record.identity === undefined) {
    throw new MutationJournalStorageError("Runtime prepared mutation record is invalid");
  }
  validateMutationCommand(record);
  return record as PreparedRecord;
}

// Exact input and its replay identity share one atomic storage record. The
// ordinary journal deliberately stores only fingerprints, not credential-bearing inputs.
export function createPreparedMutationJournal(
  options: MutationJournalOptions,
): PreparedMutationJournal {
  let preparing: MutationCommand | undefined;
  const readRecord = (key: string): PreparedRecord | undefined => {
    let value: unknown;
    try {
      value = options.storage.get(key);
    } catch (cause) {
      throw new MutationJournalStorageError("Runtime prepared mutation record is unreadable", {
        cause,
      });
    }
    return value === undefined ? undefined : preparedRecord(value);
  };
  const journal = openDurableMutationJournal({
    ...options,
    now: options.now ?? Date.now,
    storage: {
      keys: () => options.storage.keys(),
      get: (key) => readRecord(key)?.identity,
      set: (key, identity) => {
        const command = readRecord(key) ?? preparing;
        if (!command) {
          throw new MutationJournalStorageError(
            "Runtime prepared mutation input is no longer retained",
          );
        }
        options.storage.set(
          key,
          structuredClone({ method: command.method, params: command.params, identity }),
        );
      },
      remove: (key) => options.storage.remove(key),
    },
  });
  const authority = new MutationAuthority(journal);
  const read = (idempotencyKey: string): PreparedMutation | undefined => {
    authority.assertCurrent();
    const record = readRecord(entryKey(idempotencyKey));
    if (!record) return undefined;
    if (!journal.inspect(record.method, record.params, idempotencyKey)) return undefined;
    return structuredClone({
      method: record.method,
      params: record.params,
      idempotencyKey,
    }) as PreparedMutation;
  };
  return {
    prepare(command) {
      authority.assertCurrent();
      preparing = structuredClone(command);
      try {
        validateMutationCommand(preparing);
        const identity = journal.prepare(preparing.method, preparing.params);
        return structuredClone({ ...preparing, idempotencyKey: identity.entry.idempotencyKey });
      } finally {
        preparing = undefined;
      }
    },
    read,
    list() {
      authority.assertCurrent();
      return journal.entries().flatMap(({ entry }) => {
        const command = read(entry.idempotencyKey);
        return command ? [command] : [];
      });
    },
    recover(idempotencyKey, method, params) {
      if (!read(idempotencyKey)) {
        throw new MutationJournalOwnershipError(
          "Runtime prepared mutation identity is no longer retained",
        );
      }
      return authority.recover(method, params, idempotencyKey);
    },
    dispose() {
      authority.dispose();
    },
  };
}
