import { stringifyJSON, parseJSON } from "@qimu/sdk/json";
import { readFileSync } from "node:fs";
import { afterEach, expect, it, vi } from "vitest";
import type { CallPluginRequest, CallPluginResponse } from "@flame/runtime-client/wire";
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

const contract = stringifyJSON({
  protocol: "fixture/backend/v1",
  reference: "records",
  composition: "selection",
  scope: "installation",
  methods: [
    { name: "records.delete", kind: "command" },
    { name: "records.update", kind: "command" },
    { name: "records.list", kind: "query" },
  ].map((method) => ({
    ...method,
    target: "installation",
    description: method.name,
    input: { type: "object" },
    output: { type: "object" },
  })),
});
const id = "mem_00000000000000000000000000000001";
const input = {
  contract,
  method: "records.delete",
  input: stringifyJSON({ ID: id }),
};
const installationId = "12345678-1234-1234-1234-123456789abc";
const binding = {
  installationId,
  digest: "1".repeat(64),
  contribution: "record-provider",
  target: { scope: "installation", reference: installationId },
};
const intent = `record-delete:${id}`;
const journals: MutationJournal[] = [];

it("matches original stable intents across client replacement without exposing input or mutating the journal", async () => {
  const storage = new Storage();
  const original = client(storage);
  original.dispatch.mockRejectedValue(new RpcTransportError("reply lost"));
  const actions = [
    { intent: "document-a:submit:delete", input: '{"ID":9007199254740993}' },
    { intent: "document-b:submit:delete", input: '{"ID":9007199254740994}' },
    { intent: "document-a:alternate:delete", input: '{"ID":9007199254740995}' },
  ];
  for (const action of actions)
    await expect(
      original.methods.plugins.command(
        action.intent,
        { ...input, input: action.input },
        async () => binding,
        (value) => value,
      ),
    ).rejects.toBeInstanceOf(RpcTransportError);
  const evidence = structuredClone([...storage.values]);
  original.journal.dispose();
  const successor = client(storage);
  const pending = successor.methods.plugins.pendingCommands();
  expect(pending).toHaveLength(3);
  for (const action of actions) {
    const matches = pending.filter((command) => command.matchesIntent(action.intent));
    expect(matches).toHaveLength(1);
    expect(matches[0]!.binding).toEqual({
      ...binding,
      contract: input.contract,
      method: input.method,
    });
  }
  expect(pending.every((command) => !command.matchesIntent("unrelated-action"))).toBe(true);
  expect([...storage.values]).toEqual(evidence);
  expect(successor.dispatch).not.toHaveBeenCalled();
  const encoded = JSON.stringify(evidence);
  for (const action of actions) {
    expect(encoded).not.toContain(action.intent);
    expect(encoded).not.toContain(action.input);
  }
  const prepare = vi.fn(async () => binding);
  await expect(
    successor.methods.plugins.command(
      actions[0]!.intent,
      { ...input, input: '{"ID":1}' },
      prepare,
      (value) => value,
    ),
  ).rejects.toBeInstanceOf(MutationJournalOwnershipError);
  expect(prepare).not.toHaveBeenCalled();
  expect(successor.dispatch).not.toHaveBeenCalled();
  expect(pending.filter((command) => command.matchesIntent(actions[0]!.intent))).toHaveLength(1);
});

it("does not assign an intent to direct calls or to a settled original handle", async () => {
  const storage = new Storage();
  const current = client(storage);
  current.dispatch.mockRejectedValue(new RpcTransportError("reply lost"));
  await expect(current.methods.plugins.call({ ...binding, ...input })).rejects.toBeInstanceOf(
    RpcTransportError,
  );
  const direct = current.methods.plugins.pendingCommands()[0]!;
  expect(direct.matchesIntent(intent)).toBe(false);
  await expect(
    current.methods.plugins.command(
      intent,
      input,
      async () => binding,
      (value) => value,
    ),
  ).rejects.toBeInstanceOf(RpcTransportError);
  const command = current.methods.plugins
    .pendingCommands()
    .find((value) => value.matchesIntent(intent))!;
  expect(command).toBeDefined();
  current.dispatch
    .mockResolvedValueOnce({ ...binding, ...input } as unknown as CallPluginResponse)
    .mockResolvedValueOnce({ output: "{}" });
  await expect(command.recover((value) => value)).resolves.toEqual({});
  expect(command.matchesIntent(intent)).toBe(false);
  expect(direct.matchesIntent(intent)).toBe(false);
  expect(() => direct.matchesIntent("")).toThrow("plugin command intent is invalid");
  current.journal.dispose();
  expect(() => direct.matchesIntent(intent)).toThrow(MutationJournalOwnershipError);
});

it("retires only the original command and clears its journal after confirmed retirement", async () => {
  const storage = new Storage();
  const original = client(storage);
  original.dispatch.mockRejectedValue(new RpcTransportError("reply lost"));
  await expect(original.methods.plugins.call({ ...binding, ...input })).rejects.toBeInstanceOf(
    RpcTransportError,
  );
  const key = original.dispatch.mock.calls[0]![2].idempotencyKey;
  original.journal.dispose();
  const successor = client(storage);
  successor.dispatch.mockResolvedValue({});
  await successor.methods.plugins.pendingCommands()[0]!.retire();
  expect(successor.dispatch.mock.calls).toEqual([
    ["plugins.retireCommand", { namespace: "original-store", key }, { signal: undefined }],
  ]);
  expect(storage.keys()).toEqual([]);
});

it.each(["internal_error", "plugin_not_found", "idempotency_store_mismatch"] as const)(
  "retains the original journal after retirement fails with %s",
  async (type) => {
    const storage = new Storage();
    const current = client(storage);
    current.dispatch.mockRejectedValue(new RpcTransportError("reply lost"));
    await expect(current.methods.plugins.call({ ...binding, ...input })).rejects.toBeInstanceOf(
      RpcTransportError,
    );
    current.dispatch.mockClear();
    current.dispatch.mockRejectedValue(new RpcError({ message: type, data: { type } }));
    await expect(current.methods.plugins.pendingCommands()[0]!.retire()).rejects.toBeInstanceOf(
      MutationLookupError,
    );
    expect(storage.keys()).toHaveLength(1);
    expect(current.dispatch.mock.calls.map(([method]) => method)).toEqual([
      "plugins.retireCommand",
    ]);
  },
);

it("observes a retired command after a lost retirement reply without executing its draft", async () => {
  const storage = new Storage();
  const current = client(storage);
  current.dispatch.mockRejectedValue(new RpcTransportError("reply lost"));
  await expect(current.methods.plugins.call({ ...binding, ...input })).rejects.toBeInstanceOf(
    RpcTransportError,
  );
  await expect(current.methods.plugins.pendingCommands()[0]!.retire()).rejects.toBeInstanceOf(
    MutationLookupError,
  );
  current.dispatch.mockClear();
  current.dispatch.mockRejectedValue(
    new RpcError({ message: "retired", data: { type: "plugin_command_retired" } }),
  );
  await expect(
    current.methods.plugins.pendingCommands()[0]!.recover((value) => value),
  ).rejects.toMatchObject({ data: { type: "plugin_command_retired" } });
  expect(current.dispatch.mock.calls.map(([method]) => method)).toEqual(["plugins.getCommand"]);
  expect(storage.keys()).toEqual([]);
});

it.each(["direct", "stable intent"])(
  "keeps an unresolved %s plugin command independent of management mutations",
  async (kind) => {
    const storage = new Storage();
    const original = client(storage);
    const lost = new RpcTransportError("reply lost");
    original.dispatch.mockRejectedValue(lost);
    const pending =
      kind === "direct"
        ? original.methods.plugins.call({ ...binding, ...input })
        : original.methods.plugins.command(
            intent,
            input,
            async () => binding,
            (value) => value,
          );
    await expect(pending).rejects.toBe(lost);
    const key = original.dispatch.mock.calls[0]![2].idempotencyKey;
    original.journal.dispose();
    const successor = client(storage);
    const held = new RpcError({ message: "plugin_in_use", data: { type: "plugin_in_use" } });
    successor.dispatch.mockRejectedValue(held);
    const uninstall = { installationId };
    await expect(successor.methods.plugins.uninstall(installationId)).rejects.toBe(held);
    expect(successor.dispatch.mock.calls.map(([method, params]) => ({ method, params }))).toEqual([
      { method: "plugins.uninstall", params: uninstall },
    ]);
    expect(
      successor.methods.plugins.pendingCommands().map((command) => command.idempotencyKey),
    ).toEqual([key]);
    expect(storage.keys()).toHaveLength(1);
  },
);

it("recovers an unresolved unary call through its Runtime-owned request after client replacement", async () => {
  const storage = new Storage();
  const original = client(storage);
  original.dispatch.mockRejectedValue(new RpcTransportError("reply lost"));
  await expect(original.methods.plugins.call({ ...binding, ...input })).rejects.toBeInstanceOf(
    RpcTransportError,
  );
  const [, request, options] = original.dispatch.mock.calls[0]!;
  const key = options.idempotencyKey;
  original.journal.dispose();
  const successor = client(storage);
  const [command] = successor.methods.plugins.pendingCommands();
  expect(command!.idempotencyKey).toBe(key);
  expect(command!.binding).toEqual({ ...binding, contract: input.contract, method: input.method });
  expect(JSON.stringify([...storage.values.values()])).not.toContain(id);
  const admittedRequest = {
    ...request,
    contract: stringifyJSON(parseJSON(request.contract), true),
    input: stringifyJSON(parseJSON(request.input), true),
  };
  successor.dispatch.mockResolvedValueOnce(admittedRequest as unknown as CallPluginResponse);
  successor.dispatch.mockResolvedValueOnce({ output: stringifyJSON({ known: true }) });
  await expect(command!.recover((value) => value)).resolves.toEqual({ known: true });
  expect(successor.dispatch.mock.calls.map(([method]) => method)).toEqual([
    "plugins.getCommand",
    "plugins.call",
  ]);
  expect(successor.dispatch.mock.calls[1]).toEqual(["plugins.call", admittedRequest, options]);
  expect(storage.keys()).toEqual([]);
});

it.each(["missing", "different input", "unavailable", "invalid acknowledgement"])(
  "keeps the original recovery identity when lookup is %s",
  async (failure) => {
    const storage = new Storage();
    const original = client(storage);
    original.dispatch.mockRejectedValue(new RpcTransportError("reply lost"));
    await expect(original.methods.plugins.call({ ...binding, ...input })).rejects.toBeInstanceOf(
      RpcTransportError,
    );
    const [, request, options] = original.dispatch.mock.calls[0]!;
    original.journal.dispose();
    const successor = client(storage);
    const [command] = successor.methods.plugins.pendingCommands();
    if (failure === "missing")
      successor.dispatch.mockRejectedValueOnce(
        new RpcError({ message: "missing", data: { type: "plugin_not_found" } }),
      );
    else
      successor.dispatch.mockResolvedValueOnce({
        ...request,
        ...(failure === "different input" ? { input: stringifyJSON({ ID: "other" }) } : {}),
      } as unknown as CallPluginResponse);
    if (failure === "unavailable")
      successor.dispatch.mockRejectedValueOnce(
        new RpcError({ message: "withdrawn", data: { type: "plugin_unavailable" } }),
      );
    else if (failure === "invalid acknowledgement")
      successor.dispatch.mockResolvedValueOnce({ output: "{" });
    await expect(command!.recover((value) => value)).rejects.toBeInstanceOf(
      failure === "different input" ? MutationJournalOwnershipError : MutationLookupError,
    );
    expect(storage.keys()).toHaveLength(1);
    expect(successor.methods.plugins.pendingCommands()[0]!.idempotencyKey).toBe(
      options.idempotencyKey,
    );
    expect(
      successor.dispatch.mock.calls.filter(([method]) => method === "plugins.call"),
    ).toHaveLength(failure === "missing" || failure === "different input" ? 0 : 1);
  },
);

it("keeps the shared Go numeric corpus through lost-acknowledgement recovery and fences changed lexemes", async () => {
  const source = readFileSync(
    new URL("../testing/generated/plugin-values.json", import.meta.url),
    "utf8",
  ).trim();
  const storage = new Storage();
  const original = client(storage);
  const captured = { ...input, input: source };
  original.dispatch.mockRejectedValue(new RpcTransportError("response lost"));
  await expect(
    original.methods.plugins.command(
      intent,
      captured,
      async () => binding,
      (value) => value,
    ),
  ).rejects.toBeInstanceOf(RpcTransportError);
  const [, request, options] = original.dispatch.mock.calls[0]!;
  expect(request.input).toBe(source);
  original.journal.dispose();
  const successor = client(storage);
  const prepare = vi.fn(async () => binding);
  await expect(
    successor.methods.plugins.command(
      intent,
      { ...captured, input: source.replace('"scaled":1.0', '"scaled":1') },
      prepare,
      (value) => value,
    ),
  ).rejects.toBeInstanceOf(MutationJournalOwnershipError);
  successor.dispatch.mockResolvedValue({ output: source });
  const recovered = await successor.methods.plugins.command(
    intent,
    captured,
    prepare,
    (value) => value,
  );
  expect(stringifyJSON(recovered)).toBe(source);
  expect(prepare).not.toHaveBeenCalled();
  expect(successor.dispatch.mock.calls).toEqual([["plugins.call", request, options]]);
  expect(storage.keys()).toHaveLength(0);
});
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
  successor.dispatch.mockResolvedValue({ output: stringifyJSON({}) });
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
      { ...input, input: stringifyJSON({ ID: "changed" }) },
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
  current.dispatch.mockResolvedValue({ output: stringifyJSON({ invalid: true }) });
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
      { ...input, input: "undefined" },
      async () => binding,
      () => undefined,
    ),
  ).rejects.toBeInstanceOf(SyntaxError);
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
      { ...input, method: "records.list", input: stringifyJSON({}) },
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
    method: "records.update",
    input: stringifyJSON({ ID: id, Content: "do not persist this authored content" }),
  };
  current.dispatch.mockResolvedValue({ output: stringifyJSON({}) });
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
      {
        ...authored,
        input: stringifyJSON({
          Content: (parseJSON(authored.input) as { Content: string }).Content,
          ID: id,
        }),
      },
      prepare,
      () => undefined,
    ),
  ).resolves.toBeUndefined();
  expect(prepare).not.toHaveBeenCalled();
  expect(current.dispatch).toHaveBeenCalledTimes(2);
  expect(current.dispatch.mock.calls[0]![2]).toEqual(current.dispatch.mock.calls[1]![2]);
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
