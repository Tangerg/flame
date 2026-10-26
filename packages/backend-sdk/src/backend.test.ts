import { PassThrough } from "node:stream";
import { afterEach, expect, it } from "vitest";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { z } from "zod";
import { Backend } from "./backend";
import { Capability, PROTOCOL } from "./contract";
import { StdioTransport } from "./transport";
import wire from "./protocol.generated.json";

const contract = new Capability({
  protocol: PROTOCOL,
  reference: "fixture",
  composition: "collection",
  scope: "installation",
  methods: [
    {
      name: "fixture.read",
      kind: "query",
      target: "installation",
      description: "Read one original context.",
      input: {
        type: "object",
        properties: { value: { type: "string" } },
        required: ["value"],
        additionalProperties: false,
      },
      output: { type: "string" },
    },
  ],
});
const cleanup: Array<() => Promise<void>> = [];
afterEach(async () => {
  for (const close of cleanup.splice(0).reverse()) await close();
});
async function connect(backend: Backend) {
  const a = new PassThrough(),
    b = new PassThrough();
  const client = new Client(
    { name: "fixture", version: "1" },
    { capabilities: {} },
  );
  backend.publish();
  await backend.server.connect(new StdioTransport(a, b));
  await client.connect(new StdioTransport(b, a));
  cleanup.push(async () => {
    await client.close();
    await backend.close();
    await backend.server.close();
  });
  return client;
}
function params(input: unknown, extra = {}) {
  return {
    protocol: PROTOCOL,
    method: "fixture.read",
    contract: contract.method("fixture.read").identity,
    export: "fixture",
    capability: contract.identity,
    input,
    ...extra,
  };
}
const response = z.record(z.string(), z.unknown());
const contribution = { id: "fixture", export: "fixture", requires: [] };

it("keeps repeated structural admission pure and rejects invalid input before business dispatch", async () => {
  let executions = 0;
  const backend = new Backend({ name: "fixture", version: "1" });
  backend.bindCapability("fixture", contract);
  backend.implement(
    contract.method("fixture.read"),
    (value) => {
      const input = value as { value: string };
      if (input.value === "decoder-invalid")
        throw new Error("local codec rejected the value");
      return input;
    },
    (_context, input) => {
      executions++;
      return input.value;
    },
  );
  const client = await connect(backend);
  for (const value of [
    { value: "ok" },
    {},
    null,
    { value: "ok", extra: true },
    { value: "decoder-invalid" },
  ]) {
    const output = await client.request(
      { method: wire.constants.validateMethod!, params: params(value) },
      response,
    );
    expect(output.value === "").toBe(
      value !== null && Object.keys(value).length === 1 && value.value === "ok",
    );
  }
  expect(executions).toBe(0);
  await expect(
    client.request(
      {
        method: wire.constants.invokeMethod!,
        params: params({ value: "decoder-invalid" }),
      },
      response,
    ),
  ).rejects.toThrow();
  await expect(
    client.request(
      {
        method: wire.constants.invokeMethod!,
        params: params({ value: "ok" }, { contract: "different" }),
      },
      response,
    ),
  ).rejects.toThrow("contract mismatch");
  expect(executions).toBe(0);
  expect(
    await client.request(
      { method: wire.constants.invokeMethod!, params: params({ value: "ok" }) },
      response,
    ),
  ).toEqual({ value: "ok" });
  expect(executions).toBe(1);
});

it("captures independent immutable call contexts while asynchronous handlers interleave", async () => {
  const entered = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const backend = new Backend({ name: "fixture", version: "1" });
  backend.bindCapability("fixture", contract);
  backend.implement(
    contract.method("fixture.read"),
    (value) => value as { value: string },
    async (context, input) => {
      if (input.value === "first") {
        entered.resolve();
        await release.promise;
      }
      expect(Object.isFrozen(context.invocation!.target)).toBe(true);
      return (
        context.invocation!.id +
        ":" +
        context.invocation!.target.reference +
        ":" +
        context.callback!.call +
        ":" +
        input.value
      );
    },
  );
  const client = await connect(backend);
  const input = (id: string) =>
    params(
      { value: id },
      {
        _meta: {
          [wire.constants.InvocationMetadataKey!]: {
            id,
            origin: "human",
            target: { scope: "installation", reference: "target-" + id },
          },
          [wire.constants.callbackMetadataKey!]: {
            endpoint: "http://127.0.0.1:1234/",
            credential: "1".repeat(64),
            call: (id === "first" ? "2" : "3").repeat(64),
            maxResponseBytes: 1000,
          },
        },
      },
    );
  const first = client.request(
    { method: wire.constants.invokeMethod!, params: input("first") },
    response,
  );
  await entered.promise;
  expect(
    await client.request(
      { method: wire.constants.invokeMethod!, params: input("second") },
      response,
    ),
  ).toEqual({ value: "second:target-second:" + "3".repeat(64) + ":second" });
  release.resolve();
  expect(await first).toEqual({
    value: "first:target-first:" + "2".repeat(64) + ":first",
  });
});

it("closes partially acquired setup resources once after failure", async () => {
  let closes = 0;
  const backend = new Backend({ name: "fixture", version: "1" });
  backend.bindCapability("fixture", contract);
  backend.implement(
    contract.method("fixture.read"),
    (value) => value,
    () => "ok",
  );
  backend.onSetup((context) => {
    context.defer(() => {
      closes++;
    });
    throw new Error("setup failed after acquisition");
  });
  const client = await connect(backend);
  await expect(
    client.request(
      {
        method: wire.constants.setupMethod!,
        params: { protocol: PROTOCOL, contributions: [contribution] },
      },
      response,
    ),
  ).rejects.toThrow("setup failed");
  await backend.close();
  await backend.close();
  expect(closes).toBe(1);
  await expect(
    client.request(
      {
        method: wire.constants.activateMethod!,
        params: { protocol: PROTOCOL },
      },
      response,
    ),
  ).rejects.toThrow();
});

it("retains accepted setup bindings until EOF and cancels and joins active work before cleanup", async () => {
  const entered = Promise.withResolvers<void>();
  const finished = Promise.withResolvers<void>();
  let closes = 0;
  const backend = new Backend({ name: "fixture", version: "1" });
  backend.bindCapability("fixture", contract);
  backend.onSetup((context) => {
    expect(Object.isFrozen(context.contributions[0]!.requires)).toBe(true);
    return {
      close: async () => {
        await finished.promise;
        closes++;
      },
    };
  });
  backend.implement(
    contract.method("fixture.read"),
    (value) => value,
    async (context) => {
      entered.resolve();
      await new Promise<void>((resolve) =>
        context.signal.addEventListener("abort", () => resolve(), {
          once: true,
        }),
      );
      finished.resolve();
      context.signal.throwIfAborted();
      return "unreachable";
    },
  );
  const client = await connect(backend);
  await client.request(
    {
      method: wire.constants.setupMethod!,
      params: { protocol: PROTOCOL, contributions: [contribution] },
    },
    response,
  );
  await client.request(
    { method: wire.constants.activateMethod!, params: { protocol: PROTOCOL } },
    response,
  );
  const pending = client
    .request(
      {
        method: wire.constants.invokeMethod!,
        params: params({ value: "wait" }),
      },
      response,
    )
    .catch(() => {});
  await entered.promise;
  await client.close();
  await backend.close();
  await pending;
  expect(closes).toBe(1);
});

it("cancels setup and disposes acquisitions even when cancellation arrives during its await", async () => {
  const entered = Promise.withResolvers<void>();
  const disposed = Promise.withResolvers<void>();
  let closes = 0;
  const backend = new Backend({ name: "fixture", version: "1" });
  backend.bindCapability("fixture", contract);
  backend.implement(
    contract.method("fixture.read"),
    (value) => value,
    () => "ok",
  );
  backend.onSetup(async (context) => {
    context.defer(() => {
      closes++;
      disposed.resolve();
    });
    entered.resolve();
    await new Promise<void>((resolve) =>
      context.signal.addEventListener("abort", () => resolve(), { once: true }),
    );
  });
  const client = await connect(backend);
  const controller = new AbortController();
  const pending = client
    .request(
      {
        method: wire.constants.setupMethod!,
        params: { protocol: PROTOCOL, contributions: [contribution] },
      },
      response,
      { signal: controller.signal },
    )
    .catch(() => {});
  await entered.promise;
  controller.abort();
  await pending;
  await disposed.promise;
  await backend.close();
  expect(closes).toBe(1);
});
