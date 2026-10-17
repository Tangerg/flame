import { describe, expect, it } from "vitest";
import { createFlameClient } from "@flame/runtime-contract/client";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { VISUAL_SESSION_ID } from "./agentSessionSnapshots";
import {
  VISUAL_PACKAGES,
  VISUAL_PLUGIN_PROVIDERS,
  VISUAL_MEMORY_BINDING,
  VISUAL_MEMORY_PAGE,
  VISUAL_SCHEDULES_BINDING,
  VISUAL_SCHEDULES_PAGE,
  VISUAL_USAGE,
} from "./componentSnapshots";
import { REVIEW_DIFF } from "./diffSnapshot";
import { commitVisualInterruptResponses } from "./interruptResponses";
import { VISUAL_MODELS, VISUAL_PROVIDERS } from "./runtimeSnapshots";

describe("visual Runtime transport", () => {
  it("serves installed views through validated public reads and the same retained Item facts", async () => {
    const snapshot = structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.waiting);
    const client = createFlameClient(
      createVisualRuntimeTransport(snapshot, VISUAL_CAPABILITIES, async () => REVIEW_DIFF),
      {
        capabilities: () => VISUAL_CAPABILITIES,
      },
    );
    try {
      for (const [binding, method, page] of [
        [VISUAL_MEMORY_BINDING, "flame.memory/list.v1", VISUAL_MEMORY_PAGE],
        [VISUAL_SCHEDULES_BINDING, "flame.schedules/list.v1", VISUAL_SCHEDULES_PAGE],
      ] as const) {
        const response = await client.plugins.call({
          ...binding,
          method,
          input: JSON.stringify({ Cursor: "" }),
          target: { scope: "installation", reference: binding.installationId },
        });
        expect(JSON.parse(response.output!)).toEqual(page);
      }
      expect((await client.runtime.discover()).capabilities).toEqual(VISUAL_CAPABILITIES);
      expect(await client.usage.summary()).toEqual(VISUAL_USAGE);
      const workspace = await client.workspaces.open({ path: "/Users/visual/scope" });
      expect(await workspace.diff.get({ mode: "worktree", format: "rows" })).toEqual(REVIEW_DIFF);
      expect((await workspace.skills.listDiscovered()).skills).toHaveLength(2);
      expect((await workspace.skills.listProposals()).data).toHaveLength(2);
      expect((await client.skills.listLibrary()).data).toHaveLength(2);
      commitVisualInterruptResponses(snapshot, [
        { itemId: "item_approval", response: { type: "approval", decision: "deny" } },
      ]);
      const page = await client.sessions.trajectory({
        sessionId: VISUAL_SESSION_ID,
        includeDescendants: true,
      });
      expect(
        page.data.find((row) => row.type === "item" && row.item.id === "item_approval"),
      ).toMatchObject({ item: { approvalDecision: "deny", status: "incomplete" } });
    } finally {
      await client.close();
    }
  });

  it("uses the client's negotiated capability owner when Skills is unavailable", async () => {
    const capabilities = {
      ...VISUAL_CAPABILITIES,
      features: { git: VISUAL_CAPABILITIES.features!.git! },
    };
    const client = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        capabilities,
        async () => REVIEW_DIFF,
      ),
      { capabilities: () => capabilities },
    );
    try {
      const workspace = await client.workspaces.open({ path: "/Users/visual/scope" });
      await expect(workspace.skills.listDiscovered()).rejects.toMatchObject({
        data: { type: "capability_not_negotiated" },
      });
    } finally {
      await client.close();
    }
  });

  it("answers Settings reads through the validated Runtime client", async () => {
    const client = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
      ),
    );
    try {
      expect((await client.providers.list()).data).toEqual(VISUAL_PROVIDERS);
      expect((await client.models.list("openai")).data).toEqual(
        VISUAL_MODELS.filter((model) => model.provider === "openai"),
      );
      expect((await client.models.list("alibaba")).data).toEqual(
        VISUAL_MODELS.filter((model) => model.provider === "alibaba"),
      );
      expect(await client.models.getUtilityRole()).toEqual({
        provider: VISUAL_MODELS[0]!.provider,
        model: VISUAL_MODELS[0]!.id,
      });
      expect(await client.models.getEmbeddingRole()).toEqual({});
      expect((await client.mcp.list()).data).toEqual([]);
      expect((await client.plugins.list()).data).toEqual(VISUAL_PACKAGES);
      expect((await client.plugins.listProviders()).data).toEqual(VISUAL_PLUGIN_PROVIDERS);
      expect(await client.approval.listRules()).toEqual({ rules: [] });
      expect((await client.approval.getMode()).mode).toBe("balanced");
      expect((await client.approval.setMode("safe")).mode).toBe("safe");
      expect((await client.approval.getMode()).mode).toBe("safe");
      const workspace = await client.workspaces.open({ path: "/Users/visual/scope" });
      expect(await workspace.hooks.list()).toEqual({ hooks: [], projectTrusted: false });
    } finally {
      await client.close();
    }
  });

  it.each(["plugins.changed", "skills.changed", "files.changed"] as const)(
    "keeps the inert %s subscription open until its consumer retires",
    async (topic) => {
      const client = createFlameClient(
        createVisualRuntimeTransport(
          structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
          VISUAL_CAPABILITIES,
          async () => REVIEW_DIFF,
        ),
      );
      try {
        const subscription = await client.runtimeEvents.subscribe({ topics: [topic] });
        expect(subscription.result).toEqual({});
        const events = subscription.events[Symbol.asyncIterator]();
        const pending = events.next();
        await events.return?.();
        expect(await pending).toMatchObject({ done: true });
      } finally {
        await client.close();
      }
    },
  );

  it("refuses unmodelled writes instead of leaving them pending or reporting success", async () => {
    const client = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
      ),
    );
    try {
      await expect(client.models.setUtilityRole({})).rejects.toThrow(
        'Visual Runtime has no response for "models.setUtilityRole"',
      );
    } finally {
      await client.close();
    }
  });

  it("respects cancellation and refuses requests after closure", async () => {
    const client = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
      ),
    );
    const controller = new AbortController();
    const cancelled = new Error("visual read cancelled");
    controller.abort(cancelled);
    try {
      await expect(client.providers.list(controller.signal)).rejects.toMatchObject({
        name: "RpcTransportError",
        message: "aborted",
      });
      await client.close();
      await expect(client.providers.list()).rejects.toThrow("client closed");
    } finally {
      await client.close();
    }
  });
});
