import { toolCommandId } from "@flame/backend-sdk";
import { Lifetime } from "@qimu/sdk";
import { mkdtempSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { randomUUID } from "node:crypto";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  canonicalJSON,
  digestJSON,
  type CallContext,
  type Invocation,
} from "@qimu/node";
import type {
  CallToolResult,
  ElicitRequestParams,
} from "@modelcontextprotocol/sdk/types.js";
import { Control, type EngineConnection } from "./control";
import { Receipts, type ReceiptLimits } from "./receipts";
import type {
  EngineCallScope,
  EngineHooks,
  EngineResult,
  EngineSurface,
} from "./engine";
import { EngineStartupFailure } from "./engine";
import { LIMITS } from "../contracts/limits";
import type { ExecuteInput } from "../contracts/capability";

const rawResult: CallToolResult = {
  content: [
    { type: "text", text: "observed" },
    { type: "image", mimeType: "image/png", data: "AQID" },
  ],
};
const surface: EngineSurface = {
  instructions:
    "Read the original instructions before operating the test computer.",
  tools: [
    {
      name: "js",
      description: "Original js description",
      inputSchema: {
        type: "object",
        properties: {
          code: { type: "string" },
          timeout_ms: { type: "integer", minimum: 1 },
        },
        required: ["code"],
        additionalProperties: false,
      },
    },
    {
      name: "js_reset",
      description: "Original reset description",
      inputSchema: {
        type: "object",
        properties: {},
        additionalProperties: false,
      },
    },
  ],
  serverInfo: { name: "test-engine", version: "1" },
  capabilities: { tools: {} },
};
const nativeRequest: ElicitRequestParams = {
  mode: "form",
  message: "Allow the test application?",
  requestedSchema: { type: "object", properties: {} },
  _meta: {
    codex_approval_kind: "mcp_tool_call",
    connector_id: "computer-use",
    tool_params: { app: "org.example.application" },
    persist: ["session"],
  },
};

function context(
  input: unknown,
  origin: Invocation["origin"] = "human",
  id: string = randomUUID(),
  signal = new AbortController().signal,
): CallContext {
  const lifetime = new Lifetime("computer-test-call");
  cleanup.push(async () => {
    await lifetime.close();
  });
  return {
    signal,
    commandId: id,
    requestInput: canonicalJSON(input),
    inputDigest: digestJSON(input),
    invocation: {
      id: `invocation.${id}`,
      origin,
      target: { scope: "installation", reference: "computer.installation" },
    },
    lifetime,
    defer: (cleanup) => lifetime.cleanup(cleanup),
  };
}
function waitForAbort(signal: AbortSignal): Promise<never> {
  return new Promise((_resolve, reject) => {
    if (signal.aborted) reject(signal.reason);
    else
      signal.addEventListener("abort", () => reject(signal.reason), {
        once: true,
      });
  });
}

class TestEngine implements EngineConnection {
  readonly hooks: EngineHooks;
  readonly close = vi.fn(async () => {});
  readonly call = vi.fn(
    (
      tool: "js" | "js_reset",
      args: Record<string, unknown>,
      scope: EngineCallScope,
    ) => this.perform(tool, args, scope),
  );
  perform: (
    tool: "js" | "js_reset",
    args: Record<string, unknown>,
    scope: EngineCallScope,
  ) => Promise<EngineResult> = async () => ({
    result: rawResult,
    cleanup: "ended",
  });
  constructor(hooks: EngineHooks) {
    this.hooks = hooks;
  }
  surface() {
    return surface;
  }
}

const cleanup: Array<() => Promise<void>> = [];
afterEach(async () => {
  vi.useRealTimers();
  for (const close of cleanup.splice(0).reverse()) await close();
});

function fixture(configured = true, limits: ReceiptLimits = LIMITS) {
  const directory = mkdtempSync(join(tmpdir(), "flame-computer-control-"));
  const lifetime = new AbortController();
  const receipts = new Receipts(join(directory, "receipts.db"), limits);
  const engines: TestEngine[] = [];
  const connect = vi.fn(async (_config, hooks: EngineHooks) => {
    const engine = new TestEngine(hooks);
    engines.push(engine);
    return engine;
  });
  const control = new Control(
    configured
      ? {
          application: directory,
          packageRoot: directory,
          data: directory,
          browser: false,
          audio: false,
          environment: {},
        }
      : undefined,
    receipts,
    lifetime.signal,
    connect,
  );
  cleanup.push(async () => {
    await control.close().catch(() => {});
    receipts.close();
    rmSync(directory, { recursive: true, force: true });
  });
  return { control, receipts, engines, connect, lifetime, directory };
}
async function opened() {
  const value = fixture();
  await value.control.open(context({ leaseSeconds: 900 }), 900);
  return { ...value, engine: value.engines[0]! };
}
function input(control: Control, tool: "js" | "js_reset" = "js"): ExecuteInput {
  const value = control.context();
  return {
    sessionId: value.sessionId,
    workerId: value.workerId,
    surfaceDigest: value.surfaceDigest,
    tool,
    argumentsJSON: tool === "js" ? '{"code":"await cua.getState()"}' : "{}",
  };
}

describe("computer control ownership", () => {
  it("keeps an unconfigured plugin inspectable and refuses execution startup", async () => {
    const { control, connect } = fixture(false);
    expect(control.snapshot()).toMatchObject({ configured: false });
    await expect(
      control.open(context({ leaseSeconds: 900 }), 900),
    ).rejects.toMatchObject({ code: "engine_unconfigured" });
    expect(connect).not.toHaveBeenCalled();
  });

  it("projects authored startup diagnostics without exposing the dependency cause", async () => {
    const { control, connect } = fixture();
    const failure = new EngineStartupFailure(
      "closed",
      new Error("private dependency environment and stderr"),
      ["initialize the selected CUA application for this account"],
    );
    connect.mockRejectedValueOnce(failure);
    await expect(
      control.open(context({ leaseSeconds: 900 }), 900),
    ).rejects.toMatchObject({
      code: "engine_unavailable",
      message: failure.message,
    });
    expect(failure.message).not.toContain("private dependency");
  });

  it("only the captured human caller can open a control lease", async () => {
    const { control, connect } = fixture();
    await expect(
      control.open(context({ leaseSeconds: 900 }, "model"), 900),
    ).rejects.toMatchObject({ code: "human_required" });
    expect(connect).not.toHaveBeenCalled();
  });

  it("publishes exact original instructions and schemas on the actual Runtime host", async () => {
    const { control } = await opened();
    expect(control.context()).toMatchObject({
      instructions: surface.instructions,
      host: { platform: process.platform, arch: process.arch },
    });
    expect(JSON.parse(control.context().toolsJSON)).toEqual(surface.tools);
  });

  it("does not replace an active connection even for another human open command", async () => {
    const { control, connect } = await opened();
    await expect(
      control.open(context({ leaseSeconds: 900 }), 900),
    ).rejects.toMatchObject({ code: "session_active" });
    expect(connect).toHaveBeenCalledTimes(1);
  });

  it.each(["sessionId", "workerId", "surfaceDigest"] as const)(
    "refuses a stale %s before engine dispatch",
    async (field) => {
      const { control, engine } = await opened();
      const value = { ...input(control), [field]: "other" };
      await expect(
        control.execute(context(value), value),
      ).rejects.toMatchObject({
        code: field === "sessionId" ? "session_replaced" : "worker_replaced",
      });
      expect(engine.call).not.toHaveBeenCalled();
    },
  );

  it("uses the original tool schema and refuses unrepresentable arguments without executing", async () => {
    const { control, engine } = await opened();
    for (const args of [
      '{"wrong":"field"}',
      '{"code":"x","timeout_ms":9007199254740993}',
    ]) {
      const value = { ...input(control), argumentsJSON: args };
      await expect(
        control.execute(context(value), value),
      ).rejects.toMatchObject({ code: "invalid_arguments" });
    }
    expect(engine.call).not.toHaveBeenCalled();
  });

  it("reuses the observed original result and never dispatches a repeated command", async () => {
    const { control, engine } = await opened();
    const value = input(control);
    const call = context(value);
    const first = await control.execute(call, value);
    expect(first.result).toEqual(rawResult);
    expect(await control.execute(call, value)).toEqual(first);
    expect(engine.call).toHaveBeenCalledTimes(1);
  });

  it("binds original identity to exact input and rejects a changed request", async () => {
    const { control, engine } = await opened();
    const value = input(control);
    const call = context(value);
    await control.execute(call, value);
    const changed = { ...value, argumentsJSON: '{"code":"different"}' };
    await expect(
      control.execute(context(changed, "human", call.commandId), changed),
    ).rejects.toMatchObject({ code: "command_conflict" });
    expect(engine.call).toHaveBeenCalledTimes(1);
  });

  it("serializes the actual desktop and refuses concurrent execution without aborting its owner", async () => {
    const { control, engine } = await opened();
    const gate = Promise.withResolvers<EngineResult>();
    engine.perform = () => gate.promise;
    const value = input(control);
    const first = control.execute(context(value), value);
    await vi.waitFor(() => expect(engine.call).toHaveBeenCalledTimes(1));
    await expect(control.execute(context(value), value)).rejects.toMatchObject({
      code: "computer_unavailable",
    });
    expect(engine.call.mock.calls[0]![2].signal.aborted).toBe(false);
    gate.resolve({ result: rawResult, cleanup: "ended" });
    await first;
  });

  it("changes the accepted worker identity after reset without relaunching the connection", async () => {
    const { control, connect } = await opened();
    const old = input(control);
    const reset = input(control, "js_reset");
    await control.execute(context(reset), reset);
    expect(control.context().workerId).not.toBe(old.workerId);
    expect(control.context().sessionId).toBe(old.sessionId);
    await expect(control.execute(context(old), old)).rejects.toMatchObject({
      code: "worker_replaced",
    });
    expect(connect).toHaveBeenCalledTimes(1);
  });

  it("answers only the exact live permission and only from the human origin", async () => {
    const { control, engine } = await opened();
    engine.perform = async (_tool, _args, scope) => {
      const reply = await engine.hooks.onElicitation(nativeRequest, {
        signal: scope.signal,
      });
      expect(reply).toEqual({
        action: "accept",
        content: {},
        _meta: { persist: "session" },
      });
      return { result: rawResult, cleanup: "ended" };
    };
    const value = input(control);
    const running = control.execute(context(value), value);
    await vi.waitFor(() =>
      expect(control.snapshot().session?.approval).toBeDefined(),
    );
    const approval = control.snapshot().session!.approval!;
    const answer = {
      sessionId: value.sessionId,
      requestId: approval.id,
      responseJSON:
        '{"action":"accept","content":{},"_meta":{"persist":"session"}}',
    };
    await expect(
      control.reply(context(answer, "model"), answer),
    ).rejects.toMatchObject({ code: "human_required" });
    await expect(
      control.reply(context({ ...answer, requestId: "stale" }), {
        ...answer,
        requestId: "stale",
      }),
    ).rejects.toMatchObject({ code: "permission_retired" });
    expect(await control.reply(context(answer), answer)).toBe(true);
    await running;
    expect(control.snapshot().session!.approval).toBeUndefined();
    await expect(control.reply(context(answer), answer)).rejects.toMatchObject({
      code: "permission_retired",
    });
  });

  it("refuses an application grant for Flame itself", async () => {
    const { control, engine } = await opened();
    engine.perform = async (_tool, _args, scope) => {
      const result = await engine.hooks.onElicitation(
        {
          ...nativeRequest,
          _meta: {
            ...nativeRequest._meta,
            tool_params: { app: "com.tangerg.flame" },
          },
        },
        { signal: scope.signal },
      );
      expect(result).toEqual({ action: "decline" });
      expect(control.snapshot().session!.approval).toBeUndefined();
      return {
        result: {
          isError: true,
          content: [{ type: "text", text: "declined" }],
        },
        cleanup: "ended",
      };
    };
    const value = input(control);
    expect((await control.execute(context(value), value)).result.isError).toBe(
      true,
    );
  });

  it("cancels a pending permission with its operation and rejects its late human reply", async () => {
    const { control, engine } = await opened();
    engine.perform = async (_tool, _args, scope) => {
      expect(
        await engine.hooks.onElicitation(nativeRequest, {
          signal: scope.signal,
        }),
      ).toEqual({ action: "cancel" });
      throw scope.signal.reason;
    };
    const abort = new AbortController();
    const value = input(control);
    const call = context(value, "human", randomUUID(), abort.signal);
    const running = control.execute(call, value);
    const failed = expect(running).rejects.toThrow("unconfirmed");
    await vi.waitFor(() =>
      expect(control.snapshot().session?.approval).toBeDefined(),
    );
    const approval = control.snapshot().session!.approval!;
    abort.abort(new Error("user cancelled"));
    await failed;
    const reply = {
      sessionId: value.sessionId,
      requestId: approval.id,
      responseJSON: '{"action":"accept","content":{}}',
    };
    await expect(control.reply(context(reply), reply)).rejects.toMatchObject({
      code: "permission_retired",
    });
    expect(control.result(call.commandId!).settled).toBe(false);
  });

  it("retains uncertainty after possible effects and never retries that original identity", async () => {
    const { control, engine } = await opened();
    let effects = 0;
    engine.perform = async () => {
      effects++;
      throw new Error("reply lost after effect");
    };
    const value = input(control);
    const call = context(value);
    await expect(control.execute(call, value)).rejects.toThrow("unconfirmed");
    await expect(control.execute(call, value)).rejects.toThrow("unconfirmed");
    expect(effects).toBe(1);
    expect(control.result(call.commandId!)).toMatchObject({
      settled: false,
      dispatched: true,
    });
  });

  it("keeps known external output even when turn cleanup is unconfirmed", async () => {
    const { control, engine } = await opened();
    engine.perform = async () => ({
      result: rawResult,
      cleanup: "unconfirmed",
    });
    const value = input(control);
    const call = context(value);
    expect(await control.execute(call, value)).toMatchObject({
      result: rawResult,
      cleanup: "unconfirmed",
    });
    expect(control.result(call.commandId!).settled).toBe(true);
    await vi.waitFor(() => expect(engine.close).toHaveBeenCalled());
    expect(() => control.context()).toThrow();
  });

  it("records an observed result despite cancellation after the effect completed", async () => {
    const { control, engine } = await opened();
    const abort = new AbortController();
    engine.perform = async () => {
      abort.abort(new Error("late cancellation"));
      return { result: rawResult, cleanup: "ended" };
    };
    const value = input(control);
    const call = context(value, "human", randomUUID(), abort.signal);
    expect((await control.execute(call, value)).result).toEqual(rawResult);
    expect(control.result(call.commandId!).settled).toBe(true);
  });

  it("lets the operator close while another command is executing and joins that original engine", async () => {
    const { control, engine } = await opened();
    engine.perform = (_tool, _args, scope) => waitForAbort(scope.signal);
    const value = input(control);
    const running = control.execute(context(value), value);
    const failed = expect(running).rejects.toThrow("unconfirmed");
    await vi.waitFor(() => expect(engine.call).toHaveBeenCalled());
    const close = { sessionId: value.sessionId };
    expect(
      (await control.closeSession(context(close), value.sessionId)).state,
    ).toBe("closed");
    await failed;
    expect(engine.close).toHaveBeenCalledTimes(1);
  });

  it("does not let a stale stop interrupt a later operation", async () => {
    const { control, engine } = await opened();
    engine.perform = (_tool, _args, scope) => waitForAbort(scope.signal);
    const value = input(control);
    const running = control.execute(context(value), value);
    const failed = expect(running).rejects.toThrow("unconfirmed");
    await vi.waitFor(() => expect(engine.call).toHaveBeenCalled());
    const stop = { sessionId: value.sessionId, operationId: "old-operation" };
    await expect(
      control.stop(context(stop), stop.sessionId, stop.operationId),
    ).rejects.toMatchObject({ code: "operation_retired" });
    expect(engine.call.mock.calls[0]![2].signal.aborted).toBe(false);
    await control.closeSession(
      context({ sessionId: value.sessionId }),
      value.sessionId,
    );
    await failed;
  });

  it("retains durable human close capacity after ordinary receipt admission is exhausted", async () => {
    const { control, engines } = fixture(true, { ...LIMITS, maxRecords: 2 });
    const session = await control.open(context({ leaseSeconds: 900 }), 900);
    const close = { sessionId: session.id };
    const call = context(close);
    await expect(
      control.open(context({ leaseSeconds: 900 }), 900),
    ).rejects.toMatchObject({ code: "receipt_capacity" });
    expect((await control.closeSession(call, session.id)).state).toBe("closed");
    expect(control.result(call.commandId!)).toMatchObject({
      dispatched: true,
      settled: true,
    });
    expect((await control.closeSession(call, session.id)).state).toBe("closed");
    expect(engines[0]!.close).toHaveBeenCalledTimes(1);
  });

  it.each(["records", "bytes"] as const)(
    "retains exact human stop capacity when %s are exhausted during a permission request",
    async (budget) => {
      const limits =
        budget === "records"
          ? { ...LIMITS, maxRecords: 3 }
          : {
              maxRecords: 100,
              maxBytes: 8192,
              maxInputBytes: 1024,
              maxResultBytes: 2048,
            };
      const { control, engines } = fixture(true, limits);
      await control.open(context({ leaseSeconds: 900 }), 900);
      const engine = engines[0]!;
      engine.perform = async (_tool, _args, scope) => {
        expect(
          await engine.hooks.onElicitation(nativeRequest, {
            signal: scope.signal,
          }),
        ).toEqual({ action: "cancel" });
        throw scope.signal.reason;
      };
      const value = input(control);
      const running = control.execute(context(value), value);
      const failed = expect(running).rejects.toThrow("unconfirmed");
      await vi.waitFor(() =>
        expect(control.snapshot().session?.approval).toBeDefined(),
      );
      let exhausted = false;
      for (let index = 0; index < 20; index++) {
        try {
          await control.open(context({ leaseSeconds: 900 }), 900);
        } catch (cause) {
          const code = (cause as { code?: string }).code;
          if (code === "receipt_capacity") {
            exhausted = true;
            break;
          }
          expect(code).toBe("session_active");
        }
      }
      expect(exhausted).toBe(true);
      const session = control.snapshot().session!;
      const stop = {
        sessionId: session.id,
        operationId: session.operation!.id,
      };
      const stale = { ...stop, operationId: "old-operation" };
      await expect(
        control.stop(context(stale), stale.sessionId, stale.operationId),
      ).rejects.toMatchObject({ code: "receipt_capacity" });
      await expect(
        control.stop(context(stop, "model"), stop.sessionId, stop.operationId),
      ).rejects.toMatchObject({ code: "receipt_capacity" });
      const answer = {
        sessionId: session.id,
        requestId: session.approval!.id,
        responseJSON: '{"action":"cancel"}',
      };
      await expect(
        control.reply(context(answer), answer),
      ).rejects.toMatchObject({ code: "receipt_capacity" });
      const call = context(stop);
      expect(
        (await control.stop(call, stop.sessionId, stop.operationId)).state,
      ).toBe("closed");
      await failed;
      expect(control.result(call.commandId!)).toMatchObject({
        dispatched: true,
        settled: true,
      });
      expect(engine.close).toHaveBeenCalledTimes(1);
      expect(engine.call).toHaveBeenCalledTimes(1);
    },
  );

  it("expires a lease and cancels the operation even while a permission is waiting", async () => {
    vi.useFakeTimers();
    const { control, engines } = fixture();
    await control.open(context({ leaseSeconds: 60 }), 60);
    const engine = engines[0]!;
    engine.perform = async (_tool, _args, scope) => {
      await engine.hooks.onElicitation(nativeRequest, { signal: scope.signal });
      throw scope.signal.reason;
    };
    const value = input(control);
    const running = control.execute(context(value), value);
    const failed = expect(running).rejects.toThrow("unconfirmed");
    await vi.advanceTimersByTimeAsync(60_001);
    await failed;
    expect(control.snapshot().session).toMatchObject({
      state: "closed",
      reason: "expired",
    });
    expect(engine.close).toHaveBeenCalledTimes(1);
  });

  it("refuses an operation whose lease expires between admission and driver dispatch", async () => {
    vi.useFakeTimers();
    const { control, engines } = fixture();
    const openedAt = Date.now();
    await control.open(context({ leaseSeconds: 60 }), 60);
    const value = input(control);
    const call = context(value);
    const running = control.execute(call, value);
    vi.setSystemTime(openedAt + 60_001);
    await expect(running).rejects.toMatchObject({ code: "lease_expired" });
    await control.close();
    expect(engines[0]!.call).not.toHaveBeenCalled();
    expect(control.result(call.commandId!)).toMatchObject({
      settled: true,
      dispatched: false,
    });
    expect(control.snapshot().session).toMatchObject({
      state: "closed",
      reason: "expired",
    });
  });

  it("preserves original model media and derives its receipt without replay", async () => {
    const { control, engine, receipts } = await opened();
    const value = input(control);
    const call = context(value, "model");
    expect(await control.tool(call, "js", value)).toEqual(rawResult);
    expect(await control.tool(call, "js", value)).toEqual(rawResult);
    const receipt = receipts.toolReceipt({
      id: toolCommandId(call),
      method: "js",
      inputDigest: call.inputDigest,
    });
    expect(receipt.found).toBe(true);
    expect(JSON.parse(receipt.output!).content[1].kind).toBe("media");
    expect(engine.call).toHaveBeenCalledTimes(1);
  });

  it("streams current snapshots and releases a cancelled subscriber", async () => {
    const { control } = fixture();
    const abort = new AbortController();
    const stream = control.watch(abort.signal);
    expect((await stream.next()).value).toMatchObject({
      kind: "chunk",
      value: { configured: true },
    });
    const next = stream.next();
    const open = control.open(context({ leaseSeconds: 900 }), 900);
    expect((await next).value).toMatchObject({
      kind: "chunk",
      value: { session: { state: "opening" } },
    });
    await open;
    abort.abort();
    await expect(stream.next()).rejects.toBeDefined();
  });
});
