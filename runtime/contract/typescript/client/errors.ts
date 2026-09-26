import type { ProblemData } from "@flame/runtime-contract/wire";
import type { WireViolation } from "@flame/runtime-contract/wire-check";

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
  return error instanceof RpcError && error.data?.type === type;
}

export class RpcError extends Error {
  readonly code?: number;
  readonly data?: ProblemData;
  readonly requestId?: string;

  constructor(payload: BusinessErrorPayload, requestId?: string) {
    super(payload.message);
    this.name = "RpcError";
    this.code = payload.code;
    this.data = payload.data;
    this.requestId = requestId;
  }
}

interface BusinessErrorPayload {
  code?: number;
  message: string;
  data?: ProblemData;
}

export class RpcTransportError extends Error {
  readonly status?: number;
  readonly requestId?: string;
  readonly problemType?: string;

  constructor(message: string, status?: number, requestId?: string, problemType?: string) {
    super(message);
    this.name = "RpcTransportError";
    this.status = status;
    this.requestId = requestId;
    this.problemType = problemType;
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

interface TransportProblem {
  type?: string;
  detail?: string;
  requestId?: string;
}

export function parseTransportProblem(text: string): TransportProblem | undefined {
  try {
    const value: unknown = JSON.parse(text);
    if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
    const fields = value as Record<string, unknown>;
    return {
      type: typeof fields.type === "string" ? fields.type : undefined,
      detail: typeof fields.detail === "string" ? fields.detail : undefined,
      requestId: typeof fields.requestId === "string" ? fields.requestId : undefined,
    };
  } catch {
    return undefined;
  }
}
