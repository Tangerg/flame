import { describe, expect, it, vi } from "vitest";
import {
  createFlameClient,
  type FlameClient,
  JSONRPC_VERSION,
  RpcError,
  RpcProtocolError,
  RpcTransportError,
  asRunId,
  asSegmentId,
  asSessionId,
  createSidecarClient,
  createHttpTransport,
} from "@flame/runtime-client";
import {
  HTTP_HEADERS,
  MAXIMUM_IDEMPOTENCY_KEY_CHARACTERS,
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
import { MemoryMutationStorage } from "./mutationStorage";

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: protocol delivery", () => {
  const fixture = useRuntimeFixture("protocol");

  it("keeps incoming notifications and ambiguous envelopes out of command delivery", async () => {
    const sessionIds = async () =>
      (await fixture.client.sessions.list().autoPagingToArray()).map(({ id }) => id).sort();
    const existing = await sessionIds();
    const method = "sessions.create";
    const title = "No session from this envelope";
    const noContent = 204;
    const badRequest = 400;
    for (const params of [{ title }, { title, _meta: false }]) {
      const notification = JSON.stringify({ jsonrpc: JSONRPC_VERSION, method, params });
      const response = await fixture.postRPCDocument(notification);
      expect(response.status).toBe(noContent);
      await expect(response.text()).resolves.toBe("");
    }

    const call = JSON.stringify({
      jsonrpc: JSONRPC_VERSION,
      id: method,
      method,
      params: { title },
    });
    const duplicateTitle = call.replace('"title":', '"title":"First value","title":');
    for (const ambiguous of [
      duplicateTitle,
      `${call} {}`,
      call.replace(`"id":"${method}"`, '"id":null'),
    ]) {
      const response = await fixture.postRPCDocument(ambiguous);
      expect(response.status).toBe(badRequest);
      await expect(response.json()).resolves.toMatchObject({ status: badRequest });
    }
    await expect(sessionIds()).resolves.toEqual(existing);
  });

  it("returns a correlated reply for command admission failures", async () => {
    const id = "rejected-call";
    for (const { method, params, type } of [
      { method: "runtime.unknown", params: {}, type: "method_not_found" },
      { method: "sessions.create", params: { _meta: false }, type: "invalid_params" },
    ]) {
      const response = await fixture.postRPCDocument(
        JSON.stringify({ jsonrpc: JSONRPC_VERSION, id, method, params }),
      );
      expect(response.status).toBe(RPC_HTTP.callStatus);
      await expect(response.json()).resolves.toMatchObject({
        id,
        error: { data: { problem: { type } } },
      });
    }
  });

  it("rejects a mismatched unary reply without settling the request it names", async () => {
    const first = await fixture.client.sessions.create({ title: "Mismatched reply origin" });
    const second = await fixture.client.sessions.create({ title: "Matching reply origin" });
    const matchingId = Promise.withResolvers<string>();
    const releaseMatchingReply = Promise.withResolvers<void>();
    const client = fixture.createRuntimeClient(async (input, init) => {
      const response = await fetch(input, init);
      const request = JSON.parse(String(init?.body)) as {
        id: string;
        params: { sessionId: string };
      };
      if (request.params.sessionId === second.id) {
        matchingId.resolve(request.id);
        await releaseMatchingReply.promise;
        return response;
      }
      const reply = (await response.json()) as object;
      return new Response(JSON.stringify({ ...reply, id: await matchingId.promise }), {
        status: response.status,
        headers: response.headers,
      });
    });
    try {
      const mismatched = client.sessions.get(asSessionId(first.id));
      const matching = client.sessions.get(asSessionId(second.id));
      await Promise.all([
        expect(within(mismatched, "the mismatched reply to be refused"))
          .rejects.toBeInstanceOf(RpcProtocolError)
          .finally(releaseMatchingReply.resolve),
        expect(within(matching, "the matching reply to settle")).resolves.toMatchObject({
          id: second.id,
        }),
      ]);
    } finally {
      await client.close();
      await fixture.client.sessions.delete(asSessionId(first.id));
      await fixture.client.sessions.delete(asSessionId(second.id));
    }
  });

  it("rejects an undeclared field on a live unary RPC reply", async () => {
    const session = await fixture.client.sessions.create({ title: "Strict reply envelope" });
    const sessionId = asSessionId(session.id);
    let changeReply = true;
    const client = fixture.createRuntimeClient(async (input, init) => {
      const response = await fetch(input, init);
      if (!changeReply) return response;
      changeReply = false;
      expect(response.headers.get(HTTP_HEADERS.contentType)).toContain(RPC_HTTP.documentMediaType);
      const reply = (await response.json()) as object;
      return new Response(JSON.stringify({ ...reply, undeclared: true }), {
        status: response.status,
        headers: response.headers,
      });
    });
    try {
      await expect(client.sessions.get(sessionId)).rejects.toBeInstanceOf(RpcProtocolError);
      await expect(client.sessions.get(sessionId)).resolves.toMatchObject({ id: sessionId });
    } finally {
      await client.close();
      await fixture.client.sessions.delete(sessionId);
    }
  });

  it("correlates an HTTP refusal with the Runtime response header", async () => {
    const unsupportedMediaType = 415;
    let original: { requestId: string; body: object } | undefined;
    const client = fixture.createRuntimeClient(async (input, init) => {
      if (original) return fetch(input, init);
      const headers = new Headers(init?.headers);
      headers.set(HTTP_HEADERS.contentType, "text/plain");
      const response = await fetch(input, { ...init, headers });
      expect(response.status).toBe(unsupportedMediaType);
      const requestId = response.headers.get(HTTP_HEADERS.requestId);
      if (!requestId) throw new Error("expected the Runtime to issue a request ID");
      const body = (await response.json()) as object;
      original = { requestId, body };
      return new Response(JSON.stringify({ ...body, requestId: "unrelated-http-request" }), {
        status: response.status,
        headers: response.headers,
      });
    });
    try {
      const failure = await client.runtime.discover().then(
        () => {
          throw new Error("expected the HTTP request to be refused");
        },
        (error: unknown) => error,
      );
      if (!original) throw new Error("expected a refusal from the running Runtime");
      expect(failure).toBeInstanceOf(RpcTransportError);
      expect(failure).toMatchObject({
        requestId: original.requestId,
        status: unsupportedMediaType,
      });
      expect(original.body).not.toHaveProperty("requestId");
      await expect(client.runtime.discover()).resolves.toMatchObject({
        protocolVersion: PROTOCOL_VERSION,
      });
    } finally {
      await client.close();
    }
  });

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
    const staleStoreResponse = await fixture.postRPC(
      "sessions.create",
      { workspace: { path: fixture.root }, title: staleStoreTitle },
      {
        [HTTP_HEADERS.idempotencyKey]: "stale-store-session-create",
        [HTTP_HEADERS.idempotencyNamespace]: "idp_00000000000000000000000000000000",
      },
    );
    expect(staleStoreResponse.status).toBe(RPC_HTTP.callStatus);
    await expect(staleStoreResponse.json()).resolves.toMatchObject({
      error: { data: { problem: { type: "idempotency_store_mismatch" } } },
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

  it("refuses invalid topics before opening or joining a shared subscription", async () => {
    let openings = 0;
    const client = fixture.createRuntimeClient((input, init) => {
      openings++;
      return fetch(input, init);
    });
    const topic = "sessions.changed";
    try {
      await expect(client.runtimeEvents.subscribe({ topics: [] })).rejects.toBeInstanceOf(
        RpcProtocolError,
      );
      expect(openings).toBe(0);

      const subscription = await client.runtimeEvents.subscribe({ topics: [topic] });
      const events = subscription.events[Symbol.asyncIterator]();
      await expect(client.runtimeEvents.subscribe({ topics: [topic, topic] })).rejects.toBeInstanceOf(
        RpcProtocolError,
      );
      expect(openings).toBe(1);
      await events.return?.();
    } finally {
      await client.close();
    }
  });

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
    const response = await fixture.postRPC(
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
            problem: {
              type: "invalid_params",
              errors: expect.arrayContaining([expect.objectContaining({ field: "sessionId" })]),
            },
          },
        },
      });
    } finally {
      if (!response.bodyUsed) await response.body?.cancel();
    }
  });

  it("refuses an empty export format and defaults an omitted format", async () => {
    const session = await fixture.client.sessions.create({ title: "Session export format" });
    const omitted = await fixture.postRPC("sessions.export", { sessionId: session.id });
    await expect(omitted.json()).resolves.toMatchObject({ result: { format: "json" } });

    const empty = await fixture.postRPC("sessions.export", {
      sessionId: session.id,
      format: "",
    });
    expect(empty.status).toBe(RPC_HTTP.callStatus);
    await expect(empty.json()).resolves.toMatchObject({
      error: { data: { problem: { type: "invalid_params", errors: [{ field: "format" }] } } },
    });
  });

  it("refuses an empty request version and admits an omitted version", async () => {
    const title = "Request protocol version admission";
    const headers = { [HTTP_HEADERS.idempotencyKey]: crypto.randomUUID() };
    const sessionsWithTitle = async () =>
      (await fixture.client.sessions.list().autoPagingToArray()).filter(
        (session) => session.title === title,
      );

    try {
      const empty = await fixture.postRPC(
        "sessions.create",
        { title, _meta: { protocolVersion: "" } },
        headers,
      );
      expect(empty.status).toBe(RPC_HTTP.callStatus);
      await expect(empty.json()).resolves.toMatchObject({
        error: { data: { problem: { type: "invalid_protocol_version" } } },
      });
      await expect(sessionsWithTitle()).resolves.toEqual([]);

      const omitted = await fixture.postRPC("sessions.create", { title, _meta: {} }, headers);
      expect(omitted.status).toBe(RPC_HTTP.callStatus);
      const persisted = await sessionsWithTitle();
      expect(persisted).toHaveLength(1);
      await expect(omitted.json()).resolves.toMatchObject({
        result: { id: persisted[0]?.id, title },
      });
    } finally {
      for (const session of await sessionsWithTitle())
        await fixture.client.sessions.delete(asSessionId(session.id));
    }
  });

  it("refuses empty idempotency metadata and admits omitted metadata", async () => {
    const title = `Idempotency header presence ${crypto.randomUUID()}`;
    const sessionsWithTitle = async () =>
      (await fixture.client.sessions.list().autoPagingToArray()).filter(
        (session) => session.title === title,
      );

    try {
      const malformed: Record<string, string>[] = [
        { [HTTP_HEADERS.idempotencyKey]: "" },
        {
          [HTTP_HEADERS.idempotencyKey]: crypto.randomUUID(),
          [HTTP_HEADERS.idempotencyNamespace]: "",
        },
      ];
      for (const headers of malformed) {
        const empty = await fixture.postRPC("sessions.create", { title }, headers);
        await expect(sessionsWithTitle()).resolves.toEqual([]);
        expect(empty.status).toBe(400);
        expect(empty.headers.get(HTTP_HEADERS.contentType)).toContain(RPC_HTTP.problemMediaType);
        await expect(empty.json()).resolves.toMatchObject({
          type: "urn:flame:transport:invalid_request",
        });
      }

      const omitted = await fixture.postRPC("sessions.create", { title });
      expect(omitted.status).toBe(RPC_HTTP.callStatus);
      const persisted = await sessionsWithTitle();
      expect(persisted).toHaveLength(1);
      await expect(omitted.json()).resolves.toMatchObject({
        result: { id: persisted[0]?.id, title },
      });
    } finally {
      for (const session of await sessionsWithTitle())
        await fixture.client.sessions.delete(asSessionId(session.id));
    }
  });

  it("admits the same idempotency key through dispatch and receipt lookup", async () => {
    const namespace = fixture.negotiated?.limits.idempotency.namespace;
    if (!namespace) throw new Error("expected the Runtime's idempotency namespace");
    const lookup = (key: string) =>
      fixture.postRPC("runs.getAcceptance", { namespace, key, operation: "start" });
    const cancel = (key: string) =>
      fixture.postRPC(
        "runs.cancel",
        { runId: asRunId(crypto.randomUUID()) },
        { [HTTP_HEADERS.idempotencyKey]: key },
      );

    const multibyteKey = "é".repeat(128);
    const dispatched = await cancel(multibyteKey);
    expect(dispatched.status).toBe(RPC_HTTP.callStatus);
    await expect(dispatched.json()).resolves.toMatchObject({
      error: { data: { problem: { type: "invalid_params" } } },
    });

    const refused = await lookup(multibyteKey);
    expect(refused.status).toBe(RPC_HTTP.callStatus);
    await expect(refused.json()).resolves.toMatchObject({
      error: { data: { problem: { type: "invalid_params", errors: [{ field: "key" }] } } },
    });
    const portableKey = "k".repeat(MAXIMUM_IDEMPOTENCY_KEY_CHARACTERS - 1) + "/";
    await expect((await lookup(portableKey)).json()).resolves.toMatchObject({
      result: { type: "notRecorded" },
    });
    await expect((await cancel(portableKey)).json()).resolves.toMatchObject({
      error: { data: { problem: { type: "run_not_found" } } },
    });
    await expect((await lookup(portableKey)).json()).resolves.toMatchObject({
      error: { data: { problem: { type: "idempotency_conflict" } } },
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
        const response = await fixture.postRPC(
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
          data: {
            problem: { type: "invalid_params", errors: fields.map((field) => ({ field })) },
          },
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

  it("withdraws an unsent mutation and keeps its confirmed retry settled", async () => {
    const mutationStorage = new MemoryMutationStorage();
    let dispatched = 0;
    let available = true;
    const client = createFlameClient(
      createHttpTransport({
        baseUrl: fixture.baseUrl,
        fetch: (url, init) => {
          if (new Headers(init?.headers).get(HTTP_HEADERS.method) === "sessions.create") {
            dispatched++;
            if (!available) return Promise.resolve(new Response(null, { status: 503 }));
          }
          return fetch(url, init);
        },
      }),
      { capabilities: () => fixture.negotiated, mutationStorage },
    );
    let sessionId: ReturnType<typeof asSessionId> | undefined;
    try {
      const controller = new AbortController();
      const command = client.sessions.create(
        { title: "Confirmed before disconnect" },
        controller.signal,
      );
      controller.abort();
      await expect(command).rejects.toMatchObject({ name: "RpcCancellationError" });
      expect(mutationStorage.size).toBe(0);
      expect(dispatched).toBe(0);

      const resumed = command.retry({});
      expect(resumed.idempotencyKey).toBe(command.idempotencyKey);
      const created = await resumed;
      sessionId = asSessionId(created.id);
      expect(mutationStorage.size).toBe(0);

      available = false;
      const retry = command.retry();
      expect(retry.idempotencyKey).toBe(command.idempotencyKey);
      const observation = await retry.then(
        (result) => ({ result }),
        (error: unknown) => ({ error }),
      );
      expect(mutationStorage.size).toBe(0);
      expect(dispatched).toBe(1);
      expect(observation).toEqual({ result: created });
      await expect(fixture.client.sessions.get(sessionId)).resolves.toEqual(created);
    } finally {
      await client.close();
      if (sessionId) await fixture.client.sessions.delete(sessionId);
    }
  });

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

  it("requires the durable command to recover a cold opening and its snapshot tail", async () => {
    if (!fixture.negotiated) throw new Error("runtime client was not initialized");
    const session = await fixture.client.sessions.create({
      workspace: { path: fixture.root },
      title: "cold accepted opening",
    });
    const mutationStorage = new MemoryMutationStorage();
    let now = Date.now();
    const clock = vi.spyOn(Date, "now").mockImplementation(() => now);
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
    let coldLookupAvailable = true;
    let coldLookups = 0;
    const params = {
      sessionId: asSessionId(session.id),
      input: [{ type: "text" as const, text: "E2E_RUN_REPLAY_CUTPOINT cold receipt." }],
    };
    try {
      const pending = first.runs.start(params);
      await expect(pending).rejects.toMatchObject({ name: "MutationLookupError" });
      await within(gate.arrived.promise, "the cold opening model request");
      expect(mutationStorage.size).toBe(1);
      const evidence = mutationStorage.snapshot();
      await first.close();
      now += fixture.negotiated!.limits.idempotency.retentionSeconds * 1_000;
      cold = createFlameClient(
        createHttpTransport({
          baseUrl: fixture.baseUrl,
          fetch: (url, init) => {
            if (new Headers(init?.headers).get(HTTP_HEADERS.method) === "runs.getAcceptance") {
              coldLookups++;
              if (!coldLookupAvailable) return Promise.resolve(new Response(null, { status: 503 }));
            }
            return fetch(url, init);
          },
        }),
        { requestMeta, capabilities: () => fixture.negotiated, mutationStorage },
      );
      const [storageKey] = mutationStorage.keys();
      if (storageKey === undefined) throw new Error("expected the original journal key");
      const record = mutationStorage.get(storageKey);
      if (!record || typeof record !== "object" || Array.isArray(record))
        throw new Error("expected the original journal record");
      mutationStorage.set(storageKey, { ...record, command: null });
      const damagedRecovery = await cold.runs.recoverCommands().then(
        (commands) => ({ commands }),
        (error: unknown) => ({ error }),
      );
      const damagedRecord = mutationStorage.get(storageKey);
      mutationStorage.set(storageKey, record);
      const commands = await cold.runs.recoverCommands();
      expect(commands).toHaveLength(1);
      const [recorded] = commands;
      if (recorded?.operation !== "start") throw new Error("expected the original Run opening");
      expect(recorded).toMatchObject({
        idempotencyKey: pending.idempotencyKey,
        observation: {
          type: "recorded",
          acceptance: { type: "accepted", sessionId: session.id },
        },
      });
      expect(mutationStorage.snapshot()).toEqual(evidence);
      const recovered = cold.runs.start(params);
      expect(recovered.idempotencyKey).toBe(pending.idempotencyKey);
      expect(mutationStorage.snapshot()).toEqual(evidence);
      const opening = await recovered;
      expect(opening.observation).toEqual({ type: "snapshot", sessionId: session.id });
      expect(mutationStorage.size).toBe(0);
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
      expect(damagedRecovery).toMatchObject({ error: { name: "MutationJournalStorageError" } });
      expect(damagedRecord).toMatchObject({ command: null });

      coldLookupAvailable = false;
      const observedLookups = coldLookups;
      const replay = await recorded.retry().then(
        (result) => ({ result }),
        (error: unknown) => ({ error }),
      );
      expect(mutationStorage.size).toBe(0);
      expect(coldLookups).toBe(observedLookups);
      expect(replay).toEqual({ result: opening });
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
