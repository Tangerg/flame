import { Lifetime, type Cleanup, type Release } from "@frame/sdk";
import { isJSONNumber } from "@frame/sdk/json";
import {
  canonicalJSON,
  digestJSON,
  exactFields,
  freezeJSON,
  validCommand,
  validName,
} from "./contract.ts";
import {
  callbackRef,
  InvocationOrigin,
  type CallbackRef,
  type Invocation,
} from "./callback.ts";
import wire from "./protocol.generated.json";

// The calling owner's Run scope on a model Tool call; Project is empty when the
// Run's Session belongs to none.
export interface ToolContext {
  readonly cwd: string;
  readonly project?: string;
  readonly embeddingSpace?: string;
}

export interface CallContext {
  readonly signal: AbortSignal;
  readonly commandId?: string;
  readonly invocation?: Invocation;
  readonly toolContext?: ToolContext;
  readonly requestInput: string;
  readonly inputDigest: string;
  readonly callback?: CallbackRef;
  readonly lifetime: Lifetime;
  defer(cleanup: () => void | Release | Promise<void | Release>): Cleanup;
}

export interface ReceiptRequest {
  id: string;
  method: string;
  inputDigest: string;
}

export function admittedInvocation(
  value: unknown,
  target: string | undefined,
): Invocation | undefined {
  if (value === undefined) return undefined;
  const invocation = value as Invocation;
  exactFields(invocation, ["id", "origin", "target"], ["parent"]);
  exactFields(invocation.target, ["scope", "reference"]);
  if (
    !validName(invocation.id) ||
    !Object.values<string>(InvocationOrigin).includes(invocation.origin) ||
    (invocation.parent !== undefined &&
      (!validName(invocation.parent) || invocation.parent === invocation.id)) ||
    !validName(invocation.target.scope) ||
    typeof invocation.target.reference !== "string" ||
    !invocation.target.reference ||
    /\p{Cc}/u.test(invocation.target.reference) ||
    (target !== undefined && invocation.target.scope !== target)
  )
    throw new Error("plugin: invalid calling-owner invocation");
  return invocation;
}

export function admittedToolContext(value: unknown): ToolContext | undefined {
  if (value === undefined) return undefined;
  const toolContext = value as ToolContext;
  exactFields(toolContext, ["cwd"], ["project", "embeddingSpace"]);
  if (
    typeof toolContext.cwd !== "string" ||
    !toolContext.cwd.trim() ||
    (toolContext.project !== undefined &&
      typeof toolContext.project !== "string") ||
    (toolContext.embeddingSpace !== undefined &&
      typeof toolContext.embeddingSpace !== "string")
  )
    throw new Error("plugin: invalid captured Tool context");
  return toolContext;
}

export function decodeReceipt(value: unknown): ReceiptRequest {
  const request = value as ReceiptRequest;
  if (
    !validCommand(request.id) ||
    !validName(request.method) ||
    !/^[a-f0-9]{64}$/.test(request.inputDigest)
  )
    throw new Error("plugin: invalid receipt identity");
  return request;
}

export function callMetadata(
  params: Record<string, unknown>,
): Record<string, unknown> {
  const meta: unknown = freezeJSON(params._meta ?? {});
  if (
    meta === null ||
    typeof meta !== "object" ||
    Array.isArray(meta) ||
    isJSONNumber(meta)
  )
    throw new Error("plugin: metadata must be an object");
  return meta as Record<string, unknown>;
}

export function callContext(
  params: Record<string, unknown>,
  owner: Lifetime,
  target?: string,
): CallContext {
  const meta = callMetadata(params);
  const invocation = admittedInvocation(
    meta[wire.constants.invocationMetadataKey!],
    target,
  );
  const ref =
    meta[wire.constants.callbackMetadataKey!] === undefined
      ? undefined
      : callbackRef(meta[wire.constants.callbackMetadataKey!]);
  const toolContext = admittedToolContext(
    meta[wire.constants.toolContextMetadataKey!],
  );
  const lifetime = owner.signal;
  return Object.freeze({
    signal: lifetime,
    commandId: params.commandId as string | undefined,
    invocation,
    toolContext,
    callback: ref,
    requestInput: canonicalJSON(params.input),
    inputDigest: digestJSON(params.input),
    lifetime: owner,
    defer: (cleanup: () => void | Release | Promise<void | Release>) =>
      owner.cleanup(cleanup),
  });
}
