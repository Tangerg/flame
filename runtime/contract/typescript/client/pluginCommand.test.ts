import { afterEach, expect, it, vi } from "vitest";
import { CAPABILITY_JSON } from "../../../../plugins/memory/io.github.tangerg.flame/contracts/methods.generated";
import type { CallPluginRequest, CallPluginResponse } from "@flame/runtime-contract/wire";
import type { RpcClient } from "./client";
import { OperationFailure, RpcError, RpcTransportError } from "./errors";
import { createMethods } from "./methods";
import { MutationLookupError } from "./mutation";
import {
  createMutationJournal,
  MutationJournalOwnershipError,
  MutationJournalStorageError,
  type MutationJournal,
  type MutationJournalStorage,
} from "./mutationJournal";
import { MutationJournalError } from "./durableMutationJournal";

class Storage implements MutationJournalStorage {
  readonly values = new Map<string, unknown>();
  failWrites = false;
  get(key: string): unknown {
    return structuredClone(this.values.get(key));
  }
  set(key: string, value: unknown): void {
    if (this.failWrites) throw new Error("write failed");
    this.values.set(key, structuredClone(value));
  }
  remove(key: string): void {
    this.values.delete(key);
  }
  keys(): string[] {
    return [...this.values.keys()];
  }
}

const id = "mem_00000000000000000000000000000001";
const input = { contract: CAPABILITY_JSON, method: "flame.memory/delete.v1", input: { ID: id } };
const installationId = "12345678-1234-1234-1234-123456789abc";
const binding = {
  installationId,
  digest: "1".repeat(64),
  contribution: "memory-provider",
  target: { scope: "installation", reference: installationId },
};
const intent = `memory-delete:${id}`;
const journals: MutationJournal[] = [];
afterEach(() => {
  for (const journal of journals.splice(0)) journal.dispose();
});

function client(
  storage: Storage,
  scope = () => ({ namespace: "original-store", retentionSeconds: 3600 }),
  now = Date.now,
) {
  const journal = createMutationJournal({ storage, scope, now });
  journals.push(journal);
  const dispatch =
    vi.fn<
      (
        method: string,
        request: CallPluginRequest,
        options: { idempotencyKey: string; idempotencyNamespace: string },
      ) => Promise<CallPluginResponse>
    >();
  const methods = createMethods({ call: dispatch } as unknown as RpcClient, {
    mutationJournal: journal,
  });
  return { journal, dispatch, methods };
}

it("recovers the original installed request before provider preparation after client replacement", async () => {
  const storage = new Storage();
  const original = client(storage);
  original.dispatch.mockRejectedValue(new RpcTransportError("response lost"));
  const prepare = vi.fn(async () => binding);
  await expect(
    original.methods.plugins.command(intent, input, prepare, () => undefined),
  ).rejects.toBeInstanceOf(RpcTransportError);
  const [, request, options] = original.dispatch.mock.calls[0]!;
  expect(original.dispatch).toHaveBeenCalledTimes(2);
  expect(request).toEqual({ ...binding, ...input });
  expect(options.idempotencyNamespace).toBe("original-store");
  expect(storage.keys()).toHaveLength(1);
  original.journal.dispose();

  const successor = client(storage);
  successor.dispatch.mockResolvedValue({ output: {} });
  const replacement = vi.fn(async () => ({
    ...binding,
    installationId: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  }));
  const unavailable = new RpcError({
    message: "original provider is unavailable",
    data: { type: "plugin_unavailable" },
  });
  successor.dispatch.mockRejectedValueOnce(unavailable);
  await expect(
    successor.methods.plugins.command(intent, input, replacement, () => undefined),
  ).rejects.toMatchObject({ cause: unavailable });
  expect(storage.keys()).toHaveLength(1);
  await successor.methods.plugins.command(intent, input, replacement, () => undefined);
  expect(replacement).not.toHaveBeenCalled();
  expect(successor.dispatch.mock.calls[0]).toEqual(["plugins.call", request, options]);
  expect(successor.dispatch.mock.calls[1]).toEqual(["plugins.call", request, options]);
  expect(storage.keys()).toEqual([]);
});

it("keeps original input and guards expired or replaced-store recovery without another dispatch", async () => {
  const storage = new Storage();
  let namespace = "original-store";
  let now = 1000;
  const scope = () => ({ namespace, retentionSeconds: 3600 });
  const original = client(storage, scope, () => now);
  original.dispatch.mockRejectedValue(new RpcTransportError("response lost"));
  await expect(
    original.methods.plugins.command(
      intent,
      input,
      async () => binding,
      () => undefined,
    ),
  ).rejects.toBeInstanceOf(RpcTransportError);
  original.journal.dispose();
  const successor = client(storage, scope, () => now);
  const prepare = vi.fn(async () => binding);
  await expect(
    successor.methods.plugins.command(
      intent,
      { ...input, input: { ID: "changed" } },
      prepare,
      () => undefined,
    ),
  ).rejects.toBeInstanceOf(MutationJournalOwnershipError);
  namespace = "replacement-store";
  await expect(
    successor.methods.plugins.command(intent, input, prepare, () => undefined),
  ).rejects.toBeInstanceOf(MutationJournalOwnershipError);
  namespace = "original-store";
  now += 3600 * 1000;
  await expect(
    successor.methods.plugins.command(intent, input, prepare, () => undefined),
  ).rejects.toBeInstanceOf(MutationJournalError);
  expect(prepare).not.toHaveBeenCalled();
  expect(successor.dispatch).not.toHaveBeenCalled();
  expect(storage.keys()).toHaveLength(1);
});

it("retains a decoder refusal as unknown and retires only a witnessed owner refusal", async () => {
  const storage = new Storage();
  const current = client(storage);
  current.dispatch.mockResolvedValue({ output: { invalid: true } });
  const prepare = vi.fn(async () => binding);
  const refusal = new OperationFailure("not_found", "decoder cannot confirm");
  await expect(
    current.methods.plugins.command(intent, input, prepare, () => {
      throw refusal;
    }),
  ).rejects.toMatchObject({ cause: refusal });
  expect(storage.keys()).toHaveLength(1);
  const key = current.dispatch.mock.calls[0]![2].idempotencyKey;
  current.dispatch.mockResolvedValue({ failure: { code: "not_found", message: "owner refused" } });
  const decode = vi.fn();
  await expect(
    current.methods.plugins.command(intent, input, prepare, decode),
  ).rejects.toBeInstanceOf(OperationFailure);
  expect(current.dispatch.mock.calls[1]![2].idempotencyKey).toBe(key);
  expect(prepare).toHaveBeenCalledTimes(1);
  expect(decode).not.toHaveBeenCalled();
  expect(storage.keys()).toEqual([]);
});

it("stages one complete record before dispatch and refuses a failed durable write", async () => {
  const storage = new Storage();
  storage.failWrites = true;
  const current = client(storage);
  await expect(
    current.methods.plugins.command(
      intent,
      { ...input, input: { unserializable: () => undefined } },
      async () => binding,
      () => undefined,
    ),
  ).rejects.toMatchObject({ name: "DataCloneError" });
  await expect(
    current.methods.plugins.command(
      intent,
      input,
      async () => binding,
      () => undefined,
    ),
  ).rejects.toBeInstanceOf(MutationJournalStorageError);
  expect(current.dispatch).not.toHaveBeenCalled();
  expect(storage.keys()).toEqual([]);
  storage.failWrites = false;
  await expect(
    current.methods.plugins.command(
      "query-intent",
      { ...input, method: "flame.memory/list.v1", input: {} },
      async () => binding,
      () => undefined,
    ),
  ).rejects.toBeInstanceOf(MutationJournalError);
  expect(current.dispatch).not.toHaveBeenCalled();
  expect(storage.keys()).toEqual([]);
  current.dispatch.mockImplementation(async () => {
    expect(storage.keys()).toHaveLength(1);
    const record = [...storage.values.values()][0] as { installed: { route: unknown } };
    expect(record.installed.route).toEqual({
      ...binding,
      contract: input.contract,
      method: input.method,
    });
    throw new RpcTransportError("response lost");
  });
  await expect(
    current.methods.plugins.command(
      intent,
      input,
      async () => binding,
      () => undefined,
    ),
  ).rejects.toBeInstanceOf(RpcTransportError);
  expect(storage.keys()).toHaveLength(1);
});

it("does not persist authored input while retaining an unknown acknowledgement", async () => {
  const storage = new Storage();
  const current = client(storage);
  const authored = {
    ...input,
    method: "flame.memory/update.v1",
    input: { ID: id, Content: "do not persist this authored content" },
  };
  current.dispatch.mockResolvedValue({ output: {} });
  await expect(
    current.methods.plugins.command(
      intent,
      authored,
      async () => binding,
      () => {
        throw new Error("bad acknowledgement");
      },
    ),
  ).rejects.toBeInstanceOf(MutationLookupError);
  expect(JSON.stringify([...storage.values.values()])).not.toContain(
    "do not persist this authored content",
  );
  const prepare = vi.fn(async () => binding);
  await expect(
    current.methods.plugins.command(
      intent,
      { ...authored, input: { Content: authored.input.Content, ID: id } },
      prepare,
      () => undefined,
    ),
  ).rejects.toBeInstanceOf(MutationJournalOwnershipError);
  expect(prepare).not.toHaveBeenCalled();
  expect(current.dispatch).toHaveBeenCalledTimes(1);
});

it("fences concurrent preparation and revoked client ownership before staging", async () => {
  const storage = new Storage();
  const current = client(storage);
  let release!: (value: typeof binding) => void;
  const prepare = vi.fn(
    () =>
      new Promise<typeof binding>((resolve) => {
        release = resolve;
      }),
  );
  const pending = current.methods.plugins.command(intent, input, prepare, () => undefined);
  const rejected = pending.catch((error: unknown) => error);
  await expect(
    current.methods.plugins.command(intent, input, prepare, () => undefined),
  ).rejects.toBeInstanceOf(MutationJournalOwnershipError);
  current.journal.dispose();
  release(binding);
  expect(await rejected).toBeInstanceOf(MutationJournalOwnershipError);
  expect(prepare).toHaveBeenCalledTimes(1);
  expect(current.dispatch).not.toHaveBeenCalled();
  expect(storage.keys()).toEqual([]);
  const capturedClient = client(storage);
  const captured = await capturedClient.journal.capturePluginCommand(
    intent,
    input,
    async () => binding,
  );
  capturedClient.journal.dispose();
  const create = vi.fn(() => {
    throw new Error("retired client created a delivery");
  });
  expect(() => captured.reservation.track(create)).toThrow(MutationJournalOwnershipError);
  expect(create).not.toHaveBeenCalled();
  expect(storage.keys()).toHaveLength(1);
});
