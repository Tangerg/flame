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
import { isErrorType, OperationFailure, RpcCancellationError } from "./errors";
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

export interface PluginCommandIdentity {
  readonly idempotencyKey: string;
  readonly binding: PluginCallRoute;
  matchesIntent(intent: string): boolean;
}

export interface PluginCommandReservation extends PluginCommandIdentity {
  readonly state: "recoverable";
  readonly namespace: string;
  reserve(action: PluginRecoveryAction): MutationReservation;
  validate(request: CallPluginRequest): void;
}

export type ActivePluginCommand = PluginCommandIdentity & { readonly state: "active" };
type PluginCommandEntry = ActivePluginCommand | PluginCommandReservation;

type PluginRecoveryAction = "recover" | "retire";
type PluginCommandResolution =
  | { readonly type: "confirmed" }
  | { readonly type: "retired" }
  | { readonly type: "refused"; readonly code?: string };

export type PluginCommandChange = { readonly idempotencyKey: string } & (
  | { readonly type: "availability" }
  | { readonly type: "resolved"; readonly resolution: PluginCommandResolution }
);

function recoveryResolution(
  action: PluginRecoveryAction,
  confirmation: MutationConfirmation<unknown>,
): PluginCommandResolution {
  if (confirmation.type === "accepted")
    return { type: action === "retire" ? "retired" : "confirmed" };
  if (isErrorType(confirmation.error, "plugin_command_retired")) return { type: "retired" };
  return confirmation.error instanceof OperationFailure
    ? { type: "refused", code: confirmation.error.code }
    : { type: "refused" };
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
  #publishResolution?: () => void;

  constructor(
    private readonly confirmation: MutationConfirmation<T>,
    cleanup: () => void,
    publishResolution?: () => void,
  ) {
    this.#cleanup = cleanup;
    this.#publishResolution = publishResolution;
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

  publishResolution(): boolean {
    const publish = this.#publishResolution;
    if (!publish) return false;
    this.#publishResolution = undefined;
    publish();
    return true;
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

const PLUGIN_COMMANDS_CHANGED = "plugin-commands-changed";

export class MutationJournal {
  readonly #journal: DurableMutationJournal;
  readonly #claims = new Map<string, DurableMutationIdentity>();
  readonly #preparing = new Set<string>();
  readonly #lifetime = new AbortController();
  readonly #pluginCommandChanges = new EventTarget();
  #pluginCommandVersion = 0;

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

  pluginCommands(): PluginCommandEntry[] {
    this.#assertCurrent();
    return this.#journal.pendingEntries().flatMap<PluginCommandEntry>((entry) => {
      const { command } = entry;
      if (command.kind !== "plugin") return [];
      const identity = mutationIdentity(entry);
      const reference: PluginCommandIdentity = {
        idempotencyKey: identity.idempotencyKey,
        binding: copyJournalProjection(command.route),
        matchesIntent: (intent) => {
          this.#assertCurrent();
          return this.#journal.matchesPluginCommandIntent(identity, intent);
        },
      };
      if (this.#claims.has(identity.idempotencyKey))
        return [{ ...reference, state: "active" }];
      return [
        {
          ...reference,
          state: "recoverable",
          namespace: identity.namespace,
          reserve: (action) => {
            this.#assertCurrent();
            if (this.#claims.has(identity.idempotencyKey))
              throw new MutationJournalOwnershipError(
                "plugin command is already owned by an active attempt",
              );
            return this.#claim(identity, action);
          },
          validate: (request) => {
            this.#assertClaim(identity);
            this.#journal.validatePluginCommand(identity, request);
          },
        },
      ];
    });
  }

  pluginCommandsVersion = (): number => this.#pluginCommandVersion;

  subscribePluginCommands = (listener: (change: PluginCommandChange) => void): (() => void) => {
    const changes = this.#pluginCommandChanges;
    const notify = (event: Event) => listener((event as CustomEvent<PluginCommandChange>).detail);
    changes.addEventListener(PLUGIN_COMMANDS_CHANGED, notify, {
      signal: this.#lifetime.signal,
    });
    return () => changes.removeEventListener(PLUGIN_COMMANDS_CHANGED, notify);
  };

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

  #claim(identity: DurableMutationIdentity, recovery?: PluginRecoveryAction): MutationReservation {
    this.#own(identity);
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
      track: (create) => this.#track(identity, create, recovery),
    };
  }

  dispose(): void {
    if (this.#lifetime.signal.aborted) return;
    this.#lifetime.abort();
    this.#claims.clear();
  }

  #own(identity: DurableMutationIdentity): void {
    this.#claims.set(identity.idempotencyKey, identity);
    this.#publishPluginCommandChange(identity);
  }

  #publishPluginCommandChange(
    identity: DurableMutationIdentity,
    resolution?: PluginCommandResolution,
  ): void {
    if (identity.command.kind !== "plugin") return;
    this.#pluginCommandVersion += 1;
    const idempotencyKey = identity.idempotencyKey;
    const change: PluginCommandChange = resolution
      ? { type: "resolved", idempotencyKey, resolution }
      : { type: "availability", idempotencyKey };
    this.#pluginCommandChanges.dispatchEvent(
      new CustomEvent(PLUGIN_COMMANDS_CHANGED, { detail: change }),
    );
  }

  #track<T>(
    identity: DurableMutationIdentity,
    create: () => MutationPromise<T>,
    recovery?: PluginRecoveryAction,
  ): MutationPromise<T> {
    const idempotencyKey = identity.idempotencyKey;
    let activeAttempts = 0;
    let confirmed: ConfirmedMutation<T> | undefined;
    let reopen: (() => void) | undefined;
    const releaseClaim = () => {
      if (this.#claims.get(idempotencyKey) !== identity) return;
      this.#claims.delete(idempotencyKey);
      if (!confirmed?.publishResolution()) this.#publishPluginCommandChange(identity);
    };
    const finish = (outcome: AttemptOutcome<T>) => {
      if (outcome.type === "confirmed" && !confirmed) {
        const resolution = recovery && recoveryResolution(recovery, outcome.confirmation);
        confirmed = new ConfirmedMutation(
          outcome.confirmation,
          () => {
            this.#journal.settle(identity);
            releaseClaim();
          },
          resolution && (() => this.#publishPluginCommandChange(identity, resolution)),
        );
      }
      activeAttempts--;
      if (activeAttempts !== 0) return;
      if (confirmed) {
        this.#cleanConfirmed(confirmed);
        confirmed.publishResolution();
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
            this.#own(identity);
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
    if (this.#lifetime.signal.aborted) {
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
