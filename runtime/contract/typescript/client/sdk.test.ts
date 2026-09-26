import { createMutationJournal } from "./mutationJournal";
import { RpcProtocolError } from "./errors";
import { asSessionId } from "./ids";
import { PROBLEM_CODES } from "@flame/runtime-contract/wire";
import { describe, expect, it, vi } from "vitest";
import { createFlameClient } from "@flame/runtime-contract/client/sdk";
import { createMemoryTransport } from "@flame/runtime-contract/client/transports/memory";
import { waitForRequest } from "@flame/runtime-contract/client/transports/memory.testkit";
import { JSONRPC_VERSION, type RpcMessage } from "@flame/runtime-contract/client/types";
import { PROTOCOL_VERSION, type ServerCapabilities } from "@flame/runtime-contract/wire";
import discoverResponse from "@flame/runtime-contract/samples/method.discover.resp.json";

describe("createFlameClient", () => {
  it.each([
    { code: -32000, message: "garbled" },
    { code: -32000, message: "session_not_found", data: { type: "session_not_found" } },
    { code: -32603, message: "internal_error", data: { type: "session_not_found" } },
  ])(
    "keeps an unresolved command identity after a malformed acknowledgement: %j",
    async (error) => {
      const entries = new Map<string, unknown>();
      const journal = () =>
        createMutationJournal({
          storage: {
            get: (key) => structuredClone(entries.get(key)),
            set: (key, value) => {
              entries.set(key, structuredClone(value));
            },
            remove: (key) => {
              entries.delete(key);
            },
            keys: () => [...entries.keys()],
          },
          scope: () => ({ namespace: "idp_store", retentionSeconds: 3600 }),
        });
      const firstTransport = createMemoryTransport();
      const first = createFlameClient(firstTransport, { mutationJournal: journal() });
      const deletion = first.sessions.delete(asSessionId("ses_1"));
      const request = await waitForRequest(firstTransport, "sessions.delete");
      firstTransport.inject({ jsonrpc: "2.0", id: request.id, error });
      await expect(deletion).rejects.toBeInstanceOf(RpcProtocolError);
      expect(entries.size).toBe(1);
      await first.close();

      const successorTransport = createMemoryTransport();
      const successor = createFlameClient(successorTransport, { mutationJournal: journal() });
      const retried = successor.sessions.delete(asSessionId("ses_1"));
      expect(retried.idempotencyKey).toBe(deletion.idempotencyKey);
      const replay = await waitForRequest(successorTransport, "sessions.delete");
      successorTransport.inject({ jsonrpc: "2.0", id: replay.id, result: {} });
      await expect(retried).resolves.toBeUndefined();
      expect(entries.size).toBe(0);
      await successor.close();
    },
  );

  it("accepts only the code and message published for a typed Runtime problem", async () => {
    const transport = createMemoryTransport();
    const client = createFlameClient(transport);
    const result = client.sessions.get(asSessionId("missing"));
    const request = await waitForRequest(transport, "sessions.get");
    transport.inject({
      jsonrpc: "2.0",
      id: request.id,
      error: {
        code: PROBLEM_CODES.session_not_found,
        message: "session_not_found",
        data: { type: "session_not_found" },
      },
    });
    await expect(result).rejects.toMatchObject({
      name: "RpcError",
      data: { type: "session_not_found" },
    });
    await client.close();
  });

  it("disposes journal ownership before closing the transport", async () => {
    const transport = createMemoryTransport();
    const dispose = vi.fn();
    const client = createFlameClient(transport, {
      mutationJournal: { reserve: () => undefined, dispose },
    });

    await client.close();

    expect(dispose).toHaveBeenCalledOnce();
  });

  it("still closes the transport when journal ownership cleanup fails", async () => {
    const transport = createMemoryTransport();
    const closeTransport = vi.spyOn(transport, "close");
    const failure = new Error("journal cleanup failed");
    const client = createFlameClient(transport, {
      mutationJournal: {
        reserve: () => undefined,
        dispose: () => {
          throw failure;
        },
      },
    });

    await expect(client.close()).rejects.toBe(failure);
    expect(closeTransport).toHaveBeenCalledOnce();
  });

  it("preserves both journal and transport cleanup failures", async () => {
    const transport = createMemoryTransport();
    const journalFailure = new Error("journal cleanup failed");
    const transportFailure = new Error("transport cleanup failed");
    const closeMemoryTransport = transport.close.bind(transport);
    vi.spyOn(transport, "close").mockImplementation(async () => {
      await closeMemoryTransport();
      throw transportFailure;
    });
    const client = createFlameClient(transport, {
      mutationJournal: {
        reserve: () => undefined,
        dispose: () => {
          throw journalFailure;
        },
      },
    });

    const failure = await client.close().catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(AggregateError);
    expect((failure as AggregateError).errors).toEqual([journalFailure, transportFailure]);
  });

  it("shares one cleanup settlement across concurrent close callers", async () => {
    const transport = createMemoryTransport();
    const journalFailure = new Error("journal cleanup failed");
    const transportFailure = new Error("transport cleanup failed");
    let rejectTransport!: (error: unknown) => void;
    const closeMemoryTransport = transport.close.bind(transport);
    const closeTransport = vi.spyOn(transport, "close").mockImplementation(async () => {
      await closeMemoryTransport();
      await new Promise<void>((_resolve, reject) => {
        rejectTransport = reject;
      });
    });
    let disposed = false;
    const dispose = vi.fn(() => {
      if (disposed) return;
      disposed = true;
      throw journalFailure;
    });
    const client = createFlameClient(transport, {
      mutationJournal: { reserve: () => undefined, dispose },
    });

    const first = client.close();
    const second = client.close();
    expect(dispose).toHaveBeenCalledOnce();
    expect(closeTransport).toHaveBeenCalledOnce();
    await vi.waitFor(() => expect(rejectTransport).toBeTypeOf("function"));
    rejectTransport(transportFailure);

    const [firstResult, secondResult] = await Promise.allSettled([first, second]);
    if (firstResult.status !== "rejected" || secondResult.status !== "rejected") {
      throw new Error("concurrent close unexpectedly resolved");
    }
    expect(secondResult.reason).toBe(firstResult.reason);
    expect((firstResult.reason as AggregateError).errors).toEqual([
      journalFailure,
      transportFailure,
    ]);
  });

  it("attaches request metadata to typed calls", async () => {
    const transport = createMemoryTransport();
    const client = createFlameClient(transport, {
      requestMeta: () => ({
        protocolVersion: PROTOCOL_VERSION,
        clientInfo: { name: "test", version: "0" },
        clientCapabilities: {
          events: [],
          features: {},
          interruptTypes: ["approval"],
        },
      }),
    });

    expect("rpc" in client).toBe(false);

    const promise = client.runtime.discover();
    const req = await waitForRequest(transport, "runtime.discover");

    expect(req.params).toMatchObject({
      _meta: {
        protocolVersion: PROTOCOL_VERSION,
        clientCapabilities: { interruptTypes: ["approval"] },
      },
    });

    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      result: discoverResponse,
    } as RpcMessage);
    await promise;
    await client.close();
  });

  it("preflights and emits the same request metadata snapshot", async () => {
    const transport = createMemoryTransport();
    let reads = 0;
    const capabilities = {
      runEvents: [],
      runtimeTopics: [],
      streamingMethods: [],
      features: {
        subagents: {
          enabled: true,
          clientOptIn: true,
          requiredByRunProtocol: true,
        },
      },
      limits: {
        idempotency: {
          namespace: "idp_fedcba9876543210fedcba9876543210",
          retentionSeconds: 86_400,
        },
        runReplay: {
          scope: "runtimeInstanceRootSegment",
          maxEvents: 1,
          maxBytes: 1,
        },
        mcpAuthorizationAttempts: { retentionSeconds: 600 },
        runtimeSubscription: {
          maxTopics: 1,
          maxWatches: 1,
          maxPaths: 256,
          maxDirectoryEntries: 10000,
          maxFileBytes: 1048576,
        },
      },
    } satisfies ServerCapabilities;
    const client = createFlameClient(transport, {
      capabilities: () => capabilities,
      requestMeta: () => {
        reads += 1;
        return {
          protocolVersion: PROTOCOL_VERSION,
          clientInfo: { name: "test", version: "0" },
          clientCapabilities: {
            features: { subagents: { enabled: reads === 1 } },
          },
        };
      },
    });

    const promise = client.runs.list({ includeDescendants: true });
    const req = await waitForRequest(transport, "runs.list");
    expect(reads).toBe(1);
    expect(req.params).toMatchObject({
      _meta: {
        clientCapabilities: {
          features: { subagents: { enabled: true } },
        },
      },
    });

    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      result: { data: [] },
    } as RpcMessage);
    await promise;
    await client.close();
  });
});
