import { afterEach, expect, it, vi } from "vitest";
import { createRpcClient } from "./client";
import { RpcTransportError } from "./errors";
import { createMethods } from "./methods";
import { createMutationJournal, type MutationJournalStorage } from "./mutationJournal";
import { createMemoryTransport } from "./transports/memory";
import type { TransportRequest } from "./transport";
import { createWireCallPath } from "./wireCallPath";
import { MutationJournalStorageError } from "./mutationJournal";

const cleanups: (() => Promise<void>)[] = [];
afterEach(async () => {
  for (const cleanup of cleanups.splice(0)) await cleanup();
});

function fixture(onPersist = () => {}, entries = new Map<string, unknown>()) {
  const storage: MutationJournalStorage = {
    get: (key) => structuredClone(entries.get(key)),
    set: (key, value) => {
      entries.set(key, structuredClone(value));
      onPersist();
    },
    remove: (key) => void entries.delete(key),
    keys: () => [...entries.keys()],
  };
  const journal = createMutationJournal({
    storage,
    scope: () => ({ namespace: "admission-store", retentionSeconds: 3600 }),
  });
  const transport = createMemoryTransport();
  let dispatched = Promise.withResolvers<TransportRequest>();
  const send = transport.send.bind(transport);
  const sends = vi.spyOn(transport, "send").mockImplementation((...args) => {
    const result = send(...args);
    dispatched.resolve(args[0]);
    return result;
  });
  const rpc = createRpcClient(transport);
  cleanups.push(async () => {
    journal.dispose();
    await rpc.close();
  });
  return {
    entries,
    storage,
    journal,
    transport,
    sends,
    call: createWireCallPath(rpc, { mutationJournal: journal }).call,
    methods: createMethods(rpc, { mutationJournal: journal }),
    nextRequest: () => dispatched.promise,
    resetRequest: () => {
      dispatched = Promise.withResolvers<TransportRequest>();
    },
  };
}

it("does not reserve capacity for 256 distinct pre-canceled mutations", async () => {
  const f = fixture();
  const signal = AbortSignal.abort();
  for (let index = 0; index < 256; index++) {
    await expect(
      f.call("sessions.delete", { sessionId: `ses_${index}` }, { signal }),
    ).rejects.toBeInstanceOf(RpcTransportError);
  }
  expect(f.entries.size).toBe(0);
  expect(f.sends).not.toHaveBeenCalled();
  const normal = f.call("sessions.delete", { sessionId: "ses_normal" });
  const request = await f.nextRequest();
  f.transport.inject({ jsonrpc: "2.0", id: request.id, result: {} });
  await expect(normal).resolves.toBeUndefined();
  expect(f.entries.size).toBe(0);
});

it("releases a new reservation canceled before its execution microtask", async () => {
  const controller = new AbortController();
  const f = fixture(() => controller.abort());
  await expect(
    f.call("sessions.delete", { sessionId: "ses_reserved" }, { signal: controller.signal }),
  ).rejects.toBeInstanceOf(RpcTransportError);
  expect(f.sends).not.toHaveBeenCalled();
  expect(f.entries.size).toBe(0);
});

it("releases a new reservation canceled at RPC admission before transport dispatch", async () => {
  const f = fixture();
  const controller = new AbortController();
  await expect(
    f.call(
      "sessions.delete",
      { sessionId: "ses_reserved" },
      {
        signal: controller.signal,
        onRequestRpcId: () => controller.abort(),
      },
    ),
  ).rejects.toBeInstanceOf(RpcTransportError);
  expect(f.sends).not.toHaveBeenCalled();
  expect(f.entries.size).toBe(0);
});

it("retains a dispatched identity through cancellation and a pre-canceled retry", async () => {
  const f = fixture();
  const controller = new AbortController();
  const original = f.call(
    "sessions.delete",
    { sessionId: "ses_sent" },
    { signal: controller.signal },
  );
  const rejected = original.catch((error: unknown) => error);
  await f.nextRequest();
  controller.abort();
  expect(await rejected).toBeInstanceOf(RpcTransportError);
  expect(f.entries.size).toBe(1);
  const canceled = original.retry({ signal: AbortSignal.abort() });
  await expect(canceled).rejects.toBeInstanceOf(RpcTransportError);
  expect(canceled.idempotencyKey).toBe(original.idempotencyKey);
  expect(f.sends).toHaveBeenCalledTimes(1);
  expect(f.entries.size).toBe(1);
  f.resetRequest();
  const retry = canceled.retry({});
  const request = await f.nextRequest();
  expect(f.sends.mock.calls.map((call) => call[2]?.idempotencyKey)).toEqual([
    original.idempotencyKey,
    original.idempotencyKey,
  ]);
  f.transport.inject({ jsonrpc: "2.0", id: request.id, result: {} });
  await expect(retry).resolves.toBeUndefined();
  expect(f.entries.size).toBe(0);
});

it("retains dispatched evidence across client replacement and cancellation before replay", async () => {
  const first = fixture();
  const params = { sessionId: "ses_restart" };
  const controller = new AbortController();
  const original = first.call("sessions.delete", params, { signal: controller.signal });
  const failed = original.catch((error: unknown) => error);
  await first.nextRequest();
  controller.abort();
  expect(await failed).toBeInstanceOf(RpcTransportError);
  first.journal.dispose();
  const second = fixture(undefined, first.entries);
  const canceled = new AbortController();
  const recovered = second.call("sessions.delete", params, { signal: canceled.signal });
  canceled.abort();
  await expect(recovered).rejects.toBeInstanceOf(RpcTransportError);
  expect(recovered.idempotencyKey).toBe(original.idempotencyKey);
  expect(second.sends).not.toHaveBeenCalled();
  expect(second.entries.size).toBe(1);
  const retry = recovered.retry({});
  const request = await second.nextRequest();
  second.transport.inject({ jsonrpc: "2.0", id: request.id, result: {} });
  await expect(retry).resolves.toBeUndefined();
  expect(second.entries.size).toBe(0);
});

it("refuses transport dispatch when its durable dispatch record cannot be written", async () => {
  const f = fixture();
  const persist = f.storage.set;
  f.storage.set = (key, value) => {
    if ((value as { delivery: string }).delivery === "dispatched") throw new Error("write refused");
    persist(key, value);
  };
  await expect(f.call("sessions.delete", { sessionId: "ses_write_failure" })).rejects.toThrow(
    "not persisted",
  );
  expect(f.sends).not.toHaveBeenCalled();
  expect(f.entries.size).toBe(1);
});

it.each([undefined, "future-phase"])(
  "refuses a journal with delivery phase %s without discarding evidence",
  async (delivery) => {
    const first = fixture();
    first.journal.reserve("sessions.delete", { sessionId: "ses_old" });
    first.journal.dispose();
    const [key, raw] = [...first.entries][0]!;
    const entry = raw as Record<string, unknown>;
    if (delivery === undefined) delete entry.delivery;
    else entry.delivery = delivery;
    const evidence = structuredClone(entry);
    const second = fixture(undefined, first.entries);
    await expect(second.call("sessions.delete", { sessionId: "ses_new" })).rejects.toBeInstanceOf(
      MutationJournalStorageError,
    );
    expect(second.sends).not.toHaveBeenCalled();
    expect(second.entries.get(key)).toEqual(evidence);
  },
);

const installationId = "12345678-1234-1234-1234-123456789abc";
const binding = {
  installationId,
  digest: "1".repeat(64),
  contribution: "records",
  target: { scope: "installation", reference: installationId },
};
const descriptor = {
  protocol: "fixture/backend/v1",
  reference: "records",
  composition: "selection",
  scope: "installation",
  methods: [
    {
      name: "records.delete",
      kind: "command",
      target: "installation",
      description: "Delete records",
      input: { type: "object" },
      output: { type: "object" },
    },
  ],
};

function startPlugin(
  f: ReturnType<typeof fixture>,
  prepare: () => Promise<typeof binding>,
  signal: AbortSignal,
  streaming: boolean,
) {
  const input = {
    contract: JSON.stringify({
      ...descriptor,
      methods: descriptor.methods.map((method) => ({
        ...method,
        ...(streaming ? { stream: { type: "string" } } : {}),
      })),
    }),
    method: "records.delete",
    input: "{}",
  };
  return streaming
    ? f.methods.plugins.stream(
        "delete",
        input,
        prepare,
        () => {},
        (value) => value,
        signal,
      )
    : f.methods.plugins.command("delete", input, prepare, (value) => value, signal);
}

it.each([false, true])(
  "refuses pre-canceled plugin preparation (streaming=%s)",
  async (streaming) => {
    const f = fixture();
    const prepare = vi.fn(async () => binding);
    await expect(startPlugin(f, prepare, AbortSignal.abort(), streaming)).rejects.toBeInstanceOf(
      RpcTransportError,
    );
    expect(prepare).not.toHaveBeenCalled();
    expect(f.entries.size).toBe(0);
    expect(f.sends).not.toHaveBeenCalled();
  },
);

it.each([false, true])(
  "does not stage a canceled plugin preparation (streaming=%s)",
  async (streaming) => {
    const f = fixture();
    const prepared = Promise.withResolvers<typeof binding>();
    const entered = Promise.withResolvers<void>();
    const controller = new AbortController();
    const pending = startPlugin(
      f,
      () => {
        entered.resolve();
        return prepared.promise;
      },
      controller.signal,
      streaming,
    );
    const rejected = pending.catch((error: unknown) => error);
    await entered.promise;
    controller.abort();
    prepared.resolve(binding);
    expect(await rejected).toBeInstanceOf(RpcTransportError);
    expect(f.entries.size).toBe(0);
    expect(f.sends).not.toHaveBeenCalled();
  },
);
