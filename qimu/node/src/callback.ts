import { request as httpRequest } from "node:http";
import { QimuError } from "@qimu/sdk";
import { parseJSON, stringifyJSON } from "@qimu/sdk/json";
import {
  exactFields,
  freezeJSON,
  Method,
  PROTOCOL,
  validCommand,
  type MethodDescriptor,
} from "./contract.ts";
import wire from "./protocol.generated.json";

export interface CallbackRef {
  endpoint: string;
  credential: string;
  call: string;
  maxResponseBytes: number | bigint;
}
export interface Invocation {
  id: string;
  origin: string;
  parent?: string;
  target: { scope: string; reference: string };
}
export function callbackRef(value: unknown): CallbackRef {
  exactFields(value, ["endpoint", "credential", "call", "maxResponseBytes"]);
  const ref = value as unknown as CallbackRef;
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
    ![ref.credential, ref.call].every(
      (value) => typeof value === "string" && /^[a-fA-F0-9]{64}$/.test(value),
    ) ||
    (typeof ref.maxResponseBytes === "number" &&
      !Number.isSafeInteger(ref.maxResponseBytes)) ||
    !["number", "bigint"].includes(typeof ref.maxResponseBytes) ||
    BigInt(ref.maxResponseBytes) <= 0n ||
    BigInt(ref.maxResponseBytes) >= 9223372036854775807n
  )
    throw new Error("plugin: invalid callback reference");
  return freezeJSON(ref);
}

export function callbackFetch(
  ref: CallbackRef,
  invocation?: Invocation,
  path = "/",
): typeof fetch {
  ref = callbackRef(ref);
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
    if (invocation) {
      // Header bytes are ASCII; JSON escapes preserve the original Unicode
      // identity without changing admission, request bodies or receipt digests.
      const encoded = stringifyJSON(invocation).replace(
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
          const responseHeaders = new Headers();
          for (const [key, value] of Object.entries(response.headers)) {
            if (value !== undefined)
              responseHeaders.set(
                key,
                Array.isArray(value) ? value.join(", ") : value,
              );
          }
          const streaming = (response.headers["content-type"] ?? "").includes(
            "text/event-stream",
          );
          let size = 0n;
          const bound = BigInt(ref.maxResponseBytes);
          const iterator = response[Symbol.asyncIterator]();
          const body = new ReadableStream<Uint8Array>({
            async pull(controller) {
              try {
                const next = await iterator.next();
                if (next.done) {
                  controller.close();
                  return;
                }
                const bytes = next.value as Buffer;
                if (!streaming) {
                  size += BigInt(bytes.length);
                  if (size > bound)
                    throw new Error(
                      "plugin: callback response capacity exceeded",
                    );
                }
                controller.enqueue(bytes);
              } catch (error) {
                response.destroy();
                await iterator.return?.();
                throw error;
              }
            },
            async cancel() {
              response.destroy();
              await iterator.return?.();
            },
          });
          resolve(
            new Response(body, {
              status: response.statusCode!,
              headers: responseHeaders,
            }),
          );
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
  if (
    method.descriptor.kind === "command"
      ? !validCommand(commandId)
      : commandId !== undefined
  )
    throw new Error(
      "plugin: invalid callback command identity for method kind",
    );
  method.validateInput(input);
  const response = await callbackFetch(callbackRef(ref), invocation)(
    ref.endpoint,
    {
      method: "POST",
      signal,
      headers: { "Content-Type": "application/json" },
      body: stringifyJSON({
        protocol: PROTOCOL,
        method: method.descriptor.name,
        contract: method.identity,
        input,
        ...(commandId ? { commandId } : {}),
      }),
    },
  );
  if (response.status !== 200) {
    await response.body?.cancel();
    if (response.status === 429)
      throw new QimuError("capacity", "plugin: callback capacity exceeded");
    if (response.status === 410)
      throw new QimuError(
        "closed",
        "plugin: callback call is no longer active",
      );
    throw new QimuError(
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
  method.validateOutput(value);
  return value;
}
export const streamDelivery = new Method(
  parseJSON(wire.streamDelivery) as MethodDescriptor,
);
