import { canonicalJSON, cloneJSON, parseJSON } from "@frame/sdk/json";
import { z } from "zod";
import type { CallPluginRequest } from "@flame/runtime-client/wire";
import { checkRequest, requestRequiresIdempotency, pluginDeliveryMethod } from "./request";
import type { RequestMeta } from "./generated/wire.generated";
import { validateWire } from "./generated/wire.validate.generated";
import { MutationJournalError } from "./mutationJournalErrors";

const FINGERPRINT_FIELD_SEPARATOR = "\u0000";

export const MAX_IDENTITY_LENGTH = 255;

const FINGERPRINT_HEX_LENGTH = 32;

const FINGERPRINT_PATTERN = new RegExp(`^[0-9a-f]{${FINGERPRINT_HEX_LENGTH}}$`);

type InstalledRoute = Omit<CallPluginRequest, "input">;

function commandParams<M extends "runs.start" | "runs.resume" | "runs.steer">(method: M) {
  return z.custom<ReturnType<typeof checkRequest<M>>>((value) => {
    try {
      checkRequest(method, value);
      return true;
    } catch {
      return false;
    }
  });
}

const runCommand = z.discriminatedUnion("operation", [
  z.strictObject({ operation: z.literal("start"), params: commandParams("runs.start") }),
  z.strictObject({ operation: z.literal("resume"), params: commandParams("runs.resume") }),
  z.strictObject({ operation: z.literal("steer"), params: commandParams("runs.steer") }),
]);

export type RunCommand = z.infer<typeof runCommand>;

export function captureRunCommand(command: RunCommand): RunCommand {
  return runCommand.parse(cloneJSON(command));
}

const boundedText = z.string().min(1).max(MAX_IDENTITY_LENGTH);

const digest = z.string().regex(FINGERPRINT_PATTERN);

const entrySchema = z.strictObject({
  delivery: z.enum(["reserved", "dispatched"]),
  method: boundedText,
  steerItemId: z.custom<string>(validSteerItem).nullable(),
  salt: boundedText,
  namespace: boundedText,
  fingerprint: digest,
  idempotencyKey: boundedText,
  createdAt: z.number(),
  expiresAt: z.number(),
  installed: z
    .strictObject({
      intentFingerprint: digest.nullable(),
      route: z.custom<InstalledRoute>(validInstalledRoute),
    })
    .optional(),
  command: runCommand.nullable(),
  requestMeta: z
    .custom<RequestMeta>(
      (value) => value !== undefined && validateWire("RequestMeta", value, "request").length === 0,
    )
    .nullable(),
});

export type JournalEntry = z.infer<typeof entrySchema>;

export type PluginCommandInput = Pick<CallPluginRequest, "contract" | "method" | "input">;

export type PluginCommandBinding = Omit<CallPluginRequest, keyof PluginCommandInput>;

export interface CapturedPluginCommand {
  identity: DurableMutationIdentity;
  request: CallPluginRequest;
}

export interface DurableMutationIdentity {
  readonly entry: JournalEntry;
}

export function validText(value: unknown, maximum: number): value is string {
  return typeof value === "string" && value.length > 0 && value.length <= maximum;
}

export function validSteerItem(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    validateWire("RunAcceptance", { type: "notRecorded", candidateUserItemId: value }).length === 0
  );
}

function validInstalledRoute(value: unknown): value is InstalledRoute {
  if (!value || typeof value !== "object" || Array.isArray(value) || "input" in value) return false;
  try {
    const route = checkRequest("plugins.call", { ...value, input: "null" });
    return requestRequiresIdempotency(pluginDeliveryMethod(route), route);
  } catch {
    return false;
  }
}

export function captureInstalledRoute(route: InstalledRoute): InstalledRoute {
  const captured = cloneJSON(route);
  if (!validInstalledRoute(captured))
    throw new MutationJournalError("plugin command binding is invalid");
  return captured;
}

export function validEntry(value: unknown): value is JournalEntry {
  const admission = entrySchema.safeParse(value);
  if (!admission.success) return false;
  const entry = admission.data;
  if (
    entry.expiresAt <= entry.createdAt ||
    (entry.steerItemId !== null && entry.command?.operation !== "steer")
  )
    return false;
  if (entry.installed)
    return entry.command === null && entry.method === pluginDeliveryMethod(entry.installed.route);
  if (entry.command === null) return true;
  try {
    return matchesCommand(
      entry,
      `runs.${entry.command.operation}`,
      entry.command.params,
      entry.requestMeta,
    );
  } catch {
    return false;
  }
}

export function prepareCommand(
  method: string,
  params: unknown,
  requestMeta: RequestMeta | null,
): RunCommand | null {
  switch (method) {
    case "runs.start":
      return {
        operation: "start",
        params: checkRequest(method, cloneJSON(params), requestMeta),
      };
    case "runs.resume":
      return {
        operation: "resume",
        params: checkRequest(method, cloneJSON(params), requestMeta),
      };
    case "runs.steer":
      return {
        operation: "steer",
        params: checkRequest(method, cloneJSON(params), requestMeta),
      };
    default:
      return null;
  }
}

export function fingerprint(value: string): string {
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

export function fingerprintFields(...fields: readonly string[]): string {
  return fields.join(FINGERPRINT_FIELD_SEPARATOR);
}

export function matchesPluginIntent(entry: JournalEntry, intent: string): boolean {
  return (
    entry.installed?.intentFingerprint ===
    fingerprint(fingerprintFields(entry.salt, "plugins.call", intent))
  );
}

export function commandFingerprintInput(
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
    canonicalJSON({ request: prepareCommand(method, params, requestMeta) ?? request, requestMeta }),
  );
}

export function matchesCommand(
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

export function sameEntry(left: JournalEntry, right: JournalEntry): boolean {
  return (
    left.steerItemId === right.steerItemId &&
    left.method === right.method &&
    left.salt === right.salt &&
    left.namespace === right.namespace &&
    left.fingerprint === right.fingerprint &&
    left.idempotencyKey === right.idempotencyKey &&
    left.createdAt === right.createdAt &&
    left.expiresAt === right.expiresAt &&
    canonicalJSON(left.installed ?? null) === canonicalJSON(right.installed ?? null) &&
    canonicalJSON(left.command) === canonicalJSON(right.command) &&
    canonicalJSON(left.requestMeta) === canonicalJSON(right.requestMeta)
  );
}
