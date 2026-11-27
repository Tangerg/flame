import { describe, expect, it } from "vitest";
import {
  asRunId,
  asSegmentId,
  asSessionId,
  createFlameClient,
  createHttpTransport,
} from "@flame/runtime-client";
import { HTTP_HEADERS } from "@flame/runtime-client/wire";
import { bindAgentSession } from "@/features/agent/adapters/agentSessionBinding";
import { createProviderGate } from "./fakeProvider";
import { useRuntimeFixture } from "./fixture";
import { MemoryMutationStorage } from "./mutationStorage";
import { collectRunEvents, originalOpeningEvents, within } from "./observation";
import { openWebSession } from "./webSessionFixture";

describe("Web agent Session recovery over Runtime HTTP", () => {
  const fixture = useRuntimeFixture("agent-session-recovery");

  it("publishes one response lifecycle and persists its original answer", async () => {
    const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    const sessionId = asSessionId(session.id);
    const opening = await fixture.client.runs.start({
      sessionId,
      input: [{ type: "text", text: "E2E_HITL ask before continuing." }],
    });
    const rootRunId = asRunId(opening.result.runId);
    const events = await collectRunEvents(originalOpeningEvents(opening));
    expect(events.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      run: { status: "waiting" },
    });
    const durable = await fixture.client.interrupts.list({ rootRunId });
    const question = durable.data[0]?.interrupts[0];
    if (question?.type !== "question") throw new Error("Runtime did not park the question");

    const dispatching = Promise.withResolvers<void>();
    const releaseDispatch = Promise.withResolvers<void>();
    let resumeRequests = 0;
    const client = fixture.createRuntimeClient(async (url, init) => {
      if (new Headers(init?.headers).get(HTTP_HEADERS.method) !== "runs.resume")
        return fetch(url, init);
      resumeRequests++;
      dispatching.resolve();
      await releaseDispatch.promise;
      const response = await fetch(url, init);
      const body = await response.arrayBuffer();
      return new Response(body, { status: response.status, headers: response.headers });
    });

    const web = openWebSession(client, fixture.baseUrl, sessionId);
    const unbind = bindAgentSession(
      web.agent.sessionDependencies(),
      () => client,
      sessionId,
      () => {},
    );
    const { interrupts, view } = web.agent.dependencies;
    const finished = Promise.withResolvers<void>();
    const stopView = view.subscribeSessions((sessions) => {
      if (sessions[sessionId]?.view.runsById[rootRunId]?.status === "finished") finished.resolve();
    });
    const states: string[] = [];
    const stopResponse = interrupts.subscribe(() => {
      states.push(interrupts.responseState(sessionId, question.itemId));
    });
    try {
      const synchronize = view.getSession(sessionId)?.synchronize;
      if (!synchronize) throw new Error("Session is not bound");
      await expect(within(synchronize("after-live"), "waiting Session material")).resolves.toBe(true);
      const ref = { sessionId, rootRunId, itemId: question.itemId };
      const answers = [["Original answer"]];
      expect(interrupts.stage(ref, { type: "answer", answers })).toBe(true);
      await within(dispatching.promise, "the original answer dispatch");
      expect(states).toEqual(["pending"]);
      expect(interrupts.stage(ref, { type: "answer", answers: [["Replacement answer"]] })).toBe(false);
      releaseDispatch.resolve();
      await within(finished.promise, "the resumed Run settling");
      expect(resumeRequests).toBe(1);
      expect(states).toEqual(["pending", "accepted", "unsubmitted"]);
      const items = await client.items
        .list({ scope: { type: "run", runId: rootRunId } })
        .autoPagingToArray();
      const answered = items.find(({ id }) => id === question.itemId);
      if (answered?.type !== "question") throw new Error("Runtime lost the original question");
      expect(answered.question.answers).toEqual(answers);
      await expect(client.interrupts.list({ rootRunId })).resolves.toMatchObject({ data: [] });
    } finally {
      releaseDispatch.resolve();
      stopResponse();
      stopView();
      unbind();
      web.dispose();
      await client.close();
    }
  }, 30_000);

  it.each(["refresh", "explicit retry"] as const)(
    "retires the displayed Start retry when %s confirms the original Run",
    async (settlement) => {
      const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
      const sessionId = asSessionId(session.id);
      let loseAcknowledgement = true;
      let runFinished = false;
      const client = fixture.createRuntimeClient(async (url, init) => {
        const method = new Headers(init?.headers).get(HTTP_HEADERS.method);
        if (method === "runs.getAcceptance" && runFinished && loseAcknowledgement)
          throw new TypeError("the original acceptance read was interrupted");
        const response = await fetch(url, init);
        if (method === "runs.start" && loseAcknowledgement) {
          await response.arrayBuffer();
          runFinished = true;
          throw new TypeError("the original Start response was interrupted");
        }
        return response;
      });
      const web = openWebSession(client, fixture.baseUrl, sessionId);
      const unbind = bindAgentSession(
        web.agent.sessionDependencies(),
        () => client,
        sessionId,
        () => {},
      );
      try {
        const view = web.agent.dependencies.view;
        const entry = view.getSession(sessionId);
        if (!entry?.synchronize || !entry.retryOpening) throw new Error("Session is not bound");
        await expect(
          within(entry.synchronize("after-live"), "initial Session material"),
        ).resolves.toBe(true);
        const original = client.runs.start({
          sessionId,
          input: [{ type: "text", text: "Complete this Start with a lost reply." }],
        });
        await expect(original).rejects.toThrow();
        await expect(
          within(entry.synchronize("after-live"), "the unconfirmed command"),
        ).resolves.toBe(true);
        const pending = view.getSession(sessionId)?.openingCommands[0];
        if (!pending) throw new Error("the original Start was not offered for recovery");
        expect(pending).toMatchObject({
          idempotencyKey: original.idempotencyKey,
          operation: "start",
          receipt: "unknown",
        });
        loseAcknowledgement = false;
        if (settlement === "explicit retry") {
          const retry = entry.retryOpening(pending);
          if (!retry) throw new Error("the current original Start could not be retried");
          await expect(within(retry, "the original Start retry")).resolves.toMatchObject({
            status: "accepted",
            sessionId,
          });
        }
        await expect(
          within(entry.synchronize("after-live"), "the confirmed original command"),
        ).resolves.toBe(true);
        expect(view.getSession(sessionId)?.openingCommands).toEqual([]);
        const staleRetry = entry.retryOpening(pending);
        expect(staleRetry).toBeNull();
        const runs = await fixture.client.runs.list({ sessionId });
        expect(runs.data).toHaveLength(1);
        expect(runs.data[0]?.status).toBe("finished");
      } finally {
        unbind();
        web.dispose();
        await client.close();
      }
    },
    30_000,
  );

  it.each([
    { scenario: "keeps applied input in the coherent transcript", retire: false },
    { scenario: "retains a retired receipt outside the transcript", retire: true },
  ])(
    "$scenario after a pending Steer settles",
    async ({ retire }) => {
      const gate = createProviderGate("E2E_STEER_RECOVERY");
      fixture.providerGate = gate;
      const mutationStorage = new MemoryMutationStorage();
      const reachedDispatch = Promise.withResolvers<void>();
      const allowDispatch = Promise.withResolvers<void>();
      const dispatch = Promise.withResolvers<Response>();
      const allowAcknowledgement = Promise.withResolvers<void>();
      const openClient = (transportFetch: typeof fetch) =>
        createFlameClient(
          createHttpTransport({ baseUrl: fixture.baseUrl, fetch: transportFetch }),
          {
            capabilities: () => fixture.negotiated,
            mutationStorage,
          },
        );
      const original = openClient(async (input, init) => {
        if (new Headers(init?.headers).get(HTTP_HEADERS.method) !== "runs.steer")
          return fetch(input, init);
        reachedDispatch.resolve();
        await allowDispatch.promise;
        const response = await fetch(input, init);
        dispatch.resolve(response);
        await allowAcknowledgement.promise;
        return response;
      });
      try {
        const session = await fixture.client.sessions.create({
          title: "Steer recovery ordering",
          workspace: { path: fixture.root },
        });
        const sessionId = asSessionId(session.id);
        const opening = await fixture.client.runs.start({
          sessionId,
          input: [{ type: "text", text: "E2E_STEER_RECOVERY hold the provider boundary." }],
        });
        await within(gate.arrived.promise, "the held provider request");
        const runId = asRunId(opening.result.runId);
        const instruction = "Apply this instruction while the Session is loading.";
        const submitted = original.runs.steer(runId, asSegmentId(opening.result.segmentId), [
          { type: "text", text: instruction },
        ]);
        void submitted.catch(() => {});
        const userItemId = await within(submitted.inputIdentity, "the Steer input identity");
        await within(reachedDispatch.promise, "the held Steer dispatch");
        const events = collectRunEvents(originalOpeningEvents(opening));
        void events.catch(() => {});
        async function finishSteer() {
          allowDispatch.resolve();
          expect((await within(dispatch.promise, "the Runtime Steer acknowledgement")).ok).toBe(
            true,
          );
          gate.release.resolve();
          const finished = (await within(events, "the original Run finishing")).at(-1)?.event;
          expect(finished).toMatchObject({ type: "segment.finished", run: { status: "finished" } });
          if (retire) {
            await fixture.client.sessions.rollback({
              sessionId,
              expectedLastRootRunId: runId,
              expectedWorkspacePath: session.workspace.ref.path,
              restoreType: "history",
            });
          }
        }
        let settlement: Promise<void> | undefined;
        const settle = () => (settlement ??= finishSteer());
        let snapshots = 0;
        const recovering = openClient(async (input, init) => {
          const method = new Headers(init?.headers).get(HTTP_HEADERS.method);
          if (method === "runs.getAcceptance") await settle();
          const response = await fetch(input, init);
          if (method === "sessions.snapshot") {
            snapshots++;
            await settle();
          }
          return response;
        });
        const web = openWebSession(recovering, fixture.baseUrl, session.id);
        try {
          expect(mutationStorage.size).toBe(1);
          await expect(web.agent.access.synchronizeRewrittenSessionView(session.id)).resolves.toBe(
            true,
          );
          const view = web.agent.dependencies.view.getCurrentView();
          if (retire) {
            expect(view.runsById[runId]).toBeUndefined();
            expect(view.messages).toEqual([
              expect.objectContaining({
                role: "user",
                runId: null,
                steer: { runId, receipt: "retired" },
              }),
            ]);
            expect(view.messages[0]?.id).not.toBe(userItemId);
          } else {
            expect(view.runsById[runId]?.status).toBe("finished");
            const applied = view.messages.findIndex((message) => message.id === userItemId);
            const answer = view.messages.findLastIndex(
              (message) => message.phase === "finalAnswer",
            );
            expect(applied).toBeGreaterThanOrEqual(0);
            expect(view.messages[applied]).toMatchObject({
              runId,
              steer: { runId, receipt: "accepted" },
            });
            expect(answer).toBeGreaterThan(applied);
          }
          expect(snapshots).toBe(1);
          expect(mutationStorage.size).toBe(0);
          allowAcknowledgement.resolve();
          await expect(submitted).resolves.toMatchObject({ type: "accepted", userItemId });
        } finally {
          web.dispose();
          await recovering.close();
        }
      } finally {
        allowDispatch.resolve();
        allowAcknowledgement.resolve();
        gate.release.resolve();
        fixture.providerGate = undefined;
        await original.close();
      }
    },
    30_000,
  );
});
