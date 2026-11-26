import { parseMcpImport } from "../../../../../plugins/mcp/component/import";
import { candidate as mcpCandidate } from "../../../../../plugins/mcp/component/model";
import { createServer as createHttpServer } from "node:http";
import { describe, expect, it } from "vitest";
import { RpcError, asSessionId } from "@flame/runtime-client";
import { liveMCPToolName } from "./fakeProvider";
import {
  collectRunEvents,
  nextUserMCPChange,
  originalOpeningEvents,
  waitForUserMCPConnection,
  watchRuntimeTopic,
} from "./observation";
import { useRuntimeFixture } from "./fixture";

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: MCP", () => {
  const fixture = useRuntimeFixture("mcp");

  it("reconciles disabled MCP configuration through mcp.changed", async () => {
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["mcp.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();
    const server = "http-e2e-disabled";
    const serverId = { origin: { type: "user" as const }, name: server };

    await expect(fixture.client.mcp.create({
      connection: { type: "stdio", command: "runtime-http-e2e-mcp" },
      description: "Disabled MCP E2E fixture",
      enabled: false,
      handshakeTimeout: { type: "unbounded" },
      name: server,
    })).resolves.toBeUndefined();
    await expect(nextUserMCPChange(runtimeEvents, server)).resolves.toMatchObject({
      servers: expect.arrayContaining([serverId]),
    });
    expect(
      (await fixture.client.mcp.list()).data.filter((entry) => entry.id.origin.type === "user"),
    ).toEqual([expect.objectContaining({ id: serverId, status: { type: "disabled" } })]);

    const updated = fixture.client.mcp.update({
      server: serverId,
      description: "Updated disabled MCP E2E fixture",
    });
    await expect(updated).resolves.toBeUndefined();
    await expect(nextUserMCPChange(runtimeEvents, server)).resolves.toMatchObject({
      servers: expect.arrayContaining([serverId]),
    });

    const latestDescription = "Newer disabled MCP E2E fixture";
    await fixture.client.mcp.update({ server: serverId, description: latestDescription });
    await expect(nextUserMCPChange(runtimeEvents, server)).resolves.toMatchObject({
      servers: expect.arrayContaining([serverId]),
    });
    await expect(updated.retry()).resolves.toBeUndefined();
    expect(
      (await fixture.client.mcp.list()).data.find((entry) => entry.id.name === server),
    ).toMatchObject({ id: serverId, description: latestDescription, status: { type: "disabled" } });

    await fixture.client.mcp.delete(serverId);
    await expect(nextUserMCPChange(runtimeEvents, server)).resolves.toMatchObject({
      servers: expect.arrayContaining([serverId]),
    });
    expect(
      (await fixture.client.mcp.list()).data.filter((entry) => entry.id.origin.type === "user"),
    ).toEqual([]);

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("handshakes imported MCP servers and reads authorization from the owner", async () => {
    const requests: string[] = [];
    const authorization: (string | undefined)[] = [];
    const headers: (string | string[] | undefined)[] = [];
    const remote = createHttpServer(async (request, response) => {
      if (request.method !== "POST") {
        response.writeHead(405).end();
        return;
      }
      const chunks: Buffer[] = [];
      for await (const chunk of request) chunks.push(Buffer.from(chunk));
      const message = JSON.parse(Buffer.concat(chunks).toString()) as {
        method: string;
        id?: string | number;
        params: { protocolVersion: string };
      };
      requests.push(message.method);
      authorization.push(request.headers.authorization);
      headers.push(request.headers["x-import"]);
      if (message.id === undefined) {
        response.writeHead(202).end();
        return;
      }
      if (message.method !== "initialize" && message.method !== "tools/list") {
        response.writeHead(400).end();
        return;
      }
      const result =
        message.method === "initialize"
          ? {
              protocolVersion: message.params.protocolVersion,
              capabilities: { tools: {} },
              serverInfo: { name: "import-fixture", version: "1" },
            }
          : { tools: [] };
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ jsonrpc: "2.0", id: message.id, result }));
    });
    await new Promise<void>((resolve) => remote.listen(0, "127.0.0.1", resolve));
    const address = remote.address();
    if (address === null || typeof address === "string") throw new Error("missing fixture address");
    try {
      const imported = parseMcpImport(
        JSON.stringify({
          mcpServers: {
            "import-stdio": {
              type: "stdio",
              command: process.execPath,
              args: [fixture.mcpFixturePath],
            },
            "import-http": {
              type: "streamableHttp",
              url: `http://127.0.0.1:${address.port}/mcp`,
              headers: { AuThOrIzAtIoN: "Bearer import-fixture", "X-Import": "exact" },
            },
          },
        }),
      );
      for (const input of imported) {
        await expect(fixture.client!.mcp.test(mcpCandidate(input))).resolves.toEqual({
          outcome: "reachable",
        });
      }
      expect(requests).toContain("initialize");
      expect(requests).toContain("tools/list");
      expect(new Set(authorization)).toEqual(new Set(["Bearer import-fixture"]));
      expect(new Set(headers)).toEqual(new Set(["exact"]));

      const withStaticAuth = imported.map(mcpCandidate).find((input) => input.name === "import-http");
      if (!withStaticAuth) throw new Error("missing imported Streamable HTTP server");
      const staticServer = { origin: { type: "user" as const }, name: withStaticAuth.name };
      await fixture.client.mcp.create(withStaticAuth);
      const staticStatus = async () =>
        (await fixture.client.mcp.list()).data.find((entry) => entry.id.name === staticServer.name);
      await expect.poll(staticStatus).toMatchObject({ status: { type: "connected" } });
      await expect(fixture.client.mcp.authorizationAttempts.create(staticServer)).rejects.toSatisfy(
        (error: unknown) => error instanceof RpcError && error.data.type === "invalid_params",
      );
      await expect(staticStatus()).resolves.toMatchObject({ status: { type: "connected" } });

      const serverId = { origin: { type: "user" as const }, name: "http-e2e-authorization" };
      await fixture.client.mcp.create({
        name: serverId.name,
        enabled: true,
        connection: { type: "streamableHttp", url: `http://127.0.0.1:${address.port}/mcp` },
        handshakeTimeout: { type: "unbounded" },
      });
      const start = fixture.client.mcp.authorizationAttempts.create(serverId);
      const reference = await start;
      expect(reference).toEqual({ attemptId: expect.any(String) });
      const read = () => fixture.client.mcp.authorizationAttempts.get(reference.attemptId);
      await expect.poll(read, { timeout: 5_000 }).toMatchObject({
        id: reference.attemptId, server: serverId, status: { type: "succeeded" },
      });
      const settled = await read();
      await expect(start.retry()).resolves.toEqual(reference);
      await expect(read()).resolves.toEqual(settled);
    } finally {
      remote.closeAllConnections();
      await new Promise<void>((resolve, reject) =>
        remote.close((error) => (error ? reject(error) : resolve())),
      );
    }
  }, 30_000);

  it("probes, connects, lists and reconnects a real stdio MCP server", async () => {
    const server = "http-e2e-stdio";
    const serverId = { origin: { type: "user" as const }, name: server };
    const candidate = {
      connection: {
        type: "stdio" as const,
        command: process.execPath,
        args: [fixture.mcpFixturePath],
      },
      description: "Live MCP E2E fixture",
      enabled: true,
      handshakeTimeout: { type: "unbounded" as const },
      name: server,
    };

    await expect(fixture.client.mcp.test(candidate)).resolves.toEqual({ outcome: "reachable" });

    const connection = await watchRuntimeTopic(fixture.client, "mcp.changed");
    await expect(fixture.client.mcp.create(candidate)).resolves.toBeUndefined();

    await expect(
      waitForUserMCPConnection(fixture.client, connection.events, server),
    ).resolves.toMatchObject({
      id: serverId,
      status: { type: "connected", toolCount: 1 },
    });
    await connection.close();

    await fixture.client.approval.setRule({
      tool: { type: "mcp", server: serverId, name: "ping" },
      scope: "global",
      subject: { type: "all" },
      decision: "allow",
    });
    await expect(fixture.client.approval.listRules()).resolves.toMatchObject({
      rules: expect.arrayContaining([
        expect.objectContaining({
          tool: { type: "mcp", server: serverId, name: "ping" },
          modelName: `${server}_ping`,
          scope: "global",
          subject: { type: "all" },
          decision: "allow",
          stale: false,
        }),
      ]),
    });
    await fixture.client.mcp.setToolExposure({ server: serverId, name: "ping", disabled: true });
    await expect(fixture.client.mcp.toolExposure(serverId)).resolves.toEqual({
      server: serverId,
      disabledTools: ["ping"],
    });
    await fixture.client.mcp.setToolExposure({ server: serverId, name: "ping", disabled: false });
    await expect(fixture.client.mcp.toolExposure(serverId)).resolves.toEqual({
      server: serverId,
      disabledTools: [],
    });

    const allTools = await fixture.client.mcp.listTools();
    expect(allTools.data.filter((entry) => entry.server.origin.type === "user")).toMatchObject([
      {
        server: serverId,
        name: "ping",
        description: "Returns pong for Runtime HTTP E2E.",
        inputSchema: { type: "object", properties: {}, additionalProperties: false },
      },
    ]);
    await expect(fixture.client.mcp.listTools(serverId)).resolves.toMatchObject({
      data: [expect.objectContaining({ server: serverId, name: "ping" })],
    });

    const runSession = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP live MCP tool",
    });
    const mcpRun = await fixture.client.runs.start({
      sessionId: asSessionId(runSession.id),
      input: [{ type: "text", text: "E2E_MCP_TOOL invoke the live ping capability." }],
    });
    const mcpRunEvents = await collectRunEvents(originalOpeningEvents(mcpRun));
    expect(mcpRunEvents).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          event: expect.objectContaining({
            type: "item.completed",
            item: expect.objectContaining({
              type: "toolCall",
              tool: expect.objectContaining({ name: liveMCPToolName }),
            }),
          }),
        }),
      ]),
    );
    await expect(
      fixture.client.items.list({ scope: { type: "run", runId: mcpRun.result.runId } }),
    ).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({
          type: "agentMessage",
          content: [{ type: "text", text: "MCP pong observed." }],
        }),
      ]),
    });

    await expect(fixture.client.mcp.authorizationAttempts.create(serverId)).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && error.data.type === "invalid_params",
    );
    await expect(
      fixture.client.mcp.authorizationAttempts.get("mcpauth_ABCDEFGHIJKLMNOPQRSTUVWXYZ"),
    ).rejects.toSatisfy(
      (error: unknown) =>
        error instanceof RpcError && error.data.type === "mcp_authorization_attempt_not_found",
    );

    const reconnection = await watchRuntimeTopic(fixture.client, "mcp.changed");
    await fixture.client.mcp.reconnect(serverId);
    await waitForUserMCPConnection(fixture.client, reconnection.events, server);
    await reconnection.close();
    await expect(fixture.client.mcp.listTools(serverId)).resolves.toMatchObject({
      data: [expect.objectContaining({ server: serverId, name: "ping" })],
    });

    const deletion = await watchRuntimeTopic(fixture.client, "mcp.changed");
    await fixture.client.mcp.delete(serverId);
    await expect(nextUserMCPChange(deletion.events, server)).resolves.toMatchObject({
      servers: expect.arrayContaining([serverId]),
    });
    await expect(fixture.client.mcp.listTools(serverId)).resolves.toMatchObject({ data: [] });
  }, 30_000);
});
