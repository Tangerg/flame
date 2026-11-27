import { canonicalJSON, cloneJSON, parseJSON } from "@frame/sdk/json";
import {
  WIRE_METHOD_POLICY,
  type WireMethodName,
  type WireMutationMethodName,
} from "@flame/runtime-client/methods";
import { z } from "zod";
import type { CallPluginRequest, PluginCallRoute, RequestMeta } from "@flame/runtime-client/wire";
import {
  checkRequest,
  isPluginDeliveryMethod,
  pluginMethod,
  type PluginMethodReference,
} from "./request";
import { validateWire } from "./generated/wire.validate.generated";
import { MutationJournalError } from "./mutationJournalErrors";

const FINGERPRINT_FIELD_SEPARATOR = "\u0000";

export const MAX_IDENTITY_LENGTH = 255;

const FINGERPRINT_HEX_LENGTH = 32;

const FINGERPRINT_PATTERN = new RegExp(`^[0-9a-f]{${FINGERPRINT_HEX_LENGTH}}$`);

type MethodOnlyCommandName = {
  [M in WireMutationMethodName]: (typeof WIRE_METHOD_POLICY)[M]["idempotency"] extends "ownerRun"
    ? never
    : M;
}[WireMutationMethodName];

function isMethodOnlyCommand(value: unknown): value is MethodOnlyCommandName {
  if (typeof value !== "string" || !Object.hasOwn(WIRE_METHOD_POLICY, value)) return false;
  const policy = WIRE_METHOD_POLICY[value as WireMethodName];
  return policy.operation === "command" && policy.idempotency !== "ownerRun";
}

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

const boundedText = z.string().min(1).max(MAX_IDENTITY_LENGTH);

const digest = z.string().regex(FINGERPRINT_PATTERN);

const journalCommand = z.discriminatedUnion("kind", [
  z.strictObject({ kind: z.literal("run"), request: runCommand }),
  z.strictObject({
    kind: z.literal("plugin"),
    intentFingerprint: digest.nullable(),
    route: z.custom<PluginCallRoute>(isInstalledCommandRoute),
  }),
  z.strictObject({
    kind: z.literal("method"),
    method: z.custom<MethodOnlyCommandName>(isMethodOnlyCommand),
  }),
]);

type JournalCommand = z.infer<typeof journalCommand>;

const entrySchema = z.strictObject({
  delivery: z.enum(["reserved", "dispatched"]),
  steerItemId: z.custom<string>(validSteerItem).nullable(),
  salt: boundedText,
  namespace: boundedText,
  fingerprint: digest,
  idempotencyKey: boundedText,
  createdAt: z.number(),
  expiresAt: z.number(),
  command: journalCommand,
  requestMeta: z
    .custom<RequestMeta>(
      (value) => value !== undefined && validateWire("RequestMeta", value, "request").length === 0,
    )
    .nullable(),
});

export type JournalEntry = z.infer<typeof entrySchema>;

export type PluginCommandInput = PluginMethodReference & Pick<CallPluginRequest, "input">;

export type PluginCommandBinding = Omit<PluginCallRoute, keyof PluginMethodReference>;

export interface CapturedPluginCommand {
  identity: DurableMutationIdentity;
  request: CallPluginRequest;
}

export type DurableMutationIdentity = Readonly<Omit<JournalEntry, "delivery" | "steerItemId">>;

export function mutationIdentity(entry: JournalEntry): DurableMutationIdentity {
  const { delivery: _, steerItemId: __, ...identity } = entry;
  return identity;
}

function commandMethod(command: JournalCommand): WireMethodName {
  switch (command.kind) {
    case "run":
      return `runs.${command.request.operation}`;
    case "plugin":
      return pluginMethod(command.route).delivery;
    case "method":
      return command.method;
  }
}

export function isSteerCommand(command: JournalCommand): boolean {
  return command.kind === "run" && command.request.operation === "steer";
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

function isInstalledCommandRoute(value: unknown): value is PluginCallRoute {
  if (validateWire("PluginCallRoute", value, "request").length !== 0) return false;
  try {
    return pluginMethod(value as PluginCallRoute).kind === "command";
  } catch {
    return false;
  }
}

export function decodeJournalEntry(value: unknown): JournalEntry | undefined {
  const admission = entrySchema.safeParse(value);
  if (!admission.success) return undefined;
  const entry = admission.data;
  if (
    entry.expiresAt <= entry.createdAt ||
    (entry.steerItemId !== null && !isSteerCommand(entry.command))
  )
    return undefined;
  if (
    entry.command.kind === "run" &&
    !matchesCommand(
      entry,
      commandMethod(entry.command),
      entry.command.request.params,
      entry.requestMeta,
    )
  )
    return undefined;
  return entry;
}

export function captureJournalCommand(
  method: WireMethodName,
  params: unknown,
  salt: string,
  intent: string | null,
): JournalCommand {
  switch (method) {
    case "runs.start":
      return {
        kind: "run",
        request: { operation: "start", params: checkRequest(method, cloneJSON(params)) },
      };
    case "runs.resume":
      return {
        kind: "run",
        request: { operation: "resume", params: checkRequest(method, cloneJSON(params)) },
      };
    case "runs.steer":
      return {
        kind: "run",
        request: { operation: "steer", params: checkRequest(method, cloneJSON(params)) },
      };
    case "plugins.call":
    case "plugins.stream": {
      const { route } = checkRequest(method, cloneJSON(params));
      const declaration = pluginMethod(route);
      if (declaration.kind !== "command" || declaration.delivery !== method)
        throw new MutationJournalError("plugin command requires its declared delivery method");
      return {
        kind: "plugin",
        intentFingerprint: intent === null ? null : pluginIntentFingerprint(salt, intent),
        route,
      };
    }
    default:
      if (!isMethodOnlyCommand(method))
        throw new MutationJournalError("Runtime method cannot reserve a command");
      return { kind: "method", method };
  }
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

function pluginIntentFingerprint(salt: string, intent: string): string {
  return fingerprint([salt, "plugins.call", intent].join(FINGERPRINT_FIELD_SEPARATOR));
}

export function pluginRouteForIntent(
  entry: DurableMutationIdentity,
  intent: string,
): PluginCallRoute | undefined {
  const { command } = entry;
  if (
    command.kind === "plugin" &&
    command.intentFingerprint === pluginIntentFingerprint(entry.salt, intent)
  )
    return command.route;
  return undefined;
}

export function fingerprintCommand(
  salt: string,
  method: WireMethodName,
  params: unknown,
  requestMeta: RequestMeta | null,
): string {
  let request = params;
  if (isPluginDeliveryMethod(method)) {
    // The original route and exact feature JSON identify the command across encodings.
    const { route, input } = params as CallPluginRequest;
    request = { ...route, contract: parseJSON(route.contract), input: parseJSON(input) };
  }
  return fingerprint(
    [salt, method, canonicalJSON({ request, requestMeta })].join(FINGERPRINT_FIELD_SEPARATOR),
  );
}

export function matchesCommand(
  identity: DurableMutationIdentity,
  method: WireMethodName,
  params: unknown,
  requestMeta: RequestMeta | null,
): boolean {
  return (
    commandMethod(identity.command) === method &&
    identity.fingerprint === fingerprintCommand(identity.salt, method, params, requestMeta)
  );
}
