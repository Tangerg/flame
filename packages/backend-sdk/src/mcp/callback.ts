import { request as httpRequest, type IncomingMessage } from "node:http";
import { constants as httpStatus } from "node:http2";
import { Readable } from "node:stream";
import { FrameError } from "@frame/sdk";
import { parseJSON, stringifyJSON } from "@frame/sdk/json";
import {
  exactFields,
  freezeJSON,
  declareMethod,
  type Method,
  PROTOCOL,
} from "./contract.ts";
import wire from "./protocol.generated.json";

export interface CallbackRef {
  readonly endpoint: string;
  readonly credential: string;
  readonly call: string;
  readonly maxResponseBytes: number | bigint;
}
export interface Invocation {
  readonly id: string;
  readonly origin: string;
  readonly parent?: string;
  readonly target: { readonly scope: string; readonly reference: string };
}
export function callbackRef(value: unknown): CallbackRef {
  const ref: unknown = freezeJSON(value);
  exactFields(ref, ["endpoint", "credential", "call", "maxResponseBytes"]);
  if (typeof ref.endpoint !== "string")
    throw new Error("plugin: invalid callback reference");
  const endpoint = new URL(ref.endpoint);
  if (
    endpoint.protocol !== "http:" ||
    endpoint.hostname !== "127.0.0.1" ||
    !endpoint.port ||
    endpoint.pathname !== "/" ||
    endpoint.search ||
    endpoint.hash ||
    endpoint.username ||
    endpoint.password ||
    typeof ref.credential !== "string" ||
    !/^[a-fA-F0-9]{64}$/.test(ref.credential) ||
    typeof ref.call !== "string" ||
    !/^[a-fA-F0-9]{64}$/.test(ref.call) ||
    (typeof ref.maxResponseBytes === "number" &&
      !Number.isSafeInteger(ref.maxResponseBytes)) ||
    (typeof ref.maxResponseBytes !== "number" &&
      typeof ref.maxResponseBytes !== "bigint") ||
    BigInt(ref.maxResponseBytes) <= 0n ||
    BigInt(ref.maxResponseBytes) >= 9223372036854775807n
  )
    throw new Error("plugin: invalid callback reference");
  return Object.freeze({
    endpoint: ref.endpoint,
    credential: ref.credential,
    call: ref.call,
    maxResponseBytes: ref.maxResponseBytes,
  });
}

export function callbackFetch(
  ref: CallbackRef,
  invocation?: Invocation,
  path = "/",
): typeof fetch {
  ref = callbackRef(ref);
  const owner = invocation === undefined ? undefined : freezeJSON(invocation);
  if (
    !path.startsWith("/") ||
    path.startsWith("//") ||
    path.includes("?") ||
    path.includes("#")
  )
    throw new Error("plugin: invalid callback path");
  return (async (input: string | URL | Request, init: RequestInit = {}) => {
    const url = new URL(
      typeof input === "string" || input instanceof URL ? input : input.url,
    );
    if (
      url.origin !== new URL(ref.endpoint).origin ||
      url.pathname !== path ||
      url.search ||
      url.hash
    )
      throw new Error(
        "plugin: callback destination differs from its captured reference",
      );
    const headers = new Headers(init.headers);
    headers.set("Authorization", ref.credential);
    headers.set(wire.constants.CallHeader, ref.call);
    if (owner) {
      // Header bytes are ASCII; JSON escapes preserve the original Unicode
      // identity without changing admission, request bodies or receipt digests.
      const encoded = stringifyJSON(owner).replace(
        /[\u007f-\uffff]/g,
        (unit) => "\\u" + unit.charCodeAt(0).toString(16).padStart(4, "0"),
      );
      headers.set(wire.constants.InvocationHeader!, encoded);
    }
    return await new Promise<Response>((resolve, reject) => {
      const request = httpRequest(
        url,
        {
          method: init.method ?? "POST",
          headers: Object.fromEntries(headers),
          signal: init.signal ?? undefined,
          agent: false,
        },
        (response) => {
          try {
            resolve(callbackResponse(response, BigInt(ref.maxResponseBytes)));
          } catch (cause) {
            response.destroy();
            reject(new Error("plugin: invalid callback response", { cause }));
          }
        },
      );
      request.once("error", (cause) =>
        reject(new Error("plugin: callback transport failed", { cause })),
      );
      if (typeof init.body !== "string") {
        request.destroy();
        reject(new Error("plugin: callback requires an encoded request"));
      } else request.end(init.body);
    });
  }) as typeof fetch;
}

function callbackResponse(response: IncomingMessage, bound: bigint): Response {
  const headers = new Headers();
  for (const [key, value] of Object.entries(response.headers)) {
    if (value !== undefined)
      headers.set(key, Array.isArray(value) ? value.join(", ") : value);
  }
  const metadata = { status: response.statusCode!, headers };
  switch (response.statusCode) {
    case httpStatus.HTTP_STATUS_NO_CONTENT:
    case httpStatus.HTTP_STATUS_RESET_CONTENT:
    case httpStatus.HTTP_STATUS_NOT_MODIFIED:
      response.destroy();
      return new Response(null, metadata);
  }
  const source = Readable.toWeb(response, {
    strategy: { size: (bytes: Uint8Array) => bytes.byteLength },
  }) as ReadableStream<Uint8Array>;
  if ((headers.get("content-type") ?? "").includes("text/event-stream"))
    return new Response(source, metadata);
  let size = 0n;
  return new Response(
    source.pipeThrough(
      new TransformStream<Uint8Array, Uint8Array>({
        transform(bytes, controller) {
          size += BigInt(bytes.length);
          if (size > bound)
            throw new Error("plugin: callback response capacity exceeded");
          controller.enqueue(bytes);
        },
      }),
    ),
    metadata,
  );
}

// The refusal code every method reports when a command identity is reused with
// different input; the Go framework names the same code.
export const invocationConflictCode = "command_conflict";

export class Refusal extends Error {
  constructor(
    readonly code: string,
    message: string,
  ) {
    if (typeof code !== "string" || !code || typeof message !== "string")
      throw new Error(
        "plugin: refusal requires a nonempty code and string message",
      );
    stringifyJSON({ code, message });
    super(message);
    this.name = "Refusal";
  }
}
export function decodeReply(value: unknown): unknown {
  if (value === null || typeof value !== "object")
    throw new Error("plugin: invalid operation reply");
  if (Object.hasOwn(value, "failure")) {
    exactFields(value, ["failure"]);
    exactFields(value.failure, ["code", "message"]);
    if (
      typeof value.failure.code !== "string" ||
      typeof value.failure.message !== "string"
    )
      throw new Error("plugin: invalid refusal");
    throw new Refusal(value.failure.code, value.failure.message);
  }
  exactFields(value, ["value"]);
  return value.value;
}
export async function callHost(
  ref: CallbackRef,
  method: Method,
  input: unknown,
  signal: AbortSignal,
  invocation?: Invocation,
  commandId?: string,
): Promise<unknown> {
  const admitted = method.admitInvocation(input, commandId);
  const captured = callbackRef(ref);
  const response = await callbackFetch(captured, invocation)(
    captured.endpoint,
    {
      method: "POST",
      signal,
      headers: { "Content-Type": "application/json" },
      body: stringifyJSON({
        protocol: PROTOCOL,
        method: method.descriptor.name,
        contract: method.identity,
        input: admitted,
        ...(commandId ? { commandId } : {}),
      }),
    },
  );
  if (response.status !== 200) {
    await response.body?.cancel();
    if (response.status === 429)
      throw new FrameError("capacity", "plugin: callback capacity exceeded");
    if (response.status === 410)
      throw new FrameError(
        "closed",
        "plugin: callback call is no longer active",
      );
    throw new FrameError(
      "outcome_unknown",
      "plugin: host operation could not be confirmed",
    );
  }
  const value = decodeReply(
    parseJSON(
      new TextDecoder("utf-8", { fatal: true }).decode(
        await response.arrayBuffer(),
      ),
    ),
  );
  return method.admitOutput(value);
}
export const streamDelivery = declareMethod(parseJSON(wire.streamDelivery));
