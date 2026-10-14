import type { CallPluginRequest } from "@flame/runtime-contract/wire";
import { checkRequest, requestRequiresIdempotency, pluginDeliveryMethod } from "./request";

const MAX_ENTRIES = 256;
const ENTRY_PREFIX = "entry:";
const FINGERPRINT_FIELD_SEPARATOR = "\u0000";
const JOURNAL_ENTRY_FIELD_COUNT = 6;
const MAX_IDENTITY_LENGTH = 255;
const FINGERPRINT_HEX_LENGTH = 32;
const MILLISECONDS_PER_SECOND = 1_000;
const FINGERPRINT_PATTERN = new RegExp(`^[0-9a-f]{${FINGERPRINT_HEX_LENGTH}}$`);

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

export class MutationJournalError extends Error {
  override readonly name: string = "MutationJournalError";
}

export class MutationJournalStorageError extends MutationJournalError {
  override readonly name = "MutationJournalStorageError";
}

export class MutationJournalOwnershipError extends MutationJournalError {
  override readonly name = "MutationJournalOwnershipError";
}

export class MutationJournalScopeUnavailableError extends MutationJournalError {
  override readonly name = "MutationJournalScopeUnavailableError";
}

export class MutationJournalCapacityError extends MutationJournalError {
  override readonly name = "MutationJournalCapacityError";
}

interface JournalEntry {
  salt: string;
  namespace: string;
  fingerprint: string;
  idempotencyKey: string;
  createdAt: number;
  expiresAt: number;
  installed?: {
    intentFingerprint: string;
    route: Omit<CallPluginRequest, "input">;
  };
}

export type PluginCommandInput = Pick<CallPluginRequest, "contract" | "method" | "input">;
export type PluginCommandBinding = Omit<CallPluginRequest, keyof PluginCommandInput>;

export interface CapturedPluginCommand {
  identity: DurableMutationIdentity;
  request: CallPluginRequest;
}

export interface DurableMutationIdentity {
  readonly entry: JournalEntry;
}

export interface DurableMutationJournal {
  findPluginCommand(
    intent: string,
    input: PluginCommandInput,
    claimed: (idempotencyKey: string) => boolean,
  ): CapturedPluginCommand | undefined;
  stagePluginCommand(
    intent: string,
    request: CallPluginRequest,
    claimed: (idempotencyKey: string) => boolean,
  ): CapturedPluginCommand;
  reserve(
    method: string,
    params: unknown,
    preferredKey: string | undefined,
    claimed: (idempotencyKey: string) => boolean,
  ): DurableMutationIdentity | undefined;
  authorizeDispatch(identity: DurableMutationIdentity): string;
  lookupNamespace(identity: DurableMutationIdentity): string;
  retain(identity: DurableMutationIdentity): void;
  settle(identity: DurableMutationIdentity): void;
}

interface DurableMutationJournalOptions {
  storage: MutationJournalStorage;
  scope: () => MutationJournalScope | null | undefined;
  now: () => number;
}

function storageError(message: string, cause?: unknown): MutationJournalStorageError {
  if (cause instanceof MutationJournalStorageError) return cause;
  return new MutationJournalStorageError(message, cause === undefined ? undefined : { cause });
}

function validText(value: unknown, maximum: number): value is string {
  return typeof value === "string" && value.length > 0 && value.length <= maximum;
}

function validEntry(value: unknown): value is JournalEntry {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const entry = value as Partial<JournalEntry>;
  return (
    Object.keys(value).length ===
      JOURNAL_ENTRY_FIELD_COUNT + (entry.installed === undefined ? 0 : 1) &&
    validText(entry.salt, MAX_IDENTITY_LENGTH) &&
    validText(entry.namespace, MAX_IDENTITY_LENGTH) &&
    typeof entry.fingerprint === "string" &&
    FINGERPRINT_PATTERN.test(entry.fingerprint) &&
    validText(entry.idempotencyKey, MAX_IDENTITY_LENGTH) &&
    typeof entry.createdAt === "number" &&
    Number.isFinite(entry.createdAt) &&
    typeof entry.expiresAt === "number" &&
    Number.isFinite(entry.expiresAt) &&
    entry.expiresAt > entry.createdAt &&
    (entry.installed === undefined || validInstalled(entry.installed))
  );
}

function validInstalled(value: JournalEntry["installed"]): boolean {
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    Object.keys(value).length !== 2 ||
    typeof value.intentFingerprint !== "string" ||
    !FINGERPRINT_PATTERN.test(value.intentFingerprint)
  )
    return false;
  if (
    !value.route ||
    typeof value.route !== "object" ||
    Array.isArray(value.route) ||
    Object.keys(value.route).length !== 6 ||
    "input" in value.route
  )
    return false;
  try {
    const method = pluginDeliveryMethod(value.route);
    const request = checkRequest(method, { ...value.route, input: null });
    return requestRequiresIdempotency(method, request);
  } catch {
    return false;
  }
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

function readValue(storage: MutationJournalStorage, key: string): unknown {
  try {
    return storage.get(key);
  } catch (error) {
    throw storageError(`Runtime mutation journal record is unreadable: ${key}`, error);
  }
}

function persistValue(storage: MutationJournalStorage, key: string, value: unknown): void {
  try {
    storage.set(key, value);
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

function loadEntries(storage: MutationJournalStorage): JournalEntry[] {
  return readKeys(storage)
    .filter((key) => key.startsWith(ENTRY_PREFIX))
    .flatMap((key) => {
      const value = readValue(storage, key);
      if (value === undefined) return [];
      if (!validEntry(value) || entryKey(value.idempotencyKey) !== key) {
        throw storageError(`Runtime mutation journal entry is corrupted: ${key}`);
      }
      return [value];
    });
}

function canonicalJSON(value: unknown): string {
  if (value === null) return "null";
  if (typeof value === "string" || typeof value === "boolean") return JSON.stringify(value);
  if (typeof value === "number") return Number.isFinite(value) ? JSON.stringify(value) : "null";
  if (Array.isArray(value)) {
    return `[${value
      .map((item) =>
        item === undefined || typeof item === "function" || typeof item === "symbol"
          ? "null"
          : canonicalJSON(item),
      )
      .join(",")}]`;
  }
  if (typeof value === "object") {
    const record = value as Record<string, unknown>;
    const fields = Object.keys(record)
      .filter((key) => {
        const field = record[key];
        return field !== undefined && typeof field !== "function" && typeof field !== "symbol";
      })
      .sort()
      .map((key) => `${JSON.stringify(key)}:${canonicalJSON(record[key])}`);
    return `{${fields.join(",")}}`;
  }
  return "null";
}

function fingerprint(value: string): string {
  let h1 = 1_779_033_703;
  let h2 = 3_144_134_277;
  let h3 = 1_013_904_242;
  let h4 = 2_773_480_762;
  for (let index = 0; index < value.length; index++) {
    const code = value.charCodeAt(index);
    h1 = h2 ^ Math.imul(h1 ^ code, 597_399_067);
    h2 = h3 ^ Math.imul(h2 ^ code, 2_869_860_233);
    h3 = h4 ^ Math.imul(h3 ^ code, 951_274_213);
    h4 = h1 ^ Math.imul(h4 ^ code, 2_716_044_179);
  }
  h1 = Math.imul(h3 ^ (h1 >>> 18), 597_399_067);
  h2 = Math.imul(h4 ^ (h2 >>> 22), 2_869_860_233);
  h3 = Math.imul(h1 ^ (h3 >>> 17), 951_274_213);
  h4 = Math.imul(h2 ^ (h4 >>> 19), 2_716_044_179);
  const hex = (part: number) => (part >>> 0).toString(16).padStart(8, "0");
  return hex(h1) + hex(h2) + hex(h3) + hex(h4);
}

function fingerprintFields(...fields: readonly string[]): string {
  return fields.join(FINGERPRINT_FIELD_SEPARATOR);
}

function matchesCommand(entry: JournalEntry, method: string, params: unknown): boolean {
  const command = commandFields(method, params, entry.installed !== undefined);
  return entry.fingerprint === fingerprint(fingerprintFields(entry.salt, command));
}

function commandFields(method: string, params: unknown, installed: boolean): string {
  const command = fingerprintFields(method, canonicalJSON(params));
  if (!installed) return command;
  // Installed receipts bind input bytes; semantic JSON equality cannot authorize reordered input.
  const input = JSON.stringify((params as CallPluginRequest).input);
  if (input === undefined)
    throw new MutationJournalError("plugin command input is not serializable");
  return fingerprintFields(command, input);
}

function sameEntry(left: JournalEntry, right: JournalEntry): boolean {
  return (
    left.salt === right.salt &&
    left.namespace === right.namespace &&
    left.fingerprint === right.fingerprint &&
    left.idempotencyKey === right.idempotencyKey &&
    left.createdAt === right.createdAt &&
    left.expiresAt === right.expiresAt &&
    canonicalJSON(left.installed) === canonicalJSON(right.installed)
  );
}

export function openDurableMutationJournal(
  options: DurableMutationJournalOptions,
): DurableMutationJournal {
  const readEntry = (idempotencyKey: string): JournalEntry | undefined => {
    const key = entryKey(idempotencyKey);
    const value = readValue(options.storage, key);
    if (value === undefined) return undefined;
    if (!validEntry(value) || entryKey(value.idempotencyKey) !== key) {
      throw storageError(`Runtime mutation journal entry is corrupted: ${key}`);
    }
    return value;
  };

  const persistEntry = (entry: JournalEntry) =>
    persistValue(options.storage, entryKey(entry.idempotencyKey), entry);

  const removeEntry = (entry: JournalEntry) =>
    removeValue(options.storage, entryKey(entry.idempotencyKey));

  const readReservedEntry = (identity: DurableMutationIdentity): JournalEntry | undefined => {
    const persisted = readEntry(identity.entry.idempotencyKey);
    if (persisted && !sameEntry(persisted, identity.entry)) {
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
    const entry = identity.entry;
    if (entry.namespace !== scope.namespace) {
      throw new MutationJournalOwnershipError(
        "Runtime mutation identity belongs to a replaced Runtime store",
      );
    }
    const effectiveExpiry = Math.min(
      entry.expiresAt,
      entry.createdAt + scope.retentionSeconds * MILLISECONDS_PER_SECOND,
    );
    if (effectiveExpiry <= options.now()) {
      throw new MutationJournalError("Runtime mutation identity expired before delivery");
    }
  };

  const createEntry = (
    method: string,
    params: unknown,
    scope: MutationJournalScope,
    idempotencyKey: string,
    installed?: { intent: string; route: Omit<CallPluginRequest, "input"> },
  ) => {
    const createdAt = options.now();
    const salt = crypto.randomUUID();
    const entry: JournalEntry = {
      salt,
      namespace: scope.namespace,
      fingerprint: fingerprint(
        fingerprintFields(salt, commandFields(method, params, installed !== undefined)),
      ),
      idempotencyKey,
      createdAt,
      expiresAt: createdAt + scope.retentionSeconds * MILLISECONDS_PER_SECOND,
      ...(installed === undefined
        ? {}
        : {
            installed: {
              intentFingerprint: fingerprint(
                fingerprintFields(salt, "plugins.call", installed.intent),
              ),
              route: installed.route,
            },
          }),
    };
    persistEntry(entry);
    return entry;
  };

  const journal: DurableMutationJournal = {
    findPluginCommand(intent, input, claimed) {
      if (!validText(intent, MAX_IDENTITY_LENGTH))
        throw new MutationJournalError("plugin command intent is invalid");
      const matches = loadEntries(options.storage).filter(
        (entry) =>
          entry.installed?.intentFingerprint ===
          fingerprint(fingerprintFields(entry.salt, "plugins.call", intent)),
      );
      if (matches.length > 1)
        throw new MutationJournalOwnershipError(
          "plugin command intent names multiple unresolved commands",
        );
      const entry = matches[0];
      if (!entry) return undefined;
      const route = entry.installed!.route;
      const request = { ...structuredClone(route), input: structuredClone(input.input) };
      if (
        input.method !== route.method ||
        input.contract !== route.contract ||
        !matchesCommand(entry, pluginDeliveryMethod(request), request)
      )
        throw new MutationJournalOwnershipError(
          "plugin command intent requires its original contract and input",
        );
      if (claimed(entry.idempotencyKey))
        throw new MutationJournalOwnershipError(
          "plugin command intent is already owned by an active command",
        );
      return { identity: { entry }, request };
    },
    stagePluginCommand(intent, request, claimed) {
      const original = journal.findPluginCommand(intent, request, claimed);
      if (original) return original;
      const method = pluginDeliveryMethod(request);
      checkRequest(method, request);
      if (!requestRequiresIdempotency(method, request))
        throw new MutationJournalError("plugin command requires a command operation");
      const scope = currentScope();
      if (loadEntries(options.storage).length >= MAX_ENTRIES)
        throw new MutationJournalCapacityError("Runtime mutation journal capacity is exhausted");
      const { input: _, ...route } = structuredClone(request);
      const entry = createEntry(method, request, scope, crypto.randomUUID(), {
        intent,
        route,
      });
      return { identity: { entry }, request: structuredClone(request) };
    },
    reserve(method, params, preferredKey, claimed) {
      const scope = options.scope();
      if (preferredKey !== undefined && !validText(preferredKey, MAX_IDENTITY_LENGTH)) {
        throw new MutationJournalError("Runtime mutation identity candidate is invalid");
      }
      // These records are unresolved evidence, not a cache of completed receipts.
      // Neither expiry nor replacement of the serving store settles their outcome.
      const entries = loadEntries(options.storage);
      const matches = (entry: JournalEntry) =>
        entry.installed === undefined && matchesCommand(entry, method, params);
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
        if (!validScope(scope)) return undefined;
        if (entries.length >= MAX_ENTRIES) {
          throw new MutationJournalCapacityError("Runtime mutation journal capacity is exhausted");
        }
        entry = createEntry(method, params, scope, preferredKey ?? crypto.randomUUID());
      }
      const identity = { entry };
      return identity;
    },
    authorizeDispatch(identity) {
      const scope = currentScope();
      validateIdentity(identity, scope);
      readReservedEntry(identity);
      return identity.entry.namespace;
    },
    lookupNamespace(identity) {
      readReservedEntry(identity);
      return identity.entry.namespace;
    },
    retain(identity) {
      if (readReservedEntry(identity)) return;
      persistEntry(identity.entry);
    },
    settle(identity) {
      const persisted = readReservedEntry(identity);
      if (!persisted) return;
      removeEntry(persisted);
    },
  };
  return journal;
}
