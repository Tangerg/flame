import type { TransportRequest } from "@flame/runtime-contract/client/transport";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  RpcConnectionError,
  RpcProtocolError,
  RpcTransportError,
} from "@flame/runtime-contract/client/errors";
import type { WireMethodName } from "@flame/runtime-contract/methods";
import { createHttpTransport } from "@flame/runtime-contract/client/transports/http";
import { createFlameClient } from "@flame/runtime-contract/client/sdk";
import { asSessionId } from "@flame/runtime-contract/client/ids";
import { createMutationJournal } from "@flame/runtime-contract/client/mutationJournal";

afterEach(() => vi.restoreAllMocks());

function sseResponse(chunks: string[], requestId?: string): Response {
  const enc = new TextEncoder();
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(enc.encode(c));
      controller.close();
    },
  });
  return new Response(body, {
    status: 200,
    headers: {
      "Content-Type": "text/event-stream",
      ...(requestId ? { "Request-Id": requestId } : {}),
    },
  });
}

function abortingSseResponse(firstChunk: string): Response {
  const enc = new TextEncoder();
  let sent = false;
  const body = new ReadableStream<Uint8Array>({
    pull(controller) {
      if (!sent) {
        sent = true;
        controller.enqueue(enc.encode(firstChunk));
      } else {
        controller.error(Object.assign(new Error("aborted"), { name: "AbortError" }));
      }
    },
  });
  return new Response(body, {
    status: 200,
    headers: { "Content-Type": "text/event-stream" },
  });
}

function jsonResponse(obj: unknown, requestId?: string): Response {
  return new Response(JSON.stringify(obj), {
    status: 200,
    headers: {
      "Content-Type": "application/json",
      ...(requestId ? { "Request-Id": requestId } : {}),
    },
  });
}

const frame = (obj: unknown, id?: string): string =>
  `${id ? `id: ${id}\n` : ""}data: ${JSON.stringify(obj)}\n\n`;

const req = (id: string, method: WireMethodName): TransportRequest => ({
  jsonrpc: "2.0",
  id,
  method,
  params: {},
});

function deferred(): { promise: Promise<void>; resolve: () => void } {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("HTTPTransport — streamable HTTP", () => {
  it("rejects an invalid event-stream frame capacity before opening transport resources", () => {
    const fetchStub = vi.fn<typeof fetch>();

    expect(() =>
      createHttpTransport({
        baseUrl: "http://x",
        fetch: fetchStub,
        maximumEventStreamFrameCharacters: 0,
      }),
    ).toThrow("event-stream frame capacity must be a positive safe integer");
    expect(fetchStub).not.toHaveBeenCalled();
  });

  it("terminates an SSE response whose unfinished frame exceeds its resource envelope", async () => {
    const fetchStub = (async () =>
      sseResponse([
        frame({
          jsonrpc: "2.0",
          id: "1",
          result: { runId: "run_01", padding: "x".repeat(128) },
        }),
      ])) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
      maximumEventStreamFrameCharacters: 64,
    });
    const iterator = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("1", "runs.start"));
    await expect(iterator.next()).resolves.toMatchObject({
      value: {
        type: "requestError",
        rpcId: "1",
        error: { message: "event-stream frame exceeds 64 characters" },
      },
      done: false,
    });
    await expect(iterator.next()).resolves.toMatchObject({
      value: {
        type: "streamEnd",
        requestRpcId: "1",
        error: { message: "event-stream frame exceeds 64 characters" },
      },
      done: false,
    });

    await transport.close();
  });

  it("paces every frame in one network chunk against the sole recv consumer", async () => {
    const responseFrame = frame({
      jsonrpc: "2.0",
      id: "1",
      result: { runId: "run_01" },
    });
    const liveFrames = Array.from({ length: 64 }, (_, index) =>
      frame({
        jsonrpc: "2.0",
        method: "notifications.run.event",
        params: {
          runId: "run_01",
          segmentId: "seg_01",
          eventId: `evt_${index}`,
          occurredAt: "2026-08-30T00:00:00Z",
          event: { type: "item.delta", itemId: "itm_01", delta: String(index) },
        },
      }),
    );
    const parse = vi.spyOn(JSON, "parse");
    const parsedFrameCount = (): number =>
      parse.mock.calls.filter(
        ([input]) => typeof input === "string" && input.startsWith('{"jsonrpc":"2.0"'),
      ).length;
    const fetchStub = (async () =>
      sseResponse([responseFrame + liveFrames.join("")])) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });

    await transport.send(req("1", "runs.start"));
    await vi.waitFor(() => expect(parsedFrameCount()).toBeGreaterThan(0));
    await Promise.resolve();

    expect(parsedFrameCount()).toBe(1);

    const iterator = transport.recv()[Symbol.asyncIterator]();
    await expect(iterator.next()).resolves.toMatchObject({
      value: { type: "message", requestRpcId: "1", message: { id: "1" } },
      done: false,
    });
    await vi.waitFor(() => expect(parsedFrameCount()).toBe(2));

    await transport.close();
    expect(parsedFrameCount()).toBe(2);
  });

  it("does not pull another body chunk until recv accepts the current frame", async () => {
    const chunks = [
      frame({ jsonrpc: "2.0", id: "1", result: { runId: "run_01" } }),
      frame({
        jsonrpc: "2.0",
        method: "notifications.run.event",
        params: { event: { type: "item.delta" } },
      }),
      frame({
        jsonrpc: "2.0",
        method: "notifications.run.event",
        params: { event: { type: "segment.progress" } },
      }),
    ];
    const encoder = new TextEncoder();
    let pulls = 0;
    const body = new ReadableStream<Uint8Array>(
      {
        pull(controller) {
          const chunk = chunks[pulls++];
          if (chunk === undefined) controller.close();
          else controller.enqueue(encoder.encode(chunk));
        },
      },
      { highWaterMark: 0 },
    );
    const fetchStub = (async () =>
      new Response(body, {
        status: 200,
        headers: { "Content-Type": "text/event-stream" },
      })) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });

    await transport.send(req("1", "runs.start"));
    await vi.waitFor(() => expect(pulls).toBe(1));
    await Promise.resolve();
    expect(pulls).toBe(1);

    const iterator = transport.recv()[Symbol.asyncIterator]();
    await expect(iterator.next()).resolves.toMatchObject({
      value: { type: "message", requestRpcId: "1", message: { id: "1" } },
      done: false,
    });
    await vi.waitFor(() => expect(pulls).toBe(2));
    expect(pulls).toBe(2);

    await transport.close();
  });

  it("streaming method: POST response stream yields the call response then its events", async () => {
    const responseFrame = frame({
      jsonrpc: "2.0",
      id: "1",
      result: { runId: "run_01" },
    });
    const started = frame(
      {
        jsonrpc: "2.0",
        method: "notifications.run.event",
        params: { event: { type: "segment.started" } },
      },
      "evt_0001",
    );
    const finished = frame(
      {
        jsonrpc: "2.0",
        method: "notifications.run.event",
        params: { event: { type: "segment.finished" } },
      },
      "evt_0002",
    );
    const wire = responseFrame + started + finished;
    const cut = Math.floor(wire.length / 2);

    const fetchStub = (async () =>
      sseResponse(
        [wire.slice(0, cut), wire.slice(cut)],
        "req_stream_01",
      )) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("1", "runs.start"));
    const r0 = await it.next();
    const r1 = await it.next();
    const r2 = await it.next();
    await transport.close();

    expect(r0.value).toMatchObject({
      type: "message",
      message: { id: "1", result: { runId: "run_01" } },
      requestRpcId: "1",
      metadata: { requestId: "req_stream_01" },
    });
    expect(r1.value).toMatchObject({
      type: "message",
      message: { params: { event: { type: "segment.started" } } },
      requestRpcId: "1",
    });
    expect(r2.value).toMatchObject({
      type: "message",
      message: { params: { event: { type: "segment.finished" } } },
      requestRpcId: "1",
    });
  });

  it("non-streaming method: POST returns a single application/json message", async () => {
    const fetchStub = vi.fn(async () =>
      jsonResponse(
        {
          jsonrpc: "2.0",
          id: "2",
          result: { id: "ses_1" },
        },
        "req_unary_01",
      ),
    );
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });
    const it = transport.recv()[Symbol.asyncIterator]();

    const receiving = it.next();
    await transport.send(req("2", "sessions.get"));
    const r = await receiving;
    await transport.close();

    expect(r.value).toMatchObject({
      type: "message",
      message: { id: "2", result: { id: "ses_1" } },
      requestRpcId: "2",
      metadata: { requestId: "req_unary_01" },
    });
    expect(fetchStub).toHaveBeenCalledWith(
      "http://x/v2/rpc",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("rejects an event stream returned for a non-streaming method", async () => {
    const fetchStub = (async () => sseResponse([])) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });

    await expect(transport.send(req("2", "sessions.get"))).rejects.toThrow(
      new RpcProtocolError("RPC response", [
        { path: "$", detail: "must not be an event stream for non-streaming method sessions.get" },
      ]),
    );
    await transport.close();
  });

  it("sends the logical mutation identity and Runtime store as transport metadata", async () => {
    const fetchStub = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      jsonResponse({ jsonrpc: "2.0", id: "2", result: { id: "ses_1" } }),
    );
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });
    const it = transport.recv()[Symbol.asyncIterator]();

    const receiving = it.next();
    await transport.send(req("2", "sessions.create"), undefined, {
      idempotencyKey: "operation-key-1",
      idempotencyNamespace: "idp_store_a",
    });
    await receiving;
    await transport.close();

    const headers = fetchStub.mock.calls[0]?.[1]?.headers as Record<string, string>;
    expect(headers["Idempotency-Key"]).toBe("operation-key-1");
    expect(headers["Idempotency-Namespace"]).toBe("idp_store_a");
  });

  it("rejects a no-content response for a call", async () => {
    const fetchStub = (async () => new Response(null, { status: 204 })) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });

    await expect(transport.send(req("2", "sessions.get"))).rejects.toBeInstanceOf(RpcProtocolError);
    await transport.close();
  });

  it("rejects a response correlated to another request", async () => {
    const fetchStub = (async () =>
      jsonResponse({
        jsonrpc: "2.0",
        id: "other",
        result: {},
      })) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });

    await expect(transport.send(req("2", "sessions.get"))).rejects.toThrow(
      new RpcProtocolError("RPC response", [
        { path: "$.id", detail: "must match the outbound request" },
      ]),
    );
    await transport.close();
  });

  it("close aborts an in-flight request owned by the transport", async () => {
    let requestSignal: AbortSignal | undefined;
    const fetchStub = ((_url: string, init?: RequestInit) => {
      requestSignal = init?.signal ?? undefined;
      return new Promise<Response>((_resolve, reject) => {
        requestSignal?.addEventListener(
          "abort",
          () => reject(new DOMException("aborted", "AbortError")),
          { once: true },
        );
      });
    }) as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });
    const sending = transport.send(req("2", "sessions.get"));
    await Promise.resolve();

    await transport.close();
    await expect(sending).rejects.toThrow("fetch failed: aborted");
    expect(requestSignal?.aborted).toBe(true);
  });

  it("does not report closed until an in-flight request releases after abort", async () => {
    const aborted = deferred();
    const release = deferred();
    const fetchStub = ((_url: string, init?: RequestInit) =>
      new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener(
          "abort",
          () => {
            aborted.resolve();
            void release.promise.then(() => reject(new DOMException("aborted", "AbortError")));
          },
          { once: true },
        );
      })) as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });
    const sending = transport.send(req("2", "sessions.get")).catch((error: unknown) => error);
    let closed = false;
    const closing = transport.close().then(() => {
      closed = true;
    });

    await aborted.promise;
    await Promise.resolve();
    expect(closed).toBe(false);

    release.resolve();
    await closing;
    await expect(sending).resolves.toBeInstanceOf(RpcConnectionError);
  });

  it("does not report closed until an active stream reader releases", async () => {
    const cancelStarted = deferred();
    const release = deferred();
    const pullRelease = deferred();
    const body = new ReadableStream<Uint8Array>({
      pull: () => pullRelease.promise,
      async cancel() {
        cancelStarted.resolve();
        await release.promise;
      },
    });
    const fetchStub = (async () =>
      new Response(body, {
        status: 200,
        headers: { "Content-Type": "text/event-stream" },
      })) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });
    await transport.send(req("1", "runs.start"));
    let closed = false;
    const closing = transport.close().then(() => {
      closed = true;
    });

    await cancelStarted.promise;
    await Promise.resolve();
    expect(closed).toBe(false);

    release.resolve();
    await closing;
    pullRelease.resolve();
    await pullRelease.promise;
  });

  it("non-2xx surfaces structured transport diagnostics", async () => {
    const fetchStub = (async () =>
      new Response(
        JSON.stringify({
          type: "urn:flame:transport:invalid_request",
          detail: "bad request",
          requestId: "req_123",
        }),
        {
          status: 400,
          headers: { "Content-Type": "application/problem+json" },
        },
      )) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });
    await expect(transport.send(req("3", "runs.start"))).rejects.toMatchObject({
      name: "RpcTransportError",
      status: 400,
      requestId: "req_123",
      problemType: "urn:flame:transport:invalid_request",
    } satisfies Partial<RpcTransportError>);
    await transport.close();
  });

  it("stays quiet when the stream is aborted (expected teardown, not an error)", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const responseFrame = frame({
      jsonrpc: "2.0",
      id: "1",
      result: { runId: "run_01" },
    });
    const fetchStub = (async () => abortingSseResponse(responseFrame)) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("1", "runs.start"));
    await it.next();
    await new Promise((resolve) => setTimeout(resolve, 0));
    await transport.close();

    expect(warn).not.toHaveBeenCalled();
  });

  it("a stream dying mid-run reports a typed stream termination", async () => {
    const responseFrame = frame({
      jsonrpc: "2.0",
      id: "1",
      result: { runId: "run_01" },
    });
    const enc = new TextEncoder();
    let sent = false;
    const body = new ReadableStream<Uint8Array>({
      pull(controller) {
        if (!sent) {
          sent = true;
          controller.enqueue(enc.encode(responseFrame));
        } else {
          controller.error(new Error("connection reset"));
        }
      },
    });
    const fetchStub = (async () =>
      new Response(body, {
        status: 200,
        headers: { "Content-Type": "text/event-stream" },
      })) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("1", "runs.start"));
    const r0 = await it.next();
    const r1 = await it.next();
    await transport.close();

    expect(r0.value).toMatchObject({
      type: "message",
      message: { id: "1", result: { runId: "run_01" } },
    });
    expect(r1.value).toMatchObject({
      type: "streamEnd",
      method: "runs.start",
      requestRpcId: "1",
      error: expect.any(RpcConnectionError),
    });
  });

  it("a stream ending before the call's response reports a request failure", async () => {
    const fetchStub = (async () => sseResponse([])) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("7", "runs.start"));
    const r = await it.next();
    await transport.close();

    expect(r.value).toMatchObject({
      type: "requestError",
      rpcId: "7",
      error: expect.any(RpcTransportError),
    });
  });

  it("fails a stream carrying a malformed JSON-RPC envelope", async () => {
    const wire =
      frame({ jsonrpc: "2.0", id: "1", result: { runId: "run_01" } }) + `data: {not json}\n\n`;
    const fetchStub = (async () => sseResponse([wire])) as unknown as typeof fetch;
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: fetchStub,
    });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("1", "runs.start"));
    const response = await it.next();
    const ended = await it.next();
    await transport.close();

    expect(response.value).toMatchObject({
      type: "message",
      message: { id: "1" },
    });
    expect(ended.value).toMatchObject({
      type: "streamEnd",
      method: "runs.start",
      requestRpcId: "1",
      error: expect.any(RpcProtocolError),
    });
  });

  it("cancels an unread response body after rejecting a malformed frame", async () => {
    const canceled = vi.fn();
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("data: {not json}\n\n"));
      },
      cancel: canceled,
    });
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: vi.fn(
        async () =>
          new Response(body, {
            headers: { "Content-Type": "text/event-stream" },
          }),
      ),
    });
    const events = transport.recv()[Symbol.asyncIterator]();
    try {
      await transport.send(req("1", "runs.start"));
      await expect(events.next()).resolves.toMatchObject({
        value: { type: "requestError", rpcId: "1" },
      });
      expect(canceled).toHaveBeenCalledOnce();
      expect(body.locked).toBe(false);
    } finally {
      await transport.close();
    }
  });

  it("cancels an unread response body when its sole consumer returns", async () => {
    const canceled = vi.fn();
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          new TextEncoder().encode(
            frame({ jsonrpc: "2.0", id: "1", result: {} }) +
              frame({ jsonrpc: "2.0", method: "notifications.run.event", params: {} }),
          ),
        );
      },
      cancel: canceled,
    });
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: vi.fn(
        async () => new Response(body, { headers: { "Content-Type": "text/event-stream" } }),
      ),
    });
    const events = transport.recv()[Symbol.asyncIterator]();
    try {
      await transport.send(req("1", "runs.start"));
      await events.next();
      await events.return?.();
      await vi.waitFor(() => expect(canceled).toHaveBeenCalledOnce());
      expect(body.locked).toBe(false);
    } finally {
      await transport.close();
    }
  });

  it("reports an abort-shaped body failure while the request owner remains live", async () => {
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: vi.fn(async () =>
        abortingSseResponse(frame({ jsonrpc: "2.0", id: "1", result: { runId: "run_01" } })),
      ),
    });
    const events = transport.recv()[Symbol.asyncIterator]();
    try {
      await transport.send(req("1", "runs.start"));
      await events.next();
      await expect(events.next()).resolves.toMatchObject({
        done: false,
        value: { type: "streamEnd", error: expect.any(RpcConnectionError) },
      });
    } finally {
      await transport.close();
    }
  });

  it("retains the primary stream failure when canceling its body also fails", async () => {
    const failure = new RpcConnectionError("response read failed");
    const cleanup = new Error("response cancellation failed");
    const response = sseResponse([]);
    const reader = response.body!.getReader();
    reader.releaseLock();
    vi.spyOn(response.body!, "getReader").mockReturnValue(reader);
    vi.spyOn(reader, "read").mockRejectedValue(failure);
    vi.spyOn(reader, "cancel").mockRejectedValue(cleanup);
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: vi.fn(async () => response),
    });
    const events = transport.recv()[Symbol.asyncIterator]();
    try {
      await transport.send(req("1", "runs.start"));
      const event = await events.next();
      expect(event.value).toMatchObject({ type: "requestError", error: failure });
      expect(failure.cause).toBe(cleanup);
    } finally {
      await transport.close();
    }
  });

  it.each([200, 400])("rejects invalid UTF-8 in an HTTP %d response", async (status) => {
    const prefix = new TextEncoder().encode('{"jsonrpc":"2.0","id":"1","result":{"text":"');
    const suffix = new TextEncoder().encode('"}}');
    const transport = createHttpTransport({
      baseUrl: "http://x",
      fetch: vi.fn(
        async () =>
          new Response(new Uint8Array([...prefix, 0xff, ...suffix]), {
            status,
            headers: { "Content-Type": "application/json", "Request-Id": "req_utf8" },
          }),
      ),
    });
    const pendingEvent = transport.recv()[Symbol.asyncIterator]().next();
    try {
      await expect(transport.send(req("1", "sessions.get"))).rejects.toMatchObject({
        name: "RpcProtocolError",
        requestId: "req_utf8",
      });
    } finally {
      await transport.close();
      await pendingEvent;
    }
  });

  it.each(["invalid byte", "truncated sequence"])(
    "rejects an SSE %s before delivering repaired content",
    async (kind) => {
      const prefix = new TextEncoder().encode('data: {"jsonrpc":"2.0","id":"1","result":"');
      const suffix = new TextEncoder().encode('"}\n\n');
      const bytes =
        kind === "invalid byte"
          ? new Uint8Array([...prefix, 0xff, ...suffix])
          : new Uint8Array([...prefix, 0xe2, 0x82]);
      const transport = createHttpTransport({
        baseUrl: "http://x",
        fetch: vi.fn(
          async () =>
            new Response(bytes, {
              headers: { "Content-Type": "text/event-stream", "Request-Id": "req_utf8" },
            }),
        ),
      });
      const events = transport.recv()[Symbol.asyncIterator]();
      try {
        await transport.send(req("1", "runs.start"));
        await expect(events.next()).resolves.toMatchObject({
          value: {
            type: "requestError",
            error: expect.any(RpcProtocolError),
          },
        });
      } finally {
        await transport.close();
      }
    },
  );
});

it("keeps an authenticated request on its exact configured target", async () => {
  const fetchStub = vi.fn<typeof fetch>(async (_input, options) => {
    expect(options?.redirect).toBe("error");
    throw new TypeError("redirect refused");
  });
  const transport = createHttpTransport({
    baseUrl: "https://runtime.example/prefix",
    localToken: "private-token",
    fetch: fetchStub,
  });
  await expect(transport.send(req("redirect", "sessions.get"))).rejects.toBeInstanceOf(
    RpcConnectionError,
  );
  expect(fetchStub).toHaveBeenCalledOnce();
  await transport.close();
});

describe("HTTP mutation acknowledgement", () => {
  it.each([
    ["no content", () => new Response(null, { status: 204 })],
    [
      "another request's response",
      () =>
        new Response(JSON.stringify({ jsonrpc: "2.0", id: "other", result: {} }), {
          headers: { "Content-Type": "application/json" },
        }),
    ],
  ])("keeps the command unresolved without replaying it after %s", async (_case, respond) => {
    const entries = new Map<string, unknown>();
    const fetchStub = vi.fn(async () => respond());
    const client = createFlameClient(
      createHttpTransport({ baseUrl: "http://x", fetch: fetchStub as unknown as typeof fetch }),
      {
        mutationJournal: createMutationJournal({
          storage: {
            get: (key) => structuredClone(entries.get(key)),
            set: (key, value) => void entries.set(key, structuredClone(value)),
            remove: (key) => void entries.delete(key),
            keys: () => [...entries.keys()],
          },
          scope: () => ({ namespace: "idp_store", retentionSeconds: 3600 }),
        }),
      },
    );

    await expect(client.sessions.delete(asSessionId("ses_1"))).rejects.toBeInstanceOf(
      RpcProtocolError,
    );
    expect(fetchStub).toHaveBeenCalledOnce();
    expect(entries.size).toBe(1);
    await client.close();
  });
});
