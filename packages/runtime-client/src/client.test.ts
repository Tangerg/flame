import { once } from "node:events";
import { setImmediate } from "node:timers/promises";
import { describe, expect, it, onTestFinished, vi } from "vitest";
import { createRpcClient } from "./client";
import { RpcError, RpcProtocolError, RpcTransportError } from "./errors";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import { waitForRequest } from "@flame/runtime-client/testing";
import type { Transport } from "./transport";
import type { RpcMessage } from "./types";
import { JSONRPC_VERSION } from "./types";
import { PROBLEM_CODES } from "@flame/runtime-client/wire";
import session from "@flame/runtime-client/testing/samples/session.json";
import segmentFinished from "@flame/runtime-client/testing/samples/segment.finished.json";
import { MAXIMUM_RUN_EVENT_ID_CHARACTERS, RUN_EVENT_ID_PREFIX } from "@flame/runtime-client/wire";

describe("RpcClient", () => {
  it("does not dispatch after its request observer closes the client", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    let closing: Promise<void> | undefined;

    await expect(
      client.call(
        "runtime.discover",
        {},
        {
          onRequestRpcId: () => {
            closing = client.close();
          },
        },
      ),
    ).rejects.toThrow("client closed");

    expect(transport.outbox()).toEqual([]);
    await closing;
  });

  it("releases cancellation ownership when transport admission throws synchronously", async () => {
    const memory = createMemoryTransport();
    const failure = new Error("admission failed");
    const client = createRpcClient({
      ...memory,
      send: () => {
        throw failure;
      },
    });
    const controller = new AbortController();
    const detach = vi.spyOn(controller.signal, "removeEventListener");

    await expect(client.call("runtime.discover", {}, { signal: controller.signal })).rejects.toBe(
      failure,
    );

    expect(detach).toHaveBeenCalledWith("abort", expect.any(Function));
    await client.close();
  });

  it("rejects method-incompatible transport metadata before send", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);

    await expect(
      client.call("runtime.discover", {}, { idempotencyKey: "query-key" }),
    ).rejects.toThrow("runtime.discover does not accept an idempotency key");
    await expect(
      client.call("runtime.discover", {}, { idempotencyNamespace: "idp_store" }),
    ).rejects.toThrow("An idempotency namespace requires an idempotency key");
    await expect(
      client.call(
        "runtime.subscribe",
        { topics: ["sessions.changed"] },
        { lastEventId: "evt_cursor" },
      ),
    ).rejects.toThrow("runtime.subscribe does not accept a run replay cursor");
    await expect(
      client.call(
        "runs.start",
        { sessionId: "ses_01", input: [{ type: "text", text: "hello" }] },
        { lastEventId: "evt_cursor" },
      ),
    ).rejects.toThrow("A run command replay cursor requires an idempotency key");

    expect(transport.outbox()).toHaveLength(0);
    await client.close();
  });

  it("rejects malformed or oversized run replay cursors before send", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);

    await expect(
      client.call(
        "runs.subscribe",
        { runId: "run_01", segmentId: "seg_01" },
        { lastEventId: "opaque" },
      ),
    ).rejects.toThrow("Run replay cursor has invalid event-id framing");
    await expect(
      client.call(
        "runs.subscribe",
        { runId: "run_01", segmentId: "seg_01" },
        {
          lastEventId: RUN_EVENT_ID_PREFIX + "x".repeat(MAXIMUM_RUN_EVENT_ID_CHARACTERS),
        },
      ),
    ).rejects.toThrow(`Run replay cursor exceeds ${MAXIMUM_RUN_EVENT_ID_CHARACTERS} characters`);

    expect(transport.outbox()).toHaveLength(0);
    await client.close();
  });

  it("sends a registered Request and resolves a validated result", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);

    const promise = client.call("sessions.get", { sessionId: "ses_01" });
    const request = await waitForRequest(transport, "sessions.get");
    expect(request.jsonrpc).toBe(JSONRPC_VERSION);

    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: request.id,
      result: session,
    } as RpcMessage);

    await expect(promise).resolves.toEqual(session);
    await client.close();
  });

  it("rejects a malformed method result at the generated contract boundary", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);

    const promise = client.call("sessions.get", { sessionId: "ses_01" });
    const request = await waitForRequest(transport, "sessions.get");
    const { revision: _revision, ...malformed } = session;
    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: request.id,
      result: malformed,
    } as RpcMessage);

    await expect(promise).rejects.toMatchObject({
      name: "RpcProtocolError",
      violations: [{ path: "sessions.get.result.revision", detail: "is required" }],
    } satisfies Partial<RpcProtocolError>);
    await client.close();
  });

  it("rejects a response carried by another request without settling that request", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);

    const sessionsPromise = client.call("sessions.get", {
      sessionId: "ses_01",
    });
    const runsPromise = client.call("runs.list", {});
    const sessionsRequest = await waitForRequest(transport, "sessions.get");
    const runsRequest = await waitForRequest(transport, "runs.list");

    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        id: runsRequest.id,
        result: { data: [] },
      } as RpcMessage,
      { requestId: "req_mismatch_01" },
      sessionsRequest.id,
    );
    await expect(sessionsPromise).rejects.toMatchObject({
      name: "RpcProtocolError",
      requestId: "req_mismatch_01",
      violations: [
        {
          path: "sessions.get.response.id",
          detail: `must match request id ${sessionsRequest.id}`,
        },
      ],
    } satisfies Partial<RpcProtocolError>);

    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: runsRequest.id,
      result: { data: [] },
    } as RpcMessage);
    await expect(runsPromise).resolves.toEqual({ data: [] });
    await client.close();
  });

  it("acknowledges consumption of a rejected response without waiting for cancellation", async () => {
    const memory = createMemoryTransport();
    const consumed = vi.fn<(delivery: Promise<void>) => void>();
    const client = createRpcClient({
      ...memory,
      async *recv() {
        for await (const event of memory.recv()) {
          yield event.type === "message" ? { ...event, consumed } : event;
        }
      },
    });
    try {
      const response = client.call("sessions.get", { sessionId: "ses_01" });
      const request = await waitForRequest(memory, "sessions.get");
      memory.inject(
        { jsonrpc: JSONRPC_VERSION, id: `${request.id}-other`, result: session },
        undefined,
        request.id,
      );
      await expect(response).rejects.toBeInstanceOf(RpcProtocolError);
      expect(consumed).toHaveBeenCalledOnce();
      await expect(consumed.mock.calls[0]![0]).resolves.toBeUndefined();
    } finally {
      await client.close();
    }
  });

  it("reports synchronous dispatch failure through transport consumption", async () => {
    const memory = createMemoryTransport();
    const consumed = vi.fn((delivery: Promise<void>) => {
      void delivery.catch(() => undefined);
    });
    const client = createRpcClient({
      ...memory,
      async *recv() {
        for await (const event of memory.recv()) {
          yield event.type === "message" ? { ...event, consumed } : event;
        }
      },
    });
    const failure = new Error("notification observer failed");
    const closed = once(client.closedSignal, "abort");
    client.subscribe("notifications.runtime.event", {
      next: vi.fn(),
      error: () => {
        throw failure;
      },
    });
    try {
      memory.inject(
        { jsonrpc: JSONRPC_VERSION, method: "notifications.runtime.event", params: {} },
        undefined,
        "subscription",
      );
      await closed;
      expect(consumed).toHaveBeenCalledOnce();
      await expect(consumed.mock.calls[0]![0]).rejects.toBe(failure);
    } finally {
      await expect(client.close()).rejects.toBe(failure);
    }
  });

  it("validates an ack result and exposes it as void", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);

    const promise = client.call("sessions.delete", { sessionId: "ses_01" });
    const request = await waitForRequest(transport, "sessions.delete");
    transport.inject({ jsonrpc: JSONRPC_VERSION, id: request.id, result: {} });

    await expect(promise).resolves.toBeUndefined();
    await client.close();
  });

  it("rejects with RpcError on a valid error response", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);

    const promise = client.call("sessions.get", { sessionId: "missing" });
    const request = await waitForRequest(transport, "sessions.get");
    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        id: request.id,
        error: {
          code: PROBLEM_CODES.session_not_found,
          message: "session_not_found",
          data: { type: "session_not_found" },
        },
      },
      { requestId: "req_business_01" },
    );

    await expect(promise).rejects.toMatchObject({
      name: "RpcError",
      code: PROBLEM_CODES.session_not_found,
      requestId: "req_business_01",
    } satisfies Partial<RpcError>);
    await client.close();
  });

  it("terminates subscribers when notification params violate the contract", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    onTestFinished(() => client.close());
    const next = vi.fn();
    const failed = Promise.withResolvers<unknown[]>();
    client.subscribe("notifications.runtime.event", {
      next,
      error: (...args) => failed.resolve(args),
    });

    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: "notifications.runtime.event",
        params: { event: { type: "skills.changed", sequence: 0 } },
      },
      undefined,
      "rpc_runtime",
    );
    await expect(failed.promise).resolves.toEqual([expect.any(RpcProtocolError), "rpc_runtime"]);
    expect(next).not.toHaveBeenCalled();
  });

  it("drops a malformed ephemeral run event without terminating its stream", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    onTestFinished(() => client.close());
    const received = Promise.withResolvers<unknown>();
    const next = vi.fn((params) => received.resolve(params));
    const error = vi.fn();
    client.subscribe("notifications.run.event", { next, error });

    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: "notifications.run.event",
        params: {
          runId: "run_01",
          segmentId: "seg_01",
          eventId: "evt_01",
          timestamp: "2026-08-02T00:00:00Z",
          event: { type: "item.delta" },
        },
      },
      undefined,
      "rpc_run",
    );
    transport.inject(
      { jsonrpc: JSONRPC_VERSION, method: "notifications.run.event", params: segmentFinished },
      undefined,
      "rpc_run",
    );
    await expect(received.promise).resolves.toMatchObject({ event: { type: "segment.finished" } });
    expect(next).toHaveBeenCalledOnce();
    expect(error).not.toHaveBeenCalled();
  });

  it("terminates a run stream when an authoritative event is malformed", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    onTestFinished(() => client.close());
    const next = vi.fn();
    const failed = Promise.withResolvers<unknown[]>();
    client.subscribe("notifications.run.event", {
      next,
      error: (...args) => failed.resolve(args),
    });

    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: "notifications.run.event",
        params: {
          runId: "run_01",
          segmentId: "seg_01",
          eventId: "evt_01",
          timestamp: "2026-08-02T00:00:00Z",
          event: { type: "segment.finished" },
        },
      },
      undefined,
      "rpc_run",
    );
    await expect(failed.promise).resolves.toEqual([expect.any(RpcProtocolError), "rpc_run"]);
    expect(next).not.toHaveBeenCalled();
  });

  it("close rejects pending calls and prevents further use", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    const promise = client.call("sessions.get", { sessionId: "ses_01" });
    await waitForRequest(transport, "sessions.get");

    await client.close();
    await expect(promise).rejects.toBeInstanceOf(RpcTransportError);
    await expect(client.call("sessions.get", { sessionId: "ses_01" })).rejects.toBeInstanceOf(
      RpcTransportError,
    );
    expect(() =>
      client.subscribe("notifications.runtime.event", { next: vi.fn(), error: vi.fn() }),
    ).toThrow(RpcTransportError);
    expect(() => client.onStreamEnd(vi.fn())).toThrow(RpcTransportError);
  });

  it.each([false, true])(
    "joins teardown and notifies every observer when an error observer throws (transport fails=%s)",
    async (transportFails) => {
      const inner = createMemoryTransport();
      const observerFailure = new Error("terminal observer failed");
      const transportFailure = new Error("transport close failed");
      const close = vi.fn(async () => {
        await inner.close();
        if (transportFails) throw transportFailure;
      });
      const client = createRpcClient({ ...inner, close });
      const first = vi.fn().mockImplementationOnce(() => {
        throw observerFailure;
      });
      const second = vi.fn();
      client.subscribe("notifications.runtime.event", { next: vi.fn(), error: first });
      client.subscribe("notifications.runtime.event", { next: vi.fn(), error: second });
      try {
        const closing = client.close();
        const failure: unknown = await closing.catch((cause: unknown) => cause);
        const causes = failure instanceof AggregateError ? failure.errors : [failure];
        expect(causes).toEqual(
          transportFails ? [transportFailure, observerFailure] : [observerFailure],
        );
        expect(close).toHaveBeenCalledOnce();
        expect(first).toHaveBeenCalledOnce();
        expect(second).toHaveBeenCalledOnce();
        expect(client.close()).toBe(closing);
      } finally {
        await inner.close();
      }
    },
  );

  it("reports a thrown undefined teardown failure", async () => {
    const inner = createMemoryTransport();
    const client = createRpcClient({
      ...inner,
      close: async () => {
        await inner.close();
        throw undefined;
      },
    });
    await expect(client.close()).rejects.toBeUndefined();
  });

  it("publishes one close operation before terminal callbacks can reenter", async () => {
    const inner = createMemoryTransport();
    const close = vi.fn(() => inner.close());
    const client = createRpcClient({ ...inner, close });
    let reentered: Promise<void> | undefined;
    client.subscribe("notifications.runtime.event", {
      next: vi.fn(),
      error: () => {
        reentered = client.close();
      },
    });
    const closing = client.close();
    await closing;
    expect(reentered).toBe(closing);
    expect(close).toHaveBeenCalledOnce();
  });

  it("keeps concurrent close callers joined to the same transport teardown", async () => {
    const inner = createMemoryTransport();
    let release!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const close = vi.fn(async () => {
      await held;
      await inner.close();
    });
    const client = createRpcClient({ ...inner, close });

    const first = client.close();
    let secondSettled = false;
    const second = client.close().then(() => {
      secondSettled = true;
    });
    await Promise.resolve();

    expect(close).toHaveBeenCalledOnce();
    expect(secondSettled).toBe(false);

    release();
    await Promise.all([first, second]);
  });

  it("joins the receive pump after transport teardown closes its source", async () => {
    const inner = createMemoryTransport();
    const unwinding = Promise.withResolvers<void>();
    const release = Promise.withResolvers<void>();
    let receiveFinished = false;
    const transport: Transport = {
      ...inner,
      recv: () =>
        (async function* () {
          try {
            yield* inner.recv();
          } finally {
            unwinding.resolve();
            await release.promise;
            receiveFinished = true;
          }
        })(),
    };
    const client = createRpcClient(transport);

    let closeSettled = false;
    const closing = client.close().then(() => {
      closeSettled = true;
    });
    try {
      await unwinding.promise;
      await setImmediate();
      expect(closeSettled).toBe(false);
    } finally {
      release.resolve();
      await closing;
    }
    expect(receiveFinished).toBe(true);
  });

  it("still tears down the transport after its receive stream ended", async () => {
    let endReceive!: () => void;
    const receiveEnded = new Promise<void>((resolve) => {
      endReceive = resolve;
    });
    const close = vi.fn(async () => undefined);
    const transport: Transport = {
      send: vi.fn(async () => undefined),
      recv: () =>
        (async function* () {
          await receiveEnded;
          yield* [];
        })(),
      close,
    };
    const client = createRpcClient(transport);
    const closed = once(client.closedSignal, "abort");

    endReceive();
    await closed;
    await client.close();

    expect(close).toHaveBeenCalledOnce();
  });

  it("rejects in-flight calls when the transport stream ends cleanly", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    const promise = client.call("sessions.get", { sessionId: "ses_01" });
    await waitForRequest(transport, "sessions.get");

    await transport.close();
    await expect(promise).rejects.toBeInstanceOf(RpcTransportError);
    await expect(client.call("sessions.get", { sessionId: "ses_01" })).rejects.toBeInstanceOf(
      RpcTransportError,
    );
  });

  it("AbortSignal cancels an in-flight call without a second protocol message", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    const controller = new AbortController();
    const promise = client.call(
      "runs.start",
      { sessionId: "ses_01", input: [{ type: "text", text: "hello" }] },
      { signal: controller.signal },
    );
    await waitForRequest(transport, "runs.start");
    controller.abort();

    await expect(promise).rejects.toBeInstanceOf(RpcTransportError);
    expect(transport.outbox()).toHaveLength(1);
    await client.close();
  });

  it("ignores unknown server notifications without poisoning correlation", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: "notifications.unknown",
        params: {},
      },
      undefined,
      "rpc_unknown",
    );
    await Promise.resolve();

    const promise = client.call("sessions.get", { sessionId: "ses_01" });
    const request = await waitForRequest(transport, "sessions.get");
    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: request.id,
      result: session,
    } as RpcMessage);
    await expect(promise).resolves.toEqual(session);
    await client.close();
  });
});
