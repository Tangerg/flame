import type { ProblemData } from "@flame/runtime-client/wire";
import type { WireViolation } from "./validation/wireCheck";

type ProblemOf<Type extends ProblemData["type"]> = Type extends `plugin:${string}/${string}`
  ? Extract<ProblemData, { type: `plugin:${string}/${string}` }>
  : Extract<ProblemData, { type: Type }>;

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function isErrorType<Type extends ProblemData["type"]>(
  error: unknown,
  type: Type,
): error is RpcError & { readonly data: ProblemOf<Type> } {
  return error instanceof RpcError && error.data.type === type;
}

export class OperationFailure extends Error {
  constructor(
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "OperationFailure";
  }
}

export class RpcError extends Error {
  readonly code?: number;
  readonly data: ProblemData;
  readonly requestId?: string;
  readonly recordedOutcome: boolean;

  constructor(payload: BusinessErrorPayload, requestId?: string) {
    super(payload.message);
    this.name = "RpcError";
    this.code = payload.code;
    this.data = payload.data;
    this.requestId = requestId;
    this.recordedOutcome = payload.recordedOutcome === true;
  }
}

interface BusinessErrorPayload {
  code?: number;
  message: string;
  data: ProblemData;
  recordedOutcome?: boolean;
}

export class RpcTransportError extends Error {
  readonly status?: number;
  readonly requestId?: string;

  constructor(message: string, status?: number, requestId?: string, options?: ErrorOptions) {
    super(message, options);
    this.name = "RpcTransportError";
    this.status = status;
    this.requestId = requestId;
  }
}

export class RpcCancellationError extends RpcTransportError {
  constructor(reason: unknown) {
    super("aborted", undefined, undefined, { cause: reason });
    this.name = "AbortError";
  }
}

export class RpcConnectionError extends RpcTransportError {
  constructor(message: string, requestId?: string) {
    super(message, undefined, requestId);
    this.name = "RpcConnectionError";
  }
}

export class RpcProtocolError extends Error {
  readonly violations: readonly WireViolation[];
  readonly requestId?: string;

  constructor(subject: string, violations: readonly WireViolation[], requestId?: string) {
    const detail = violations
      .map((violation) => `${violation.path} ${violation.detail}`)
      .join("; ");
    super(`invalid ${subject}: ${detail}`);
    this.name = "RpcProtocolError";
    this.violations = violations;
    this.requestId = requestId;
  }
}
