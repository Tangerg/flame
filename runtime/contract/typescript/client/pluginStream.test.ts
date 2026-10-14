import { expect, it, vi } from "vitest";
import { createFlameClient } from "./sdk";
import { createHttpTransport } from "./transports/http";
import { createMutationJournal, type MutationJournalStorage } from "./mutationJournal";
import { MutationLookupError } from "./mutation";
import type { ServerCapabilities } from "@flame/runtime-contract/wire";

const contract = JSON.stringify({
  protocol: "io.github.tangerg.flame/backend/2026-10-14.2",
  reference: "records",
  composition: "selection",
  scope: "installation",
  methods: [
    {
      name: "records.stream",
      kind: "command",
      target: "installation",
      description: "Stream original records.",
      input: { type: "object" },
      output: { type: "boolean" },
      stream: { type: "string" },
    },
  ],
});
const input = { contract, method: "records.stream", input: {} };
const binding = {
  installationId: "12345678-1234-1234-1234-123456789abc",
  digest: "1".repeat(64),
  contribution: "records",
  target: { scope: "installation", reference: "12345678-1234-1234-1234-123456789abc" },
};

function storage() {
  const entries = new Map<string, unknown>();
  return {
    get: (key: string) => structuredClone(entries.get(key)),
    set: (key: string, value: unknown) => void entries.set(key, structuredClone(value)),
    remove: (key: string) => void entries.delete(key),
    keys: () => [...entries.keys()],
  } satisfies MutationJournalStorage;
}

function frame(value: unknown): string {
  return `event: message\ndata: ${JSON.stringify(value)}\n\n`;
}
function notification(kind: string, value: unknown): string {
  return frame({ jsonrpc: "2.0", method: "notifications.plugin.stream", params: { kind, value } });
}

function open(store: MutationJournalStorage, events: string) {
  const scope = { namespace: "original-store", retentionSeconds: 3600 };
  const journal = createMutationJournal({ storage: store, scope: () => scope });
  const requests: { params: typeof binding; key: string | null }[] = [];
  const fetch = vi.fn<typeof globalThis.fetch>(async (_url, init) => {
    const request = JSON.parse(init!.body as string) as {
      id: string;
      method: string;
      params: typeof binding;
    };
    if (request.method !== "plugins.stream")
      return Response.json({ jsonrpc: "2.0", id: request.id, result: { data: [] } });
    requests.push({
      params: request.params,
      key: new Headers(init!.headers).get("Idempotency-Key"),
    });
    const body = frame({ jsonrpc: "2.0", id: request.id, result: {} }) + events;
    return new Response(
      new ReadableStream({
        start(controller) {
          controller.enqueue(new TextEncoder().encode(body));
          controller.close();
        },
      }),
      { headers: { "Content-Type": "text/event-stream" } },
    );
  });
  const client = createFlameClient(createHttpTransport({ baseUrl: "http://runtime.test", fetch }), {
    mutationJournal: journal,
    capabilities: () =>
      ({
        features: { plugins: { enabled: true } },
        limits: { idempotency: scope },
      }) as unknown as ServerCapabilities,
  });
  return { client, requests };
}

it("awaits chunk consumption without blocking a nested RPC or settling the original journal early", async () => {
  const store = storage();
  const { client } = open(store, notification("chunk", "partial") + notification("result", true));
  const gate = Promise.withResolvers<void>();
  const nested = Promise.withResolvers<void>();
  const decode = vi.fn((value: unknown) => value);
  try {
    const result = client.plugins.stream(
      "original",
      input,
      async () => binding,
      async (chunk) => {
        expect(chunk).toBe("partial");
        await client.plugins.list();
        nested.resolve();
        await gate.promise;
      },
      decode,
    );
    await nested.promise;
    expect(decode).not.toHaveBeenCalled();
    expect(store.keys()).toHaveLength(1);
    gate.resolve();
    await expect(result).resolves.toBe(true);
    expect(store.keys()).toHaveLength(0);
  } finally {
    gate.resolve();
    await client.close();
  }
});

it("retains unknown output and recovers the original receipt binding after client replacement without chunk replay", async () => {
  const store = storage();
  const first = open(store, notification("chunk", "partial"));
  await expect(
    first.client.plugins.stream(
      "original",
      input,
      async () => binding,
      () => undefined,
      (value) => value,
    ),
  ).rejects.toBeInstanceOf(MutationLookupError);
  expect(first.requests).toHaveLength(1);
  expect(store.keys()).toHaveLength(1);
  await first.client.close();
  const next = open(store, notification("result", true));
  const prepare = vi.fn(async () => ({ ...binding, contribution: "replacement" }));
  const consume = vi.fn();
  try {
    await expect(
      next.client.plugins.stream("original", input, prepare, consume, (value) => value),
    ).resolves.toBe(true);
    expect(prepare).not.toHaveBeenCalled();
    expect(consume).not.toHaveBeenCalled();
    expect(next.requests[0]!.params.installationId).toBe(binding.installationId);
    expect(next.requests[0]!.params.contribution).toBe(binding.contribution);
    expect(next.requests[0]!.key).toBe(first.requests[0]!.key);
    expect(store.keys()).toHaveLength(0);
  } finally {
    await next.client.close();
  }
});

it("refuses unsafe numeric stream evidence without retiring the original command or retrying", async () => {
  const store = storage();
  const unsafe =
    'event: message\ndata: {"jsonrpc":"2.0","method":"notifications.plugin.stream","params":{"kind":"result","value":18446744073709551615}}\n\n';
  const { client, requests } = open(store, unsafe);
  const decode = vi.fn();
  try {
    await expect(
      client.plugins.stream(
        "original",
        input,
        async () => binding,
        () => undefined,
        decode,
      ),
    ).rejects.toBeInstanceOf(MutationLookupError);
    expect(decode).not.toHaveBeenCalled();
    expect(requests).toHaveLength(1);
    expect(store.keys()).toHaveLength(1);
  } finally {
    await client.close();
  }
});

it("client closure cancels and joins the in-flight chunk consumer while retaining original uncertainty", async () => {
  const store = storage();
  const { client } = open(store, notification("chunk", "partial"));
  const started = Promise.withResolvers<void>();
  const gate = Promise.withResolvers<void>();
  let stopped = false;
  const result = client.plugins.stream(
    "original",
    input,
    async () => binding,
    async (_chunk, signal) => {
      started.resolve();
      await new Promise<void>((resolve) =>
        signal.addEventListener("abort", () => resolve(), { once: true }),
      );
      await gate.promise;
      stopped = true;
    },
    (value) => value,
  );
  const failure = result.then(
    () => undefined,
    (error: unknown) => error,
  );
  await started.promise;
  let closed = false;
  const close = client.close().then(() => {
    closed = true;
  });
  expect(await failure).toBeInstanceOf(MutationLookupError);
  expect(closed).toBe(false);
  gate.resolve();
  await close;
  expect(stopped).toBe(true);
  expect(store.keys()).toHaveLength(1);
});

it("reads a query stream without reserving a mutation and permits reentrant reads", async () => {
  const store = storage();
  const { client, requests } = open(
    store,
    notification("chunk", "partial") + notification("result", true),
  );
  const declaration = JSON.parse(contract) as { methods: { kind: string }[] };
  declaration.methods[0]!.kind = "query";
  try {
    const result = client.plugins.queryStream(
      { ...binding, ...input, contract: JSON.stringify(declaration) },
      async (chunk) => {
        expect(chunk).toBe("partial");
        expect(store.keys()).toHaveLength(0);
        await client.plugins.list();
      },
      (value) => value,
    );
    await expect(result).resolves.toBe(true);
    expect(requests).toHaveLength(1);
    expect(requests[0]!.key).toBeNull();
    expect(store.keys()).toHaveLength(0);
  } finally {
    await client.close();
  }
});

it("keeps missing query stream evidence a read failure without mutation recovery", async () => {
  const store = storage();
  const { client, requests } = open(store, notification("chunk", "partial"));
  const declaration = JSON.parse(contract) as { methods: { kind: string }[] };
  declaration.methods[0]!.kind = "query";
  try {
    const failure = await client.plugins
      .queryStream(
        { ...binding, ...input, contract: JSON.stringify(declaration) },
        () => undefined,
        (value) => value,
      )
      .catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(Error);
    expect(failure).not.toBeInstanceOf(MutationLookupError);
    expect(requests).toHaveLength(1);
    expect(store.keys()).toHaveLength(0);
  } finally {
    await client.close();
  }
});
