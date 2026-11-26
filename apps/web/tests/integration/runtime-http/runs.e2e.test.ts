import { parseJSON, stringifyJSON, type JSONValue } from "@frame/sdk/json";
import { describe, expect, it } from "vitest";
import {
  asRunId,
  asSegmentId,
  asSessionId,
  createFlameClient,
  createHttpTransport,
  RpcProtocolError,
} from "@flame/runtime-client";
import { wireMethodRequiresIdempotency, type WireMethodName } from "@flame/runtime-client/methods";
import { HTTP_HEADERS, RPC_HTTP } from "@flame/runtime-client/wire";
import { createProviderGate } from "./fakeProvider";
import {
  collectRunEvents,
  collectSessionRuntimeEvents,
  nextRuntimeEvent,
  originalOpeningEvents,
  within,
} from "./observation";
import { useRuntimeFixture } from "./fixture";
import { MemoryMutationStorage } from "./mutationStorage";

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

const sseCursorLine = /^id: .+\n/m;

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

  it("retains unread Run events after the terminal frame closes the response", async () => {
    const closed = Promise.withResolvers<void>();
    const client = fixture.createRuntimeClient(async (input, init) => {
      if (new Headers(init?.headers).get(HTTP_HEADERS.method) === "runs.start") {
        init?.signal?.addEventListener("abort", () => closed.resolve(), { once: true });
      }
      return fetch(input, init);
    });
    try {
      const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
      const started = await client.runs.start({
        sessionId: asSessionId(session.id),
        input: [{ type: "text", text: "E2E_RUN complete before observation starts." }],
      });
      await within(closed.promise, "the completed Run response to close");
      const events = await collectRunEvents(originalOpeningEvents(started));
      expect(events.at(0)?.event.type).toBe("segment.started");
      expect(events.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        run: { status: "finished", outcome: { type: "completed" } },
      });
    } finally {
      await client.close();
    }
  }, 30_000);

  it.each(["runId", "segmentId"] as const)(
    "refuses a root terminal with another Run's %s",
    async (field) => {
      const foreignSession = await fixture.client.sessions.create();
      const session = await fixture.client.sessions.create();
      const foreign = await fixture.client.runs.start({
        sessionId: asSessionId(foreignSession.id),
        input: [{ type: "text", text: "E2E_RUN complete a separate run." }],
      });
      await within(collectRunEvents(originalOpeningEvents(foreign)), "the separate Run to finish");
      const client = fixture.createRuntimeClient(async (input, init) => {
        const response = await fetch(input, init);
        if (new Headers(init?.headers).get(HTTP_HEADERS.method) !== "runs.start") return response;
        expect(response.headers.get(HTTP_HEADERS.contentType)).toContain(RPC_HTTP.streamMediaType);
        const frames = (await response.text()).split("\n\n");
        const terminal = frames.findIndex((frame) => frame.includes('"type":"segment.finished"'));
        if (terminal < 0) throw new Error("expected the root Run's terminal frame");
        frames[terminal] = frames[terminal]!.replace(/^(data: )(.*)$/m, (_, prefix, encoded) => {
          const notification = parseJSON(encoded) as { params: Record<string, JSONValue> };
          notification.params[field] = foreign.result[field];
          return prefix + stringifyJSON(notification);
        });
        return new Response(frames.join("\n\n"), {
          status: response.status,
          headers: response.headers,
        });
      });
      try {
        const opening = await client.runs.start({
          sessionId: asSessionId(session.id),
          input: [{ type: "text", text: "E2E_RUN finish with its original identity." }],
        });
        await expect(
          within(collectRunEvents(originalOpeningEvents(opening)), "the misaddressed Run frame"),
        ).rejects.toSatisfy(
          (error: unknown) =>
            error instanceof RpcProtocolError &&
            error.violations[0]?.path === `$.${field}` &&
            Boolean(error.requestId),
        );
        await expect(fixture.client.runs.get(asRunId(opening.result.runId))).resolves.toMatchObject(
          { status: "finished", outcome: { type: "completed" } },
        );
      } finally {
        await client.close();
        await fixture.client.sessions.delete(asSessionId(session.id));
        await fixture.client.sessions.delete(asSessionId(foreignSession.id));
      }
    },
    30_000,
  );

  it.each([
    {
      fault: "a mismatched replay cursor",
      path: "$.id",
      change: (frame: string, initialCursorLine: string) =>
        frame.replace(sseCursorLine, initialCursorLine),
    },
    {
      fault: "an omitted replay cursor",
      path: "$.id",
      change: (frame: string) => frame.replace(sseCursorLine, ""),
    },
    {
      fault: "a different event name",
      path: "$.event",
      change: (frame: string) => `event: diagnostic\n${frame}`,
    },
    {
      fault: "empty event data",
      path: "$",
      change: (frame: string) => frame.replace(/^data: .+$/m, "data:"),
    },
  ] as const)(
    "refuses a Run's terminal SSE frame with $fault",
    async ({ change, path }) => {
      const session = await fixture.client.sessions.create();
      const sessionId = asSessionId(session.id);
      const client = fixture.createRuntimeClient(async (input, init) => {
        const response = await fetch(input, init);
        if (new Headers(init?.headers).get(HTTP_HEADERS.method) !== "runs.start") return response;
        expect(response.headers.get(HTTP_HEADERS.contentType)).toContain(RPC_HTTP.streamMediaType);
        const frames = (await response.text()).split("\n\n");
        const firstCursor = frames
          .find((frame) => sseCursorLine.test(frame))
          ?.match(sseCursorLine)?.[0];
        const terminal = frames.findIndex((frame) => frame.includes('"type":"segment.finished"'));
        if (!firstCursor || terminal < 0 || !sseCursorLine.test(frames[terminal]!))
          throw new Error("expected two replayable Run frames");
        frames[terminal] = change(frames[terminal]!, firstCursor);
        return new Response(frames.join("\n\n"), {
          status: response.status,
          headers: response.headers,
        });
      });
      try {
        const opening = await client.runs.start({
          sessionId,
          input: [{ type: "text", text: "E2E_RUN finish with replayable events." }],
        });
        await expect(
          within(collectRunEvents(originalOpeningEvents(opening)), "the malformed SSE frame"),
        ).rejects.toSatisfy(
          (error: unknown) =>
            error instanceof RpcProtocolError &&
            error.violations[0]?.path === path &&
            Boolean(error.requestId),
        );
        await expect(fixture.client.runs.get(asRunId(opening.result.runId))).resolves.toMatchObject(
          { status: "finished", outcome: { type: "completed" } },
        );
      } finally {
        await client.close();
        await fixture.client.sessions.delete(sessionId);
      }
    },
    30_000,
  );

  it.each([
    { fault: "notification before acknowledgement", phase: "opening" },
    { fault: "duplicate acknowledgement", phase: "observation" },
  ] as const)(
    "refuses a Run stream with $fault",
    async ({ phase }) => {
      const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
      const sessionId = asSessionId(session.id);
      const gate = createProviderGate("E2E_SUBSCRIBE");
      fixture.providerGate = gate;
      const subscribed = Promise.withResolvers<void>();
      const client = fixture.createRuntimeClient(async (input, init) => {
        const response = await fetch(input, init);
        expect(response.headers.get(HTTP_HEADERS.contentType)).toContain(RPC_HTTP.streamMediaType);
        subscribed.resolve();
        const [acknowledgement, firstEvent, ...tail] = (await response.text()).split("\n\n");
        if (!acknowledgement?.includes('"result"') || !firstEvent?.includes('"method"'))
          throw new Error("expected Runtime acknowledgement followed by a Run event");
        const frames =
          phase === "opening"
            ? [firstEvent, acknowledgement, ...tail]
            : [acknowledgement, acknowledgement, firstEvent, ...tail];
        return new Response(frames.join("\n\n"), {
          status: response.status,
          headers: response.headers,
        });
      });
      try {
        const started = await fixture.client.runs.start({
          sessionId,
          input: [{ type: "text", text: "E2E_SUBSCRIBE finish after the subscriber opens." }],
        });
        const runId = asRunId(started.result.runId);
        const segmentId = asSegmentId(started.result.segmentId);
        await within(gate.arrived.promise, "the held model request");
        const observation = client.runs.subscribe({ runId, segmentId }).then(
          async ({ events }) => {
            try {
              await collectRunEvents(events);
              return { type: "completed" };
            } catch (error) {
              return { type: "observation", error };
            }
          },
          (error: unknown) => {
            subscribed.reject(error);
            return { type: "opening", error };
          },
        );
        try {
          await within(subscribed.promise, "Runtime's response to the subscription");
          gate.release.resolve();
          expect(await within(observation, "the invalid observation to settle")).toEqual({
            type: phase,
            error: expect.any(RpcProtocolError),
          });
        } finally {
          gate.release.resolve();
          const events = await collectRunEvents(originalOpeningEvents(started));
          expect(events.at(-1)?.event).toMatchObject({
            type: "segment.finished",
            run: { status: "finished", outcome: { type: "completed" } },
          });
        }
      } finally {
        gate.release.resolve();
        fixture.providerGate = undefined;
        await client.close();
        await fixture.client.sessions.delete(sessionId);
      }
    },
    30_000,
  );

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

  it.each([
    { timing: "after its nomination is recorded", waitForOriginalDispatch: true },
    { timing: "before the original client reads the receipt", waitForOriginalDispatch: false },
  ])(
    "recovers a Steer command $timing",
    async ({ waitForOriginalDispatch }) => {
      const gate = createProviderGate("E2E_SHARED_STEER");
      fixture.providerGate = gate;
      const lookupOpened = Promise.withResolvers<void>();
      const allowLookup = Promise.withResolvers<void>();
      const dispatchOpened = Promise.withResolvers<void>();
      const allowDispatch = Promise.withResolvers<void>();
      const mutationStorage = new MemoryMutationStorage();
      const openClient = (fetch?: typeof globalThis.fetch) =>
        createFlameClient(createHttpTransport({ baseUrl: fixture.baseUrl, fetch }), {
          capabilities: () => fixture.negotiated,
          mutationStorage,
        });
      const original = openClient(async (input, init) => {
        const method = new Headers(init?.headers).get(HTTP_HEADERS.method);
        if (method === "runs.getAcceptance") {
          lookupOpened.resolve();
          await allowLookup.promise;
        } else if (method === "runs.steer") {
          dispatchOpened.resolve();
          await allowDispatch.promise;
        }
        return fetch(input, init);
      });
      const recovering = openClient();
      try {
        const session = await fixture.client.sessions.create({ title: "Shared Steer recovery" });
        const started = await fixture.client.runs.start({
          sessionId: asSessionId(session.id),
          input: [{ type: "text", text: "E2E_SHARED_STEER hold the model boundary." }],
        });
        await within(gate.arrived.promise, "the held model request");
        const runId = asRunId(started.result.runId);
        const steerText = "Apply the recovered instruction once.";
        const first = original.runs.steer(runId, asSegmentId(started.result.segmentId), [
          { type: "text", text: steerText },
        ]);
        const originalResult = first.then(
          (value) => ({ value }),
          (error: unknown) => ({ error }),
        );
        await within(lookupOpened.promise, "the original Steer lookup");
        const pending = (await recovering.runs.recoverCommands()).find(
          (command) => command.idempotencyKey === first.idempotencyKey,
        );
        if (pending?.operation !== "steer") throw new Error("expected the original Steer");
        expect(pending.userItemId).toBeNull();
        expect(pending.observation.type).toBe("notRecorded");

        if (waitForOriginalDispatch) {
          allowLookup.resolve();
          await within(dispatchOpened.promise, "the original Steer dispatch");
        }
        const retry = pending.retry();
        expect(retry.idempotencyKey).toBe(first.idempotencyKey);
        const accepted = await retry;
        const nominated = await within(retry.inputIdentity, "the Runtime's input nomination");
        expect(accepted).toMatchObject({ type: "accepted", userItemId: nominated });
        expect(mutationStorage.size).toBe(0);

        allowLookup.resolve();
        allowDispatch.resolve();
        expect(await originalResult).toEqual({ value: accepted });
        await expect(first.inputIdentity).resolves.toBe(nominated);
        expect(mutationStorage.size).toBe(0);
        gate.release.resolve();
        const events = await collectRunEvents(originalOpeningEvents(started));
        const inputs = events.flatMap(({ event }) =>
          event.type === "item.completed" &&
          event.item.type === "userMessage" &&
          event.item.content?.some((block) => block.type === "text" && block.text === steerText)
            ? [event.item.id]
            : [],
        );
        expect(inputs).toEqual([nominated]);
      } finally {
        allowLookup.resolve();
        allowDispatch.resolve();
        gate.release.resolve();
        fixture.providerGate = undefined;
        await Promise.all([original.close(), recovering.close()]);
      }
    },
    30_000,
  );

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
        error: { data: { problem: { type: "invalid_params" } } },
      });
    }
  }, 30_000);
});
