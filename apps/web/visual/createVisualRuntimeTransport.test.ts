import { describe, expect, it, vi } from "vitest";
import { createFlameClient } from "@flame/runtime-client";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES, VISUAL_WORKSPACE_PATH } from "./agentFixtureFacts";
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
import { REVIEW_DIFF, BRANCH_REVIEW_DIFF } from "./diffSnapshot";
import { commitVisualInterruptResponses } from "./interruptResponses";
import { VISUAL_MODELS, VISUAL_PROVIDERS } from "./runtimeSnapshots";
import { VISUAL_SKILL_PROPOSALS } from "./skillSnapshots";
describe("visual Runtime transport", () => {
  it("serves installed views through validated public reads and the same retained Item facts", async () => {
    const snapshot = structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.waiting);
    const client = createFlameClient(
      createVisualRuntimeTransport(snapshot, VISUAL_CAPABILITIES, async (_signal, request) =>
        request?.mode === "base" ? BRANCH_REVIEW_DIFF : REVIEW_DIFF,
      ),
      {
        capabilities: () => VISUAL_CAPABILITIES,
      },
    );
    try {
      for (const [binding, method, page] of [
        [VISUAL_MEMORY_BINDING, "flame.memory/list.v2", VISUAL_MEMORY_PAGE],
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
      expect(await workspace.diff.get({ mode: "base", format: "rows" })).toEqual(
        BRANCH_REVIEW_DIFF,
      );
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

  it.each(["approveProposal", "rejectProposal"] as const)(
    "accepts an opted-in %s through its exact public target and rereads the same isolated page",
    async (decision) => {
      const transport = createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
        { acceptSkillProposals: true },
      );
      const send = vi.spyOn(transport, "send");
      const client = createFlameClient(transport, { capabilities: () => VISUAL_CAPABILITIES });
      const otherClient = createFlameClient(
        createVisualRuntimeTransport(
          structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
          VISUAL_CAPABILITIES,
          async () => REVIEW_DIFF,
          { acceptSkillProposals: true },
        ),
        { capabilities: () => VISUAL_CAPABILITIES },
      );
      try {
        const workspace = await client.workspaces.open({ path: VISUAL_WORKSPACE_PATH });
        const before = await workspace.skills.listProposals();
        const proposal = before.data[0]!;
        const ref = {
          scope: proposal.scope,
          name: proposal.name,
          revision: proposal.revision,
        };
        const command = workspace.skills[decision](ref);
        await expect(command).resolves.toBeUndefined();
        const method = `skills.proposals.${decision === "approveProposal" ? "approve" : "reject"}`;
        const call = send.mock.calls.find(([request]) => request.method === method);
        expect(call?.[0].params).toEqual({ ...ref, workspace: { path: VISUAL_WORKSPACE_PATH } });
        expect(call?.[2]?.idempotencyKey).toBe(command.idempotencyKey);
        expect((await workspace.skills.listProposals()).data).toEqual(before.data.slice(1));
        expect(before.data).toHaveLength(2);
        const otherWorkspace = await otherClient.workspaces.open({ path: VISUAL_WORKSPACE_PATH });
        expect(await otherWorkspace.skills.listProposals()).toEqual(VISUAL_SKILL_PROPOSALS);
        expect(VISUAL_SKILL_PROPOSALS.data).toHaveLength(2);
      } finally {
        await Promise.all([client.close(), otherClient.close()]);
      }
    },
  );

  it.each(["approveProposal", "rejectProposal"] as const)(
    "keeps %s refused unless the fixture opts in",
    async (decision) => {
      const client = createFlameClient(
        createVisualRuntimeTransport(
          structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
          VISUAL_CAPABILITIES,
          async () => REVIEW_DIFF,
        ),
        { capabilities: () => VISUAL_CAPABILITIES },
      );
      try {
        const workspace = await client.workspaces.open({ path: VISUAL_WORKSPACE_PATH });
        const { scope, name, revision } = VISUAL_SKILL_PROPOSALS.data[0]!;
        await expect(workspace.skills[decision]({ scope, name, revision })).rejects.toThrow(
          "Visual Runtime has no response",
        );
        expect(await workspace.skills.listProposals()).toEqual(VISUAL_SKILL_PROPOSALS);
      } finally {
        await client.close();
      }
    },
  );

  it("refuses a mismatched workspace, scope, name or revision without changing the proposal page", async () => {
    const client = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
        { acceptSkillProposals: true },
      ),
      { capabilities: () => VISUAL_CAPABILITIES },
    );
    try {
      const workspace = await client.workspaces.open({ path: VISUAL_WORKSPACE_PATH });
      const anotherWorkspace = await client.workspaces.open({ path: "/Users/visual/another" });
      const { scope, name, revision } = VISUAL_SKILL_PROPOSALS.data[0]!;
      const ref = { scope, name, revision };
      await expect(anotherWorkspace.skills.approveProposal(ref)).rejects.toThrow(
        "another workspace",
      );
      for (const changed of [
        { ...ref, scope: "user" as const },
        { ...ref, name: "another-skill" },
        { ...ref, revision: "f".repeat(64) },
      ]) {
        await expect(workspace.skills.rejectProposal(changed)).rejects.toThrow(
          "no matching proposal revision",
        );
      }
      expect(await workspace.skills.listProposals()).toEqual(VISUAL_SKILL_PROPOSALS);
    } finally {
      await client.close();
    }
  });

  it("answers advertised Settings reads through the validated Runtime client", async () => {
    const client = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
      ),
      { capabilities: () => VISUAL_CAPABILITIES },
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
      expect(await client.mcp.list()).toEqual({ data: [] });
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
        { capabilities: () => VISUAL_CAPABILITIES },
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
      { capabilities: () => VISUAL_CAPABILITIES },
    );
    try {
      await expect(client.models.setUtilityRole({})).rejects.toMatchObject({
        name: "RpcError",
        data: {
          type: "method_not_found",
          detail: 'Visual Runtime has no response for "models.setUtilityRole"',
        },
      });
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
      { capabilities: () => VISUAL_CAPABILITIES },
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
