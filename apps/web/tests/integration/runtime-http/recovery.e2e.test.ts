import { lstat } from "node:fs/promises";
import { join } from "node:path";
import { describe, expect, it, onTestFinished } from "vitest";
import { asRunId, asSessionId } from "@flame/runtime-client";
import { HTTP_HEADERS } from "@flame/runtime-client/wire";
import { composeAgent } from "@/features/agent/adapters/agent";
import { createRuntimeConnectionProjection } from "@/features/connection/adapters/runtimeConnectionProjection";
import { createRuntimeEndpoint } from "@/features/connection/application/runtimeEndpoint";
import { NotificationFeed } from "@/features/notifications/public/notifications";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import {
  type ProviderGate,
  compactionCutpointMarker,
  compactionRecoveryMarker,
  compactionSummaryMarker,
  createProviderGate,
  killedMCPToolName,
  unavailableProviderStatus,
} from "./fakeProvider";
import {
  collectRunEvents,
  defaultTestDeadlineMilliseconds,
  nextRuntimeEvent,
  originalOpeningEvents,
  waitForUserMCPConnection,
  watchGoal,
  watchRuntimeTopic,
  within,
} from "./observation";
import { useRuntimeFixture } from "./fixture";

const compactionCutpointDeadlineMilliseconds = 15_000;

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: restart and crash recovery", () => {
  const fixture = useRuntimeFixture("recovery");

  it("returns the committed rollback input when the next Session read fails", async () => {
    const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    const sessionId = asSessionId(session.id);
    const input = "The input belongs to the committed rollback receipt.";
    const run = await fixture.client.runs.start({
      sessionId,
      input: [{ type: "text", text: input }],
    });
    await collectRunEvents(originalOpeningEvents(run));

    const refreshing = Promise.withResolvers<void>();
    let committed = false;
    const client = fixture.createRuntimeClient(async (url, init) => {
      const method = new Headers(init?.headers).get(HTTP_HEADERS.method);
      if (method === "sessions.snapshot" && committed) {
        refreshing.resolve();
        throw new Error("Session read unavailable after the rollback committed");
      }
      const response = await fetch(url, init);
      if (method === "sessions.rollback") committed = true;
      return response;
    });
    const connection = createRuntimeConnectionProjection();
    const target = () => ({ endpoint: fixture.baseUrl });
    const runtime = {
      capabilities: connection.capabilities,
      status: connection.status,
      endpoint: createRuntimeEndpoint({
        read: target,
        defaultTarget: target,
        replace: () => false,
        reset: () => false,
      }),
    };
    const notifications = new NotificationFeed(() => {});
    const agent = composeAgent(
      () => client,
      createMemoryNavigator({ session: sessionId }),
      runtime,
      connection.stream,
      connection.scope,
      notifications,
    );
    agent.sessions.store.getState().holdOpen(sessionId);
    agent.projection.store.getState().ensureSession(sessionId);
    try {
      await expect(
        agent.access.rollbackSessionToBeforeRun(sessionId, run.result.runId),
      ).resolves.toEqual({ status: "committed", userInput: { parts: [{ kind: "text", text: input }] } });
      await within(refreshing.promise, "Session refresh after the committed rollback");
      await expect(fixture.client.sessions.snapshot(sessionId)).resolves.toMatchObject({ runs: [] });
      await expect
        .poll(() => notifications.snapshot().some(({ level }) => level === "error"))
        .toBe(true);
    } finally {
      agent.dispose();
      notifications.dispose();
      connection.dispose();
      await client.close();
      await fixture.client.sessions.delete(sessionId);
    }
  }, 30_000);

  it("reopens durable reads and event reconciliation after a Runtime process restart", async () => {
    const beforeRestart = await fixture.client.runtime.discover();
    const durable = await fixture.client.sessions.create({ title: "HTTP restart durability" });
    await fixture.client.close();
    fixture.client = undefined;
    await fixture.stopRuntimeProcess();

    await fixture.startRuntimeProcess();
    fixture.client = fixture.createRuntimeClient();
    fixture.negotiated = (await fixture.client.runtime.discover()).capabilities;
    await fixture.awaitManagedOwners();

    const afterRestart = await fixture.client.runtime.discover();
    expect(afterRestart).toMatchObject({
      capabilities: {
        limits: {
          idempotency: {
            namespace: beforeRestart.capabilities.limits.idempotency.namespace,
          },
        },
      },
    });
    expect(afterRestart.serverInfo.instanceId).not.toBe(beforeRestart.serverInfo.instanceId);
    await expect(fixture.client.sessions.get(asSessionId(durable.id))).resolves.toMatchObject({
      id: durable.id,
      title: durable.title,
    });

    const controller = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["sessions.changed"] },
      controller.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();
    const created = await fixture.client.sessions.create({ title: "HTTP restart event" });
    await expect(nextRuntimeEvent(events, "sessions.changed")).resolves.toMatchObject({
      type: "sessions.changed",
      sessionIds: [created.id],
    });
    controller.abort();
    await events.return?.();
  }, 30_000);

  it("keeps the durable compaction winner when the following provider call fails", async () => {
    const openingCall = createProviderGate(compactionCutpointMarker, { requiredTool: "get_goal" });
    const failedCall = createProviderGate(compactionSummaryMarker, {
      failureStatus: unavailableProviderStatus,
      requiredTool: "get_goal",
    });
    let recoveryCall: ProviderGate | undefined;
    fixture.providerGate = openingCall;
    try {
      const session = await fixture.client.sessions.create({
        workspace: { path: fixture.root },
        title: "HTTP provider failure after compaction commit",
      });
      const sessionId = asSessionId(session.id);
      const started = await fixture.client.runs.start({
        sessionId,
        input: [
          {
            type: "text",
            text: `${compactionCutpointMarker} advance the Plan while processing long context.`,
          },
        ],
      });
      const failedEvents = collectRunEvents(originalOpeningEvents(started));
      onTestFinished(async () => {
        await failedEvents;
      }, compactionCutpointDeadlineMilliseconds);
      await within(openingCall.arrived.promise, "the opening model call");
      fixture.providerGate = failedCall;
      openingCall.release.resolve();
      await within(
        Promise.race([
          failedCall.arrived.promise,
          failedEvents.then((events) => {
            throw new Error(`Run ended before compaction: ${JSON.stringify(events.at(-1)?.event)}`);
          }),
        ]),
        "the post-compaction provider failure cutpoint",
        compactionCutpointDeadlineMilliseconds,
      );
      if (!failedCall.request) throw new Error("provider failure gate did not capture its request");
      const cutpointTranscript = JSON.stringify(failedCall.request.messages ?? []);
      expect(cutpointTranscript).toContain(compactionSummaryMarker);
      expect(cutpointTranscript).not.toContain(compactionCutpointMarker);
      expect(failedCall.request.tools?.some((entry) => entry.function?.name === "get_goal")).toBe(
        true,
      );
      const { plan } = await fixture.client.sessions.snapshot(sessionId);
      expect(plan?.state?.revision).toBeGreaterThan(1);

      failedCall.release.resolve();
      const terminalEvents = await failedEvents;
      expect(terminalEvents.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        run: { status: "finished", outcome: { type: "failed" } },
      });

      recoveryCall = createProviderGate(compactionRecoveryMarker, { requiredTool: "get_goal" });
      fixture.providerGate = recoveryCall;
      const recovered = await fixture.client.runs.start({
        sessionId,
        input: [
          {
            type: "text",
            text: `${compactionRecoveryMarker} continue from the only durable history winner.`,
          },
        ],
      });
      const recoveredEvents = collectRunEvents(originalOpeningEvents(recovered));
      onTestFinished(async () => {
        await recoveredEvents;
      }, compactionCutpointDeadlineMilliseconds);
      await within(recoveryCall.arrived.promise, "the provider-failure recovery model call");
      if (!recoveryCall.request) throw new Error("recovery gate did not capture its request");
      const recoveryTranscript = JSON.stringify(recoveryCall.request.messages ?? []);
      expect(recoveryTranscript).toContain(compactionSummaryMarker);
      expect(recoveryTranscript).not.toContain(compactionCutpointMarker);
      expect(recoveryCall.request.tools?.some((entry) => entry.function?.name === "get_goal")).toBe(
        true,
      );
      recoveryCall.release.resolve();
      expect((await recoveredEvents).at(-1)?.event).toMatchObject({
        type: "segment.finished",
        run: { status: "finished", outcome: { type: "completed" } },
      });
      await expect(fixture.client.sessions.snapshot(sessionId)).resolves.toMatchObject({ plan });
    } finally {
      openingCall.release.resolve();
      failedCall.release.resolve();
      recoveryCall?.release.resolve();
      fixture.providerGate = undefined;
    }
  }, 30_000);

  it("keeps the durable compaction winner after SIGKILL before Strategy settlement", async () => {
    const openingCall = createProviderGate(compactionCutpointMarker, { requiredTool: "get_goal" });
    const cutpoint = createProviderGate(compactionSummaryMarker, { requiredTool: "get_goal" });
    let recoveryCall: ProviderGate | undefined;
    fixture.providerGate = openingCall;
    try {
      const session = await fixture.client.sessions.create({
        workspace: { path: fixture.root },
        title: "HTTP SIGKILL after compaction commit",
      });
      const sessionId = asSessionId(session.id);
      const started = await fixture.client.runs.start({
        sessionId,
        input: [
          {
            type: "text",
            text: `${compactionCutpointMarker} reach the compaction crash cutpoint.`,
          },
        ],
      });
      const runId = asRunId(started.result.runId);
      const runEvents = collectRunEvents(originalOpeningEvents(started));
      const killedStream = runEvents.then(
        () => undefined,
        () => undefined,
      );
      onTestFinished(() => killedStream, compactionCutpointDeadlineMilliseconds);
      await within(openingCall.arrived.promise, "the SIGKILL opening model call");
      fixture.providerGate = cutpoint;
      openingCall.release.resolve();
      await within(
        Promise.race([
          cutpoint.arrived.promise,
          runEvents.then((events) => {
            throw new Error(
              `Run ended before the crash cutpoint: ${JSON.stringify(events.at(-1)?.event)}`,
            );
          }),
        ]),
        "the post-compaction SIGKILL cutpoint",
        compactionCutpointDeadlineMilliseconds,
      );
      if (!cutpoint.request) throw new Error("SIGKILL gate did not capture its request");
      const cutpointTranscript = JSON.stringify(cutpoint.request.messages ?? []);
      expect(cutpointTranscript).toContain(compactionSummaryMarker);
      expect(cutpointTranscript).not.toContain(compactionCutpointMarker);
      expect(cutpoint.request.tools?.some((entry) => entry.function?.name === "get_goal")).toBe(
        true,
      );
      const { plan } = await fixture.client.sessions.snapshot(sessionId);
      expect(plan?.state?.revision).toBeGreaterThan(1);

      await fixture.killRuntimeProcess();
      await within(cutpoint.closed.promise, "the compacted provider connection to close");
      cutpoint.release.resolve();
      fixture.providerGate = undefined;
      await within(killedStream, "the compacted Run stream to close after SIGKILL");
      await fixture.client.close();
      fixture.client = undefined;

      await fixture.startRuntimeProcess();
      fixture.client = fixture.createRuntimeClient();
      await fixture.awaitManagedOwners();
      await expect(fixture.client.runs.get(runId)).resolves.toMatchObject({
        status: "finished",
        outcome: { type: "lost", loss: expect.objectContaining({ reason: expect.any(String) }) },
      });

      recoveryCall = createProviderGate(compactionRecoveryMarker, { requiredTool: "get_goal" });
      fixture.providerGate = recoveryCall;
      const recovered = await fixture.client.runs.start({
        sessionId,
        input: [
          {
            type: "text",
            text: `${compactionRecoveryMarker} continue after the lost in-flight Strategy.`,
          },
        ],
      });
      const recoveredEvents = collectRunEvents(originalOpeningEvents(recovered));
      onTestFinished(async () => {
        await recoveredEvents;
      }, compactionCutpointDeadlineMilliseconds);
      await within(recoveryCall.arrived.promise, "the SIGKILL compaction recovery model call");
      if (!recoveryCall.request) throw new Error("SIGKILL recovery gate omitted its request");
      const recoveryTranscript = JSON.stringify(recoveryCall.request.messages ?? []);
      expect(recoveryTranscript).toContain(compactionSummaryMarker);
      expect(recoveryTranscript).not.toContain(compactionCutpointMarker);
      expect(recoveryCall.request.tools?.some((entry) => entry.function?.name === "get_goal")).toBe(
        true,
      );
      recoveryCall.release.resolve();
      expect((await recoveredEvents).at(-1)?.event).toMatchObject({
        type: "segment.finished",
        run: { status: "finished", outcome: { type: "completed" } },
      });
      await expect(fixture.client.sessions.snapshot(sessionId)).resolves.toMatchObject({ plan });
    } finally {
      openingCall.release.resolve();
      cutpoint.release.resolve();
      recoveryCall?.release.resolve();
      fixture.providerGate = undefined;
    }
  }, 30_000);

  it("recovers durable HITL, Plan, Goal, Run and Tool reads after SIGKILL", async () => {
    const beforeKill = await fixture.client.runtime.discover();
    const planSession = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP SIGKILL plan durability",
    });
    const planned = await fixture.client.runs.start({
      sessionId: asSessionId(planSession.id),
      input: [{ type: "text", text: "E2E_PLAN preserve this plan across SIGKILL." }],
    });
    await collectRunEvents(originalOpeningEvents(planned));

    const goalSession = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP SIGKILL Goal and HITL durability",
    });
    const goalSessionId = asSessionId(goalSession.id);
    const waitForGoal = await watchGoal(fixture.client, goalSessionId);
    await fixture.client.goals.start({
      sessionId: goalSessionId,
      objective: "E2E_HITL preserve this Goal wait across SIGKILL.",
    });
    const goal = await waitForGoal("paused");
    expect(goal).toMatchObject({
      status: "paused",
      reason: { code: "awaitingInput" },
    });
    const goalRuns = await fixture.client.runs.list({ sessionId: goalSessionId });
    const waitingRun = goalRuns.data[0];
    if (!waitingRun) throw new Error("SIGKILL fixture omitted the Goal-owned Run");
    const waitingRunId = asRunId(waitingRun.id);
    const pending = await fixture.client.interrupts.list({ rootRunId: waitingRunId });
    const question = pending.data[0]?.interrupts[0];
    if (!question || question.type !== "question") {
      throw new Error("SIGKILL fixture omitted the Goal-owned HITL question");
    }

    const activeGoalSession = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP SIGKILL active Goal recovery",
    });
    const activeGoalSessionId = asSessionId(activeGoalSession.id);
    const gate = createProviderGate("E2E_FORCE_KILL_GOAL");
    fixture.providerGate = gate;
    try {
      await fixture.client.goals.start({
        sessionId: activeGoalSessionId,
        objective: "E2E_FORCE_KILL_GOAL remain active until process death.",
      });
      await within(gate.arrived.promise, "the SIGKILL Goal model request");
      const activeGoalRuns = await fixture.client.runs.list({ sessionId: activeGoalSessionId });
      const activeGoalRun = activeGoalRuns.data.find((run) => run.status === "running");
      if (!activeGoalRun) throw new Error("SIGKILL fixture omitted the active Goal-owned Run");
      const activeGoalRunId = asRunId(activeGoalRun.id);

      await fixture.killRuntimeProcess();
      await within(gate.closed.promise, "the SIGKILL provider connection to close");
      gate.release.resolve();
      fixture.providerGate = undefined;
      await fixture.client.close();
      fixture.client = undefined;

      await fixture.startRuntimeProcess();
      fixture.client = fixture.createRuntimeClient();
      await fixture.awaitManagedOwners();

      const afterKill = await fixture.client.runtime.discover();
      expect(afterKill).toMatchObject({
        capabilities: {
          limits: {
            idempotency: {
              namespace: beforeKill.capabilities.limits.idempotency.namespace,
            },
          },
        },
      });
      expect(afterKill.serverInfo.instanceId).not.toBe(beforeKill.serverInfo.instanceId);
      const planSnapshot = await fixture.client.sessions.snapshot(asSessionId(planSession.id));
      expect(planSnapshot).toMatchObject({
        plan: {
          state: {
            revision: 1,
            steps: [
              { description: "Inspect the runtime contract", status: "completed" },
              { description: "Verify frontend reconciliation", status: "in_progress" },
            ],
          },
        },
        items: expect.arrayContaining([
          expect.objectContaining({
            type: "toolCall",
            status: "completed",
            tool: expect.objectContaining({ name: "set_plan" }),
          }),
        ]),
      });
      await expect(fixture.client.goals.get(goalSessionId)).resolves.toMatchObject({
        status: "paused",
        reason: { code: "awaitingInput" },
      });
      await expect(fixture.client.sessions.snapshot(goalSessionId)).resolves.toMatchObject({
        goal: {
          status: "paused",
          reason: { code: "awaitingInput" },
        },
        runs: [expect.objectContaining({ id: waitingRunId, status: "waiting" })],
        interrupts: [
          expect.objectContaining({
            interrupts: [expect.objectContaining({ itemId: question.itemId, type: "question" })],
          }),
        ],
      });
      await expect(fixture.client.runs.get(activeGoalRunId)).resolves.toMatchObject({
        status: "finished",
        outcome: { type: "lost", loss: expect.objectContaining({ reason: expect.any(String) }) },
      });
      await expect(fixture.client.goals.get(activeGoalSessionId)).resolves.toMatchObject({
        status: "paused",
        reason: { code: "runNotCompleted", runOutcome: "lost" },
        used: { runs: 1 },
      });
      await expect(fixture.client.sessions.snapshot(activeGoalSessionId)).resolves.toMatchObject({
        goal: {
          status: "paused",
          reason: { code: "runNotCompleted", runOutcome: "lost" },
          used: { runs: 1 },
        },
        runs: expect.arrayContaining([
          expect.objectContaining({
            id: activeGoalRunId,
            status: "finished",
            outcome: expect.objectContaining({ type: "lost" }),
          }),
        ]),
      });
      const controller = new AbortController();
      const subscription = await fixture.client.runtimeEvents.subscribe(
        { topics: ["sessions.changed"] },
        controller.signal,
      );
      const events = subscription.events[Symbol.asyncIterator]();
      const created = await fixture.client.sessions.create({
        title: "HTTP SIGKILL recovered event",
      });
      await expect(nextRuntimeEvent(events, "sessions.changed")).resolves.toMatchObject({
        type: "sessions.changed",
        sessionIds: [created.id],
      });
      controller.abort();
      await events.return?.();
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
    }
  }, 30_000);

  it("marks an in-flight Tool incomplete and its Run lost after SIGKILL", async () => {
    const server = "http-e2e-kill";
    const serverId = { origin: { type: "user" as const }, name: server };
    const toolCallMarker = join(fixture.environmentRoot, "sigkill-tool-call-arrived");
    const gate = createProviderGate("E2E_FORCE_KILL_TOOL", { minimumToolResults: 1 });
    let serverCreated = false;
    fixture.providerGate = gate;
    try {
      const connection = await watchRuntimeTopic(fixture.client, "mcp.changed");
      await fixture.client.mcp.create({
        connection: {
          type: "stdio",
          command: process.execPath,
          args: [fixture.mcpFixturePath, `--hold-tool-call=${toolCallMarker}`],
        },
        description: "SIGKILL Tool recovery fixture",
        enabled: true,
        handshakeTimeout: { type: "unbounded" },
        name: server,
      });
      serverCreated = true;
      await fixture.client.approval.setRule({
        tool: { type: "mcp", server: serverId, name: "ping" },
        scope: "global",
        subject: { type: "all" },
        decision: "allow",
      });
      await expect(
        waitForUserMCPConnection(fixture.client, connection.events, server),
      ).resolves.toMatchObject({
        status: { type: "connected", toolCount: 1 },
      });
      await connection.close();

      const session = await fixture.client.sessions.create({
        workspace: { path: fixture.root },
        title: "HTTP SIGKILL active Tool recovery",
      });
      const started = await fixture.client.runs.start({
        sessionId: asSessionId(session.id),
        input: [{ type: "text", text: "E2E_FORCE_KILL_TOOL remain active until process death." }],
      });
      const runId = asRunId(started.result.runId);
      const drain = collectRunEvents(originalOpeningEvents(started)).then(
        () => undefined,
        () => undefined,
      );
      await within(gate.arrived.promise, "the SIGKILL Tool model request");
      gate.release.resolve();
      fixture.providerGate = undefined;

      const deliveredBefore = Date.now() + defaultTestDeadlineMilliseconds;
      while (true) {
        try {
          await lstat(toolCallMarker);
          break;
        } catch (err) {
          if ((err as NodeJS.ErrnoException).code !== "ENOENT") throw err;
          if (Date.now() >= deliveredBefore)
            throw new Error("MCP peer did not receive the Tool call", { cause: err });
        }
        await new Promise((resolve) => setTimeout(resolve, 25));
      }
      const items = await fixture.client.items.list({ scope: { type: "run", runId } });
      expect(items.data).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            type: "toolCall",
            status: "running",
            tool: expect.objectContaining({ name: killedMCPToolName }),
          }),
        ]),
      );

      await fixture.killRuntimeProcess();
      await within(drain, "the killed Tool Run stream to settle");
      await fixture.client.close();
      fixture.client = undefined;

      await fixture.startRuntimeProcess();
      fixture.client = fixture.createRuntimeClient();
      await fixture.awaitManagedOwners();
      await expect(fixture.client.runs.get(runId)).resolves.toMatchObject({
        status: "finished",
        outcome: { type: "lost", loss: expect.objectContaining({ reason: expect.any(String) }) },
      });
      await expect(
        fixture.client.items.list({ scope: { type: "run", runId } }),
      ).resolves.toMatchObject({
        data: expect.arrayContaining([
          expect.objectContaining({
            type: "toolCall",
            status: "incomplete",
            tool: expect.objectContaining({ name: killedMCPToolName }),
          }),
        ]),
      });
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
      if (serverCreated && fixture.client)
        await fixture.client.mcp.delete(serverId).catch(() => undefined);
    }
  }, 30_000);
});
