import { describe, expect, it } from "vitest";
import {
  createFlameClient,
  type FlameClient,
  asRunId,
  asSegmentId,
  asSessionId,
  createHttpTransport,
} from "@flame/runtime-client";
import { PROTOCOL_VERSION, type RequestMeta } from "@flame/runtime-client/wire";
import { type ProviderGate, createProviderGate } from "./fakeProvider";
import { collectRunEvents, nextRuntimeEvent, originalOpeningEvents, within } from "./observation";
import { useRuntimeFixture } from "./fixture";
import { faultStreamOpening, faultUnaryResponse } from "./transportFaults";

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

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: run commands after lost responses", () => {
  const fixture = useRuntimeFixture("lost-run-responses");

  it("keeps one Goal drive across lost start, stop and resume responses", async () => {
    const requestMeta = (): RequestMeta => ({
      protocolVersion: PROTOCOL_VERSION,
      clientInfo: { name: "runtime-http-goal-cutpoint-e2e", version: "1" },
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
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP Goal replay cutpoints",
    });
    const sessionId = asSessionId(session.id);
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["goals.changed"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();
    const firstGate = createProviderGate("E2E_GOAL_REPLAY_CUTPOINT");
    let resumedGate: ProviderGate | undefined;
    fixture.providerGate = firstGate;

    try {
      await expect(
        callAfterCommitLoss("goals.start", (faultClient) =>
          faultClient.goals.start({
            sessionId,
            objective: "E2E_GOAL_REPLAY_CUTPOINT preserve one drive per command.",
          }),
        ),
      ).resolves.toMatchObject({ status: "active", used: { runs: 0 } });
      await within(firstGate.arrived.promise, "the replayed Goal's first model request");
      await expect(nextRuntimeEvent(events, "goals.changed")).resolves.toMatchObject({
        type: "goals.changed",
        sessionIds: [session.id],
      });
      await expect(fixture.client.goals.get(sessionId)).resolves.toMatchObject({
        status: "active",
        used: { runs: 0 },
      });
      await expect(fixture.client.runs.list({ sessionId })).resolves.toMatchObject({
        data: [expect.objectContaining({ status: "running" })],
      });

      await expect(
        callAfterCommitLoss("goals.stop", (faultClient) => faultClient.goals.stop(sessionId)),
      ).resolves.toBeUndefined();
      await within(
        firstGate.closed.promise,
        "the replay-stopped Goal provider connection to close",
      );
      firstGate.release.resolve();
      fixture.providerGate = undefined;
      await expect(nextRuntimeEvent(events, "goals.changed")).resolves.toMatchObject({
        type: "goals.changed",
        sessionIds: [session.id],
      });
      await expect(fixture.client.goals.get(sessionId)).resolves.toMatchObject({
        status: "paused",
        reason: { code: "stoppedByUser" },
        used: { runs: 1 },
      });
      await expect(fixture.client.runs.list({ sessionId })).resolves.toMatchObject({
        data: [
          expect.objectContaining({
            status: "finished",
            outcome: expect.objectContaining(unconfirmedModelOutcome),
          }),
        ],
      });

      resumedGate = createProviderGate("E2E_GOAL_REPLAY_CUTPOINT");
      fixture.providerGate = resumedGate;
      await expect(
        callAfterCommitLoss("goals.resume", (faultClient) => faultClient.goals.resume(sessionId)),
      ).resolves.toMatchObject({ status: "active", used: { runs: 1 } });
      await within(resumedGate.arrived.promise, "the replayed Goal's resumed model request");
      await expect(nextRuntimeEvent(events, "goals.changed")).resolves.toMatchObject({
        type: "goals.changed",
        sessionIds: [session.id],
      });
      await expect(fixture.client.goals.get(sessionId)).resolves.toMatchObject({
        status: "active",
        used: { runs: 1 },
      });
      const resumedRuns = await fixture.client.runs.list({ sessionId });
      expect(resumedRuns.data).toHaveLength(2);
      expect(resumedRuns.data.filter((run) => run.status === "running")).toHaveLength(1);
      expect(resumedRuns.data.filter((run) => run.outcome?.type === "lost")).toHaveLength(1);

      await expect(fixture.client.goals.stop(sessionId)).resolves.toBeUndefined();
      await within(resumedGate.closed.promise, "the cleanup Goal provider connection to close");
      await expect(fixture.client.goals.get(sessionId)).resolves.toMatchObject({
        status: "paused",
        used: { runs: 2 },
      });
    } finally {
      firstGate.release.resolve();
      resumedGate?.release.resolve();
      fixture.providerGate = undefined;
      streamController.abort();
      await events.return?.();
    }
  }, 30_000);

  it("consumes one durable HITL answer when the resume stream acknowledgement is lost", async () => {
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP HITL resume replay cutpoint",
    });
    const sessionId = asSessionId(session.id);
    const started = await fixture.client.runs.start({
      sessionId,
      input: [{ type: "text", text: "E2E_HITL ask before replaying the answer." }],
    });
    const runId = asRunId(started.result.runId);
    const startEvents = await collectRunEvents(originalOpeningEvents(started));
    expect(startEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "waiting" },
      interrupts: expect.arrayContaining([expect.anything()]),
    });
    const pending = await fixture.client.interrupts.list({ rootRunId: runId });
    const question = pending.data[0]?.interrupts[0];
    if (!question || question.type !== "question") {
      throw new Error("Runtime did not persist the replay-cutpoint question");
    }

    const gate = createProviderGate("E2E_HITL");
    fixture.providerGate = gate;
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["interrupts.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();
    const fault = faultStreamOpening("runs.resume");
    const faultClient = createFlameClient(
      createHttpTransport({ baseUrl: fixture.baseUrl, fetch: fault.fetch }),
      {
        capabilities: () => fixture.negotiated,
        requestMeta: (): RequestMeta => ({
          protocolVersion: PROTOCOL_VERSION,
          clientInfo: { name: "runtime-http-hitl-resume-cutpoint-e2e", version: "1" },
          clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
        }),
      },
    );

    try {
      const resumed = await faultClient.runs.resume({
        runId,
        responses: [
          {
            itemId: question.itemId,
            response: { type: "answer", answers: [["Yes"]] },
          },
        ],
      });
      expect(fault.attempts()).toBe(1);
      expect(resumed.observation).toEqual({ type: "snapshot", sessionId });
      await within(gate.arrived.promise, "the resumed model request");
      const tail = await fixture.client.runs.subscribe({
        runId,
        segmentId: asSegmentId(resumed.result.segmentId),
        snapshot: true,
      });
      expect(tail.result.snapshot?.interrupts).toEqual([]);
      gate.release.resolve();
      expect(resumed.result.runId).toBe(started.result.runId);
      expect(resumed.result.segmentId).not.toBe(started.result.segmentId);
      const resumeEvents = await collectRunEvents(tail.events);
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
      const items = await fixture.client.items
        .list({ scope: { type: "run", runId } })
        .autoPagingToArray();
      expect(
        items.filter(
          (item) =>
            item.type === "question" &&
            item.status === "completed" &&
            JSON.stringify(item.question?.answers) === JSON.stringify([["Yes"]]),
        ),
      ).toHaveLength(1);
      await expect(fixture.client.runs.list({ sessionId })).resolves.toMatchObject({
        data: [
          expect.objectContaining({
            id: runId,
            status: "finished",
            outcome: { type: "completed" },
          }),
        ],
      });
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
      await faultClient.close();
      streamController.abort();
      await runtimeEvents.return?.();
    }
  }, 30_000);
});
