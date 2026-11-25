import { Lifetime, type Cleanup, type Release } from "@frame/sdk";
import { exactFields, validCommand, validName } from "./contract.ts";
import { type CallbackRef, type Invocation } from "./callback.ts";

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
    !validName(invocation.origin) ||
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
