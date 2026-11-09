import { describe, expect, it, vi } from "vitest";
import { createFlameClient, RpcCancellationError } from "@flame/runtime-client";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES, VISUAL_WORKSPACE_PATH } from "./agentFixtureFacts";
import { VISUAL_SESSION_ID } from "./agentSessionSnapshots";
import { parseJSON, stringifyJSON } from "@frame/sdk/json";
import reportsDeclaration from "../../../../plugins/usagereports/package/io.github.tangerg.flame/declaration.json";
import {
  VISUAL_PACKAGES,
  VISUAL_PLUGIN_PROVIDERS,
  VISUAL_MEMORY_BINDING,
  VISUAL_MEMORY_PAGE,
  VISUAL_LONG_MEMORY_PAGE,
  VISUAL_LONG_USER_MEMORY_PAGE,
  VISUAL_SCHEDULES_BINDING,
  VISUAL_SCHEDULES_PAGE,
  VISUAL_USAGE,
  VISUAL_USAGE_REPORTS_BINDING,
} from "./componentSnapshots";
import { REVIEW_DIFF, BRANCH_REVIEW_DIFF } from "./diffSnapshot";
import { commitVisualInterruptResponses } from "./interruptResponses";
import { VISUAL_MODELS, VISUAL_PROVIDERS } from "./runtimeSnapshots";
import {
  VISUAL_LONG_SKILL_DOCUMENTS,
  VISUAL_SKILL_DOCUMENTS,
  VISUAL_SKILL_PROPOSALS,
} from "./skillSnapshots";
describe("visual Runtime transport", () => {
  it.each([
    { scope: "project", page: VISUAL_LONG_MEMORY_PAGE },
    { scope: "user", page: VISUAL_LONG_USER_MEMORY_PAGE },
  ] as const)("admits the recorded long $scope Memory page", async ({ scope, page }) => {
    const client = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
        {
          memoryPages: { project: VISUAL_LONG_MEMORY_PAGE, user: VISUAL_LONG_USER_MEMORY_PAGE },
        },
      ),
      { capabilities: () => VISUAL_CAPABILITIES },
    );
    try {
      const result = await client.plugins.call({
        ...VISUAL_MEMORY_BINDING,
        method: "flame.memory/list.v2",
        input: JSON.stringify({
          Scope: scope,
          CWD: scope === "project" ? VISUAL_WORKSPACE_PATH : "",
          Cursor: "",
        }),
        target: { scope: "installation", reference: VISUAL_MEMORY_BINDING.installationId },
      });
      const captured = JSON.parse(result.output!) as typeof page;
      expect(captured).toEqual(page);
      expect(captured.Rows.every((row) => row.Scope === scope)).toBe(true);
      expect(captured.Rows.some((row) => row.Content.includes("\n\n"))).toBe(true);
      expect(new Set(captured.Rows.map((row) => row.ID)).size).toBe(captured.Rows.length);
    } finally {
      await client.close();
    }
  });

  it("keeps a recorded long Memory page read-only", async () => {
    const client = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
        {
          memoryPages: { project: VISUAL_LONG_MEMORY_PAGE, user: VISUAL_LONG_USER_MEMORY_PAGE },
        },
      ),
      { capabilities: () => VISUAL_CAPABILITIES },
    );
    const row = VISUAL_LONG_MEMORY_PAGE.Rows.find((item) => item.Status === "active")!;
    try {
      await expect(
        client.plugins.call({
          ...VISUAL_MEMORY_BINDING,
          method: "flame.memory/update.v2",
          input: JSON.stringify({
            ID: row.ID,
            ExpectedRevision: row.Revision,
            Content: "A local draft is not an accepted record.",
          }),
          target: { scope: "installation", reference: VISUAL_MEMORY_BINDING.installationId },
        }),
      ).rejects.toThrow('Visual plugin has no response for "flame.memory/update.v2"');
      const result = await client.plugins.call({
        ...VISUAL_MEMORY_BINDING,
        method: "flame.memory/list.v2",
        input: JSON.stringify({ Scope: "project", CWD: VISUAL_WORKSPACE_PATH, Cursor: "" }),
        target: { scope: "installation", reference: VISUAL_MEMORY_BINDING.installationId },
      });
      expect(JSON.parse(result.output!)).toEqual(VISUAL_LONG_MEMORY_PAGE);
    } finally {
      await client.close();
    }
  });

  it.each([
    "{}",
    "null",
    '{"id":false}',
    '{"id":"unknown"}',
    stringifyJSON({ id: ` ${"a".repeat(64)}` }),
    "{",
  ])("refuses an uncaptured saved report input %s", async (input) => {
    const client = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
      ),
      { capabilities: () => VISUAL_CAPABILITIES },
    );
    try {
      await expect(
        client.plugins.call({
          ...VISUAL_USAGE_REPORTS_BINDING,
          method: "flame.usage-reports/get.v1",
          input,
          target: { scope: "installation", reference: VISUAL_USAGE_REPORTS_BINDING.installationId },
        }),
      ).rejects.toThrow(input === "{" ? SyntaxError : "Visual usage reports has no report");
    } finally {
      await client.close();
    }
  });

  it.each(["capture", "delete"])(
    "keeps saved report %s outside the reading fixture",
    async (action) => {
      const client = createFlameClient(
        createVisualRuntimeTransport(
          structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
          VISUAL_CAPABILITIES,
          async () => REVIEW_DIFF,
        ),
        { capabilities: () => VISUAL_CAPABILITIES },
      );
      try {
        await expect(
          client.plugins.call({
            ...VISUAL_USAGE_REPORTS_BINDING,
            method: `flame.usage-reports/${action}.v1`,
            input: stringifyJSON(
              action === "capture" ? { period: "daily" } : { id: "a".repeat(64) },
            ),
            target: {
              scope: "installation",
              reference: VISUAL_USAGE_REPORTS_BINDING.installationId,
            },
          }),
        ).rejects.toThrow("Visual plugin has no response");
      } finally {
        await client.close();
      }
    },
  );

  it("reads immutable saved reports without rounding or filling absent prices", async () => {
    const client = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
      ),
      { capabilities: () => VISUAL_CAPABILITIES },
    );
    const binding = VISUAL_USAGE_REPORTS_BINDING;
    try {
      const installation = (await client.plugins.list()).data.find(
        (entry) => entry.id === binding.installationId,
      );
      expect(installation?.selected.contributions[0]).toMatchObject({
        id: reportsDeclaration.contributions[0]!.id,
        server: reportsDeclaration.contributions[0]!.server,
        export: reportsDeclaration.contributions[0]!.export,
        contract: binding.contract,
      });
      const read = async (method: string, input: unknown) =>
        parseJSON(
          (
            await client.plugins.call({
              ...binding,
              method,
              input: stringifyJSON(input),
              target: { scope: "installation", reference: binding.installationId },
            })
          ).output!,
        );
      const listed = await read("flame.usage-reports/list.v2", {});
      expect(listed).toEqual({
        reports: [
          {
            id: "b".repeat(64),
            period: "weekly",
            observedAt: "2026-10-20T09:00:00Z",
            since: "2026-10-13T09:00:00Z",
          },
          {
            id: "a".repeat(64),
            period: "daily",
            observedAt: "2026-10-20T08:00:00Z",
            since: "2026-10-19T08:00:00Z",
          },
        ],
      });
      const daily = await read("flame.usage-reports/get.v1", { id: "a".repeat(64) });
      expect(daily).toMatchObject({
        id: "a".repeat(64),
        period: "daily",
        summary: {
          total: { inputTokens: 9007199254740993n, outputTokens: 0 },
          recordedCostUsd: 0,
          coverage: { calls: 6, reportedCalls: 6, pricedCalls: 5 },
          byDay: [{ key: "2026-10-19" }],
        },
      });
      expect(daily).not.toHaveProperty("summary.total.costUsd");
      expect(await read("flame.usage-reports/get.v1", { id: "b".repeat(64) })).toMatchObject({
        id: "b".repeat(64),
        period: "weekly",
        summary: { total: VISUAL_USAGE.total, coverage: VISUAL_USAGE.coverage },
      });
      expect(await read("flame.usage-reports/get.v1", { id: "a".repeat(64) })).toEqual(daily);
    } finally {
      await client.close();
    }
  });

  it.each(["byProvider", "byModel", "byDay", "byOrigin", "byScope"] as const)(
    "reconciles recorded Usage %s with the same physical attempts",
    async (group) => {
      const client = createFlameClient(
        createVisualRuntimeTransport(
          structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
          VISUAL_CAPABILITIES,
          async () => REVIEW_DIFF,
        ),
        { capabilities: () => VISUAL_CAPABILITIES },
      );
      try {
        const report = await client.usage.summary();
        const buckets = report[group];
        expect(buckets.length).toBeGreaterThan(0);
        expect(new Set(buckets.map((bucket) => bucket.key)).size).toBe(buckets.length);
        for (const key of [
          "inputTokens",
          "outputTokens",
          "cacheReadTokens",
          "cacheWriteTokens",
          "reasoningTokens",
        ] as const)
          expect(buckets.reduce((sum, bucket) => sum + (bucket.total[key] ?? 0), 0)).toBe(
            report.total[key] ?? 0,
          );
        for (const key of [
          "calls",
          "reportedCalls",
          "pricedCalls",
          "startedCalls",
          "completedCalls",
          "failedCalls",
          "unknownCalls",
        ] as const)
          expect(buckets.reduce((sum, bucket) => sum + bucket.coverage[key], 0)).toBe(
            report.coverage[key],
          );
        expect(buckets.every((bucket) => bucket.total.costUsd !== undefined)).toBe(true);
        expect(buckets.every((bucket) => bucket.recordedCostUsd !== undefined)).toBe(true);
        expect(buckets.reduce((sum, bucket) => sum + bucket.total.costUsd!, 0)).toBeCloseTo(
          report.total.costUsd!,
          10,
        );
        expect(buckets.reduce((sum, bucket) => sum + bucket.recordedCostUsd!, 0)).toBeCloseTo(
          report.recordedCostUsd!,
          10,
        );
      } finally {
        await client.close();
      }
    },
  );

  it("publishes a controlled remote schedule removal through its original subscription", async () => {
    const transport = createVisualRuntimeTransport(
      structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
      VISUAL_CAPABILITIES,
      async () => REVIEW_DIFF,
    );
    const client = createFlameClient(transport, { capabilities: () => VISUAL_CAPABILITIES });
    try {
      const subscription = await client.runtimeEvents.subscribe({
        topics: ["plugins.dataChanged"],
      });
      const events = subscription.events[Symbol.asyncIterator]();
      transport.removeSchedule("sch_nightly");
      expect(await events.next()).toMatchObject({
        done: false,
        value: { type: "plugins.dataChanged" },
      });
      const response = await client.plugins.call({
        ...VISUAL_SCHEDULES_BINDING,
        method: "flame.schedules/list.v1",
        input: JSON.stringify({ Cursor: "" }),
        target: { scope: "installation", reference: VISUAL_SCHEDULES_BINDING.installationId },
      });
      expect(JSON.parse(response.output!)).toEqual({
        ...VISUAL_SCHEDULES_PAGE,
        Rows: [VISUAL_SCHEDULES_PAGE.Rows[1]],
      });
      expect(VISUAL_SCHEDULES_PAGE.Rows.map((row) => row.ID)).toEqual([
        "sch_nightly",
        "sch_weekly",
      ]);
      expect(() => transport.removeSchedule("sch_nightly")).toThrow("has no record");
      await events.return?.();
    } finally {
      await client.close();
    }
  });

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
          input: JSON.stringify(
            method === "flame.memory/list.v2"
              ? { Scope: "project", CWD: VISUAL_WORKSPACE_PATH, Cursor: "" }
              : { Cursor: "" },
          ),
          target: { scope: "installation", reference: binding.installationId },
        });
        expect(JSON.parse(response.output!)).toEqual(page);
      }
      expect((await client.runtime.discover()).capabilities).toEqual(VISUAL_CAPABILITIES);
      const usage = await client.usage.summary();
      expect(usage).toEqual(VISUAL_USAGE);
      expect(
        usage.byScope.find((bucket) => bucket.key === "run")?.coverage.calls ?? 0,
      ).toBeGreaterThanOrEqual(usage.runs);
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

  it("keeps User and Project memory records bound to the requested scope", async () => {
    const client = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
      ),
      { capabilities: () => VISUAL_CAPABILITIES },
    );
    const page = async (scope: "project" | "user") => {
      const result = await client.plugins.call({
        ...VISUAL_MEMORY_BINDING,
        method: "flame.memory/list.v2",
        input: JSON.stringify({
          Scope: scope,
          CWD: scope === "project" ? VISUAL_WORKSPACE_PATH : "",
          Cursor: "",
        }),
        target: { scope: "installation", reference: VISUAL_MEMORY_BINDING.installationId },
      });
      return JSON.parse(result.output!) as typeof VISUAL_MEMORY_PAGE;
    };
    try {
      expect(await page("project")).toEqual(VISUAL_MEMORY_PAGE);
      const user = await page("user");
      expect(user.Rows.length).toBeGreaterThan(0);
      expect(user.Rows.every((row) => row.Scope === "user" && row.Project === "")).toBe(true);
      const projectIDs = new Set(VISUAL_MEMORY_PAGE.Rows.map((row) => row.ID));
      expect(user.Rows.some((row) => projectIDs.has(row.ID))).toBe(false);
      expect(await page("project")).toEqual(VISUAL_MEMORY_PAGE);
    } finally {
      await client.close();
    }
  });

  it.each([
    ["{}", /Visual memory has no page for scope/],
    ['{"Scope":"workspace"}', /Visual memory has no page for scope/],
    ["{", /JSON/],
  ])(
    "refuses a Memory read with an absent, unknown or malformed scope: %s",
    async (input, message) => {
      const client = createFlameClient(
        createVisualRuntimeTransport(
          structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
          VISUAL_CAPABILITIES,
          async () => REVIEW_DIFF,
        ),
        { capabilities: () => VISUAL_CAPABILITIES },
      );
      try {
        await expect(
          client.plugins.call({
            ...VISUAL_MEMORY_BINDING,
            method: "flame.memory/list.v2",
            input,
            target: { scope: "installation", reference: VISUAL_MEMORY_BINDING.installationId },
          }),
        ).rejects.toThrow(message);
      } finally {
        await client.close();
      }
    },
  );

  it("serves a configured long Skills catalog from one capture without changing the default instance", async () => {
    const selected = [...VISUAL_LONG_SKILL_DOCUMENTS];
    const client = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
        { skillDocuments: selected },
      ),
      { capabilities: () => VISUAL_CAPABILITIES },
    );
    const defaultClient = createFlameClient(
      createVisualRuntimeTransport(
        structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
        VISUAL_CAPABILITIES,
        async () => REVIEW_DIFF,
      ),
      { capabilities: () => VISUAL_CAPABILITIES },
    );
    try {
      selected.splice(0, selected.length, VISUAL_SKILL_DOCUMENTS[0]!);
      const workspace = await client.workspaces.open({ path: VISUAL_WORKSPACE_PATH });
      const discovered = await workspace.skills.listDiscovered();
      expect(discovered.skills).toHaveLength(80);
      expect(
        discovered.skills.filter((skill) => skill.name.startsWith("runtime-review-")),
      ).toHaveLength(40);
      expect(discovered.skills[34]).toEqual({
        name: "runtime-review-035",
        description: VISUAL_LONG_SKILL_DOCUMENTS[34]!.description,
        scope: "project",
      });
      expect(await workspace.skills.getDiscovered("runtime-review-001")).toEqual({
        name: "runtime-review-001",
        description: VISUAL_LONG_SKILL_DOCUMENTS[0]!.description,
        scope: "project",
        path: `${VISUAL_WORKSPACE_PATH}/.flame/skills/runtime-review-001/SKILL.md`,
        revision: `${"0".repeat(63)}1`,
        instructions: VISUAL_SKILL_DOCUMENTS[1]!.instructions,
      });
      expect(await workspace.skills.getDiscovered("runtime-review-035")).toEqual(
        VISUAL_LONG_SKILL_DOCUMENTS[34],
      );
      expect(await workspace.skills.getDiscovered("workspace-review-080")).toEqual({
        name: "workspace-review-080",
        description: VISUAL_LONG_SKILL_DOCUMENTS[79]!.description,
        scope: "user",
        path: "/Users/visual/.flame/skills/workspace-review-080/SKILL.md",
        revision: `${"0".repeat(62)}50`,
        instructions: VISUAL_SKILL_DOCUMENTS[1]!.instructions,
      });
      await expect(workspace.skills.getDiscovered("risk-first-review")).rejects.toThrow(
        'no document named "risk-first-review"',
      );
      const defaultWorkspace = await defaultClient.workspaces.open({
        path: VISUAL_WORKSPACE_PATH,
      });
      expect((await defaultWorkspace.skills.listDiscovered()).skills).toEqual(
        VISUAL_SKILL_DOCUMENTS.map(({ name, description, scope }) => ({
          name,
          description,
          scope,
        })),
      );
      for (const document of VISUAL_SKILL_DOCUMENTS) {
        expect(await defaultWorkspace.skills.getDiscovered(document.name)).toEqual(document);
      }
    } finally {
      await Promise.all([client.close(), defaultClient.close()]);
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
      await expect(client.providers.list(controller.signal)).rejects.toBeInstanceOf(
        RpcCancellationError,
      );
      await client.close();
      await expect(client.providers.list()).rejects.toThrow("client closed");
    } finally {
      await client.close();
    }
  });
});
