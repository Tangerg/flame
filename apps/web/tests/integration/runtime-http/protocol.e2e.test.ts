import { describe, expect, it, vi } from "vitest";
import {
  createFlameClient,
  type FlameClient,
  JSONRPC_VERSION,
  RpcError,
  asRunId,
  asSegmentId,
  asSessionId,
  createSidecarClient,
  createHttpTransport,
} from "@flame/runtime-client";
import type { WireMethodName } from "@flame/runtime-client/methods";
import {
  HTTP_ENDPOINTS,
  HTTP_HEADERS,
  PROTOCOL_VERSION,
  RPC_HTTP,
  type RequestMeta,
  type RuntimeSubscribeRequest,
} from "@flame/runtime-client/wire";
import { createProviderGate } from "./fakeProvider";
import {
  collectRunEvents,
  collectSessionRuntimeEvents,
  nextRuntimeEvent,
  within,
} from "./observation";
import { faultStreamOpening, faultUnaryResponse } from "./transportFaults";
import { useRuntimeFixture } from "./fixture";

function postRPC(
  baseUrl: string,
  method: WireMethodName,
  params: unknown,
  headers: Record<string, string> = {},
): Promise<Response> {
  const endpoint = HTTP_ENDPOINTS.rpc;
  return fetch(new URL(endpoint.path, baseUrl), {
    method: endpoint.method,
    headers: {
      Accept: RPC_HTTP.documentMediaType,
      [HTTP_HEADERS.contentType]: RPC_HTTP.documentMediaType,
      ...headers,
    },
    body: JSON.stringify({ jsonrpc: JSONRPC_VERSION, id: method, method, params }),
  });
}

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: protocol delivery", () => {
  const fixture = useRuntimeFixture("protocol");

  it("validates discovery, streaming notifications and session lifecycle", async () => {
    const sidecar = createSidecarClient({ baseUrl: fixture.baseUrl });
    const liveness = await sidecar.liveness();
    const readiness = await sidecar.readiness();
    const info = await sidecar.info();
    expect(liveness).toMatchObject({ status: "ok" });
    expect(readiness).toMatchObject({ status: "ok" });
    expect(info).toMatchObject({
      protocolVersion: PROTOCOL_VERSION,
      transport: "http",
    });

    const discovery = await fixture.client.runtime.discover();
    expect([
      info.server.instanceId,
      liveness.instanceId,
      readiness.instanceId,
      discovery.serverInfo.instanceId,
    ]).toEqual(Array(4).fill(discovery.serverInfo.instanceId));
    expect(discovery.protocolVersion).toBe(PROTOCOL_VERSION);
    expect(discovery.capabilities.limits.idempotency.namespace).toMatch(/^idp_[0-9a-f]{32}$/);
    const repeatDiscovery = await fixture.client.runtime.discover();
    expect(repeatDiscovery).toMatchObject({
      capabilities: {
        limits: {
          idempotency: { namespace: discovery.capabilities.limits.idempotency.namespace },
        },
      },
    });
    expect(repeatDiscovery.serverInfo.instanceId).toBe(discovery.serverInfo.instanceId);
    expect(discovery.capabilities.streamingMethods).toContain("runtime.subscribe");

    const staleStoreTitle = "must not enter replacement Runtime";
    const staleStoreResponse = await postRPC(
      fixture.baseUrl,
      "sessions.create",
      { workspace: { path: fixture.root }, title: staleStoreTitle },
      {
        [HTTP_HEADERS.idempotencyKey]: "stale-store-session-create",
        [HTTP_HEADERS.idempotencyNamespace]: "idp_00000000000000000000000000000000",
      },
    );
    expect(staleStoreResponse.status).toBe(RPC_HTTP.callStatus);
    await expect(staleStoreResponse.json()).resolves.toMatchObject({
      error: { data: { type: "idempotency_store_mismatch" } },
    });
    const sessionsAfterStoreMismatch = await fixture.client.sessions.list();
    expect(
      sessionsAfterStoreMismatch.data.some((session) => session.title === staleStoreTitle),
    ).toBe(false);

    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["sessions.changed"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();

    const created = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP contract e2e",
    });
    const sessionId = asSessionId(created.id);
    expect(created.revision).toBe(1);
    const changed = await nextRuntimeEvent(events, "sessions.changed");
    expect(changed.type === "sessions.changed" && changed.sessionIds).toContain(created.id);

    const fetched = await fixture.client.sessions.get(sessionId);
    expect(fetched).toEqual(created);
    const updated = await fixture.client.sessions.update({
      sessionId,
      expectedRevision: created.revision,
      title: "HTTP contract e2e updated",
    });
    expect(updated.revision).toBe(created.revision + 1);

    const listed = await fixture.client.sessions.list();
    expect(listed.data.some((session) => session.id === created.id)).toBe(true);

    await fixture.client.sessions.delete(sessionId);
    await expect(fixture.client.sessions.get(sessionId)).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && error.data.type === "session_not_found",
    );

    streamController.abort();
    await events.return?.();
  }, 30_000);

  it("cancels a subscriber while another owns the shared opening", async () => {
    const arrived = Promise.withResolvers<void>();
    const release = Promise.withResolvers<void>();
    let openings = 0;
    const client = fixture.createRuntimeClient(async (input, init) => {
      openings++;
      const response = await fetch(input, init);
      arrived.resolve();
      await release.promise;
      return response;
    });
    const request: RuntimeSubscribeRequest = { topics: ["sessions.changed"] };
    const staying = new AbortController();
    const leaving = new AbortController();
    const survivor = client.runtimeEvents.subscribe(request, staying.signal);
    try {
      await within(Promise.race([arrived.promise, survivor]), "shared Runtime opening");
      const canceled = client.runtimeEvents.subscribe(request, leaving.signal);
      const reason = new Error("the second subscriber left");
      leaving.abort(reason);
      await expect(within(canceled, "canceled subscriber", 1_000)).rejects.toBe(reason);
      expect(openings).toBe(1);

      release.resolve();
      const subscription = await within(survivor, "remaining subscriber");
      const events = subscription.events[Symbol.asyncIterator]();
      const session = await fixture.client.sessions.create({ title: "shared event survivor" });
      await collectSessionRuntimeEvents(events, request.topics, session.id);
      await events.return?.();

      const replacement = await within(
        client.runtimeEvents.subscribe(request, staying.signal),
        "replacement subscription",
      );
      expect(openings).toBe(2);
      await replacement.events[Symbol.asyncIterator]().return?.();
    } finally {
      release.resolve();
      staying.abort();
      await client.close();
    }
  });

  it.each(["abort", "return"] as const)(
    "stops buffered events after subscriber %s",
    async (stop) => {
      const request: RuntimeSubscribeRequest = { topics: ["sessions.changed"] };
      const departing = new AbortController();
      const observing = new AbortController();
      try {
        const buffered = await fixture.client.runtimeEvents.subscribe(request, departing.signal);
        const witness = await fixture.client.runtimeEvents.subscribe(request, observing.signal);
        const events = buffered.events[Symbol.asyncIterator]();
        const session = await fixture.client.sessions.create({
          title: "event pending for two observers",
        });
        await collectSessionRuntimeEvents(
          witness.events[Symbol.asyncIterator](),
          request.topics,
          session.id,
        );

        if (stop === "abort") departing.abort();
        else await events.return?.();
        await expect(events.next()).resolves.toEqual({ done: true, value: undefined });
      } finally {
        departing.abort();
        observing.abort();
      }
    },
  );

  it("refuses both Run targets even when the existing session ID is empty", async () => {
    const sessionId = asSessionId(crypto.randomUUID());
    const response = await postRPC(
      fixture.baseUrl,
      "runs.start",
      {
        sessionId: "",
        newSession: {
          sessionId,
          runId: asRunId(crypto.randomUUID()),
          title: "Ambiguous Run target",
        },
        input: [{ type: "text", text: "E2E_RUN complete without tools." }],
      },
      {
        Accept: `${RPC_HTTP.documentMediaType}, ${RPC_HTTP.streamMediaType}`,
        [HTTP_HEADERS.idempotencyKey]: "ambiguous-session-target",
      },
    );
    try {
      await expect(fixture.client.sessions.get(sessionId)).rejects.toMatchObject({
        data: { type: "session_not_found" },
      });
      expect(response.status).toBe(RPC_HTTP.callStatus);
      await expect(response.json()).resolves.toMatchObject({
        error: {
          data: {
            type: "invalid_params",
            errors: expect.arrayContaining([expect.objectContaining({ field: "sessionId" })]),
          },
        },
      });
    } finally {
      if (!response.bodyUsed) await response.body?.cancel();
    }
  });

  it("refuses an empty export format and defaults an omitted format", async () => {
    const session = await fixture.client.sessions.create({ title: "Session export format" });
    const omitted = await postRPC(fixture.baseUrl, "sessions.export", { sessionId: session.id });
    await expect(omitted.json()).resolves.toMatchObject({ result: { format: "json" } });

    const empty = await postRPC(fixture.baseUrl, "sessions.export", {
      sessionId: session.id,
      format: "",
    });
    expect(empty.status).toBe(RPC_HTTP.callStatus);
    await expect(empty.json()).resolves.toMatchObject({
      error: { data: { type: "invalid_params", errors: [{ field: "format" }] } },
    });
  });

  it.each([
    {
      label: "component values",
      changes: { serverChanges: { schedules: "restart" }, skillChanges: { demo: "" } },
      fields: ['serverChanges["schedules"]', 'skillChanges["demo"]'],
    },
    {
      label: "component names",
      changes: {
        valueChanges: { Input: { type: "clear" } },
        serverChanges: { Schedules: "enable" },
        skillChanges: { "Invalid Skill": "disable" },
      },
      fields: [
        'serverChanges["Schedules"]',
        'skillChanges["Invalid Skill"]',
        'valueChanges["Input"]',
      ],
    },
  ])(
    "leaves a plugin configuration key reusable after rejecting $label",
    async ({ changes, fields }) => {
      const installation = await fixture.activateManagedOwner("schedules");
      const release = { installationId: installation.id, digest: installation.selected.digest };
      const key = crypto.randomUUID();
      const configure = async (changes: object) => {
        const response = await postRPC(
          fixture.baseUrl,
          "plugins.configure",
          { ...release, valueChanges: {}, serverChanges: {}, skillChanges: {}, ...changes },
          { [HTTP_HEADERS.idempotencyKey]: key },
        );
        expect(response.status).toBe(RPC_HTTP.callStatus);
        return response.json();
      };

      const invalid = await configure(changes);
      await expect(configure({ serverChanges: { schedules: "enable" } })).resolves.toMatchObject({
        result: release,
      });
      expect(invalid).toMatchObject({
        error: {
          data: { type: "invalid_params", errors: fields.map((field) => ({ field })) },
        },
      });
    },
  );

  it("converges unary mutations across pre-admission and post-commit disconnects", async () => {
    const requestMeta = (): RequestMeta => ({
      protocolVersion: PROTOCOL_VERSION,
      clientInfo: { name: "runtime-http-cutpoint-e2e", version: "1" },
      clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
    });
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["sessions.changed"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();
    const createdIds: string[] = [];

    try {
      for (const cutpoint of ["beforeCommit", "afterCommit"] as const) {
        const fault = faultUnaryResponse("sessions.create", cutpoint);
        const faultClient = createFlameClient(
          createHttpTransport({ baseUrl: fixture.baseUrl, fetch: fault.fetch }),
          {
            capabilities: () => fixture.negotiated,
            requestMeta,
          },
        );
        try {
          const created = await faultClient.sessions.create({
            workspace: { path: fixture.root },
            title: `HTTP ${cutpoint} replay`,
          });
          createdIds.push(created.id);
          expect(fault.attempts()).toBe(2);
          await expect(fixture.client.sessions.get(asSessionId(created.id))).resolves.toEqual(
            created,
          );
          await expect(nextRuntimeEvent(events, "sessions.changed")).resolves.toMatchObject({
            type: "sessions.changed",
            sessionIds: [created.id],
          });
        } finally {
          await faultClient.close();
        }
      }

      const sessions = await fixture.client.sessions.list().autoPagingToArray();
      for (const id of createdIds) {
        expect(sessions.filter((session) => session.id === id)).toHaveLength(1);
      }

      const deletedId = createdIds.pop();
      if (!deletedId) throw new Error("post-commit Session was not created");
      const fault = faultUnaryResponse("sessions.delete", "afterCommit");
      const faultClient = createFlameClient(
        createHttpTransport({ baseUrl: fixture.baseUrl, fetch: fault.fetch }),
        {
          capabilities: () => fixture.negotiated,
          requestMeta,
        },
      );
      try {
        await faultClient.sessions.delete(asSessionId(deletedId));
        expect(fault.attempts()).toBe(2);
      } finally {
        await faultClient.close();
      }
      await expect(nextRuntimeEvent(events, "sessions.changed")).resolves.toMatchObject({
        type: "sessions.changed",
        sessionIds: [deletedId],
      });
      await expect(fixture.client.sessions.get(asSessionId(deletedId))).rejects.toSatisfy(
        (error: unknown) => error instanceof RpcError && error.data.type === "session_not_found",
      );
    } finally {
      for (const id of createdIds) await fixture.client.sessions.delete(asSessionId(id));
      streamController.abort();
      await events.return?.();
    }
  }, 30_000);

  it("reattaches one committed Run after its streaming acknowledgement is lost", async () => {
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "HTTP streaming replay cutpoint",
    });
    const gate = createProviderGate("E2E_RUN_REPLAY_CUTPOINT");
    fixture.providerGate = gate;
    const fault = faultStreamOpening("runs.start");
    const faultClient = createFlameClient(
      createHttpTransport({ baseUrl: fixture.baseUrl, fetch: fault.fetch }),
      {
        capabilities: () => fixture.negotiated,
        requestMeta: (): RequestMeta => ({
          protocolVersion: PROTOCOL_VERSION,
          clientInfo: { name: "runtime-http-stream-cutpoint-e2e", version: "1" },
          clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
        }),
      },
    );

    try {
      const opening = faultClient.runs.start({
        sessionId: asSessionId(session.id),
        input: [{ type: "text", text: "E2E_RUN_REPLAY_CUTPOINT finish exactly once." }],
      });
      await within(gate.arrived.promise, "the replay-cutpoint model request");
      const started = await opening;
      expect(fault.attempts()).toBe(1);
      expect(started.observation).toEqual({ type: "snapshot", sessionId: session.id });
      const tail = await fixture.client.runs.subscribe({
        runId: asRunId(started.result.runId),
        segmentId: asSegmentId(started.result.segmentId),
        snapshot: true,
      });
      expect(tail.result.snapshot?.runs).toContainEqual(
        expect.objectContaining({
          id: started.result.runId,
          activeSegmentId: started.result.segmentId,
        }),
      );

      gate.release.resolve();
      const events = await collectRunEvents(tail.events);
      expect(events.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        run: { status: "finished", outcome: { type: "completed" } },
      });
      const runs = await fixture.client.runs
        .list({ sessionId: asSessionId(session.id) })
        .autoPagingToArray();
      expect(runs).toHaveLength(1);
      expect(runs[0]).toMatchObject({
        id: started.result.runId,
        sessionId: session.id,
        status: "finished",
        outcome: { type: "completed" },
      });
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
      await faultClient.close();
      await fixture.client.sessions.delete(asSessionId(session.id));
    }
  }, 30_000);

  it("queries a cold persisted opening after its replay deadline and recovers its snapshot tail", async () => {
    if (!fixture.negotiated) throw new Error("runtime client was not initialized");
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "cold accepted opening",
    });
    const entries = new Map<string, unknown>();
    let now = Date.now();
    const clock = vi.spyOn(Date, "now").mockImplementation(() => now);
    const mutationStorage = {
      get: (key: string) => structuredClone(entries.get(key)),
      set: (key: string, value: unknown) => void entries.set(key, structuredClone(value)),
      remove: (key: string) => void entries.delete(key),
      keys: () => [...entries.keys()],
    };
    const requestMeta = (): RequestMeta => ({
      protocolVersion: PROTOCOL_VERSION,
      clientInfo: { name: "cold-opening-e2e", version: "1" },
      clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
    });
    const gate = createProviderGate("E2E_RUN_REPLAY_CUTPOINT");
    fixture.providerGate = gate;
    const fault = faultStreamOpening("runs.start");
    let lookups = 0;
    const lostLookup: typeof fetch = async (input, init) => {
      const body =
        typeof init?.body === "string" ? (JSON.parse(init.body) as { method: string }) : undefined;
      if (body?.method === "runs.getAcceptance" && ++lookups > 1)
        return new Response("lookup unavailable", { status: 503 });
      return fault.fetch(input, init);
    };
    const first = createFlameClient(
      createHttpTransport({ baseUrl: fixture.baseUrl, fetch: lostLookup }),
      {
        requestMeta,
        capabilities: () => fixture.negotiated,
        mutationStorage,
      },
    );
    let cold: FlameClient | undefined;
    const params = {
      sessionId: asSessionId(session.id),
      input: [{ type: "text" as const, text: "E2E_RUN_REPLAY_CUTPOINT cold receipt." }],
    };
    try {
      const pending = first.runs.start(params);
      await expect(pending).rejects.toMatchObject({ name: "MutationLookupError" });
      await within(gate.arrived.promise, "the cold opening model request");
      expect(entries.size).toBe(1);
      const evidence = structuredClone([...entries]);
      await first.close();
      now += fixture.negotiated!.limits.idempotency.retentionSeconds * 1_000;
      cold = createFlameClient(createHttpTransport({ baseUrl: fixture.baseUrl }), {
        requestMeta,
        capabilities: () => fixture.negotiated,
        mutationStorage,
      });
      await expect(cold.runs.recoverCommands()).resolves.toMatchObject([
        {
          idempotencyKey: pending.idempotencyKey,
          operation: "start",
          observation: {
            type: "recorded",
            acceptance: { type: "accepted", sessionId: session.id },
          },
        },
      ]);
      expect([...entries]).toEqual(evidence);
      const recovered = cold.runs.start(params);
      expect(recovered.idempotencyKey).toBe(pending.idempotencyKey);
      expect([...entries]).toEqual(evidence);
      const opening = await recovered;
      expect(opening.observation).toEqual({ type: "snapshot", sessionId: session.id });
      expect(entries.size).toBe(0);
      expect(fault.attempts()).toBe(1);
      const receipt = await cold.runs.getAcceptance({
        namespace: fixture.negotiated.limits.idempotency.namespace,
        key: pending.idempotencyKey,
        operation: "start",
      });
      expect(receipt).toMatchObject({ type: "accepted", ...opening.result });
      const tail = await cold.runs.subscribe({
        runId: asRunId(opening.result.runId),
        segmentId: asSegmentId(opening.result.segmentId),
        snapshot: true,
      });
      expect(tail.result.snapshot?.runs).toContainEqual(
        expect.objectContaining({
          id: opening.result.runId,
          activeSegmentId: opening.result.segmentId,
        }),
      );
      gate.release.resolve();
      const events = await collectRunEvents(tail.events);
      expect(events.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        run: { status: "finished", outcome: { type: "completed" } },
      });
      expect(
        await fixture.client.runs.list({ sessionId: params.sessionId }).autoPagingToArray(),
      ).toHaveLength(1);
    } finally {
      gate.release.resolve();
      fixture.providerGate = undefined;
      await first.close();
      await cold?.close();
      clock.mockRestore();
      await fixture.client.sessions.delete(params.sessionId);
    }
  }, 30_000);
});
