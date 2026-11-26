import { mkdir, realpath } from "node:fs/promises";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { asSessionId, createFlameClient, createHttpTransport } from "@flame/runtime-client";
import { HTTP_HEADERS, RPC_HTTP } from "@flame/runtime-client/wire";
import { useRuntimeFixture } from "./fixture";
import { faultUnaryResponse } from "./transportFaults";

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: capability conditions", () => {
  const fixture = useRuntimeFixture("capabilities", { isolationUnavailable: true });

  it("requires isolation for an explicit false patch before reserving its key", async () => {
    expect(fixture.negotiated?.features.isolation?.enabled).toBe(false);
    const created = await fixture.client.sessions.create({ title: "Original session" });
    const sessionId = asSessionId(created.id);
    const headers = { [HTTP_HEADERS.idempotencyKey]: crypto.randomUUID() };
    const update = { sessionId, expectedRevision: created.revision, title: "Edited session" };

    let dispatched = 0;
    const client = fixture.createRuntimeClient((...args) => {
      dispatched++;
      return fetch(...args);
    });
    try {
      await expect(client.sessions.update({ ...update, isolated: false })).rejects.toMatchObject({
        data: {
          type: "capability_not_negotiated",
          requiredCapabilities: [{ type: "feature", name: "isolation" }],
        },
      });
      expect(dispatched).toBe(0);
    } finally {
      await client.close();
    }

    const denied = await fixture.postRPC(
      "sessions.update",
      { ...update, isolated: false },
      headers,
    );
    await expect(fixture.client.sessions.get(sessionId)).resolves.toEqual(created);
    expect(denied.status).toBe(RPC_HTTP.callStatus);
    await expect(denied.json()).resolves.toMatchObject({
      error: {
        data: {
          type: "capability_not_negotiated",
          requiredCapabilities: [{ type: "feature", name: "isolation" }],
        },
      },
    });

    const accepted = await fixture.postRPC("sessions.update", update, headers);
    expect(accepted.status).toBe(RPC_HTTP.callStatus);
    await expect(accepted.json()).resolves.toMatchObject({
      result: { id: sessionId, revision: created.revision + 1, title: update.title },
    });
  });

  it("replays a committed relocation after the client's discovery changes", async () => {
    const discovery = fixture.negotiated;
    if (!discovery) throw new Error("Runtime discovery is unavailable");
    const relocation = discovery.features.relocate;
    if (!relocation?.enabled) throw new Error("Runtime capability relocation is unavailable");
    const destination = join(fixture.root, "relocated-session");
    await mkdir(destination);
    const original = await fixture.client.sessions.create({
      title: "original workspace",
      workspace: { path: fixture.root },
    });
    const sessionId = asSessionId(original.id);
    const unavailable = {
      ...discovery,
      features: { ...discovery.features, relocate: { ...relocation, enabled: false } },
    };
    const loss = faultUnaryResponse("sessions.update", "afterCommit");
    let capabilities = discovery;
    const sentKeys: Array<string | null> = [];
    const client = createFlameClient(
      createHttpTransport({
        baseUrl: fixture.baseUrl,
        fetch: async (input, init) => {
          sentKeys.push(new Headers(init?.headers).get(HTTP_HEADERS.idempotencyKey));
          const response = await loss.fetch(input, init);
          capabilities = unavailable;
          return response;
        },
      }),
      { capabilities: () => capabilities },
    );
    try {
      const command = client.sessions.update({
        sessionId,
        expectedRevision: original.revision,
        workspace: { path: destination },
      });
      const outcome = await command.then(
        (value) => ({ value }),
        (error: unknown) => ({ error }),
      );
      const committed = await fixture.client.sessions.get(sessionId);
      expect(committed).toMatchObject({
        revision: original.revision + 1,
        workspace: { ref: { path: await realpath(destination) } },
      });
      expect(outcome).toEqual({ value: committed });
      expect(sentKeys).toEqual([command.idempotencyKey, command.idempotencyKey]);
    } finally {
      await client.close();
    }
  });

  it("requires the child Run opt-in when the descendant flag is true", async () => {
    const withoutDescendants = await fixture.postRPC("runs.list", { includeDescendants: false });
    await expect(withoutDescendants.json()).resolves.toMatchObject({ result: { data: [] } });

    const withDescendants = await fixture.postRPC("runs.list", { includeDescendants: true });
    await expect(withDescendants.json()).resolves.toMatchObject({
      error: {
        data: {
          type: "capability_not_negotiated",
          requiredCapabilities: [{ type: "feature", name: "childRuns" }],
        },
      },
    });
  });
});
