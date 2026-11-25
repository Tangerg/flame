import { describe, expect, it } from "vitest";
import { asRunId, asSegmentId, asSessionId, RpcProtocolError } from "@flame/runtime-client";
import { wireMethodRequiresIdempotency, type WireMethodName } from "@flame/runtime-client/methods";
import { createProviderGate } from "./fakeProvider";
import {
  collectRunEvents,
  collectSessionRuntimeEvents,
  nextRuntimeEvent,
  originalOpeningEvents,
  within,
} from "./observation";
import { useRuntimeFixture } from "./fixture";

const unconfirmedModelOutcome = {
  type: "lost",
  loss: expect.objectContaining({ reason: "unresolvedEffects" }),
  unresolvedEffects: [
    expect.objectContaining({
      cause: "termination",
      detail: expect.stringContaining("model provider: exchange is unconfirmed"),
    }),
  ],
};

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: runs", () => {
  const fixture = useRuntimeFixture("runs");

  it("streams a complete run and reconciles its durable state", async () => {
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP run lifecycle",
    });
    const started = await fixture.client.runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_RUN complete without tools." }],
    });
    const runId = asRunId(started.result.runId);
    const events = await collectRunEvents(originalOpeningEvents(started));

    expect(events.at(0)?.event.type).toBe("segment.started");
    expect(events.some((event) => event.event.type === "item.delta")).toBe(true);
    expect(events.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "finished", outcome: { type: "completed" } },
    });
    await expect(fixture.client.runs.get(runId)).resolves.toMatchObject({
      id: runId,
      sessionId: session.id,
      status: "finished",
      outcome: { type: "completed" },
    });
    await expect(
      fixture.client.items.list({ scope: { type: "session", sessionId: session.id } }),
    ).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({ type: "userMessage", runId: started.result.runId }),
        expect.objectContaining({ type: "agentMessage", runId: started.result.runId }),
      ]),
    });
    await expect(fixture.client.usage.session(asSessionId(session.id))).resolves.toMatchObject({
      total: { inputTokens: 8, outputTokens: 4 },
      coverage: { calls: 1, completedCalls: 1, reportedCalls: 1 },
      byModel: [
        expect.objectContaining({
          key: "openai-compatible/e2e-model",
          total: { inputTokens: 8, outputTokens: 4 },
        }),
      ],
    });
  }, 30_000);

  it("refuses Run notifications before the subscription acknowledgement", async () => {
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
    });
    const sessionId = asSessionId(session.id);
    const gate = createProviderGate("E2E_SUBSCRIBE");
    fixture.providerGate = gate;
    const subscribed = Promise.withResolvers<void>();
    const client = fixture.createRuntimeClient(async (input, init) => {
      const response = await fetch(input, init);
      if (!response.headers.get("Content-Type")?.includes("text/event-stream"))
        throw new Error("expected an event stream from Runtime");
      subscribed.resolve();
      const [responseFrame, firstEvent, ...tail] = (await response.text()).split("\n\n");
      if (!responseFrame?.includes('"result"') || !firstEvent?.includes('"method"'))
        throw new Error("expected Runtime acknowledgement followed by a Run event");
      return new Response([firstEvent, responseFrame, ...tail].join("\n\n"), {
        status: response.status,
        headers: response.headers,
      });
    });
    try {
      const started = await fixture.client.runs.start({
        sessionId,
        input: [{ type: "text", text: "E2E_SUBSCRIBE finish after the subscriber opens." }],
      });
      await within(gate.arrived.promise, "the held model request");
      const observation = client.runs
        .subscribe({
          runId: asRunId(started.result.runId),
          segmentId: asSegmentId(started.result.segmentId),
        })
        .then(
          () => ({ type: "accepted" }),
          (error: unknown) => {
            subscribed.reject(error);
            return { type: "refused", error };
          },
        );
      try {
        await within(subscribed.promise, "Runtime's response to the subscription");
        gate.release.resolve();
        expect(await observation).toEqual({ type: "refused", error: expect.any(RpcProtocolError) });
      } finally {
        gate.release.resolve();
        await collectRunEvents(originalOpeningEvents(started));
      }
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
      await client.close();
      await fixture.client.sessions.delete(sessionId);
    }
  }, 30_000);

  it("reattaches to a live segment before releasing the in-flight model", async () => {
    const gate = createProviderGate("E2E_SUBSCRIBE");
    fixture.providerGate = gate;
    try {
      const session = await fixture.client.sessions.create({
        workspace: { path: fixture.root },
        title: "HTTP run reattach",
      });
      const started = await fixture.client.runs.start({
        sessionId: asSessionId(session.id),
        input: [{ type: "text", text: "E2E_SUBSCRIBE hold the model request." }],
      });
      await within(gate.arrived.promise, "the reattach model request");

      const attached = await fixture.client.runs.subscribe({
        runId: asRunId(started.result.runId),
        segmentId: asSegmentId(started.result.segmentId),
      });
      expect(attached.result).toMatchObject({
        runId: started.result.runId,
        segmentId: started.result.segmentId,
        headEventId: expect.stringMatching(/^evt_/),
      });

      gate.release.resolve();
      const [openingEvents, attachedEvents] = await within(
        Promise.all([
          collectRunEvents(originalOpeningEvents(started)),
          collectRunEvents(attached.events),
        ]),
        "both run streams to finish",
      );
      expect(openingEvents.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        run: { status: "finished", outcome: { type: "completed" } },
      });
      expect(attachedEvents.some((event) => event.event.type === "item.delta")).toBe(true);
      expect(attachedEvents.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        run: { status: "finished", outcome: { type: "completed" } },
      });
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
    }
  }, 30_000);

  it("applies steering only after the in-flight model boundary", async () => {
    const gate = createProviderGate("E2E_STEER");
    fixture.providerGate = gate;
    try {
      const session = await fixture.client.sessions.create({
        workspace: { path: fixture.root },
        title: "HTTP run steering",
      });
      const started = await fixture.client.runs.start({
        sessionId: asSessionId(session.id),
        input: [{ type: "text", text: "E2E_STEER hold the first model boundary." }],
      });
      await within(gate.arrived.promise, "the steer model request");

      const accepted = await fixture.client.runs.steer(
        asRunId(started.result.runId),
        asSegmentId(started.result.segmentId),
        [{ type: "text", text: "Include the queued steering instruction." }],
      );
      gate.release.resolve();

      const events = await within(
        collectRunEvents(originalOpeningEvents(started)),
        "the steered run stream to finish",
      );
      const steeredMessage = events.find(
        (event) =>
          event.event.type === "item.completed" &&
          event.event.item.type === "userMessage" &&
          event.event.item.content?.some(
            (block) =>
              block.type === "text" && block.text === "Include the queued steering instruction.",
          ),
      );
      expect(steeredMessage?.event).toMatchObject({
        type: "item.completed",
        item: { id: accepted.userItemId },
      });
      expect(events.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        run: { status: "finished", outcome: { type: "completed" } },
      });
      await expect(
        fixture.client.items.list({ scope: { type: "run", runId: started.result.runId } }),
      ).resolves.toMatchObject({
        data: expect.arrayContaining([
          expect.objectContaining({
            type: "userMessage",
            content: expect.arrayContaining([
              { type: "text", text: "Include the queued steering instruction." },
            ]),
          }),
        ]),
      });
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
    }
  }, 30_000);

  it("preserves an unconfirmed model outcome when cancellation closes its stream", async () => {
    const gate = createProviderGate("E2E_CANCEL");
    fixture.providerGate = gate;
    try {
      const session = await fixture.client.sessions.create({
        workspace: { path: fixture.root },
        title: "HTTP run cancellation",
      });
      const started = await fixture.client.runs.start({
        sessionId: asSessionId(session.id),
        input: [{ type: "text", text: "E2E_CANCEL hold the model request." }],
      });
      const runId = asRunId(started.result.runId);
      await within(gate.arrived.promise, "the cancel model request");

      await expect(
        fixture.client.runs.cancel(runId, "HTTP cancellation E2E"),
      ).rejects.toMatchObject({
        data: { type: "run_finished" },
      });
      await within(gate.closed.promise, "the canceled provider connection to close");

      const events = await within(
        collectRunEvents(originalOpeningEvents(started)),
        "the canceled run stream to finish",
      );
      expect(events.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        run: { status: "finished", outcome: unconfirmedModelOutcome },
      });
      await expect(fixture.client.runs.get(runId)).resolves.toMatchObject({
        id: runId,
        status: "finished",
        outcome: unconfirmedModelOutcome,
      });
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
    }
  }, 30_000);

  it("forks a visible durable conversation at the selected run boundary", async () => {
    const source = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP fork source",
    });
    const started = await fixture.client.runs.start({
      sessionId: asSessionId(source.id),
      input: [{ type: "text", text: "E2E_FORK preserve this visible turn." }],
    });
    const forkRunEvents = await collectRunEvents(originalOpeningEvents(started));
    expect(forkRunEvents.filter((event) => event.event.type === "item.completed")).toHaveLength(2);
    const sourceRuns = await fixture.client.runs
      .list({ sessionId: asSessionId(source.id) })
      .autoPagingToArray();
    const sourceItems = await fixture.client.items
      .list({ scope: { type: "session", sessionId: asSessionId(source.id) } })
      .autoPagingToArray();
    expect(sourceRuns).toHaveLength(1);
    expect(sourceRuns[0]?.metrics.steps).toBe(1);
    expect(sourceItems).toHaveLength(2);

    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["sessions.changed", "runs.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const fork = await fixture.client.sessions.fork({
      sessionId: asSessionId(source.id),
      fromRunId: asRunId(started.result.runId),
      title: "HTTP visible fork",
    });
    await expect(nextRuntimeEvent(runtimeEvents, "sessions.changed")).resolves.toMatchObject({
      type: "sessions.changed",
      sessionIds: [fork.id],
    });
    await expect(nextRuntimeEvent(runtimeEvents, "runs.changed")).resolves.toMatchObject({
      type: "runs.changed",
      sessionIds: [fork.id],
      runIds: [expect.not.stringMatching(started.result.runId)],
    });
    const forkRuns = await fixture.client.runs
      .list({ sessionId: asSessionId(fork.id) })
      .autoPagingToArray();
    expect(forkRuns).toHaveLength(1);
    expect(forkRuns[0]).toMatchObject({ sessionId: fork.id, status: "finished" });
    expect(forkRuns[0]?.id).not.toBe(started.result.runId);
    const forkItems = await fixture.client.items
      .list({ scope: { type: "session", sessionId: asSessionId(fork.id) } })
      .autoPagingToArray();
    expect(forkItems).toHaveLength(2);
    expect(forkItems).toEqual([
      expect.objectContaining({
        type: "userMessage",
        content: [{ type: "text", text: "E2E_FORK preserve this visible turn." }],
      }),
      expect.objectContaining({
        type: "agentMessage",
        content: [{ type: "text", text: "HTTP runtime lifecycle complete." }],
      }),
    ]);
    expect(forkItems.every((item) => item.runId === forkRuns[0]?.id)).toBe(true);

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("round-trips exports, rollback state and aggregate invalidations", async () => {
    const source = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP transfer lifecycle",
    });
    const first = await fixture.client.runs.start({
      sessionId: asSessionId(source.id),
      input: [{ type: "text", text: "E2E_PLAN preserve this plan in the archive." }],
    });
    await collectRunEvents(originalOpeningEvents(first));
    const second = await fixture.client.runs.start({
      sessionId: asSessionId(source.id),
      input: [{ type: "text", text: "E2E_TRANSFER remove and restore this turn." }],
    });
    await collectRunEvents(originalOpeningEvents(second));

    const jsonExport = await fixture.client.sessions.export(asSessionId(source.id), "json");
    if (!jsonExport.artifact) throw new Error("JSON export omitted its session artifact");
    expect(jsonExport).toMatchObject({ format: "json" });
    expect(jsonExport).not.toHaveProperty("markdown");
    expect(jsonExport.artifact).toMatchObject({
      session: { id: source.id, title: "HTTP transfer lifecycle" },
      runs: [{ id: first.result.runId }, { id: second.result.runId }],
      plan: [
        { description: "Inspect the runtime contract", status: "completed" },
        { description: "Verify frontend reconciliation", status: "in_progress" },
      ],
    });
    expect(jsonExport.artifact.items.length).toBeGreaterThanOrEqual(5);

    const markdownExport = await fixture.client.sessions.export(asSessionId(source.id), "md");
    expect(markdownExport).toEqual({
      format: "md",
      markdown: expect.stringContaining("# HTTP transfer lifecycle"),
    });
    expect(markdownExport.markdown).toContain("E2E_PLAN preserve this plan in the archive.");
    expect(markdownExport.markdown).toContain("E2E_TRANSFER remove and restore this turn.");

    const aggregateTopics = [
      "sessions.changed",
      "runs.changed",
      "interrupts.changed",
      "goals.changed",
      "plan.changed",
    ] as const;
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: [...aggregateTopics] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const rolledBack = await fixture.client.sessions.rollback({
      sessionId: asSessionId(source.id),
      toRunId: asRunId(first.result.runId),
      expectedLastRootRunId: second.result.runId,
      expectedWorkspacePath: source.workspace.ref.path,
      restoreType: "history",
    });
    expect(rolledBack.droppedRuns).toEqual([
      expect.objectContaining({
        run: expect.objectContaining({ id: second.result.runId }),
        userInput: [{ type: "text", text: "E2E_TRANSFER remove and restore this turn." }],
      }),
    ]);
    expect(
      (await collectSessionRuntimeEvents(runtimeEvents, aggregateTopics, source.id)).map(
        (event) => event.type,
      ),
    ).toEqual(aggregateTopics);
    const rolledRuns = await fixture.client.runs
      .list({ sessionId: asSessionId(source.id) })
      .autoPagingToArray();
    const rolledItems = await fixture.client.items
      .list({ scope: { type: "session", sessionId: asSessionId(source.id) } })
      .autoPagingToArray();
    expect(rolledRuns.map((run) => run.id)).toEqual([first.result.runId]);
    expect(rolledItems.every((item) => item.runId === first.result.runId)).toBe(true);
    await expect(fixture.client.plan.get(asSessionId(source.id))).resolves.toMatchObject({
      state: { steps: jsonExport.artifact.plan },
    });

    const imported = await fixture.client.sessions.import(jsonExport.artifact);
    expect(imported.session.id).toBe(source.id);
    expect(imported.session.revision).toBeGreaterThan(rolledBack.session.revision);
    expect(
      (await collectSessionRuntimeEvents(runtimeEvents, aggregateTopics, source.id)).map(
        (event) => event.type,
      ),
    ).toEqual(aggregateTopics);
    const restoredRuns = await fixture.client.runs
      .list({ sessionId: asSessionId(source.id) })
      .autoPagingToArray();
    const restoredItems = await fixture.client.items
      .list({ scope: { type: "session", sessionId: asSessionId(source.id) } })
      .autoPagingToArray();
    expect(restoredRuns.map((run) => run.id)).toEqual([second.result.runId, first.result.runId]);
    expect(restoredItems).toHaveLength(jsonExport.artifact.items.length);
    await expect(fixture.client.plan.get(asSessionId(source.id))).resolves.toMatchObject({
      state: { steps: jsonExport.artifact.plan },
    });

    const continued = await fixture.client.runs.start({
      sessionId: asSessionId(source.id),
      input: [{ type: "text", text: "E2E_TRANSFER continue after import." }],
    });
    const continuedEvents = await collectRunEvents(originalOpeningEvents(continued));
    expect(continuedEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "finished", outcome: { type: "completed" } },
    });

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("aggregates durable usage and accepts write-only feedback", async () => {
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP usage and feedback",
    });
    const started = await fixture.client.runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_USAGE produce metered output." }],
    });
    await collectRunEvents(originalOpeningEvents(started));
    await expect(fixture.client.runs.get(asRunId(started.result.runId))).resolves.toMatchObject({
      provider: "openai-compatible",
      model: "e2e-model",
      status: "finished",
    });
    const items = await fixture.client.items
      .list({ scope: { type: "run", runId: asRunId(started.result.runId) } })
      .autoPagingToArray();
    const agentItem = items.find((item) => item.type === "agentMessage");
    if (!agentItem) throw new Error("metered run omitted its agent item");

    await expect(
      fixture.client.feedback.create({
        sessionId: session.id,
        runId: started.result.runId,
        itemId: agentItem.id,
        rating: "positive",
        text: "HTTP feedback marker",
      }),
    ).resolves.toBeUndefined();

    const usage = await fixture.client.usage.session(asSessionId(session.id));
    expect(usage.byModel).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          key: "openai-compatible/e2e-model",
          coverage: expect.objectContaining({ calls: 1 }),
        }),
      ]),
    );
    expect(usage.total.inputTokens).toBeGreaterThan(0);
    expect(usage.total.outputTokens).toBeGreaterThan(0);

    const summary = await fixture.client.usage.summary({ sinceDays: 30 });
    expect(summary).toMatchObject({
      total: {
        inputTokens: expect.any(Number),
        outputTokens: expect.any(Number),
      },
      sessions: expect.any(Number),
      runs: expect.any(Number),
      byProvider: expect.arrayContaining([
        expect.objectContaining({
          key: "openai-compatible",
          coverage: expect.objectContaining({ calls: expect.any(Number) }),
        }),
      ]),
      byModel: expect.arrayContaining([
        expect.objectContaining({
          key: "openai-compatible/e2e-model",
          coverage: expect.objectContaining({ calls: expect.any(Number) }),
        }),
      ]),
    });
    expect(summary.total.inputTokens).toBeGreaterThan(0);
    expect(summary.total.outputTokens).toBeGreaterThan(0);
    expect(summary.sessions).toBeGreaterThan(0);
    expect(summary.runs).toBeGreaterThan(0);
    for (const { method, params } of [
      { method: "feedback.create", params: {} },
      { method: "usage.summary", params: { sinceDays: -1 } },
    ] satisfies Array<{ method: WireMethodName; params: unknown }>) {
      const response = await fetch(`${fixture.baseUrl}/v2/rpc`, {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          ...(wireMethodRequiresIdempotency(method)
            ? { "Idempotency-Key": crypto.randomUUID() }
            : {}),
        },
        body: JSON.stringify({ jsonrpc: "2.0", id: method, method, params }),
      });
      expect(response.status).toBe(200);
      await expect(response.json()).resolves.toMatchObject({
        error: { data: { type: "invalid_params" } },
      });
    }
  }, 30_000);
});
