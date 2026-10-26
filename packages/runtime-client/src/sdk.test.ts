import { type MutationJournalStorage } from "./mutationJournal";
import { RpcProtocolError } from "./errors";
import { asSessionId } from "./ids";
import { PROBLEM_CODES } from "@flame/runtime-client/wire";
import { describe, expect, it, vi } from "vitest";
import { createFlameClient } from "./sdk";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import { waitForRequest } from "@flame/runtime-client/testing";
import { JSONRPC_VERSION, type RpcMessage } from "./types";
import { PROTOCOL_VERSION, type ServerCapabilities } from "@flame/runtime-client/wire";
import discoverResponse from "@flame/runtime-client/testing/samples/method.discover.resp.json";

describe("createFlameClient", () => {
  it.each([
    { code: -32000, message: "garbled" },
    { code: -32000, message: "session_not_found", data: { type: "session_not_found" } },
    { code: -32603, message: "internal_error", data: { type: "session_not_found" } },
  ])(
    "keeps an unresolved command identity after a malformed acknowledgement: %j",
    async (error) => {
      const entries = new Map<string, unknown>();
      const mutationStorage: MutationJournalStorage = {
        get: (key) => structuredClone(entries.get(key)),
        set: (key, value) => {
          entries.set(key, structuredClone(value));
        },
        remove: (key) => {
          entries.delete(key);
        },
        keys: () => [...entries.keys()],
      };
      const capabilities = () => discoverResponse.capabilities as ServerCapabilities;
      const firstTransport = createMemoryTransport();
      const first = createFlameClient(firstTransport, { mutationStorage, capabilities });
      const deletion = first.sessions.delete(asSessionId("ses_1"));
      const request = await waitForRequest(firstTransport, "sessions.delete");
      firstTransport.inject({ jsonrpc: "2.0", id: request.id, error });
      await expect(deletion).rejects.toBeInstanceOf(RpcProtocolError);
      expect(entries.size).toBe(1);
      await first.close();

      const successorTransport = createMemoryTransport();
      const successor = createFlameClient(successorTransport, { mutationStorage, capabilities });
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

  it("retires mutation authority before closing the transport", async () => {
    const transport = createMemoryTransport();
    const closeTransport = vi.spyOn(transport, "close");
    const client = createFlameClient(transport, {
      capabilities: () => discoverResponse.capabilities as ServerCapabilities,
    });
    await client.close();
    await expect(client.sessions.delete(asSessionId("ses_closed"))).rejects.toThrow(
      "Runtime mutation client was closed",
    );
    expect(transport.outbox()).toEqual([]);
    expect(closeTransport).toHaveBeenCalledOnce();
  });

  it("shares one cleanup failure across concurrent close callers", async () => {
    const transport = createMemoryTransport();
    const failure = new Error("transport cleanup failed");
    const settlement = Promise.withResolvers<void>();
    const closeMemoryTransport = transport.close.bind(transport);
    const closeTransport = vi.spyOn(transport, "close").mockImplementation(async () => {
      await closeMemoryTransport();
      await settlement.promise;
    });
    const client = createFlameClient(transport);
    const first = client.close();
    const second = client.close();
    expect(second).toBe(first);
    expect(closeTransport).toHaveBeenCalledOnce();
    settlement.reject(failure);
    const results = await Promise.allSettled([first, second]);
    expect(results).toEqual([
      { status: "rejected", reason: failure },
      { status: "rejected", reason: failure },
    ]);
  });

  it("attaches request metadata to typed calls", async () => {
    const transport = createMemoryTransport();
    const client = createFlameClient(transport, {
      requestMeta: () => ({
        protocolVersion: PROTOCOL_VERSION,
        clientInfo: { name: "test", version: "0" },
        clientCapabilities: {
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
