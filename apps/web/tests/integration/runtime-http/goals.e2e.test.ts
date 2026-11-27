import { describe, expect, it } from "vitest";
import { asRunId, asSessionId, RpcError } from "@flame/runtime-client";
import { createProviderGate } from "./fakeProvider";
import { collectRunEvents, originalOpeningEvents, watchGoal, within } from "./observation";
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

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: Goals", () => {
  const fixture = useRuntimeFixture("goals");

  it("fences every Goal mutation to the incarnation observed by its caller", async () => {
    const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    const sessionId = asSessionId(session.id);
    const other = fixture.createSharedClient();
    const oldGate = createProviderGate("E2E_ORIGINAL_GOAL_TARGET");
    const newGate = createProviderGate("E2E_REPLACEMENT_GOAL_TARGET");
    fixture.providerGate = oldGate;
    try {
      const original = await fixture.client.goals.start({
        sessionId,
        objective: "E2E_ORIGINAL_GOAL_TARGET an earlier goal the user saw.",
      });
      await within(oldGate.arrived.promise, "the first Goal's model request");
      await other.goals.stop(original);
      oldGate.release.resolve();

      const objective = "E2E_REPLACEMENT_GOAL_TARGET a different goal by another client.";
      fixture.providerGate = newGate;
      const isGoalConflict = (error: unknown) =>
        error instanceof RpcError && error.data.type === "revision_conflict";
      await expect(other.goals.start({ sessionId, objective })).rejects.toSatisfy(isGoalConflict);
      const replacement = await other.goals.start({
        sessionId,
        objective,
        replaceIncarnationId: original.incarnationId,
      });
      expect(replacement.incarnationId).not.toBe(original.incarnationId);
      await within(newGate.arrived.promise, "the replacement Goal's model request");
      await expect(fixture.client.goals.stop(original)).rejects.toSatisfy(isGoalConflict);
      await expect(fixture.client.goals.resume(original)).rejects.toSatisfy(isGoalConflict);
      await expect(fixture.client.goals.clear(original)).rejects.toSatisfy(isGoalConflict);
      await expect(
        fixture.client.goals.update({
          sessionId,
          incarnationId: original.incarnationId,
          objective: "Edit an earlier goal.",
        }),
      ).rejects.toSatisfy(isGoalConflict);
      const staleReplacement = {
        sessionId,
        objective: "Replace an earlier goal.",
        replaceIncarnationId: original.incarnationId,
      };
      await expect(fixture.client.goals.start(staleReplacement)).rejects.toSatisfy(isGoalConflict);
      await expect(other.goals.get(sessionId)).resolves.toMatchObject({
        incarnationId: replacement.incarnationId,
        objective,
        status: "active",
      });

      await other.goals.clear(replacement);
      await expect(fixture.client.goals.clear(replacement)).resolves.toBeUndefined();
      await expect(fixture.client.goals.start(staleReplacement)).rejects.toSatisfy(isGoalConflict);
      await expect(other.goals.get(sessionId)).resolves.toBeNull();
    } finally {
      oldGate.release.resolve();
      newGate.release.resolve();
      fixture.providerGate = undefined;
      const remaining = await other.goals.get(sessionId);
      if (remaining) await other.goals.clear(remaining);
      await other.close();
      await fixture.client.sessions.delete(sessionId);
    }
  }, 30_000);

  it("publishes Goal changes before and after the model-reported outcome settles", async () => {
    const gate = createProviderGate("E2E_GOAL_SETTLEMENT", { minimumToolResults: 1 });
    fixture.providerGate = gate;
    try {
      const session = await fixture.client.sessions.create({
        workspace: { path: fixture.root },
        title: "HTTP goal settlement lifecycle",
      });
      const sessionId = asSessionId(session.id);
      const waitForGoal = await watchGoal(fixture.client, sessionId);

      await expect(
        fixture.client.goals.start({
          sessionId,
          objective: "E2E_GOAL_SETTLEMENT expose the terminal settlement window.",
        }),
      ).resolves.toMatchObject({ sessionId, status: "active", used: { runs: 0 } });
      await waitForGoal();

      await within(gate.arrived.promise, "the post-outcome model request");
      const completing = await waitForGoal("completing");
      expect(completing).toMatchObject({
        sessionId: session.id,
        status: "completing",
      });
      expect(completing).not.toHaveProperty("reason");

      gate.release.resolve();
      fixture.providerGate = undefined;
      await waitForGoal(null);
      await expect(fixture.client.runs.list({ sessionId })).resolves.toMatchObject({
        data: [
          expect.objectContaining({
            sessionId,
            status: "finished",
            outcome: { type: "completed" },
          }),
        ],
      });
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
    }
  }, 30_000);

  it("parks and resumes a goal run through the negotiated HITL capability", async () => {
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP goal HITL lifecycle",
    });
    const sessionId = asSessionId(session.id);
    const waitForGoal = await watchGoal(fixture.client, sessionId);
    const started = await fixture.client.goals.start({
      sessionId,
      objective: "E2E_HITL E2E_GOAL_AFTER_INPUT ask for input, then complete the goal.",
    });
    expect(started).toMatchObject({
      sessionId: session.id,
      status: "active",
      used: { runs: 0 },
    });

    const current = await waitForGoal("paused");
    expect(current).toMatchObject({
      sessionId: session.id,
      status: "paused",
      reason: { code: "awaitingInput" },
      used: { runs: 0 },
    });

    const runs = await fixture.client.runs.list({ sessionId });
    expect(runs.data).toEqual([
      expect.objectContaining({
        sessionId: session.id,
        status: "waiting",
      }),
    ]);
    const ownedRun = runs.data[0];
    if (!ownedRun) throw new Error("goal-owned waiting run disappeared");
    const runId = asRunId(ownedRun.id);
    const pending = await fixture.client.interrupts.list({ rootRunId: runId });
    const question = pending.data[0]?.interrupts[0];
    if (!question || question.type !== "question") {
      throw new Error("goal-owned run did not persist its question interrupt");
    }

    await expect(fixture.client.goals.resume(started)).resolves.toMatchObject({
      sessionId: session.id,
      status: "active",
    });

    const resumed = await fixture.client.runs.resume({
      runId,
      responses: [
        {
          itemId: question.itemId,
          response: { type: "answer", answers: [["Yes"]] },
        },
      ],
    });
    const resumeEvents = await collectRunEvents(originalOpeningEvents(resumed));
    expect(resumeEvents.slice(0, 2).map((frame) => frame.event)).toMatchObject([
      { type: "segment.started" },
      {
        type: "item.completed",
        item: { type: "question", id: question.itemId, question: { answers: [["Yes"]] } },
      },
    ]);
    expect(resumeEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "finished", outcome: { type: "completed" } },
    });
    await expect(fixture.client.interrupts.list({ rootRunId: runId })).resolves.toMatchObject({
      data: [],
    });
    await expect(
      fixture.client.items.list({ scope: { type: "run", runId } }),
    ).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({
          type: "question",
          status: "completed",
          question: expect.objectContaining({ answers: [["Yes"]] }),
        }),
      ]),
    });

    await waitForGoal(null);
  }, 30_000);

  it("stops an active goal, preserves its unconfirmed run and resumes from durable usage", async () => {
    const gate = createProviderGate("E2E_GOAL_STOP");
    fixture.providerGate = gate;
    try {
      const session = await fixture.client.sessions.create({
        workspace: { path: fixture.root },
        title: "HTTP goal stop and resume",
      });
      const sessionId = asSessionId(session.id);
      const started = await fixture.client.goals.start({
        sessionId,
        objective: "E2E_GOAL_STOP exercise stop and resume.",
      });
      expect(started).toMatchObject({ status: "active", used: { runs: 0 } });
      await within(gate.arrived.promise, "the goal-owned model request");

      await expect(fixture.client.goals.stop(started)).resolves.toBeUndefined();
      await within(gate.closed.promise, "the stopped goal provider connection to close");
      gate.release.resolve();
      fixture.providerGate = undefined;
      await expect(fixture.client.goals.get(sessionId)).resolves.toMatchObject({
        sessionId: session.id,
        status: "paused",
        reason: { code: "stoppedByUser" },
        used: { runs: 1 },
      });
      await expect(fixture.client.runs.list({ sessionId })).resolves.toMatchObject({
        data: [
          expect.objectContaining({
            outcome: expect.objectContaining(unconfirmedModelOutcome),
            status: "finished",
          }),
        ],
      });

      const waitForGoal = await watchGoal(fixture.client, sessionId);
      await expect(fixture.client.goals.resume(started)).resolves.toMatchObject({
        sessionId: session.id,
        status: "active",
        used: { runs: 1 },
      });
      await waitForGoal(null);
      const runs = await fixture.client.runs.list({ sessionId });
      expect(runs.data).toHaveLength(2);
      expect(runs.data.filter((run) => run.outcome?.type === "lost")).toHaveLength(1);
      expect(runs.data.filter((run) => run.outcome?.type === "completed")).toHaveLength(1);
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
    }
  }, 30_000);
});
