import type { CallPluginRequest, RequestMeta } from "@flame/runtime-client/wire";
import type { WireMethodName } from "@flame/runtime-client/methods";
import { checkRequest, pluginMethod } from "./request";
import { canonicalJSON, cloneJSON } from "@frame/sdk/json";
import {
  type CapturedPluginCommand,
  type DurableMutationIdentity,
  type JournalEntry,
  MAX_IDENTITY_LENGTH,
  type PluginCommandInput,
  captureJournalCommand,
  decodeJournalEntry,
  fingerprintCommand,
  isSteerCommand,
  matchesCommand,
  mutationIdentity,
  pluginRouteForIntent,
  validSteerItem,
  validText,
} from "./mutationJournalEntry";
import {
  MutationJournalCapacityError,
  MutationJournalError,
  MutationJournalOwnershipError,
  MutationJournalScopeUnavailableError,
  MutationJournalStorageError,
} from "./mutationJournalErrors";

const MAX_ENTRIES = 256;

const ENTRY_PREFIX = "entry:";

const MILLISECONDS_PER_SECOND = 1_000;

// Serialized stores must preserve exact JSON values with parseJSON/stringifyJSON.
export interface MutationJournalStorage {
  get(key: string): unknown;
  set(key: string, value: unknown): void;
  remove(key: string): void;
  keys(): string[];
}

export interface MutationJournalScope {
  namespace: string;
  retentionSeconds: number;
}

export type DispatchPhase = "initial" | "replay";

export interface DurableMutationJournal {
  matchesPluginCommandIntent(identity: DurableMutationIdentity, intent: string): boolean;
  findPluginCommand(
    intent: string,
    input: PluginCommandInput,
    claimed: (idempotencyKey: string) => boolean,
  ): CapturedPluginCommand | undefined;
  stagePluginCommand(
    intent: string,
    request: CallPluginRequest,
    claimed: (idempotencyKey: string) => boolean,
    requestMeta: RequestMeta | null,
  ): CapturedPluginCommand;
  pendingEntries(): JournalEntry[];
  validatePluginCommand(identity: DurableMutationIdentity, request: CallPluginRequest): void;
  reserve(
    method: WireMethodName,
    params: unknown,
    preferredKey: string | undefined,
    claimed: (idempotencyKey: string) => boolean,
    requestMeta: RequestMeta | null,
  ): DurableMutationIdentity;
  authorizeDispatch(identity: DurableMutationIdentity): string;
  markDispatched(identity: DurableMutationIdentity): DispatchPhase;
  lookupNamespace(identity: DurableMutationIdentity): string;
  steerItemId(identity: DurableMutationIdentity): string | null;
  bindSteerItem(identity: DurableMutationIdentity, userItemId: string): string;
  withdrawBeforeDispatch(identity: DurableMutationIdentity): (() => void) | undefined;
  settle(identity: DurableMutationIdentity): void;
}

export interface DurableMutationJournalOptions {
  storage: MutationJournalStorage;
  scope: () => MutationJournalScope | null | undefined;
  now: () => number;
  admitNewCommand(method: WireMethodName, params: unknown, requestMeta: RequestMeta | null): void;
}

function storageError(message: string, cause?: unknown): MutationJournalStorageError {
  if (cause instanceof MutationJournalStorageError) return cause;
  return new MutationJournalStorageError(message, cause === undefined ? undefined : { cause });
}

function entryKey(idempotencyKey: string): string {
  return `${ENTRY_PREFIX}${encodeURIComponent(idempotencyKey)}`;
}

function validScope(scope: MutationJournalScope | null | undefined): scope is MutationJournalScope {
  if (!scope) return false;
  return (
    validText(scope.namespace, MAX_IDENTITY_LENGTH) &&
    Number.isInteger(scope.retentionSeconds) &&
    scope.retentionSeconds > 0
  );
}

function readKeys(storage: MutationJournalStorage): string[] {
  try {
    const keys = storage.keys();
    if (!Array.isArray(keys) || keys.some((key) => typeof key !== "string")) {
      throw new TypeError("storage returned invalid keys");
    }
    return [...new Set(keys)];
  } catch (error) {
    throw storageError("Runtime mutation journal keys are unavailable", error);
  }
}

// Storage adapters may retain objects. Capture at their boundary so a record and
// the active identity cannot advance each other through a shared reference.
function readValue(storage: MutationJournalStorage, key: string): unknown {
  try {
    return cloneJSON(storage.get(key));
  } catch (error) {
    throw storageError(`Runtime mutation journal record is unreadable: ${key}`, error);
  }
}

function persistValue(storage: MutationJournalStorage, key: string, value: unknown): void {
  try {
    storage.set(key, cloneJSON(value));
  } catch (error) {
    throw storageError(`Runtime mutation journal record was not persisted: ${key}`, error);
  }
}

function removeValue(storage: MutationJournalStorage, key: string): void {
  try {
    storage.remove(key);
  } catch (error) {
    throw storageError(`Runtime mutation journal record was not removed: ${key}`, error);
  }
}

function readEntry(storage: MutationJournalStorage, key: string): JournalEntry | undefined {
  const value = readValue(storage, key);
  if (value === undefined) return undefined;
  const entry = decodeJournalEntry(value);
  if (!entry || entryKey(entry.idempotencyKey) !== key)
    throw storageError(`Runtime mutation journal entry is corrupted: ${key}`);
  return entry;
}

function loadEntries(storage: MutationJournalStorage): JournalEntry[] {
  return readKeys(storage)
    .filter((key) => key.startsWith(ENTRY_PREFIX))
    .flatMap((key) => {
      const entry = readEntry(storage, key);
      return entry === undefined ? [] : [entry];
    });
}

export function openDurableMutationJournal(
  options: DurableMutationJournalOptions,
): DurableMutationJournal {
  const persistEntry = (entry: JournalEntry) =>
    persistValue(options.storage, entryKey(entry.idempotencyKey), entry);

  const reserveEntry = (identity: DurableMutationIdentity): JournalEntry => {
    const entry: JournalEntry = { ...identity, delivery: "reserved", steerItemId: null };
    persistEntry(entry);
    return entry;
  };

  const removeEntry = (entry: JournalEntry) =>
    removeValue(options.storage, entryKey(entry.idempotencyKey));

  const readReservedEntry = (identity: DurableMutationIdentity): JournalEntry | undefined => {
    const persisted = readEntry(options.storage, entryKey(identity.idempotencyKey));
    if (persisted && canonicalJSON(mutationIdentity(persisted)) !== canonicalJSON(identity)) {
      throw new MutationJournalOwnershipError(
        "Runtime mutation identity no longer names the reserved command",
      );
    }
    return persisted;
  };

  const currentScope = () => {
    const scope = options.scope();
    if (!validScope(scope)) {
      throw new MutationJournalScopeUnavailableError(
        "Runtime mutation store identity is temporarily unavailable",
      );
    }
    return scope;
  };

  const validateIdentity = (identity: DurableMutationIdentity, scope: MutationJournalScope) => {
    if (identity.namespace !== scope.namespace) {
      throw new MutationJournalOwnershipError(
        "Runtime mutation identity belongs to a replaced Runtime store",
      );
    }
    const effectiveExpiry = Math.min(
      identity.expiresAt,
      identity.createdAt + scope.retentionSeconds * MILLISECONDS_PER_SECOND,
    );
    if (effectiveExpiry <= options.now()) {
      throw new MutationJournalError("Runtime mutation identity expired before delivery");
    }
  };

  const dispatchEntry = (identity: DurableMutationIdentity): JournalEntry => {
    validateIdentity(identity, currentScope());
    const entry = readReservedEntry(identity);
    if (!entry)
      throw new MutationJournalOwnershipError("Runtime command has no reserved journal entry");
    return entry;
  };

  const createEntry = (
    method: WireMethodName,
    params: unknown,
    scope: MutationJournalScope,
    idempotencyKey: string,
    requestMeta: RequestMeta | null,
    intent: string | null = null,
  ) => {
    const createdAt = options.now();
    const salt = crypto.randomUUID();
    const command = captureJournalCommand(method, params, salt, intent);
    options.admitNewCommand(method, params, requestMeta);
    return reserveEntry({
      salt,
      namespace: scope.namespace,
      fingerprint: fingerprintCommand(salt, method, params, requestMeta),
      idempotencyKey,
      createdAt,
      expiresAt: createdAt + scope.retentionSeconds * MILLISECONDS_PER_SECOND,
      command,
      requestMeta,
    });
  };

  const journal: DurableMutationJournal = {
    matchesPluginCommandIntent(identity, intent) {
      if (!validText(intent, MAX_IDENTITY_LENGTH))
        throw new MutationJournalError("plugin command intent is invalid");
      const entry = readReservedEntry(identity);
      return entry !== undefined && pluginRouteForIntent(entry, intent) !== undefined;
    },
    findPluginCommand(intent, input, claimed) {
      if (!validText(intent, MAX_IDENTITY_LENGTH))
        throw new MutationJournalError("plugin command intent is invalid");
      const matches = loadEntries(options.storage).flatMap((entry) => {
        const route = pluginRouteForIntent(entry, intent);
        return route === undefined ? [] : [{ entry, route }];
      });
      if (matches.length > 1)
        throw new MutationJournalOwnershipError(
          "plugin command intent names multiple unresolved commands",
        );
      const match = matches[0];
      if (!match) return undefined;
      const { entry, route } = match;
      const candidate = checkRequest("plugins.call", {
        route: { ...route, contract: input.contract, method: input.method },
        input: input.input,
      });
      if (
        !matchesCommand(entry, pluginMethod(candidate.route).delivery, candidate, entry.requestMeta)
      )
        throw new MutationJournalOwnershipError(
          "plugin command intent requires its original contract and input",
        );
      if (claimed(entry.idempotencyKey))
        throw new MutationJournalOwnershipError(
          "plugin command intent is already owned by an active command",
        );
      return { identity: mutationIdentity(entry), request: { route, input: candidate.input } };
    },
    stagePluginCommand(intent, request, claimed, requestMeta) {
      request = checkRequest("plugins.call", cloneJSON(request));
      const original = journal.findPluginCommand(
        intent,
        { contract: request.route.contract, method: request.route.method, input: request.input },
        claimed,
      );
      if (original) return original;
      const method = pluginMethod(request.route).delivery;
      const scope = currentScope();
      if (loadEntries(options.storage).length >= MAX_ENTRIES)
        throw new MutationJournalCapacityError("Runtime mutation journal capacity is exhausted");
      const entry = createEntry(method, request, scope, crypto.randomUUID(), requestMeta, intent);
      return { identity: mutationIdentity(entry), request };
    },
    pendingEntries() {
      return loadEntries(options.storage);
    },
    validatePluginCommand(identity, request) {
      validateIdentity(identity, currentScope());
      readReservedEntry(identity);
      if (
        !matchesCommand(
          identity,
          pluginMethod(request.route).delivery,
          request,
          identity.requestMeta,
        )
      ) {
        throw new MutationJournalOwnershipError(
          "original plugin command differs from retained identity",
        );
      }
    },
    reserve(method, params, preferredKey, claimed, requestMeta) {
      if (preferredKey !== undefined && !validText(preferredKey, MAX_IDENTITY_LENGTH)) {
        throw new MutationJournalError("Runtime mutation identity candidate is invalid");
      }
      // These records are unresolved evidence, not a cache of completed receipts.
      // Neither expiry nor replacement of the serving store settles their outcome.
      const entries = loadEntries(options.storage);
      const matches = (entry: JournalEntry) =>
        (entry.command.kind !== "plugin" || entry.command.intentFingerprint === null) &&
        matchesCommand(entry, method, params, requestMeta);
      const preferred =
        preferredKey === undefined
          ? undefined
          : entries.find((entry) => entry.idempotencyKey === preferredKey);
      if (preferred && !matches(preferred)) {
        throw new MutationJournalOwnershipError(
          "Runtime mutation identity candidate belongs to a different command",
        );
      }
      if (preferred && claimed(preferred.idempotencyKey)) {
        throw new MutationJournalOwnershipError(
          "Runtime mutation identity is already owned by an active command",
        );
      }
      let entry = preferred;
      entry ??= entries
        .filter((candidate) => matches(candidate) && !claimed(candidate.idempotencyKey))
        .toSorted((left, right) => left.createdAt - right.createdAt)[0];
      if (!entry) {
        const scope = currentScope();
        if (entries.length >= MAX_ENTRIES) {
          throw new MutationJournalCapacityError("Runtime mutation journal capacity is exhausted");
        }
        entry = createEntry(
          method,
          params,
          scope,
          preferredKey ?? crypto.randomUUID(),
          requestMeta,
        );
      }
      return mutationIdentity(entry);
    },
    authorizeDispatch(identity) {
      return dispatchEntry(identity).namespace;
    },
    markDispatched(identity) {
      const entry = dispatchEntry(identity);
      if (entry.delivery === "dispatched") return "replay";
      persistEntry({ ...entry, delivery: "dispatched" });
      return "initial";
    },
    lookupNamespace(identity) {
      readReservedEntry(identity);
      return identity.namespace;
    },
    steerItemId(identity) {
      return readReservedEntry(identity)?.steerItemId ?? null;
    },
    bindSteerItem(identity, userItemId) {
      if (!isSteerCommand(identity.command) || !validSteerItem(userItemId))
        throw new MutationJournalOwnershipError(
          "Runtime input identity does not belong to a Steer command",
        );
      const persisted = readReservedEntry(identity);
      if (!persisted)
        throw new MutationJournalOwnershipError("Runtime input identity has no reserved command");
      if (persisted.steerItemId !== null && persisted.steerItemId !== userItemId)
        throw new MutationJournalOwnershipError(
          "Runtime changed the original Steer input identity",
        );
      if (persisted.steerItemId === null) persistEntry({ ...persisted, steerItemId: userItemId });
      return userItemId;
    },
    withdrawBeforeDispatch(identity) {
      const withdrawn = readReservedEntry(identity);
      // A missing record cannot establish that the original command was unsent.
      if (withdrawn?.delivery !== "reserved") return undefined;
      removeEntry(withdrawn);
      return () => {
        if (readReservedEntry(identity)) return;
        validateIdentity(identity, currentScope());
        persistEntry(withdrawn);
      };
    },
    settle(identity) {
      const persisted = readReservedEntry(identity);
      if (!persisted) return;
      removeEntry(persisted);
    },
  };
  return journal;
}
