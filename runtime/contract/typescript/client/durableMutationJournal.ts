import type { CallPluginRequest } from "@flame/runtime-contract/wire";
import { checkRequest, requestRequiresIdempotency, pluginDeliveryMethod } from "./request";
import type {
  StartRunRequest,
  ResumeRunRequest,
  SteerRunRequest,
  RequestMeta,
} from "../wire.generated";
import { validateWire } from "../wire.validate.generated";
import { parseJSON, stringifyJSON } from "./json";

const MAX_ENTRIES = 256;
const ENTRY_PREFIX = "entry:";
const FINGERPRINT_FIELD_SEPARATOR = "\u0000";
const JOURNAL_ENTRY_FIELD_COUNT = 9;
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

export type RunCommand =
  | { operation: "start"; params: StartRunRequest; requestMeta: RequestMeta | null }
  | { operation: "resume"; params: ResumeRunRequest; requestMeta: RequestMeta | null }
  | { operation: "steer"; params: SteerRunRequest; requestMeta: RequestMeta | null };

interface JournalEntry {
  method: string;
  steerItemId: string | null;
  salt: string;
  namespace: string;
  fingerprint: string;
  idempotencyKey: string;
  createdAt: number;
  expiresAt: number;
  installed?: {
    intentFingerprint: string | null;
    route: Omit<CallPluginRequest, "input">;
  };
  command: RunCommand | null;
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
  pendingCommands(): DurableMutationIdentity[];
  pendingPluginCommands(): DurableMutationIdentity[];
  validatePluginCommand(identity: DurableMutationIdentity, request: CallPluginRequest): void;
  reserve(
    method: string,
    params: unknown,
    preferredKey: string | undefined,
    claimed: (idempotencyKey: string) => boolean,
    requestMeta: RequestMeta | null,
  ): DurableMutationIdentity | undefined;
  authorizeDispatch(identity: DurableMutationIdentity): string;
  lookupNamespace(identity: DurableMutationIdentity): string;
  bindSteerItem(identity: DurableMutationIdentity, userItemId: string): void;
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

function validSteerItem(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    validateWire("RunAcceptance", { type: "notRecorded", candidateUserItemId: value }).length === 0
  );
}

function validEntry(value: unknown): value is JournalEntry {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const entry = value as Partial<JournalEntry>;
  return (
    Object.keys(value).length ===
      JOURNAL_ENTRY_FIELD_COUNT + (entry.installed === undefined ? 0 : 1) &&
    (entry.steerItemId === null ||
      (validSteerItem(entry.steerItemId) && entry.command?.operation === "steer")) &&
    validText(entry.salt, MAX_IDENTITY_LENGTH) &&
    validText(entry.method, MAX_IDENTITY_LENGTH) &&
    validText(entry.namespace, MAX_IDENTITY_LENGTH) &&
    typeof entry.fingerprint === "string" &&
    FINGERPRINT_PATTERN.test(entry.fingerprint) &&
    validText(entry.idempotencyKey, MAX_IDENTITY_LENGTH) &&
    typeof entry.createdAt === "number" &&
    Number.isFinite(entry.createdAt) &&
    typeof entry.expiresAt === "number" &&
    Number.isFinite(entry.expiresAt) &&
    entry.expiresAt > entry.createdAt &&
    (entry.installed === undefined ||
      (validInstalled(entry.installed) &&
        entry.method === pluginDeliveryMethod(entry.installed.route))) &&
    validCommand(entry.command, entry as JournalEntry) &&
    (entry.command === null || entry.installed === undefined)
  );
}

function validInstalled(value: JournalEntry["installed"]): boolean {
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    Object.keys(value).length !== 2 ||
    (value.intentFingerprint !== null &&
      (typeof value.intentFingerprint !== "string" ||
        !FINGERPRINT_PATTERN.test(value.intentFingerprint)))
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
    const request = checkRequest(method, { ...value.route, input: "null" });
    return requestRequiresIdempotency(method, request);
  } catch {
    return false;
  }
}

function validCommand(value: unknown, entry: JournalEntry): boolean {
  if (value === null) return true;
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    Object.keys(value).length !== 3
  )
    return false;
  const command = value as Partial<RunCommand>;
  if (
    command.operation !== "start" &&
    command.operation !== "resume" &&
    command.operation !== "steer"
  )
    return false;
  if (
    command.requestMeta !== null &&
    (!command.requestMeta ||
      typeof command.requestMeta !== "object" ||
      Array.isArray(command.requestMeta))
  )
    return false;
  try {
    const method = `runs.${command.operation}` as const;
    return matchesCommand(entry, method, command.params, command.requestMeta);
  } catch {
    return false;
  }
}

function prepareCommand(
  method: string,
  params: unknown,
  requestMeta: RequestMeta | null,
): RunCommand | null {
  switch (method) {
    case "runs.start":
      return {
        operation: "start",
        params: structuredClone(checkRequest(method, params, requestMeta)),
        requestMeta: structuredClone(requestMeta),
      };
    case "runs.resume":
      return {
        operation: "resume",
        params: structuredClone(checkRequest(method, params, requestMeta)),
        requestMeta: structuredClone(requestMeta),
      };
    case "runs.steer":
      return {
        operation: "steer",
        params: structuredClone(checkRequest(method, params, requestMeta)),
        requestMeta: structuredClone(requestMeta),
      };
    default:
      return null;
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

function commandFingerprintInput(
  method: string,
  params: unknown,
  requestMeta: RequestMeta | null,
  installed: boolean,
): string {
  // jsoninput.Input ignores object spelling but preserves numeric lexemes.
  // Ordinary JSON.parse would collapse receipt-relevant numbers here.
  const request = installed
    ? {
        ...(params as CallPluginRequest),
        contract: parseJSON((params as CallPluginRequest).contract),
        input: parseJSON((params as CallPluginRequest).input),
      }
    : params;
  return fingerprintFields(
    method,
    stringifyJSON(prepareCommand(method, params, requestMeta) ?? request, true),
  );
}

function matchesCommand(
  entry: JournalEntry,
  method: string,
  params: unknown,
  requestMeta: RequestMeta | null,
): boolean {
  if (entry.method !== method) return false;
  const command = commandFingerprintInput(
    method,
    params,
    requestMeta,
    entry.installed !== undefined,
  );
  return entry.fingerprint === fingerprint(fingerprintFields(entry.salt, command));
}

function sameEntry(left: JournalEntry, right: JournalEntry): boolean {
  return (
    left.steerItemId === right.steerItemId &&
    left.method === right.method &&
    left.salt === right.salt &&
    left.namespace === right.namespace &&
    left.fingerprint === right.fingerprint &&
    left.idempotencyKey === right.idempotencyKey &&
    left.createdAt === right.createdAt &&
    left.expiresAt === right.expiresAt &&
    stringifyJSON(left.installed ?? null, true) === stringifyJSON(right.installed ?? null, true) &&
    stringifyJSON(left.command, true) === stringifyJSON(right.command, true)
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
    requestMeta: RequestMeta | null,
    installed?: { intent: string | null; route: Omit<CallPluginRequest, "input"> },
  ) => {
    const createdAt = options.now();
    const salt = crypto.randomUUID();
    const entry: JournalEntry = {
      method,
      steerItemId: null,
      salt,
      namespace: scope.namespace,
      fingerprint: fingerprint(
        fingerprintFields(
          salt,
          commandFingerprintInput(method, params, requestMeta, installed !== undefined),
        ),
      ),
      idempotencyKey,
      createdAt,
      expiresAt: createdAt + scope.retentionSeconds * MILLISECONDS_PER_SECOND,
      ...(installed === undefined
        ? {}
        : {
            installed: {
              intentFingerprint:
                installed.intent === null
                  ? null
                  : fingerprint(fingerprintFields(salt, "plugins.call", installed.intent)),
              route: installed.route,
            },
          }),
      command: prepareCommand(method, params, requestMeta),
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
      const request = { ...structuredClone(route), ...structuredClone(input) };
      if (!matchesCommand(entry, pluginDeliveryMethod(request), request, null))
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
      const entry = createEntry(method, request, scope, crypto.randomUUID(), null, {
        intent,
        route,
      });
      return { identity: { entry }, request: structuredClone(request) };
    },
    pendingCommands() {
      return loadEntries(options.storage)
        .filter((entry) => entry.command !== null)
        .map((entry) => ({ entry: structuredClone(entry) }));
    },
    pendingPluginCommands() {
      return loadEntries(options.storage)
        .filter(
          (entry) =>
            (entry.method === "plugins.call" || entry.method === "plugins.stream") &&
            entry.installed !== undefined,
        )
        .map((entry) => ({ entry: structuredClone(entry) }));
    },
    validatePluginCommand(identity, request) {
      validateIdentity(identity, currentScope());
      readReservedEntry(identity);
      if (!matchesCommand(identity.entry, identity.entry.method, request, null)) {
        throw new MutationJournalOwnershipError(
          "original plugin command differs from retained identity",
        );
      }
    },
    reserve(method, params, preferredKey, claimed, requestMeta) {
      const scope = options.scope();
      if (preferredKey !== undefined && !validText(preferredKey, MAX_IDENTITY_LENGTH)) {
        throw new MutationJournalError("Runtime mutation identity candidate is invalid");
      }
      // These records are unresolved evidence, not a cache of completed receipts.
      // Neither expiry nor replacement of the serving store settles their outcome.
      const entries = loadEntries(options.storage);
      const matches = (entry: JournalEntry) =>
        (entry.installed === undefined || entry.installed.intentFingerprint === null) &&
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
        if (!validScope(scope)) return undefined;
        if (entries.length >= MAX_ENTRIES) {
          throw new MutationJournalCapacityError("Runtime mutation journal capacity is exhausted");
        }
        const installed =
          method === "plugins.call" || method === "plugins.stream"
            ? (() => {
                const { input: _, ...route } = structuredClone(params as CallPluginRequest);
                return { intent: null, route };
              })()
            : undefined;
        entry = createEntry(
          method,
          params,
          scope,
          preferredKey ?? crypto.randomUUID(),
          requestMeta,
          installed,
        );
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
    bindSteerItem(identity, userItemId) {
      if (identity.entry.command?.operation !== "steer" || !validSteerItem(userItemId))
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
      if (persisted.steerItemId === userItemId) return;
      persistEntry({ ...persisted, steerItemId: userItemId });
      identity.entry.steerItemId = userItemId;
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
