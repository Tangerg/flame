import { lstat } from "node:fs/promises";
import { join } from "node:path";
import { describe, expect, it, onTestFinished } from "vitest";
import { QueryObserver } from "@tanstack/react-query";
import { asRunId, asSessionId, createSidecarClient, type FlameClient } from "@flame/runtime-client";
import { HTTP_HEADERS } from "@flame/runtime-client/wire";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { definePlugin } from "@/extensions/definePlugin";
import { registerAgentDataProviders } from "@/features/agent/adapters/runtimeDataProviders";
import {
  readRuntimeSessionSummaries,
  toAgentSessionSummary,
} from "@/features/agent/adapters/runtimeSessionSummary";
import { editAgentSession } from "@/features/agent/application/session/editSession";
import {
  AGENT_SESSIONS_KEY,
  fetchAgentSessionSummary,
  type AgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";
import {
  editAndRerunMessage,
  regenerateMessage,
} from "@/features/chat/message-actions/public/messageActions";
import { createComposer } from "@/features/composer/adapters/composerStatePorts";
import { DraftContent } from "@/features/composer/domain/draft";
import { runtimeServiceInspector } from "@/features/connection/adapters/runtimeServiceInspector";
import { NotificationFeed } from "@/features/notifications/public/notifications";
import { bindConversationImportRuntime } from "@/features/workspace/adapters/runtimeConversationImportGateway";
import { createConversationImport } from "@/features/workspace/application/conversationImport";
import { queryClient } from "@/lib/queryClient";
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
  deleteUserMCPServers,
  nextRuntimeEvent,
  originalOpeningEvents,
  waitForUserMCPConnection,
  watchGoal,
  watchRuntimeTopic,
  within,
} from "./observation";
import { useRuntimeFixture } from "./fixture";
import { openWebSession } from "./webSessionFixture";
import { faultUnaryResponse } from "./transportFaults";

const compactionCutpointDeadlineMilliseconds = 15_000;

function openWebComposer(
  { agent, connection }: ReturnType<typeof openWebSession>,
  endpoint: string,
  sessionId: string,
) {
  const access = agent.access;
  const composer = createComposer(
    {
      getActiveSessionId: access.getActiveSessionId,
      getLifecycleSnapshot: access.getAgentSessionLifecycleSnapshot,
      subscribeActiveSessionId: access.subscribeActiveSessionId,
      subscribeLifecycle: access.subscribeAgentSessionLifecycle,
      subscribeDeleted: access.subscribeDeletedAgentSession,
    },
    () => endpoint,
    connection.scope,
    connection.stream,
  );
  composer.store.getState().loadSession(sessionId);
  return composer;
}

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: restart and crash recovery", () => {
  // Bound startup work to the owners exercised at the crash boundaries.
  const fixture = useRuntimeFixture("recovery", {
    packages: ["goals", "mcp", "memory", "plans", "providers", "schedules", "skills"],
  });

  it("keeps the latest Session list when an older edit acknowledgement arrives", async () => {
    const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    const sessionId = asSessionId(session.id);
    const recorded = Promise.withResolvers<void>();
    const release = Promise.withResolvers<void>();
    const client = fixture.createRuntimeClient(async (url, init) => {
      const response = await fetch(url, init);
      if (new Headers(init?.headers).get(HTTP_HEADERS.method) === "sessions.update") {
        recorded.resolve();
        await release.promise;
      }
      return response;
    });
    const web = openWebSession(client, fixture.baseUrl, sessionId);
    const { commands, runtime } = web.agent.dependencies;
    const queryKey = [AGENT_SESSIONS_KEY];
    const edit = editAgentSession(commands.current(), sessionId, () =>
      runtime.updateSession({ sessionId, expectedRevision: session.revision, favorite: true }),
    );
    try {
      await within(recorded.promise, "recording the first Session edit");
      const saved = await fixture.client.sessions.get(sessionId);
      const latest = await fixture.client.sessions.update({
        sessionId,
        expectedRevision: saved.revision,
        title: "A newer title from the other client",
      });
      const sessions = await queryClient.fetchQuery({
        queryKey,
        queryFn: ({ signal }) => readRuntimeSessionSummaries(client, signal),
      });
      expect(sessions.find(({ id }) => id === sessionId)).toEqual(toAgentSessionSummary(latest));

      release.resolve();
      await edit;
      expect(
        queryClient
          .getQueryData<AgentSessionSummary[]>(queryKey)
          ?.find(({ id }) => id === sessionId),
      ).toEqual(toAgentSessionSummary(latest));
    } finally {
      release.resolve();
      await edit.catch(() => {});
      queryClient.removeQueries({ queryKey });
      web.dispose();
      await client.close();
      await fixture.client.sessions.delete(sessionId);
    }
  }, 30_000);

  it.each(["create", "fork"] as const)(
    "%s opens a Session with a pending catalog read",
    async (action) => {
      const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
      const sessionId = asSessionId(session.id);
      const opening = await fixture.client.runs.start({
        sessionId,
        input: [{ type: "text", text: "A conversation that should stay open in its own tab." }],
      });
      await collectRunEvents(originalOpeningEvents(opening));

      const listing = Promise.withResolvers<void>();
      const release = Promise.withResolvers<void>();
      const client = fixture.createRuntimeClient(async (url, init) => {
        const method = new Headers(init?.headers).get(HTTP_HEADERS.method);
        const response = await fetch(url, init);
        if (method === "sessions.list") {
          listing.resolve();
          await release.promise;
        }
        return response;
      });
      const web = openWebSession(client, fixture.baseUrl, sessionId);
      const access = web.agent.access;
      const queryKey = [AGENT_SESSIONS_KEY];
      const observer = new QueryObserver(queryClient, {
        queryKey,
        queryFn: ({ signal }) => readRuntimeSessionSummaries(client, signal),
      });
      const unsubscribe = observer.subscribe(() => {});
      let openingSession: Promise<string | null> | undefined;
      try {
        await within(listing.promise, "the pending Session catalog read");
        openingSession =
          action === "create"
            ? access.createSession()
            : access
                .forkAgentSession(sessionId, opening.result.runId)
                .then(access.getActiveSessionId);
        const newId = await within(
          openingSession,
          "opening a Session while its catalog read is held",
        );
        if (!newId) throw new Error("new Session ID is missing");
        expect(newId).not.toBe(sessionId);
        expect(access.getActiveSessionId()).toBe(newId);
        expect(observer.getCurrentResult().data).toBeUndefined();
        await expect(fixture.client.sessions.get(asSessionId(newId))).resolves.toMatchObject({
          id: newId,
        });
        release.resolve();
        await expect
          .poll(() => observer.getCurrentResult().data?.map(({ id }) => id))
          .toContain(newId);
        web.agent.projection.store.getState().ensureSession(newId);
        await expect(access.synchronizeRewrittenSessionView(newId)).resolves.toBe(true);
        if (action === "create") await expect(access.createSession()).resolves.toBe(newId);
      } finally {
        release.resolve();
        const newId = await openingSession?.catch(() => null);
        unsubscribe();
        queryClient.removeQueries({ queryKey });
        web.dispose();
        await client.close();
        if (newId && newId !== sessionId) await fixture.client.sessions.delete(asSessionId(newId));
        await fixture.client.sessions.delete(sessionId);
      }
    },
    30_000,
  );

  it("recovers the original creation explicitly after an independent Session is created", async () => {
    const workspace = { path: fixture.root };
    const seed = await fixture.client.sessions.create({ workspace });
    const existing = new Set(
      (await fixture.client.sessions.list().autoPagingToArray()).map(({ id }) => id),
    );
    const fault = faultUnaryResponse("sessions.create", "afterCommit");
    let loseResponse = true;
    const client = fixture.createRuntimeClient((url, init) => {
      if (
        loseResponse &&
        fault.attempts() > 0 &&
        new Headers(init?.headers).get(HTTP_HEADERS.method) === "sessions.create"
      )
        throw new Error("disconnected before the original creation could be reobserved");
      return fault.fetch(url, init);
    });
    const recoveryActions: Array<() => void> = [];
    const web = openWebSession(client, fixture.baseUrl, seed.id, ({ action }) => {
      if (action) recoveryActions.push(action.run);
    });
    const access = web.agent.access;
    const queryKey = [AGENT_SESSIONS_KEY];
    const created = async () =>
      (await fixture.client.sessions.list().autoPagingToArray()).filter(
        ({ id }) => !existing.has(id),
      );
    try {
      await queryClient.fetchQuery({
        queryKey,
        queryFn: ({ signal }) => readRuntimeSessionSummaries(client, signal),
      });
      await expect(access.createSession()).resolves.toBeNull();
      expect(access.getActiveSessionId()).toBe(seed.id);
      const [original] = await created();
      const [retry] = recoveryActions;
      if (!original || !retry) throw new Error("Session creation did not offer original recovery");

      loseResponse = false;
      const next = await access.createSession();
      expect(next).toBeTruthy();
      expect(next).not.toBe(original.id);
      expect(access.getActiveSessionId()).toBe(next);
      retry();
      await expect.poll(access.getActiveSessionId).toBe(original.id);
      expect(await created()).toHaveLength(2);
      expect(client.runtime.pendingCommands()).toEqual([]);
    } finally {
      queryClient.removeQueries({ queryKey });
      web.dispose();
      await client.close();
      for (const session of await created())
        await fixture.client.sessions.delete(asSessionId(session.id));
      await fixture.client.sessions.delete(asSessionId(seed.id));
    }
  }, 30_000);

  it("opens a JSON import after reconnect and refreshes an existing view on reimport", async () => {
    const title = "Conversation imported before its view mounts";
    const source = await fixture.client.sessions.create({
      title,
      workspace: { path: fixture.root },
    });
    const sourceId = asSessionId(source.id);
    const prompt = "This input survives an export and reimport.";
    const run = await fixture.client.runs.start({
      sessionId: sourceId,
      input: [{ type: "text", text: prompt }],
    });
    await collectRunEvents(originalOpeningEvents(run));
    const { artifact } = await fixture.client.sessions.export(sourceId, "json");
    if (!artifact) throw new Error("JSON export omitted its Session artifact");
    await fixture.client.sessions.delete(sourceId);

    const committed = Promise.withResolvers<number>();
    const release = Promise.withResolvers<void>();
    const listing = Promise.withResolvers<void>();
    const releaseCatalog = Promise.withResolvers<void>();
    let importsSent = 0;
    const client = fixture.createSharedClient(async (url, init) => {
      const method = new Headers(init?.headers).get(HTTP_HEADERS.method);
      const response = await fetch(url, init);
      if (method === "sessions.import" && ++importsSent === 1) {
        committed.resolve(response.status);
        await release.promise;
      }
      if (method === "sessions.list" && importsSent > 0) {
        listing.resolve();
        await releaseCatalog.promise;
      }
      return response;
    });
    const web = openWebSession(client, fixture.baseUrl, "");
    const access = web.agent.access;
    const sidecar = createSidecarClient({ baseUrl: fixture.baseUrl });
    web.connection.start(
      runtimeServiceInspector(
        () => client,
        () => sidecar,
      ),
    );
    const presentations: string[] = [];
    const notifications = new NotificationFeed(({ level }) => presentations.push(level));
    const imports = createConversationImport(access, web.runtime, notifications, {
      pickText: async () => JSON.stringify(artifact),
    });
    const unbind = bindConversationImportRuntime(imports, () => client, web.connection.scope);
    const kernel = await startKernel([
      definePlugin({
        name: "flame.test.session-summary-query",
        setup({ contributions }) {
          registerAgentDataProviders(contributions, () => client);
        },
      }),
    ]);
    const queryKey = [AGENT_SESSIONS_KEY];
    const catalog = new QueryObserver(queryClient, {
      queryKey,
      queryFn: ({ signal }) => readRuntimeSessionSummaries(client, signal),
    });
    const unsubscribe = catalog.subscribe(() => {});
    try {
      await web.connection.status.refresh();
      await catalog.refetch({ throwOnError: true });
      await expect(fetchAgentSessionSummary(sourceId)).resolves.toBeNull();
      const generation = web.connection.stream.connectionGeneration();
      if (!generation) throw new Error("the Runtime had no initial connection");
      const importing = imports.importConversationJson();
      await expect(within(committed.promise, "the import receipt")).resolves.toBe(200);
      web.connection.stream.reportConnectionLoss(generation);
      expect(web.connection.stream.connectionGeneration()).toBeNull();
      await web.connection.status.refresh();
      const restored = web.connection.stream.connectionGeneration();
      expect(restored?.processGeneration).toBe(generation.processGeneration);
      expect(restored).not.toBe(generation);
      release.resolve();
      await within(listing.promise, "the catalog read after import");
      await within(importing, "opening the imported Session while its catalog read is held");
      expect(access.getActiveSessionId()).toBe(sourceId);
      expect(catalog.getCurrentResult().data?.map(({ id }) => id)).not.toContain(sourceId);
      await expect(fetchAgentSessionSummary(sourceId)).resolves.toMatchObject({
        id: sourceId,
        title,
        workspace: { path: source.workspace.ref.path },
        provider: source.provider,
        model: source.model,
      });
      expect(web.agent.dependencies.view.getSession(sourceId)).toBeUndefined();
      expect(presentations).toEqual(["success"]);
      await expect(fixture.client.sessions.snapshot(sourceId)).resolves.toMatchObject({
        session: { id: sourceId, title },
        runs: [expect.objectContaining({ id: run.result.runId })],
      });
      expect(importsSent).toBe(1);
      releaseCatalog.resolve();
      await expect
        .poll(() => catalog.getCurrentResult().data?.map(({ id }) => id))
        .toContain(sourceId);

      await fixture.client.sessions.rollback({
        sessionId: sourceId,
        expectedLastRootRunId: run.result.runId,
        expectedWorkspacePath: source.workspace.ref.path,
      });
      web.agent.projection.store.getState().ensureSession(sourceId);
      await expect(access.synchronizeRewrittenSessionView(sourceId)).resolves.toBe(true);
      expect(access.activeAgentConversation()?.messages).toEqual([]);
      await imports.importConversationJson();
      expect(presentations).toEqual(["success", "success"]);
      await expect
        .poll(() => access.activeAgentConversation()?.messages)
        .toContainEqual(expect.objectContaining({ role: "user", runId: run.result.runId }));
    } finally {
      release.resolve();
      releaseCatalog.resolve();
      unsubscribe();
      queryClient.removeQueries({ queryKey });
      unbind();
      await stopKernel(kernel);
      notifications.dispose();
      web.dispose();
      await client.close();
      await fixture.client.sessions.delete(sourceId);
    }
  }, 30_000);

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
    const { agent, notifications, dispose } = openWebSession(client, fixture.baseUrl, sessionId);
    try {
      await expect(
        agent.access.rollbackSessionToBeforeRun(sessionId, run.result.runId),
      ).resolves.toEqual({
        status: "committed",
        userInput: { parts: [{ kind: "text", text: input }] },
      });
      await within(refreshing.promise, "Session refresh after the committed rollback");
      await expect(fixture.client.sessions.snapshot(sessionId)).resolves.toMatchObject({
        runs: [],
      });
      await expect
        .poll(() => notifications.snapshot().some(({ level }) => level === "error"))
        .toBe(true);
    } finally {
      dispose();
      await client.close();
      await fixture.client.sessions.delete(sessionId);
    }
  }, 30_000);

  it("recovers the original message and references beside edits made during rollback", async () => {
    const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    const sessionId = asSessionId(session.id);
    const input = "The original input is available in the rollback receipt.";
    const originalReference = { label: "notes.md:1", text: "Reference from the original input" };
    const run = await fixture.client.runs.start({
      sessionId,
      input: [
        { type: "text", text: input },
        { type: "reference", ...originalReference },
      ],
    });
    await collectRunEvents(originalOpeningEvents(run));

    const committed = Promise.withResolvers<void>();
    const release = Promise.withResolvers<void>();
    const client = fixture.createRuntimeClient(async (url, init) => {
      const response = await fetch(url, init);
      if (new Headers(init?.headers).get(HTTP_HEADERS.method) === "sessions.rollback") {
        committed.resolve();
        await release.promise;
      }
      return response;
    });
    const web = openWebSession(client, fixture.baseUrl, sessionId);
    const { agent, notifications, dispose } = web;
    const access = agent.access;
    const composer = openWebComposer(web, fixture.baseUrl, sessionId);
    try {
      await expect(access.synchronizeRewrittenSessionView(sessionId)).resolves.toBe(true);
      const opening = access
        .activeAgentConversation()
        ?.messages.find((message) => message.role === "user" && message.runId === run.result.runId);
      if (!opening) throw new Error("the Web projection omitted the original message");
      editAndRerunMessage(
        notifications,
        access,
        composer.access,
        opening,
        "history",
        (_type, apply) => apply(),
      );
      await within(committed.promise, "rollback acknowledgement before draft recovery");

      const laterText = "A new note written while the original command was pending.";
      const laterReference = { label: "notes.md:2", text: "Reference added after dispatch" };
      composer.access.checkpoint(DraftContent.fromText(laterText));
      expect(composer.access.captureAddReference()(laterReference)).toBe(true);
      const scratch = "Draft in the newly selected conversation.";
      access.selectAgentSession("");
      composer.store.getState().loadSession("");
      composer.access.checkpoint(DraftContent.fromText(scratch));
      release.resolve();

      const originalDraft = () => composer.store.getState().composer.activate(sessionId).draft;
      await expect.poll(originalDraft).toMatchObject({
        value: `${input}\n\n${laterText}`,
        references: [originalReference, laterReference],
      });
      expect(composer.access.captureSubmission().value).toBe(scratch);
      await expect(fixture.client.sessions.snapshot(sessionId)).resolves.toMatchObject({
        runs: [],
      });
    } finally {
      release.resolve();
      composer.dispose();
      dispose();
      await client.close();
      await fixture.client.sessions.delete(sessionId);
    }
  }, 30_000);

  it("retains a regeneration prompt when the restored Session cannot accept a new Run", async () => {
    const session = await fixture.client.sessions.create({ workspace: { path: fixture.root } });
    const sessionId = asSessionId(session.id);
    const original = "Preserve this prompt if it cannot be regenerated.";
    const run = await fixture.client.runs.start({
      sessionId,
      input: [{ type: "text", text: original }],
    });
    await collectRunEvents(originalOpeningEvents(run));

    let committed = false;
    const client = fixture.createRuntimeClient(async (url, init) => {
      const method = new Headers(init?.headers).get(HTTP_HEADERS.method);
      if (method === "sessions.snapshot" && committed) {
        throw new Error("Session read unavailable before regeneration could start");
      }
      const response = await fetch(url, init);
      if (method === "sessions.rollback") committed = true;
      return response;
    });
    const web = openWebSession(client, fixture.baseUrl, sessionId);
    const composer = openWebComposer(web, fixture.baseUrl, sessionId);
    const access = web.agent.access;
    const unsent = "My separate unsent notes stay in the draft.";
    composer.access.checkpoint(DraftContent.fromText(unsent));
    try {
      await expect(access.synchronizeRewrittenSessionView(sessionId)).resolves.toBe(true);
      const reply = access
        .activeAgentConversation()
        ?.messages.find(
          (message) => message.role === "assistant" && message.runId === run.result.runId,
        );
      if (!reply) throw new Error("the Web projection omitted the reply to regenerate");
      regenerateMessage(web.notifications, access, composer.access, reply);

      await expect
        .poll(() => composer.access.captureSubmission().value)
        .toBe(`${original}\n\n${unsent}`);
      await expect(fixture.client.sessions.snapshot(sessionId)).resolves.toMatchObject({
        runs: [],
      });
    } finally {
      composer.dispose();
      web.dispose();
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
    const toolCallMarker = join(fixture.root, "sigkill-tool-call-arrived");
    let serverCreated = false;
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
      if (serverCreated && fixture.client)
        await deleteUserMCPServers(fixture.client, serverId.name);
    }
  }, 30_000);
});
