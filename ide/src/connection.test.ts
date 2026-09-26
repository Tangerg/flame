import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import discovery from "@flame/runtime-contract/samples/method.discover.resp.json";
import { HTTP_ENDPOINTS, PROTOCOL_VERSION } from "@flame/runtime-contract/wire";
import { Connection } from "./connection";
import { CommandStore } from "./commandStore";

const disposers: Array<() => Promise<void> | void> = [];
afterEach(async () => {
  for (const dispose of disposers.splice(0).reverse()) await dispose();
});

async function fixture(
  handle?: (
    request: IncomingMessage,
    response: ServerResponse,
    message: Record<string, unknown>,
  ) => boolean,
) {
  const requests: Array<{
    message: Record<string, unknown>;
    key?: string;
    namespace?: string;
    authorization?: string;
  }> = [];
  const directory = mkdtempSync(join(tmpdir(), "flame-ide-http-"));
  disposers.push(() => rmSync(directory, { recursive: true, force: true }));
  const server = createServer(async (request, response) => {
    if (request.url === HTTP_ENDPOINTS.info.path) {
      response.setHeader("Content-Type", "application/json");
      response.end(
        JSON.stringify({
          protocolVersion: PROTOCOL_VERSION,
          transport: "http",
          server: { name: "flame", version: "0.1.0", instanceId: discovery.serverInfo.instanceId },
          endpoints: {
            info: HTTP_ENDPOINTS.info.path,
            liveness: HTTP_ENDPOINTS.liveness.path,
            readiness: HTTP_ENDPOINTS.readiness.path,
            rpc: HTTP_ENDPOINTS.rpc.path,
          },
        }),
      );
      return;
    }
    let body = "";
    for await (const chunk of request) body += chunk;
    const message = JSON.parse(body) as Record<string, unknown>;
    requests.push({
      message,
      key: request.headers["idempotency-key"] as string | undefined,
      namespace: request.headers["idempotency-namespace"] as string | undefined,
      authorization: request.headers.authorization,
    });
    if (handle?.(request, response, message)) return;
    response.setHeader("Content-Type", "application/json");
    response.end(JSON.stringify({ jsonrpc: "2.0", id: message.id, result: discovery }));
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("missing fixture address");
  const endpoint = `http://127.0.0.1:${address.port}`;
  disposers.push(
    () =>
      new Promise<void>((resolve, reject) => {
        server.closeAllConnections();
        server.close((error) => (error ? reject(error) : resolve()));
      }),
  );
  return { endpoint, directory, requests };
}

describe("IDE connection lifetime and replay", () => {
  it("does not retain the construction timeout after a successful connection", async () => {
    const server = await fixture();
    const opening = new AbortController();
    const connection = await Connection.open(
      server.endpoint,
      "test-local-token",
      server.directory,
      opening.signal,
    );
    disposers.push(() => connection.close());
    opening.abort(new Error("construction deadline elapsed"));
    expect(connection.signal.aborted).toBe(false);
    await expect(connection.client.runtime.discover()).resolves.toMatchObject({
      protocolVersion: PROTOCOL_VERSION,
    });
    await connection.close();
    expect(connection.signal.aborted).toBe(true);
    expect(server.requests.map((request) => request.message.method)).toEqual([
      "runtime.discover",
      "runtime.discover",
    ]);
  });

  it("reopens an unknown start with the exact saved input, key and namespace", async () => {
    let recover = false;
    const server = await fixture((_request, response, message) => {
      if (message.method !== "runs.start") return false;
      if (!recover) {
        response.writeHead(503);
        response.end("temporary transport failure");
      } else {
        response.writeHead(200, { "Content-Type": "text/event-stream" });
        response.end(
          `data: ${JSON.stringify({ jsonrpc: "2.0", id: message.id, result: { runId: "run_1", segmentId: "seg_1", userItemId: "item_1" } })}\n\n`,
        );
      }
      return true;
    });
    const first = await Connection.open(server.endpoint, "test-local-token", server.directory);
    const params = {
      sessionId: "ses_1",
      input: [{ type: "text" as const, text: "unsaved version 17" }],
    };
    const started = first.execute({ kind: "start", params });
    params.input[0]!.text = "unsaved version 18";
    await expect(started).rejects.toThrow();
    const saved = first.pendingCommands()[0]!;
    await first.close();
    recover = true;
    const successor = await Connection.open(server.endpoint, "test-local-token", server.directory);
    disposers.push(() => successor.close());
    await expect(successor.retry(saved.id)).resolves.toEqual({
      sessionId: "ses_1",
      runId: "run_1",
    });
    const starts = server.requests.filter((request) => request.message.method === "runs.start");
    expect(starts).toHaveLength(3);
    for (const request of starts) {
      expect(request.key).toBe(saved.id);
      expect(request.namespace).toBe(discovery.capabilities.limits.idempotency.namespace);
      expect(request.authorization).toBe("Bearer test-local-token");
      expect(request.message.params).toMatchObject({
        input: [{ type: "text", text: "unsaved version 17" }],
      });
    }
    expect(successor.pendingCommands()).toEqual([]);
  });

  it("can settle one shared saved command from two extension hosts without losing another command", async () => {
    const server = await fixture((_request, response, message) => {
      if (message.method !== "sessions.create") return false;
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(
        JSON.stringify({
          jsonrpc: "2.0",
          id: message.id,
          result: {
            id: "ses_1",
            title: "Test",
            status: "idle",
            model: "test",
            provider: "test",
            revision: 1,
            createdAt: "2026-09-26T00:00:00Z",
            updatedAt: "2026-09-26T00:00:00Z",
            workspace: { ref: { path: "/runtime" }, availability: "available" },
          },
        }),
      );
      return true;
    });
    const store = new CommandStore(server.directory, server.endpoint);
    const scope = discovery.capabilities.limits.idempotency;
    const pending = store.prepare(scope.namespace, scope.retentionSeconds, {
      kind: "createSession",
      params: { workspace: { path: "/runtime" } },
    });
    const other = store.prepare(scope.namespace, scope.retentionSeconds, {
      kind: "createSession",
      params: { workspace: { path: "/other" } },
    });
    const first = await Connection.open(server.endpoint, undefined, server.directory);
    const second = await Connection.open(server.endpoint, undefined, server.directory);
    disposers.push(
      () => first.close(),
      () => second.close(),
    );
    await Promise.all([first.retry(pending.id), second.retry(pending.id)]);
    expect(store.list().map((command) => command.id)).toEqual([other.id]);
    const mutations = server.requests.filter(
      (request) => request.message.method === "sessions.create",
    );
    expect(mutations.every((request) => request.key === pending.id)).toBe(true);
  });
});
