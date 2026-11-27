import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { asRunId, asSessionId } from "@flame/runtime-client";
import { HTTP_HEADERS } from "@flame/runtime-client/wire";
import { bindAgentSession } from "@/features/agent/adapters/agentSessionBinding";
import { agentTextInput } from "@/features/agent/public/input";
import { submitCapturedAgentInput } from "@/features/agent/application/input/submitAgentInput";
import { useRuntimeFixture } from "./fixture";
import { within } from "./observation";
import { openWebSession } from "./webSessionFixture";

describe("Go Runtime ↔ HTTP ↔ Web Agent: captured input", () => {
  const fixture = useRuntimeFixture("captured-input");

  beforeEach(() => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) =>
      setImmediate(() => callback(performance.now())),
    );
    vi.stubGlobal("cancelAnimationFrame", clearImmediate);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("authors a new captured input when a prior input with the same text lost its acknowledgement", async () => {
    const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    const sessionId = asSessionId(session.id);
    let originalFinished = false;
    let loseOriginal = true;
    const client = fixture.createRuntimeClient(async (url, init) => {
      const method = new Headers(init?.headers).get(HTTP_HEADERS.method);
      if (loseOriginal && originalFinished && method === "runs.getAcceptance") {
        throw new Error("the original Run acknowledgement is still unavailable");
      }
      const response = await fetch(url, init);
      if (loseOriginal && method === "runs.start") {
        await response.text();
        originalFinished = true;
        throw new Error("lost result after the original Run finished");
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
    const text = "E2E_RUN handle an explicitly repeated tool result interaction.";
    try {
      const entry = web.agent.dependencies.view.getSession(sessionId);
      if (!entry?.synchronize) throw new Error("session command binding was not installed");
      await expect(
        within(entry.synchronize("after-live"), "the source Session projection"),
      ).resolves.toBe(true);

      const original = client.runs.start({ sessionId, input: [{ type: "text", text }] });
      await expect(original).rejects.toThrow();
      loseOriginal = false;
      const previous = await fixture.client.runs.list({ sessionId });
      expect(previous.data).toHaveLength(1);
      expect(previous.data[0]?.status).toBe("finished");
      const captured = await submitCapturedAgentInput(
        web.agent.dependencies,
        sessionId,
        agentTextInput(text),
        new AbortController().signal,
      );
      expect(captured).toMatchObject({ status: "accepted", sessionId });
      if (captured.status !== "accepted") throw new Error("the new input was not accepted");
      expect(captured.runId).not.toBe(previous.data[0]?.id);
      const runs = await fixture.client.runs.list({ sessionId });
      expect(new Set(runs.data.map(({ id }) => id))).toEqual(
        new Set([previous.data[0]?.id, captured.runId]),
      );
      await expect(
        within(entry.synchronize("after-live"), "both Run receipts in the Session view"),
      ).resolves.toBe(true);
      const visibleRuns = web.agent.dependencies.view.getSession(sessionId)?.view.runsById;
      expect(Object.keys(visibleRuns ?? {})).toEqual(
        expect.arrayContaining(runs.data.map(({ id }) => id)),
      );
      expect(visibleRuns?.[captured.runId]?.status).toBe("finished");
      await expect(client.runs.recoverCommands()).resolves.toEqual([]);
    } finally {
      unbind();
      web.dispose();
      await client.close();
      await fixture.client.sessions.delete(sessionId);
    }
  }, 30_000);

  it("recovers the original Run receipt while the Session refresh is still waiting", async () => {
    const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    const sessionId = asSessionId(session.id);
    const refreshing = Promise.withResolvers<void>();
    const allowRefresh = Promise.withResolvers<void>();
    const submitted: string[] = [];
    let originalCommitted = false;
    let loseReceipt = true;
    const client = fixture.createRuntimeClient(async (url, init) => {
      const headers = new Headers(init?.headers);
      const method = headers.get(HTTP_HEADERS.method);
      if (originalCommitted && method === "sessions.snapshot") {
        refreshing.resolve();
        await allowRefresh.promise;
      }
      if (loseReceipt && originalCommitted && method === "runs.getAcceptance") {
        throw new Error("lost original Run acceptance lookup");
      }
      const response = await fetch(url, init);
      if (method === "runs.start") {
        submitted.push(headers.get(HTTP_HEADERS.idempotencyKey) ?? "");
        if (loseReceipt) {
          await response.text();
          originalCommitted = true;
          throw new Error("lost original Run opening response");
        }
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
      const entry = web.agent.dependencies.view.getSession(sessionId);
      if (!entry?.synchronize) throw new Error("session command binding was not installed");
      await expect(
        within(entry.synchronize("after-live"), "the source Session projection"),
      ).resolves.toBe(true);
      const original = await submitCapturedAgentInput(
        web.agent.dependencies,
        sessionId,
        agentTextInput("E2E_RUN retain the original input handle across the interrupted reply."),
        new AbortController().signal,
      );
      expect(original).toMatchObject({ status: "unknown" });
      if (original.status !== "unknown") throw new Error("the original reply was meant to be lost");
      await within(refreshing.promise, "the held Session snapshot request");
      const remote = await fixture.client.runs.list({ sessionId });
      expect(remote.data).toHaveLength(1);
      const committed = remote.data[0];
      expect(committed?.status).toBe("finished");

      loseReceipt = false;
      await expect(
        within(Promise.resolve().then(original.recover), "the original Run receipt"),
      ).resolves.toMatchObject({ status: "accepted", sessionId, runId: committed?.id });
      expect(submitted).toEqual([original.idempotencyKey]);

      allowRefresh.resolve();
      await expect(
        within(entry.synchronize("after-live"), "the committed Session view"),
      ).resolves.toBe(true);
      await expect(client.runs.recoverCommands()).resolves.toEqual([]);
      await expect(fixture.client.runs.list({ sessionId })).resolves.toEqual(remote);
    } finally {
      allowRefresh.resolve();
      unbind();
      web.dispose();
      await client.close();
      await fixture.client.sessions.delete(sessionId);
    }
  }, 30_000);

  it("opens a Run in the Session whose input binding was captured", async () => {
    const intended = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    const other = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    const sessionId = asSessionId(intended.id);
    const otherId = asSessionId(other.id);
    const client = fixture.createRuntimeClient(async (url, init) => {
      const response = await fetch(url, init);
      if (new Headers(init?.headers).get(HTTP_HEADERS.method) !== "runs.start") return response;
      return new Response(await response.arrayBuffer(), {
        status: response.status,
        headers: response.headers,
      });
    });
    const web = openWebSession(client, fixture.baseUrl, otherId);
    const unbind = bindAgentSession(
      web.agent.sessionDependencies(),
      () => client,
      sessionId,
      () => {},
    );
    try {
      expect(web.agent.access.getActiveSessionId()).toBe(otherId);
      const entry = web.agent.dependencies.view.getSession(sessionId);
      if (!entry?.synchronize) throw new Error("session command binding was not installed");
      await expect(
        within(entry.synchronize("after-live"), "the originally bound Session"),
      ).resolves.toBe(true);
      const submitted = await submitCapturedAgentInput(
        web.agent.dependencies,
        sessionId,
        agentTextInput("E2E_RUN belongs to the original Session after navigation"),
        new AbortController().signal,
      );
      if (submitted.status !== "accepted") throw new Error("the Run was not accepted");
      const accepted = await fixture.client.runs.get(asRunId(submitted.runId));
      expect(accepted.sessionId).toBe(sessionId);
      await expect(fixture.client.runs.list({ sessionId: otherId })).resolves.toMatchObject({
        data: [],
      });
      await expect(
        within(entry.synchronize("after-live"), "the original Session's finished Run"),
      ).resolves.toBe(true);
    } finally {
      unbind();
      web.dispose();
      await client.close();
      await fixture.client.sessions.delete(sessionId);
      await fixture.client.sessions.delete(otherId);
    }
  }, 30_000);
});
