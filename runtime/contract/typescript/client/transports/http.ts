import { requireRuntimeEndpoint } from "../endpoint";
import {
  context,
  propagation,
  type Span,
  SpanKind,
  SpanStatusCode,
  trace,
} from "@opentelemetry/api";
import { createParser } from "eventsource-parser";
import { createPushPullChannel } from "../channel";
import {
  errorMessage,
  parseTransportProblem,
  RpcConnectionError,
  RpcTransportError,
} from "../errors";
import {
  type Transport,
  type TransportEvent,
  type TransportRequest,
  type TransportResponseMetadata,
  type TransportSendOptions,
} from "../transport";
import type { RpcId } from "../types";
import { isResponse, parseRpcMessage } from "../types";
import { HTTP_ENDPOINTS } from "@flame/runtime-contract/wire";
import {
  isWireStreamingMethodName,
  type WireStreamingMethodName,
} from "@flame/runtime-contract/methods";

const MAXIMUM_EVENT_STREAM_FRAME_CHARACTERS = 128 * 1024 * 1024;

const tracer = trace.getTracer("flame-frontend");

function endSpan(span: Span, err?: unknown): void {
  if (err !== undefined) {
    span.setStatus({
      code: SpanStatusCode.ERROR,
      message: err instanceof Error ? err.message : String(err),
    });
  }
  span.end();
}

export interface HttpTransportConfig {
  baseUrl: string;
  localToken?: string;
  fetch?: typeof fetch;
  maximumEventStreamFrameCharacters?: number;
}

interface EventStreamTextParser {
  feed(chunk: string): void;
}

async function feedEventStreamText(
  parser: EventStreamTextParser,
  text: string,
  afterLine: () => Promise<boolean> | undefined,
): Promise<boolean> {
  let start = 0;
  while (start < text.length) {
    let lineEnd = start;
    while (lineEnd < text.length && text[lineEnd] !== "\n" && text[lineEnd] !== "\r") {
      lineEnd++;
    }
    if (lineEnd === text.length) {
      parser.feed(text.slice(start));
      const delivery = afterLine();
      return delivery === undefined ? true : delivery;
    }
    if (text[lineEnd] === "\r" && text[lineEnd + 1] === "\n") lineEnd++;
    parser.feed(text.slice(start, lineEnd + 1));
    const delivery = afterLine();
    if (delivery !== undefined && !(await delivery)) return false;
    start = lineEnd + 1;
  }
  return true;
}

export function createHttpTransport(config: HttpTransportConfig): Transport {
  const baseUrl = requireRuntimeEndpoint(config.baseUrl);
  const fetchImpl = config.fetch ?? globalThis.fetch.bind(globalThis);
  const maximumEventStreamFrameCharacters =
    config.maximumEventStreamFrameCharacters ?? MAXIMUM_EVENT_STREAM_FRAME_CHARACTERS;
  if (
    !Number.isSafeInteger(maximumEventStreamFrameCharacters) ||
    maximumEventStreamFrameCharacters <= 0
  ) {
    throw new RangeError("event-stream frame capacity must be a positive safe integer");
  }

  const channel = createPushPullChannel<TransportEvent>({ capacity: 0 });
  const closeController = new AbortController();
  const readers = new Set<ReadableStreamDefaultReader<Uint8Array>>();
  const activeSends = new Set<Promise<void>>();
  const activeDrains = new Set<Promise<void>>();
  let closePromise: Promise<void> | undefined;

  function requestHeaders(extra: Record<string, string>): Record<string, string> {
    const headers: Record<string, string> = { ...extra };
    if (config.localToken) headers.Authorization = `Bearer ${config.localToken}`;
    return headers;
  }

  async function drainStream(
    body: ReadableStream<Uint8Array>,
    requestId: RpcId,
    method: WireStreamingMethodName,
    metadata: TransportResponseMetadata,
    signal?: AbortSignal,
  ): Promise<void> {
    let responseSeen = false;
    let streamError: Error | undefined;
    let parsedEvent: TransportEvent | undefined;
    const parser = createParser({
      onEvent(event) {
        if (!event.data) return;
        const msg = parseRpcMessage(event.data);
        if (!msg) {
          streamError = new RpcTransportError(
            "invalid JSON-RPC envelope in event stream",
            undefined,
            metadata.requestId,
          );
          throw streamError;
        }
        if (isResponse(msg) && msg.id === requestId) {
          responseSeen = true;
        }
        parsedEvent = {
          type: "message",
          message: msg,
          requestRpcId: requestId,
          metadata,
        };
      },
      onError(error) {
        if (error.type !== "max-buffer-size-exceeded") return;
        streamError = new RpcTransportError(
          `event-stream frame exceeds ${maximumEventStreamFrameCharacters} characters`,
          undefined,
          metadata.requestId,
        );
        throw streamError;
      },
      maxBufferSize: maximumEventStreamFrameCharacters,
    });
    const deliverParsedEvent = (): Promise<boolean> | undefined => {
      if (parsedEvent === undefined) return undefined;
      const event = parsedEvent;
      parsedEvent = undefined;
      return channel.send(event);
    };
    const reader = body.getReader();
    readers.add(reader);
    const decoder = new TextDecoder();
    let aborted = false;
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        if (
          !(await feedEventStreamText(
            parser,
            decoder.decode(value, { stream: true }),
            deliverParsedEvent,
          ))
        ) {
          return;
        }
      }
      if (!(await feedEventStreamText(parser, decoder.decode(), deliverParsedEvent))) return;
    } catch (err) {
      aborted = signal?.aborted === true || (err instanceof Error && err.name === "AbortError");
      if (!aborted && !channel.closed) {
        streamError =
          err instanceof RpcTransportError
            ? err
            : new RpcConnectionError(errorMessage(err), metadata.requestId);
      }
    } finally {
      readers.delete(reader);
      reader.releaseLock();
    }
    if (aborted || channel.closed) return;
    if (!responseSeen) {
      if (
        !(await channel.send({
          type: "requestError",
          rpcId: requestId,
          error:
            streamError ??
            new RpcTransportError(
              "event stream ended before the call's response",
              undefined,
              metadata.requestId,
            ),
        }))
      ) {
        return;
      }
    }
    await channel.send({
      type: "streamEnd",
      method,
      requestRpcId: requestId,
      ...(streamError ? { error: streamError } : {}),
      metadata,
    });
  }

  async function sendRequest(
    msg: TransportRequest,
    signal?: AbortSignal,
    options: TransportSendOptions = {},
  ): Promise<void> {
    if (channel.closed) throw new RpcTransportError("transport closed");

    const method = msg.method;
    const url = `${baseUrl}${HTTP_ENDPOINTS.rpc.path}`;
    const rpcId = msg.id;
    const requestSignal = signal
      ? AbortSignal.any([signal, closeController.signal])
      : closeController.signal;

    const span = tracer.startSpan(`rpc ${method}`, {
      kind: SpanKind.CLIENT,
      attributes: { "rpc.system": "jsonrpc", "rpc.method": method },
    });
    const headers = requestHeaders({
      "Content-Type": "application/json",
      Accept: "application/json, text/event-stream",
    });
    if (options.idempotencyKey) headers["Idempotency-Key"] = options.idempotencyKey;
    if (options.idempotencyNamespace) {
      headers["Idempotency-Namespace"] = options.idempotencyNamespace;
    }
    if (options.lastEventId) headers["Last-Event-Id"] = options.lastEventId;
    propagation.inject(trace.setSpan(context.active(), span), headers);

    let res: Response;
    try {
      res = await fetchImpl(url, {
        method: HTTP_ENDPOINTS.rpc.method,
        redirect: "error",
        headers,
        body: JSON.stringify(msg),
        signal: requestSignal,
      });
    } catch (err) {
      endSpan(span, err);
      throw new RpcConnectionError(`fetch failed: ${errorMessage(err)}`);
    }
    span.setAttribute("rpc.http.status_code", res.status);
    const metadata: TransportResponseMetadata = {
      requestId: res.headers.get("Request-Id") ?? undefined,
    };
    if (metadata.requestId) span.setAttribute("flame.request_id", metadata.requestId);

    if (res.status === 204 || res.status === 202) {
      const err = new RpcTransportError(
        `http ${res.status}: RPC call ended without a response`,
        res.status,
        metadata.requestId,
      );
      endSpan(span, err);
      throw err;
    }

    if (!res.ok) {
      const text = await res.text().catch(() => "");
      const problem = parseTransportProblem(text);
      const requestId = problem?.requestId ?? metadata.requestId;
      const detail = problem?.detail || res.statusText || "transport request failed";
      const err = new RpcTransportError(
        `http ${res.status}: ${detail}${requestId ? ` (request ${requestId})` : ""}`,
        res.status,
        requestId,
        problem?.type,
      );
      endSpan(span, err);
      throw err;
    }

    if ((res.headers.get("Content-Type") ?? "").includes("text/event-stream")) {
      if (!isWireStreamingMethodName(method)) {
        const err = new RpcTransportError(
          `non-streaming RPC method ${method} returned an event stream`,
          undefined,
          metadata.requestId,
        );
        endSpan(span, err);
        await res.body?.cancel();
        throw err;
      }
      if (!res.body) {
        const err = new RpcTransportError(
          "event-stream response has no body",
          undefined,
          metadata.requestId,
        );
        endSpan(span, err);
        throw err;
      }
      endSpan(span);
      const draining = drainStream(res.body, rpcId, method, metadata, requestSignal);
      activeDrains.add(draining);
      void draining.then(
        () => activeDrains.delete(draining),
        () => activeDrains.delete(draining),
      );
      return;
    }

    let text: string;
    try {
      text = await res.text();
    } catch (cause) {
      const err = new RpcConnectionError(
        `failed to read RPC response: ${errorMessage(cause)}`,
        metadata.requestId,
      );
      endSpan(span, err);
      throw err;
    }
    if (!text) {
      const err = new RpcTransportError(
        "RPC response body is empty",
        undefined,
        metadata.requestId,
      );
      endSpan(span, err);
      throw err;
    }
    const inbound = parseRpcMessage(text);
    if (!inbound) {
      const err = new RpcTransportError(
        `invalid JSON-RPC envelope in response body: ${text.slice(0, 200)}`,
        undefined,
        metadata.requestId,
      );
      endSpan(span, err);
      throw err;
    }
    if (!isResponse(inbound) || inbound.id !== rpcId) {
      const err = new RpcTransportError(
        "RPC response does not match the outbound request",
        undefined,
        metadata.requestId,
      );
      endSpan(span, err);
      throw err;
    }
    if (
      !(await channel.send({
        type: "message",
        message: inbound,
        requestRpcId: rpcId,
        metadata,
      }))
    ) {
      const err = new RpcTransportError("transport closed before delivering the RPC response");
      endSpan(span, err);
      throw err;
    }
    endSpan(span);
  }

  function send(
    msg: TransportRequest,
    signal?: AbortSignal,
    options?: TransportSendOptions,
  ): Promise<void> {
    const sending = sendRequest(msg, signal, options);
    activeSends.add(sending);
    void sending.then(
      () => activeSends.delete(sending),
      () => activeSends.delete(sending),
    );
    return sending;
  }

  function recv(): AsyncIterable<TransportEvent> {
    return channel.iterator();
  }

  async function closeOwnedResources(): Promise<void> {
    channel.close();
    closeController.abort();
    const cancelReaders = () => [...readers].map((reader) => reader.cancel());

    await Promise.allSettled([...activeSends, ...cancelReaders()]);
    await Promise.allSettled(cancelReaders());
    await Promise.allSettled(activeDrains);
    readers.clear();
  }

  function close(): Promise<void> {
    closePromise ??= closeOwnedResources();
    return closePromise;
  }

  return { send, recv, close };
}
