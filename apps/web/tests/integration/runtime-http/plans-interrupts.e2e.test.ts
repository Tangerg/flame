import { describe, expect, it } from "vitest";
import { asRunId, asSessionId } from "@flame/runtime-client";
import { HTTP_HEADERS, type ApprovalModeResult, type MCPServerID } from "@flame/runtime-client/wire";
import { RuntimeAgentGateway } from "@/features/agent/adapters/agentRuntimeGateway";
import { createAgentCommands } from "@/features/agent/application/agentCommandOwner";
import { setApprovalMode } from "@/features/agent/application/approvalPolicy";
import { APPROVAL_MODE_KEY } from "@/features/agent/public/approvalPolicy";
import { createAgentSharedMaterial } from "@/features/agent/application/session/sharedMaterial";
import { queryClient } from "@/lib/queryClient";
import { collectRunEvents, nextRuntimeEvent, originalOpeningEvents, within } from "./observation";
import { useRuntimeFixture } from "./fixture";

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: Plans and interrupts", () => {
  const fixture = useRuntimeFixture("plans-interrupts");

  it("publishes the Plan on the stream and through the exact cold read", async () => {
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP plan lifecycle",
    });
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["plan.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const started = await fixture.client.runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_PLAN publish the two-step plan." }],
    });
    const events = await collectRunEvents(originalOpeningEvents(started));
    expect(
      events.some(
        (event) =>
          event.event.type === "plan.updated" && event.event.plan.state?.steps.length === 2,
      ),
    ).toBe(true);

    const changed = await nextRuntimeEvent(runtimeEvents, "plan.changed");
    expect(changed).toMatchObject({
      type: "plan.changed",
      sessionIds: [session.id],
    });
    await expect(fixture.client.plan.get(asSessionId(session.id))).resolves.toMatchObject({
      sessionId: session.id,
      state: {
        revision: 1,
        steps: [
          { description: "Inspect the runtime contract", status: "completed" },
          { description: "Verify frontend reconciliation", status: "in_progress" },
        ],
      },
    });

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("preserves a rejected Plan call as a durable failure without advancing the Plan", async () => {
    const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    const sessionId = asSessionId(session.id);
    const started = await fixture.client.runs.start({
      sessionId,
      input: [{ type: "text", text: "E2E_PLAN_REJECTED keep the last accepted plan." }],
    });
    const events = await collectRunEvents(originalOpeningEvents(started));
    const failed = events.find(
      ({ event }) =>
        event.type === "item.completed" && event.item.type === "toolCall" && event.item.error,
    )?.event;
    if (failed?.type !== "item.completed") throw new Error("expected a failed Plan tool Item");
    expect(failed.item).toMatchObject({
      type: "toolCall",
      tool: { name: "set_plan" },
      error: {
        type: "tool_failed",
        detail: expect.stringContaining("at most one step may be in progress"),
      },
    });
    expect(
      await fixture.client.items
        .list({ scope: { type: "run", runId: started.result.runId } })
        .autoPagingToArray(),
    ).toContainEqual(failed.item);
    await expect(fixture.client.plan.get(sessionId)).resolves.toMatchObject({
      state: {
        revision: 1,
        steps: [
          { description: "Inspect the runtime contract", status: "completed" },
          { description: "Verify frontend reconciliation", status: "in_progress" },
        ],
      },
    });
    expect(events.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "finished", outcome: { type: "completed" } },
    });
  }, 30_000);

  it("parks and resumes the same run through durable HITL identity", async () => {
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP HITL lifecycle",
    });
    const started = await fixture.client.runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_HITL ask before continuing." }],
    });
    const runId = asRunId(started.result.runId);
    const startEvents = await collectRunEvents(originalOpeningEvents(started));
    expect(startEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "waiting" },
      interrupts: expect.arrayContaining([expect.anything()]),
    });
    const waiting = await fixture.client.runs.get(runId);
    expect(waiting).toMatchObject({ status: "waiting" });
    expect(waiting).not.toHaveProperty("activeSegmentId");

    const pending = await fixture.client.interrupts.list({ rootRunId: runId });
    expect(pending.data).toHaveLength(1);
    expect(pending.data[0]?.interrupts).toHaveLength(1);
    const question = pending.data[0]?.interrupts[0];
    if (!question || question.type !== "question") {
      throw new Error("runtime did not persist the expected question interrupt");
    }
    expect(question.runId).toBe(started.result.runId);
    expect(question.payload.question.fields[0]?.prompt).toBe("Continue the HTTP lifecycle check?");

    const resumed = await fixture.client.runs.resume({
      runId,
      responses: [
        {
          itemId: question.itemId,
          response: { type: "answer", answers: [["Yes"]] },
        },
      ],
      input: [{ type: "text", text: "E2E_RESUME_INPUT preserve this once." }],
    });
    expect(resumed.result.runId).toBe(started.result.runId);
    expect(resumed.result.segmentId).not.toBe(started.result.segmentId);
    expect(resumed.result.userItemId).toBeTruthy();
    const resumeEvents = await collectRunEvents(originalOpeningEvents(resumed));
    expect(resumeEvents).toContainEqual(
      expect.objectContaining({
        event: expect.objectContaining({
          type: "item.completed",
          item: expect.objectContaining({
            type: "agentMessage",
            content: [{ type: "text", text: "Resume follow-up input observed." }],
          }),
        }),
      }),
    );
    expect(resumeEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "finished", outcome: { type: "completed" } },
    });
    await expect(fixture.client.interrupts.list({ rootRunId: runId })).resolves.toMatchObject({
      data: [],
    });
    await expect(fixture.client.runs.get(runId)).resolves.toMatchObject({
      status: "finished",
      outcome: { type: "completed" },
    });
    const items = await fixture.client.items
      .list({ scope: { type: "run", runId } })
      .autoPagingToArray();
    expect(
      items.filter(
        (item) =>
          item.type === "userMessage" &&
          item.id === resumed.result.userItemId &&
          item.content?.some(
            (block) =>
              block.type === "text" && block.text === "E2E_RESUME_INPUT preserve this once.",
          ),
      ),
    ).toHaveLength(1);
  }, 30_000);

  it("opens and clears an approval interrupt around the exact tool call", async () => {
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP approval lifecycle",
    });
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["interrupts.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const started = await fixture.client.runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_APPROVAL run the command after approval." }],
    });
    const runId = asRunId(started.result.runId);
    const startEvents = await collectRunEvents(originalOpeningEvents(started));
    expect(startEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "waiting" },
      interrupts: expect.arrayContaining([expect.anything()]),
    });
    await expect(nextRuntimeEvent(runtimeEvents, "interrupts.changed")).resolves.toMatchObject({
      type: "interrupts.changed",
      runIds: [runId],
      sessionIds: [session.id],
    });

    const pending = await fixture.client.interrupts.list({ rootRunId: runId });
    const approval = pending.data[0]?.interrupts[0];
    if (!approval || approval.type !== "approval") {
      throw new Error("runtime did not persist the expected approval interrupt");
    }
    expect(approval.runId).toBe(runId);
    expect(approval.payload.rememberable).toBe(true);
    expect(approval.payload.tool).toMatchObject({
      name: "shell",
      arguments: {
        command: "printf approval-ok",
        description: "Print the approval marker",
      },
    });

    const resumed = await fixture.client.runs.resume({
      runId,
      responses: [
        {
          itemId: approval.itemId,
          response: {
            type: "approval",
            decision: "approve",
            remember: { scope: "session" },
          },
        },
      ],
    });
    const resumeEvents = await collectRunEvents(originalOpeningEvents(resumed));
    expect(resumeEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "finished", outcome: { type: "completed" } },
    });
    await expect(nextRuntimeEvent(runtimeEvents, "interrupts.changed")).resolves.toMatchObject({
      type: "interrupts.changed",
      runIds: [runId],
      sessionIds: [session.id],
    });
    await expect(fixture.client.interrupts.list({ rootRunId: runId })).resolves.toMatchObject({
      data: [],
    });

    const rules = await fixture.client.approval.listRules(asSessionId(session.id));
    expect(rules.rules).toEqual([
      expect.objectContaining({
        scope: "session",
        tool: { type: "builtIn", name: "shell" },
        modelName: "shell",
        stale: false,
        decision: "allow",
      }),
    ]);
    const rememberedRule = rules.rules[0];
    if (!rememberedRule) throw new Error("approval decision was not remembered");
    await fixture.client.approval.forgetRule({ id: rememberedRule.id });
    await expect(fixture.client.approval.listRules(asSessionId(session.id))).resolves.toEqual({
      rules: [],
    });

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("keeps one Tool Item when edited approval resumes beside a same-name sibling after SIGKILL", async () => {
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP edited parallel approval identity",
    });
    const started = await fixture.client.runs.start({
      sessionId: asSessionId(session.id),
      input: [
        {
          type: "text",
          text: "E2E_PARALLEL_EDITED_TOOL run both commands after the first edited approval.",
        },
      ],
    });
    const runId = asRunId(started.result.runId);
    const startEvents = await collectRunEvents(originalOpeningEvents(started));
    expect(startEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "waiting" },
      interrupts: expect.arrayContaining([expect.anything()]),
    });

    const pending = await fixture.client.interrupts.list({ rootRunId: runId });
    expect(pending.data).toHaveLength(1);
    expect(pending.data[0]?.interrupts).toHaveLength(1);
    const approval = pending.data[0]?.interrupts[0];
    if (!approval || approval.type !== "approval") {
      throw new Error("runtime did not persist the expected first parallel approval");
    }
    expect(approval.payload.tool).toMatchObject({
      name: "shell",
      arguments: { command: "printf approval-original" },
    });

    // The paused tree also retains MCP sources introduced by earlier cases.
    const capturedSources = new Map<string, { server: MCPServerID; count: number }>();
    for (const tool of (await fixture.client.mcp.listTools()).data) {
      const key = JSON.stringify(tool.server);
      const source = capturedSources.get(key);
      capturedSources.set(key, { server: tool.server, count: (source?.count ?? 0) + 1 });
    }

    await fixture.client.close();
    fixture.client = undefined;
    await fixture.killRuntimeProcess();
    await fixture.startRuntimeProcess();
    fixture.client = fixture.createRuntimeClient();
    fixture.negotiated = (await fixture.client.runtime.discover()).capabilities;
    await fixture.awaitManagedOwners();

    await Promise.all(
      [...capturedSources.values()].map(({ server, count }) =>
        fixture.awaitMCPTools(server, (tools) => tools.length === count),
      ),
    );

    const restored = await fixture.client.runs.get(runId);
    expect(
      restored.status,
      `restart must retain the parked parallel approval: ${JSON.stringify(restored.outcome)}; ${fixture.processOutput
        .split("\n")
        .filter((line) => line.includes("waiting execution is not resumable"))
        .join("\n")}`,
    ).toBe("waiting");
    expect((await fixture.client.interrupts.list({ rootRunId: runId })).data).toMatchObject([
      { interrupts: [{ itemId: approval.itemId, type: "approval" }] },
    ]);

    const resumed = await fixture.client.runs.resume({
      runId,
      responses: [
        {
          itemId: approval.itemId,
          response: {
            type: "approval",
            decision: "approve",
            editedArgs: {
              command: "printf approval-edited",
              description: "Print the edited approval marker",
            },
            remember: { scope: "session" },
          },
        },
      ],
    });
    const firstResumeEvents = await collectRunEvents(originalOpeningEvents(resumed));
    expect(firstResumeEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "waiting" },
      interrupts: expect.arrayContaining([expect.anything()]),
    });

    const firstLifecycle = [...startEvents, ...firstResumeEvents];
    const starts = firstLifecycle.filter(
      (event) => event.event.type === "item.started" && event.event.item.id === approval.itemId,
    );
    const completions = firstLifecycle.filter(
      (event) => event.event.type === "item.completed" && event.event.item.id === approval.itemId,
    );
    expect(starts).toHaveLength(1);
    expect(completions).toHaveLength(1);
    const completedEvent = completions[0]?.event;
    if (completedEvent?.type !== "item.completed" || completedEvent.item.type !== "toolCall") {
      throw new Error("edited approval did not complete the original Tool Item");
    }
    expect(completedEvent.item).toMatchObject({
      approvalDecision: "approve",
      id: approval.itemId,
      status: "completed",
      tool: {
        arguments: { command: "printf approval-edited" },
      },
    });
    expect(JSON.stringify(completedEvent.item.tool?.result)).toContain("approval-edited");

    const secondPending = await fixture.client.interrupts.list({ rootRunId: runId });
    expect(secondPending.data).toHaveLength(1);
    expect(secondPending.data[0]?.interrupts).toHaveLength(1);
    const siblingApproval = secondPending.data[0]?.interrupts[0];
    if (!siblingApproval || siblingApproval.type !== "approval") {
      throw new Error("runtime did not persist the same-name sibling approval");
    }
    expect(siblingApproval.itemId).not.toBe(approval.itemId);
    expect(siblingApproval.payload.tool).toMatchObject({
      name: "shell",
      arguments: { command: "printf approval-sibling" },
    });

    const completed = await fixture.client.runs.resume({
      runId,
      responses: [
        {
          itemId: siblingApproval.itemId,
          response: { type: "approval", decision: "approve" },
        },
      ],
    });
    const finalEvents = await collectRunEvents(originalOpeningEvents(completed));
    expect(finalEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "finished", outcome: { type: "completed" } },
    });
    await expect(fixture.client.interrupts.list({ rootRunId: runId })).resolves.toMatchObject({
      data: [],
    });

    const rules = await fixture.client.approval.listRules(asSessionId(session.id));
    await Promise.all(rules.rules.map(({ id }) => fixture.client.approval.forgetRule({ id })));
  }, 30_000);

  it("round-trips the runtime approval mode without changing run policy", async () => {
    const original = await fixture.client.approval.getMode();
    const alternate = original.mode === "yolo" ? "balanced" : "yolo";
    await expect(fixture.client.approval.setMode(alternate)).resolves.toEqual({
      mode: alternate,
      modes: original.modes,
    });
    await expect(fixture.client.approval.getMode()).resolves.toEqual({
      mode: alternate,
      modes: original.modes,
    });
    await expect(fixture.client.approval.setMode(original.mode)).resolves.toEqual(original);
    await expect(fixture.client.approval.getMode()).resolves.toEqual(original);
  }, 30_000);

  it("keeps the newer approval query after an older mode acknowledgement arrives", async () => {
    const original = await fixture.client.approval.getMode();
    const submitted = "safe";
    const newer = "balanced";
    const committed = Promise.withResolvers<void>();
    const release = Promise.withResolvers<void>();
    const client = fixture.createRuntimeClient(async (input, init) => {
      const response = await fetch(input, init);
      if (new Headers(init?.headers).get(HTTP_HEADERS.method) === "approval.setMode") {
        committed.resolve();
        await release.promise;
      }
      return response;
    });
    const commands = createAgentCommands();
    commands.replace();
    const runtime = new RuntimeAgentGateway(
      () => client,
      { hasCapability: (feature) => fixture.negotiated?.features[feature]?.enabled === true },
      createAgentSharedMaterial(),
      commands,
    );
    const key = [APPROVAL_MODE_KEY];
    const saving = setApprovalMode({ commands, runtime }, submitted);
    try {
      await within(committed.promise, "approval mode acknowledgement");
      await expect(fixture.client.approval.getMode()).resolves.toMatchObject({ mode: submitted });
      await fixture.client.approval.setMode(newer);
      await expect(queryClient.fetchQuery({
        queryKey: key,
        queryFn: ({ signal }) => client.approval.getMode(signal),
        staleTime: 0,
      })).resolves.toMatchObject({ mode: newer });

      release.resolve();
      await saving;
      expect(queryClient.getQueryData<ApprovalModeResult>(key)?.mode).toBe(newer);
    } finally {
      release.resolve();
      await saving;
      queryClient.removeQueries({ queryKey: key });
      commands.dispose();
      runtime.dispose();
      await client.close();
      await fixture.client.approval.setMode(original.mode);
    }
  }, 30_000);
});
