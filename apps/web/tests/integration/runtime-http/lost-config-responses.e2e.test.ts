import {
  addInstalledMemory,
  captureMemoryBinding,
  deleteInstalledMemory,
  readInstalledMemory,
  updateInstalledMemory,
} from "./memoryOperations";
import { mkdir } from "node:fs/promises";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { createFlameClient, type FlameClient, createHttpTransport } from "@flame/runtime-client";
import { PROTOCOL_VERSION, type RequestMeta } from "@flame/runtime-client/wire";
import { nextRuntimeEvent, nextUserMCPChange } from "./observation";
import { managedSkillName, useRuntimeFixture } from "./fixture";
import { faultUnaryResponse } from "./transportFaults";

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: configuration commits after lost responses", () => {
  const fixture = useRuntimeFixture("lost-config-responses");

  it("reconciles provider and model-role commits after their responses are lost", async () => {
    const requestMeta = (): RequestMeta => ({
      protocolVersion: PROTOCOL_VERSION,
      clientInfo: { name: "runtime-http-model-config-cutpoint-e2e", version: "1" },
      clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
    });
    const callAfterCommitLoss = async <T>(
      method: string,
      call: (faultClient: FlameClient) => Promise<T>,
    ): Promise<T> => {
      const fault = faultUnaryResponse(method, "afterCommit");
      const faultClient = createFlameClient(
        createHttpTransport({ baseUrl: fixture.baseUrl, fetch: fault.fetch }),
        {
          capabilities: () => fixture.negotiated,
          requestMeta,
        },
      );
      try {
        const value = await call(faultClient);
        expect(fault.attempts()).toBe(2);
        return value;
      } finally {
        await faultClient.close();
      }
    };
    const originalUtility = await fixture.client.models.getUtilityRole();
    const originalEmbedding = await fixture.client.models.getEmbeddingRole();
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["models.changed"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();

    try {
      const provider = await callAfterCommitLoss("providers.update", (faultClient) =>
        faultClient.providers.update({
          provider: "openai",
          apiKey: { type: "set", value: "cutpoint-provider-secret" },
          baseUrl: { type: "set", value: fixture.providerBaseUrl },
        }),
      );
      expect(provider).toMatchObject({ id: "openai", credential: { source: "stored" } });
      expect(provider.credential?.masked).not.toContain("cutpoint-provider-secret");
      await expect(nextRuntimeEvent(events, "models.changed")).resolves.toMatchObject({
        type: "models.changed",
      });
      await expect(fixture.client.providers.list()).resolves.toMatchObject({
        data: expect.arrayContaining([
          expect.objectContaining({
            id: "openai",
            credential: { masked: provider.credential?.masked, source: "stored" },
          }),
        ]),
      });

      await expect(
        callAfterCommitLoss("models.setUtilityRole", (faultClient) =>
          faultClient.models.setUtilityRole({ provider: "openai", model: "e2e-model" }),
        ),
      ).resolves.toEqual({ provider: "openai", model: "e2e-model" });
      await expect(nextRuntimeEvent(events, "models.changed")).resolves.toMatchObject({
        type: "models.changed",
      });
      await expect(fixture.client.models.getUtilityRole()).resolves.toEqual({
        provider: "openai",
        model: "e2e-model",
      });

      await expect(
        callAfterCommitLoss("models.setEmbeddingRole", (faultClient) =>
          faultClient.models.setEmbeddingRole({ provider: "openai", model: "e2e-embedding" }),
        ),
      ).resolves.toEqual({ provider: "openai", model: "e2e-embedding" });
      await expect(nextRuntimeEvent(events, "models.changed")).resolves.toMatchObject({
        type: "models.changed",
      });
      await expect(fixture.client.models.getEmbeddingRole()).resolves.toEqual({
        provider: "openai",
        model: "e2e-embedding",
      });
    } finally {
      await fixture.client.models.setUtilityRole(originalUtility);
      await fixture.client.models.setEmbeddingRole(originalEmbedding);
      await fixture.client.providers.update({
        provider: "openai",
        apiKey: { type: "clear" },
        baseUrl: { type: "clear" },
      });
      streamController.abort();
      await events.return?.();
    }
  }, 30_000);

  it("reconciles Agent Memory mutations after their responses are lost", async () => {
    const requestMeta = (): RequestMeta => ({
      protocolVersion: PROTOCOL_VERSION,
      clientInfo: { name: "runtime-http-workspace-state-cutpoint-e2e", version: "1" },
      clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
    });
    const callAfterCommitLoss = async <T>(
      method: string,
      call: (faultClient: FlameClient) => Promise<T>,
    ): Promise<T> => {
      const fault = faultUnaryResponse(method, "afterCommit");
      const faultClient = createFlameClient(
        createHttpTransport({ baseUrl: fixture.baseUrl, fetch: fault.fetch }),
        {
          requestMeta,
          capabilities: () => fixture.negotiated,
        },
      );
      try {
        await faultClient.runtime.discover();
        const value = await call(faultClient);
        expect(fault.attempts()).toBe(2);
        return value;
      } finally {
        await faultClient.close();
      }
    };
    const workspaceRoot = join(fixture.root, "workspace-state-replay-cutpoints");
    await mkdir(workspaceRoot);
    const workspace = fixture.client.workspace({ path: workspaceRoot });
    const binding = await captureMemoryBinding(fixture.client);
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["plugins.dataChanged"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();
    let memoryRecord: Awaited<ReturnType<typeof addInstalledMemory>> | undefined;

    try {
      const added = await callAfterCommitLoss("plugins.call", (faultClient) =>
        addInstalledMemory(faultClient, binding, {
          scope: "project",
          cwd: workspaceRoot,
          content: "agent-memory replay cutpoint",
        }),
      );
      memoryRecord = added;
      await expect(nextRuntimeEvent(events, "plugins.dataChanged")).resolves.toMatchObject({
        type: "plugins.dataChanged",
      });
      await expect(
        readInstalledMemory(fixture.client, binding, "project", workspace.ref.path).then(
          (data) => ({
            data,
          }),
        ),
      ).resolves.toMatchObject({
        data: [expect.objectContaining({ id: added.id, content: "agent-memory replay cutpoint" })],
      });

      const updated = await callAfterCommitLoss("plugins.call", (faultClient) =>
        updateInstalledMemory(faultClient, binding, {
          id: added.id,
          expectedRevision: added.revision,
          content: "agent-memory replay cutpoint updated",
          pinned: true,
        }),
      );
      memoryRecord = updated;
      await expect(nextRuntimeEvent(events, "plugins.dataChanged")).resolves.toMatchObject({
        type: "plugins.dataChanged",
      });
      await expect(
        readInstalledMemory(fixture.client, binding, "project", workspace.ref.path).then(
          (data) => ({
            data,
          }),
        ),
      ).resolves.toMatchObject({
        data: [
          expect.objectContaining({
            id: added.id,
            content: "agent-memory replay cutpoint updated",
            pinned: true,
            updatedAt: updated.updatedAt,
          }),
        ],
      });

      await callAfterCommitLoss("plugins.call", (faultClient) =>
        deleteInstalledMemory(faultClient, binding, added.id, updated.revision),
      );
      memoryRecord = undefined;
      await expect(nextRuntimeEvent(events, "plugins.dataChanged")).resolves.toMatchObject({
        type: "plugins.dataChanged",
      });
      expect(
        (
          await readInstalledMemory(fixture.client, binding, "project", workspace.ref.path).then(
            (data) => ({
              data,
            }),
          )
        ).data,
      ).toEqual([]);
    } finally {
      if (memoryRecord)
        await deleteInstalledMemory(
          fixture.client,
          binding,
          memoryRecord.id,
          memoryRecord.revision,
        ).catch(() => undefined);
      streamController.abort();
      await events.return?.();
    }
  }, 30_000);

  it("reconciles MCP and managed-Skill moves after their responses are lost", async () => {
    const retainedSkills = (
      await fixture.client.workspace({ path: fixture.root }).skills.listDiscovered()
    ).skills.filter((skill) => !(skill.scope === "user" && skill.name === managedSkillName));
    const requestMeta = (): RequestMeta => ({
      protocolVersion: PROTOCOL_VERSION,
      clientInfo: { name: "runtime-http-resource-cutpoint-e2e", version: "1" },
      clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
    });
    const callAfterCommitLoss = async <T>(
      method: string,
      call: (faultClient: FlameClient) => Promise<T>,
    ): Promise<T> => {
      const fault = faultUnaryResponse(method, "afterCommit");
      const faultClient = createFlameClient(
        createHttpTransport({ baseUrl: fixture.baseUrl, fetch: fault.fetch }),
        {
          capabilities: () => fixture.negotiated,
          requestMeta,
        },
      );
      try {
        const value = await call(faultClient);
        expect(fault.attempts()).toBe(2);
        return value;
      } finally {
        await faultClient.close();
      }
    };
    const server = "http-e2e-replay-cutpoint";
    const serverId = { origin: { type: "user" as const }, name: server };
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["mcp.changed", "skills.changed"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();

    try {
      await expect(
        callAfterCommitLoss("mcp.servers.create", (faultClient) =>
          faultClient.mcp.create({
            connection: { type: "stdio", command: "runtime-http-e2e-replay-cutpoint" },
            description: "MCP replay cutpoint",
            enabled: false,
            handshakeTimeout: { type: "unbounded" },
            name: server,
          }),
        ),
      ).resolves.toBeUndefined();
      await expect(nextUserMCPChange(events, server)).resolves.toMatchObject({
        servers: expect.arrayContaining([serverId]),
      });
      expect(
        (await fixture.client.mcp.list()).data.filter((entry) => entry.id.name === server),
      ).toHaveLength(1);

      await expect(
        callAfterCommitLoss("mcp.servers.update", (faultClient) =>
          faultClient.mcp.update({
            server: serverId,
            description: "MCP replay cutpoint updated",
          }),
        ),
      ).resolves.toBeUndefined();
      await expect(nextUserMCPChange(events, server)).resolves.toMatchObject({
        servers: expect.arrayContaining([serverId]),
      });
      expect(
        (await fixture.client.mcp.list()).data.find((entry) => entry.id.name === server),
      ).toMatchObject({
        id: serverId,
        description: "MCP replay cutpoint updated",
        status: { type: "disabled" },
      });

      await callAfterCommitLoss("mcp.servers.delete", (faultClient) =>
        faultClient.mcp.delete(serverId),
      );
      await expect(nextUserMCPChange(events, server)).resolves.toMatchObject({
        servers: expect.arrayContaining([serverId]),
      });
      expect((await fixture.client.mcp.list()).data.some((entry) => entry.id.name === server)).toBe(
        false,
      );

      await callAfterCommitLoss("skills.library.archive", (faultClient) =>
        faultClient.skills.archive(managedSkillName),
      );
      await expect(nextRuntimeEvent(events, "skills.changed")).resolves.toMatchObject({
        type: "skills.changed",
      });
      await expect(fixture.client.skills.listLibrary()).resolves.toMatchObject({
        data: [expect.objectContaining({ name: managedSkillName, lifecycle: "archived" })],
      });
      await expect(
        fixture.client.workspace({ path: fixture.root }).skills.listDiscovered(),
      ).resolves.toMatchObject({
        skills: retainedSkills,
      });

      await callAfterCommitLoss("skills.library.restore", (faultClient) =>
        faultClient.skills.restore(managedSkillName),
      );
      await expect(nextRuntimeEvent(events, "skills.changed")).resolves.toMatchObject({
        type: "skills.changed",
      });
      await expect(fixture.client.skills.listLibrary()).resolves.toMatchObject({
        data: [expect.objectContaining({ name: managedSkillName, lifecycle: "active" })],
      });
    } finally {
      await fixture.client.mcp.delete(serverId).catch(() => undefined);
      const managed = (await fixture.client.skills.listLibrary()).data.find(
        (skill) => skill.name === managedSkillName,
      );
      if (managed?.lifecycle === "archived") {
        await fixture.client.skills.restore(managedSkillName).catch(() => undefined);
      }
      streamController.abort();
      await events.return?.();
    }
  }, 30_000);
});
