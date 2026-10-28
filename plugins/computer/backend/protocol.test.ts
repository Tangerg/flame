import { randomUUID } from "node:crypto";
import { mkdtemp, readFile, rm, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, expect, it } from "vitest";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { InMemoryTransport } from "@modelcontextprotocol/sdk/inMemory.js";
import { CallToolResultSchema } from "@modelcontextprotocol/sdk/types.js";
import {
  Capability,
  Method,
  PROTOCOL,
  canonicalJSON,
  decodeReply,
  digestJSON,
  type CapabilityDescriptor,
  type MethodDescriptor,
  type Invocation,
} from "@qimu/node";
import { z } from "zod";
import { parseJSON, stringifyJSON } from "@qimu/sdk/json";
import descriptor, {
  METHODS,
  type ControlSession,
  type ExecuteInput,
  type ExecutionResult,
  type HistoryPage,
  type OperationRecord,
  type Snapshot,
  type Surface,
} from "../contracts/capability";
import { createBackend } from "./backend";
import { Engine } from "./engine";

const rpc = {
  describe: "io.github.tangerg.qimu/operations.describe",
  setup: "io.github.tangerg.qimu/operations.setup",
  activate: "io.github.tangerg.qimu/operations.activate",
  validate: "io.github.tangerg.qimu/operations.validate",
  invoke: "io.github.tangerg.qimu/operations.invoke",
  receipt: "io.github.tangerg.qimu/commands.get",
  toolReceipt: "io.github.tangerg.flame/tools.getResult",
  invocation: "io.github.tangerg.qimu/invocation",
};
const fixture = fileURLToPath(
  new URL("./fixtures/engine.mjs", import.meta.url),
);
const response = z.record(z.string(), z.unknown());
const cleanup: Array<() => Promise<unknown>> = [];

afterEach(async () => {
  for (const close of cleanup.splice(0).reverse()) await close();
});

async function directory() {
  const path = await mkdtemp(join(tmpdir(), "flame-computer-protocol-"));
  cleanup.push(() => rm(path, { recursive: true, force: true }));
  return path;
}

async function connect(data: string) {
  const log = join(data, "engine.jsonl");
  const backend = createBackend(
    {
      PLUGIN_DATA: data,
      PLUGIN_ROOT: data,
      FLAME_COMPUTER_APPLICATION: data,
    },
    (_configuration, hooks) =>
      Engine.connect(
        {
          command: process.execPath,
          args: [fixture],
          env: { COMPUTER_FIXTURE_LOG: log },
          ownershipPath: join(data, "engine-ownership.json"),
        },
        hooks,
      ),
  );
  const client = new Client({ name: "computer-protocol-test", version: "1" });
  const [serverTransport, clientTransport] =
    InMemoryTransport.createLinkedPair();
  const serving = backend.serve(serverTransport);
  await client.connect(clientTransport);
  let closing: Promise<void> | undefined;
  const close = () =>
    (closing ??= (async () => {
      await client.close();
      await backend.close();
      await serving;
    })());
  cleanup.push(close);
  const described = decodeReply(
    await client.request(
      { method: rpc.describe, params: { protocol: PROTOCOL } },
      response,
    ),
  ) as {
    methods: MethodDescriptor[];
    capabilities: Array<{ name: string; contract: CapabilityDescriptor }>;
  };
  const methods = new Map(
    described.methods.map((value) => [value.name, new Method(value)]),
  );
  const capability = new Capability(
    described.capabilities.find((value) => value.name === "control")!.contract,
  );
  expect(capability.identity).toBe(new Capability(descriptor).identity);
  await client.request(
    {
      method: rpc.setup,
      params: {
        protocol: PROTOCOL,
        contributions: [{ id: "control", export: "control", requires: [] }],
      },
    },
    response,
  );
  await client.request(
    { method: rpc.activate, params: { protocol: PROTOCOL } },
    response,
  );

  function meta(origin: Invocation["origin"], id: string) {
    return {
      [rpc.invocation]: {
        id,
        origin,
        target: {
          scope: "installation",
          reference: "computer.protocol.fixture",
        },
      },
    };
  }
  function parameters(
    name: string,
    input: unknown,
    options: {
      id?: string;
      origin?: Invocation["origin"];
      signal?: AbortSignal;
    } = {},
  ) {
    const method = methods.get(name)!;
    if (!method) throw new Error("fixture requested an unpublished method");
    const id = options.id ?? randomUUID();
    const member =
      descriptor.methods.some((entry) => entry.name === name) ||
      name === rpc.receipt;
    return {
      protocol: PROTOCOL,
      method: name,
      contract: method.identity,
      input,
      ...(member ? { export: "control", capability: capability.identity } : {}),
      ...(method.descriptor.kind === "command" ? { commandId: id } : {}),
      _meta: meta(options.origin ?? "human", `caller.${id}`),
    };
  }
  async function invoke<T>(
    name: string,
    input: unknown,
    options: {
      id?: string;
      origin?: Invocation["origin"];
      signal?: AbortSignal;
    } = {},
  ): Promise<T> {
    let reply;
    try {
      reply = await client.request(
        { method: rpc.invoke, params: parameters(name, input, options) },
        response,
        { signal: options.signal },
      );
    } catch (cause) {
      throw new Error(`fixture protocol ${name} failed`, { cause });
    }
    return decodeReply(reply) as T;
  }
  async function tool(
    name: "context" | "js" | "reset",
    input: unknown,
    id = randomUUID(),
    signal?: AbortSignal,
  ) {
    return CallToolResultSchema.parse(
      await client.callTool(
        {
          name,
          arguments: input as Record<string, unknown>,
          _meta: meta("model", id),
        },
        CallToolResultSchema,
        { signal },
      ),
    );
  }
  async function originalTool(
    id: string,
    name: "js" | "reset",
    input: unknown,
  ) {
    const history = await invoke<HistoryPage>(METHODS.history, { limit: 50 });
    const records = await Promise.all(
      history.entries
        .filter((entry) => entry.kind === "tool" && entry.method === name)
        .map((entry) =>
          invoke<OperationRecord>(METHODS.result, { id: entry.id }),
        ),
    );
    const record = records.find(
      (entry) => (JSON.parse(entry.invocationJSON) as Invocation).id === id,
    );
    if (!record) throw new Error("fixture original tool receipt is missing");
    return {
      record,
      receipt: await invoke<{ found: boolean; output?: string }>(
        rpc.toolReceipt,
        { id: record.id, method: name, inputDigest: digestJSON(input) },
      ),
    };
  }
  return {
    client,
    invoke,
    tool,
    originalTool,
    close,
    async validate(name: string, input: unknown) {
      const params = parameters(name, input);
      delete (params as { commandId?: string }).commandId;
      return decodeReply(
        await client.request({ method: rpc.validate, params }, response),
      );
    },
    async records(): Promise<
      Array<{ kind: string; name?: string; args?: Record<string, unknown> }>
    > {
      try {
        return (await readFile(log, "utf8"))
          .trim()
          .split("\n")
          .map((line) => JSON.parse(line));
      } catch (error) {
        if (
          error instanceof Error &&
          "code" in error &&
          error.code === "ENOENT"
        )
          return [];
        throw error;
      }
    },
  };
}

function input(surface: Surface, code: string): ExecuteInput {
  return {
    sessionId: surface.sessionId,
    workerId: surface.workerId,
    surfaceDigest: surface.surfaceDigest,
    tool: "js",
    argumentsJSON: canonicalJSON({ code }),
  };
}
function toolInput(value: ExecuteInput) {
  const { tool: _tool, ...args } = value;
  return args;
}

it("publishes complete contracts while validation and context reads cannot open an engine", async () => {
  const connection = await connect(await directory());
  expect(
    (await connection.client.listTools()).tools.map((tool) => tool.name),
  ).toEqual(["context", "js", "reset"]);
  expect(await connection.validate(METHODS.open, { leaseSeconds: 900 })).toBe(
    "",
  );
  expect(await connection.validate(METHODS.open, { leaseSeconds: 1 })).not.toBe(
    "",
  );
  expect((await connection.tool("context", {})).isError).toBe(true);
  await expect(
    connection.invoke(METHODS.open, { leaseSeconds: 900 }, { origin: "model" }),
  ).rejects.toMatchObject({ code: "human_required" });
  expect(await connection.records()).toEqual([]);
  await connection.invoke(METHODS.open, { leaseSeconds: 900 });
  const context = await connection.tool("context", {});
  expect(context.structuredContent).toMatchObject({
    instructions: "Use the original fixture instructions. Preserve all media.",
  });
});

it("human commands and model tools share the actual worker and retain their exact original outcomes", async () => {
  const data = await directory();
  const first = await connect(data);
  await first.invoke(METHODS.open, { leaseSeconds: 900 });
  const surface = await first.invoke<Surface>(METHODS.context, {});
  const commandId = randomUUID();
  const commandInput = input(surface, "increment");
  const command = await first.invoke<ExecutionResult>(
    METHODS.execute,
    commandInput,
    { id: commandId },
  );
  expect(command.result).toEqual({ content: [{ type: "text", text: "1" }] });
  const toolId = randomUUID();
  const args = toolInput(input(surface, "increment"));
  expect((await first.tool("js", args, toolId)).content).toEqual([
    { type: "text", text: "2" },
  ]);
  const mediaInput = toolInput(input(surface, "media"));
  const mediaId = randomUUID();
  const media = await first.tool("js", mediaInput, mediaId);
  expect(media.content.map((block) => block.type)).toEqual([
    "text",
    "image",
    "audio",
    "resource",
    "resource_link",
  ]);
  expect(media._meta).toEqual({ fixture: "retained" });
  expect(media.structuredContent).toEqual({ original: true });
  const original = await first.originalTool(mediaId, "js", mediaInput);
  expect(JSON.parse(original.record.outcomeJSON!)).toEqual(media);
  expect(original.receipt.found).toBe(true);
  expect(original.receipt.output).toContain("aW1hZ2U=");
  const receiptInput = {
    id: commandId,
    method: METHODS.execute,
    inputDigest: digestJSON(commandInput),
  };
  expect(await first.invoke(rpc.receipt, receiptInput)).toEqual({
    found: true,
    reply: { value: command },
  });
  expect(
    await first.invoke(METHODS.execute, commandInput, { id: commandId }),
  ).toEqual(command);
  expect(await first.tool("js", mediaInput, mediaId)).toEqual(media);
  const dispatches = (await first.records()).filter(
    (entry) => entry.name === "js",
  ).length;
  expect(dispatches).toBe(3);
  await first.close();

  const reopened = await connect(data);
  expect(await reopened.invoke(rpc.receipt, receiptInput)).toEqual({
    found: true,
    reply: { value: command },
  });
  expect(
    await reopened.invoke(METHODS.execute, commandInput, { id: commandId }),
  ).toEqual(command);
  expect(await reopened.tool("js", mediaInput, mediaId)).toEqual(media);
  expect(
    (await reopened.records()).filter((entry) => entry.kind === "start"),
  ).toHaveLength(1);
  expect(
    (await reopened.records()).filter((entry) => entry.name === "js"),
  ).toHaveLength(dispatches);
});

it("reset ends its real control turn and requires the replacement worker identity", async () => {
  const connection = await connect(await directory());
  await connection.invoke(METHODS.open, { leaseSeconds: 900 });
  const original = await connection.invoke<Surface>(METHODS.context, {});
  await connection.tool("js", toolInput(input(original, "increment")));
  const reset = {
    ...toolInput(input(original, "unused")),
    argumentsJSON: "{}",
  };
  expect((await connection.tool("reset", reset)).content).toEqual([
    { type: "text", text: "reset" },
  ]);
  const current = await connection.invoke<Surface>(METHODS.context, {});
  expect(current.workerId).not.toBe(original.workerId);
  expect(current.sessionId).toBe(original.sessionId);
  expect(current.surfaceDigest).toBe(original.surfaceDigest);
  expect(
    (await connection.tool("js", toolInput(input(original, "increment"))))
      .isError,
  ).toBe(true);
  expect(
    (await connection.tool("js", toolInput(input(current, "increment"))))
      .content,
  ).toEqual([{ type: "text", text: "1" }]);
  expect(
    (await connection.records())
      .filter((entry) => entry.kind === "call")
      .map((entry) => entry.name),
  ).toEqual(["js", "turn_ended", "js_reset", "turn_ended", "js", "turn_ended"]);
});

it("retains exact upstream numeric evidence through typed replies, model tools and their durable receipts", async () => {
  const connection = await connect(await directory());
  await connection.invoke(METHODS.open, { leaseSeconds: 900 });
  const surface = await connection.invoke<Surface>(METHODS.context, {});
  const commandId = randomUUID();
  const commandInput = input(surface, "exact-result");
  const command = await connection.invoke<ExecutionResult>(
    METHODS.execute,
    commandInput,
    { id: commandId },
  );
  expect(command.result.structuredContent).toEqual(
    parseJSON(
      '{"wide":9007199254740993,"decimal":1.0000000000000001,"exponent":1e1000}',
    ),
  );
  const commandReceipt = await connection.invoke<{
    found: boolean;
    reply: { value: ExecutionResult };
  }>(rpc.receipt, {
    id: commandId,
    method: METHODS.execute,
    inputDigest: digestJSON(commandInput),
  });
  expect(commandReceipt.reply.value).toEqual(command);
  const modelId = randomUUID();
  const modelInput = toolInput(commandInput);
  const model = await connection.tool("js", modelInput, modelId);
  expect(model.structuredContent).toEqual(command.result.structuredContent);
  expect(model._meta).toEqual(command.result._meta);
  const original = await connection.originalTool(modelId, "js", modelInput);
  expect(parseJSON(original.record.outcomeJSON!)).toEqual(command.result);
  expect(original.receipt.output).toContain("9007199254740993");
  expect(original.receipt.output).toContain("1.0000000000000001");
  expect(stringifyJSON(command.result)).toContain('"negativeZero":-0');
});

it("refuses an unsupported original permission constraint before any human can approve a weakened form", async () => {
  const connection = await connect(await directory());
  await connection.invoke(METHODS.open, { leaseSeconds: 900 });
  const surface = await connection.invoke<Surface>(METHODS.context, {});
  await expect(
    connection.invoke(METHODS.execute, input(surface, "restricted-approval")),
  ).rejects.toMatchObject({ cause: { code: -32603 } });
  await expect
    .poll(
      async () =>
        (await connection.invoke<Snapshot>(METHODS.read, {})).session?.state,
    )
    .toBe("closed");
  const snapshot = await connection.invoke<Snapshot>(METHODS.read, {});
  expect(snapshot.session?.approval).toBeUndefined();
  expect(snapshot.session?.state).toBe("closed");
  expect(
    (await connection.records()).filter((record) => record.name === "js"),
  ).toHaveLength(1);
});

it("a human reply answers the original live child request while model approval is refused", async () => {
  const connection = await connect(await directory());
  await connection.invoke(METHODS.open, { leaseSeconds: 900 });
  const surface = await connection.invoke<Surface>(METHODS.context, {});
  const execution = connection.tool(
    "js",
    toolInput(input(surface, "approval")),
  );
  let snapshot: Snapshot;
  await expect
    .poll(async () => {
      snapshot = await connection.invoke<Snapshot>(METHODS.read, {});
      return snapshot.session?.approval;
    })
    .toBeDefined();
  const approval = snapshot!.session!.approval!;
  expect(JSON.parse(approval.requestJSON)).toMatchObject({
    message: "Allow the fixture to use its test window?",
    _meta: { fixture: "original request" },
  });
  const reply = {
    sessionId: surface.sessionId,
    requestId: approval.id,
    responseJSON: canonicalJSON({
      action: "accept",
      content: { reason: "reviewed by operator" },
    }),
  };
  await expect(
    connection.invoke(METHODS.reply, reply, { origin: "model" }),
  ).rejects.toMatchObject({ code: "human_required" });
  expect(
    (await connection.invoke<Snapshot>(METHODS.read, {})).session?.approval?.id,
  ).toBe(approval.id);
  expect(await connection.invoke(METHODS.reply, reply)).toBe(true);
  const result = (await execution).content[0];
  expect(result?.type === "text" && JSON.parse(result.text)).toEqual({
    action: "accept",
    content: { reason: "reviewed by operator" },
  });
  await expect(connection.invoke(METHODS.reply, reply)).rejects.toMatchObject({
    code: "permission_retired",
  });
});

it("stop cancels the original invocation and pending permission, joins the process and retains uncertainty", async () => {
  const data = await directory();
  const connection = await connect(data);
  await connection.invoke(METHODS.open, { leaseSeconds: 900 });
  const surface = await connection.invoke<Surface>(METHODS.context, {});
  const args = toolInput(input(surface, "approval"));
  const id = randomUUID();
  const execution = connection
    .tool("js", args, id)
    .catch((error: unknown) => error);
  let snapshot: Snapshot;
  await expect
    .poll(async () => {
      snapshot = await connection.invoke<Snapshot>(METHODS.read, {});
      return snapshot.session?.approval;
    })
    .toBeDefined();
  const operation = snapshot!.session!.operation!;
  const approval = snapshot!.session!.approval!;
  expect(
    (
      await connection.invoke<ControlSession>(METHODS.stop, {
        sessionId: surface.sessionId,
        operationId: operation.id,
      })
    ).state,
  ).toBe("closed");
  expect(await execution).toBeInstanceOf(Error);
  await expect(stat(join(data, "engine-ownership.json"))).rejects.toMatchObject(
    { code: "ENOENT" },
  );
  const original = await connection.originalTool(id, "js", args);
  expect(original.record).toMatchObject({ dispatched: true, settled: false });
  expect(original.receipt).toEqual({ found: false });
  await expect(
    connection.invoke(METHODS.reply, {
      sessionId: surface.sessionId,
      requestId: approval.id,
      responseJSON: '{"action":"decline"}',
    }),
  ).rejects.toMatchObject({ code: "permission_retired" });
  await expect(connection.tool("js", args, id)).rejects.toThrow();
  expect(
    (await connection.records()).filter((entry) => entry.name === "js"),
  ).toHaveLength(1);
});

it("a child crash keeps the dispatched action unknown across backend restart without a replay", async () => {
  const data = await directory();
  const first = await connect(data);
  await first.invoke(METHODS.open, { leaseSeconds: 900 });
  const surface = await first.invoke<Surface>(METHODS.context, {});
  const executionInput = input(surface, "crash-after-effect");
  const id = randomUUID();
  await expect(
    first.invoke(METHODS.execute, executionInput, { id }),
  ).rejects.toThrow();
  const receipt = {
    id,
    method: METHODS.execute,
    inputDigest: digestJSON(executionInput),
  };
  expect(await first.invoke(rpc.receipt, receipt)).toEqual({ found: false });
  expect(
    await first.invoke<OperationRecord>(METHODS.result, { id }),
  ).toMatchObject({ dispatched: true, settled: false });
  await first.close();
  const next = await connect(data);
  expect(await next.invoke(rpc.receipt, receipt)).toEqual({ found: false });
  await expect(
    next.invoke(METHODS.execute, executionInput, { id }),
  ).rejects.toThrow();
  expect(
    (await next.records()).filter((entry) => entry.name === "js"),
  ).toHaveLength(1);
  expect(
    (await next.records()).filter((entry) => entry.kind === "effect"),
  ).toHaveLength(1);
  expect(
    (await next.records()).filter((entry) => entry.kind === "start"),
  ).toHaveLength(1);
});

it("outer MCP cancellation retires its exact worker and waits for original cleanup", async () => {
  const data = await directory();
  const connection = await connect(data);
  await connection.invoke(METHODS.open, { leaseSeconds: 900 });
  const surface = await connection.invoke<Surface>(METHODS.context, {});
  const abort = new AbortController();
  const execution = connection
    .tool(
      "js",
      toolInput(input(surface, "approval")),
      randomUUID(),
      abort.signal,
    )
    .catch((error: unknown) => error);
  await expect
    .poll(
      async () =>
        (await connection.invoke<Snapshot>(METHODS.read, {})).session?.approval,
    )
    .toBeDefined();
  abort.abort(new Error("original tool cancelled"));
  expect(await execution).toBeInstanceOf(Error);
  await expect
    .poll(
      async () =>
        (await connection.invoke<Snapshot>(METHODS.read, {})).session?.state,
    )
    .toBe("closed");
  await expect(stat(join(data, "engine-ownership.json"))).rejects.toMatchObject(
    { code: "ENOENT" },
  );
  expect(
    (await connection.records()).filter(
      (entry) => entry.name === "turn_ended",
    )[0]?.args?.hook_event_name,
  ).toBe("Interrupt");
});
