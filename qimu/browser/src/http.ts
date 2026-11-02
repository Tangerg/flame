import { QimuError, type ErrorCode, type Lifetime } from "@qimu/sdk";
import { parseJSON, stringifyJSON } from "@qimu/sdk/json";
import {
  CONTROL_OPERATIONS,
  CONTROL_SHAPES,
  type ControlResults,
} from "@qimu/sdk/protocol";
import type {
  AcquireRequest,
  CloseProof,
  LocalPlan,
  Poll,
  ProofAcknowledgement,
  PublicationRequest,
  PublicationResult,
  ReadyReport,
  ReconnectRequest,
  ReconnectResult,
  RouteGrant,
  Snapshot,
  UseGrant,
  Withdrawal,
} from "@qimu/sdk/protocol";
import type { HostControl } from "./control";

export interface HTTPControlOptions {
  readonly endpoint: string;
  readonly credential: string;
  readonly maximumMessageBytes: number;
  readonly lifetime: Lifetime;
  readonly fetch?: typeof fetch;
}

const codes: ReadonlySet<ErrorCode> = new Set<ErrorCode>([
  "closed",
  "capacity",
  "contract_mismatch",
  "invalid_plan",
  "unavailable",
  "stale_binding",
  "permission_denied",
  "outcome_unknown",
  "close_unconfirmed",
  "resync_required",
  "reload_required",
  "reentrant_operation",
]);

export class HTTPControl implements HostControl {
  readonly #endpoint: string;
  #credential: string;
  readonly #maximumMessageBytes: number;
  readonly #lifetime: Lifetime;
  readonly #fetch: typeof fetch;

  constructor(options: HTTPControlOptions) {
    const endpoint = new URL(options.endpoint);
    if (
      !["https:", "http:"].includes(endpoint.protocol) ||
      endpoint.username ||
      endpoint.password ||
      endpoint.hash ||
      !options.credential
    )
      throw new TypeError("control endpoint and realm credential are required");
    if (
      !Number.isSafeInteger(options.maximumMessageBytes) ||
      options.maximumMessageBytes < 1
    )
      throw new TypeError(
        "control message capacity must be a positive safe integer",
      );
    this.#endpoint = endpoint.href;
    this.#credential = options.credential;
    this.#maximumMessageBytes = options.maximumMessageBytes;
    this.#lifetime = options.lifetime;
    options.lifetime.cleanup(() => {
      this.#credential = "";
    }, "control-credential");
    this.#fetch = (options.fetch ?? globalThis.fetch).bind(globalThis);
  }

  async #request<M extends keyof ControlResults>(
    method: M,
    input: unknown,
  ): Promise<ControlResults[M]> {
    const body = stringifyJSON({ method, input });
    if (new TextEncoder().encode(body).byteLength > this.#maximumMessageBytes)
      return Promise.reject(
        new QimuError(
          "capacity",
          "control request capacity exceeded before transmission",
        ),
      );
    return this.#lifetime.run((signal) => this.#send(method, body, signal));
  }

  async #send<M extends keyof ControlResults>(
    method: M,
    body: string,
    signal: AbortSignal,
  ): Promise<ControlResults[M]> {
    const { mutating, output } = CONTROL_OPERATIONS[method];
    let response: Response;
    try {
      response = await this.#fetch(this.#endpoint, {
        method: "POST",
        redirect: "error",
        credentials: "omit",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${this.#credential}`,
        },
        body,
        signal,
      });
    } catch (cause) {
      throw new QimuError(
        mutating ? "outcome_unknown" : "unavailable",
        "control transport did not return a response",
        { cause },
      );
    }
    let value: unknown;
    try {
      value = parseJSON(
        await readResponse(response, this.#maximumMessageBytes),
      );
    } catch (cause) {
      throw new QimuError(
        mutating ? "outcome_unknown" : "unavailable",
        "control response is incomplete or malformed",
        { cause },
      );
    }
    if (!response.ok) {
      const problem = value as { code?: ErrorCode; message?: string } | null;
      if (
        problem &&
        problem.code &&
        codes.has(problem.code) &&
        typeof problem.message === "string"
      )
        throw new QimuError(problem.code, problem.message);
      throw new QimuError(
        mutating ? "outcome_unknown" : "unavailable",
        "control operation was not confirmed",
      );
    }
    if (!matchesShape(output, value))
      throw new QimuError(
        mutating ? "outcome_unknown" : "unavailable",
        "control response does not match its contract",
      );
    return (output === "null" ? undefined : value) as ControlResults[M];
  }

  validatePlan(plan: LocalPlan): Promise<void> {
    return this.#request("validatePlan", plan);
  }
  ready(report: ReadyReport): Promise<PublicationResult> {
    return this.#request("ready", report);
  }
  acquire(request: AcquireRequest): Promise<UseGrant> {
    return this.#request("acquire", request);
  }
  validateUse(grant: UseGrant): Promise<void> {
    return this.#request("validateUse", grant);
  }
  validateRoute(grant: RouteGrant): Promise<void> {
    return this.#request("validateRoute", grant);
  }
  snapshot(points: readonly string[]): Promise<Snapshot> {
    return this.#request("snapshot", points);
  }
  closeSnapshot(snapshotId: string): Promise<void> {
    return this.#request("closeSnapshot", snapshotId);
  }
  publish(request: PublicationRequest): Promise<PublicationResult> {
    return this.#request("publish", request);
  }
  publication(operationId: string): Promise<PublicationResult> {
    return this.#request("publication", operationId);
  }
  withdraw(request: Withdrawal): Promise<PublicationResult> {
    return this.#request("withdraw", request);
  }
  prove(proof: CloseProof): Promise<ProofAcknowledgement> {
    return this.#request("prove", proof);
  }
  reconnect(request: ReconnectRequest): Promise<ReconnectResult> {
    return this.#request("reconnect", request);
  }
  poll(): Promise<Poll> {
    return this.#request("poll", {});
  }
}

type WireShape = string | readonly string[] | Readonly<Record<string, string>>;

function matchesShape(name: string, value: unknown): boolean {
  if (name === "null") return value === null;
  if (name === "string" || name === "bool")
    return typeof value === (name === "bool" ? "boolean" : "string");
  if (name.startsWith("[]"))
    return (
      Array.isArray(value) &&
      value.every((item) => matchesShape(name.slice(2), item))
    );
  const shape: WireShape | undefined = Object.hasOwn(CONTROL_SHAPES, name)
    ? CONTROL_SHAPES[name as keyof typeof CONTROL_SHAPES]
    : undefined;
  if (typeof shape === "string") return matchesShape(shape, value);
  if (Array.isArray(shape)) return shape.includes(value);
  if (
    !shape ||
    value === null ||
    typeof value !== "object" ||
    Array.isArray(value)
  )
    return false;
  const record = value as Record<string, unknown>;
  if (!Object.keys(record).every((key) => Object.hasOwn(shape, key)))
    return false;
  return Object.entries(shape).every(([key, type]) => {
    const optional = type.endsWith("?");
    if (!Object.hasOwn(record, key)) return optional;
    return matchesShape(optional ? type.slice(0, -1) : type, record[key]);
  });
}

async function readResponse(
  response: Response,
  maximum: number,
): Promise<string> {
  if (!response.body) throw new Error("control response body is missing");
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let length = 0;
  try {
    for (;;) {
      const next = await reader.read();
      if (next.done) break;
      length += next.value.byteLength;
      if (length > maximum)
        throw new Error("control response capacity exceeded");
      chunks.push(next.value);
    }
  } catch (error) {
    await reader.cancel(error);
    throw error;
  } finally {
    reader.releaseLock();
  }
  const bytes = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
}
