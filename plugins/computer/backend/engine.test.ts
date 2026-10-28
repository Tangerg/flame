import { lstat, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { randomUUID } from "node:crypto";
import { execFile, spawn } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { afterEach, expect, it, vi } from "vitest";
import { rolldown } from "rolldown";
import { parseJSON, stringifyJSON } from "@qimu/sdk/json";
import type { ElicitResult } from "@modelcontextprotocol/sdk/types.js";
import {
  Engine,
  EngineStartupFailure,
  type EngineCallScope,
  type EngineHooks,
} from "./engine";

const fixture = fileURLToPath(
  new URL("./fixtures/engine.mjs", import.meta.url),
);
const cleanups: Array<() => Promise<unknown>> = [];

afterEach(async () => {
  vi.useRealTimers();
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup();
  vi.unstubAllEnvs();
});

interface RecordEntry {
  kind: string;
  name?: string;
  pid?: number;
  args?: Record<string, unknown>;
  meta?: Record<string, unknown>;
}

async function open(
  options: {
    mode?: string;
    onElicitation?: EngineHooks["onElicitation"];
    env?: Record<string, string>;
    ownershipPath?: string;
  } = {},
) {
  const directory = await mkdtemp(join(tmpdir(), "flame-computer-engine-"));
  cleanups.push(() => rm(directory, { recursive: true, force: true }));
  const file = join(directory, "calls.jsonl");
  const ownershipPath =
    options.ownershipPath ?? join(directory, "engine-ownership.json");
  const lease = new AbortController();
  const retired = vi.fn();
  const engine = await Engine.connect(
    {
      command: process.execPath,
      args: [fixture],
      ownershipPath,
      env: {
        COMPUTER_FIXTURE_LOG: file,
        ...(options.mode ? { COMPUTER_FIXTURE_MODE: options.mode } : {}),
        ...options.env,
      },
    },
    {
      signal: lease.signal,
      onElicitation:
        options.onElicitation ?? (async () => ({ action: "cancel" })),
      onRetired: retired,
    },
  );
  cleanups.push(() => engine.close());
  return {
    engine,
    lease,
    retired,
    ownershipPath,
    async records(): Promise<RecordEntry[]> {
      return (await readFile(file, "utf8"))
        .trim()
        .split("\n")
        .map((line) => JSON.parse(line) as RecordEntry);
    },
  };
}

function scope(
  signal = new AbortController().signal,
  turnId = "control-turn-1",
): EngineCallScope {
  return {
    sessionId: "control-session-1",
    turnId,
    callId: "original-call-1",
    signal,
  };
}

it("captures original instructions and public descriptors once, excluding host tools", async () => {
  const { engine } = await open();
  const surface = engine.surface();
  expect(surface.instructions).toBe(
    "Use the original fixture instructions. Preserve all media.",
  );
  expect(surface.tools.map((tool) => tool.name)).toEqual(["js", "js_reset"]);
  expect(surface.tools[0]?.description).toBe(
    "Original JavaScript operation description.",
  );
  expect(surface.tools[0]?.inputSchema.required).toEqual(["code"]);
  expect(() => {
    surface.tools[0]!.inputSchema.required = [];
  }).toThrow();
  expect(engine.surface()).toBe(surface);
});

it.each(["missing-instructions", "missing-reset"])(
  "refuses incomplete %s handshakes and closes the child",
  async (mode) => {
    await expect(open({ mode })).rejects.toThrow(
      "engine connection could not be established",
    );
  },
);

it("refuses required-task controls during handshake", async () => {
  await expect(open({ mode: "required-task" })).rejects.toMatchObject({
    cleanup: "closed",
  });
});

it("preserves original raw result fields and exact numeric evidence", async () => {
  const { engine } = await open();
  const result = await engine.call("js", { code: "exact-result" }, scope());
  expect(result.cleanup).toBe("ended");
  expect(result.result).toEqual(
    parseJSON(
      '{"content":[{"type":"text","text":"exact","_meta":{"wide":9007199254740993},"extension":{"wide":9007199254740993}}],"structuredContent":{"wide":9007199254740993,"decimal":1.0000000000000001,"exponent":1e1000},"_meta":{"wide":9007199254740993,"negativeZero":-0}}',
    ),
  );
  expect(stringifyJSON(result.result)).toContain('"wide":9007199254740993');
  expect(stringifyJSON(result.result)).toContain(
    '"decimal":1.0000000000000001',
  );
});

it("validates original declared structured output without rewriting valid results", async () => {
  const { engine } = await open({ mode: "output-schema" });
  expect(
    await engine.call("js", { code: "structured-output" }, scope()),
  ).toEqual({
    result: { content: [], structuredContent: { value: 12 } },
    cleanup: "ended",
  });
  await expect(
    engine.call("js", { code: "invalid-structured-output" }, scope()),
  ).rejects.toThrow("outcome is unconfirmed");
});

it("forwards original elicitation constraints and absent mode without a parsed projection", async () => {
  const received = vi.fn<EngineHooks["onElicitation"]>(async () => ({
    action: "decline",
  }));
  const { engine } = await open({ onElicitation: received });
  const result = await engine.call(
    "js",
    { code: "restricted-approval" },
    scope(),
  );
  expect(received).toHaveBeenCalledOnce();
  expect(received.mock.calls[0]?.[0]).toEqual({
    message:
      "This original request includes a constraint outside the supported form vocabulary.",
    requestedSchema: {
      type: "object",
      properties: { reason: { type: "string", pattern: "^required$" } },
      required: ["reason"],
    },
    _meta: { fixture: "original restricted request" },
  });
  expect(result.result).toEqual({
    content: [{ type: "text", text: '{"action":"decline"}' }],
  });
});

it("sends only plugin-owned identities and ends the actual upstream turn", async () => {
  const { engine, records } = await open();
  expect(await engine.call("js", { code: "hello" }, scope())).toEqual({
    result: { content: [{ type: "text", text: "hello" }] },
    cleanup: "ended",
  });
  const calls = (await records()).filter((entry) => entry.kind === "call");
  expect(calls).toEqual([
    {
      kind: "call",
      name: "js",
      args: { code: "hello" },
      meta: {
        "x-codex-turn-metadata": {
          session_id: "control-session-1",
          turn_id: "control-turn-1",
          call_id: "original-call-1",
        },
      },
    },
    {
      kind: "call",
      name: "turn_ended",
      args: {
        hook_event_name: "Stop",
        session_id: "control-session-1",
        turn_id: "control-turn-1",
      },
      meta: {
        "x-codex-turn-metadata": {
          session_id: "control-session-1",
          turn_id: "control-turn-1",
          call_id: "original-call-1",
        },
      },
    },
  ]);
});

it("preserves text, images, audio, resources, structured output and metadata", async () => {
  const { engine } = await open();
  const { result, cleanup } = await engine.call(
    "js",
    { code: "media" },
    scope(),
  );
  expect(cleanup).toBe("ended");
  expect(result).toEqual({
    content: [
      { type: "text", text: "original content" },
      { type: "image", data: "aW1hZ2U=", mimeType: "image/png" },
      { type: "audio", data: "YXVkaW8=", mimeType: "audio/wav" },
      {
        type: "resource",
        resource: {
          uri: "file:///fixture.txt",
          mimeType: "text/plain",
          text: "resource body",
        },
      },
      {
        type: "resource_link",
        uri: "https://example.invalid/fixture",
        name: "fixture",
      },
    ],
    structuredContent: { original: true },
    _meta: { fixture: "retained" },
  });
});

it("retains a known original tool error and finishes its turn normally", async () => {
  const { engine, retired } = await open();
  const outcome = await engine.call("js", { code: "known-error" }, scope());
  expect(outcome).toEqual({
    result: {
      content: [{ type: "text", text: "known original refusal" }],
      isError: true,
    },
    cleanup: "ended",
  });
  expect(retired).not.toHaveBeenCalled();
});

it("keeps the JavaScript heap between ended turns and resets only on explicit reset", async () => {
  const { engine, records } = await open();
  const first = await engine.call("js", { code: "increment" }, scope());
  const second = await engine.call(
    "js",
    { code: "increment" },
    scope(undefined, "control-turn-2"),
  );
  await engine.call("js_reset", {}, scope(undefined, "reset-turn"));
  const reset = await engine.call(
    "js",
    { code: "increment" },
    scope(undefined, "control-turn-3"),
  );
  expect([first, second, reset].map((entry) => entry.result.content)).toEqual([
    [{ type: "text", text: "1" }],
    [{ type: "text", text: "2" }],
    [{ type: "text", text: "1" }],
  ]);
  expect(
    (await records())
      .filter((entry) => entry.kind === "call")
      .map((entry) => entry.name),
  ).toEqual([
    "js",
    "turn_ended",
    "js",
    "turn_ended",
    "js_reset",
    "turn_ended",
    "js",
    "turn_ended",
  ]);
});

it("preserves an observed result when later turn cleanup fails", async () => {
  const { engine, retired, records } = await open();
  expect(await engine.call("js", { code: "cleanup-fails" }, scope())).toEqual({
    result: { content: [{ type: "text", text: "effect confirmed" }] },
    cleanup: "unconfirmed",
  });
  expect(retired).toHaveBeenCalledTimes(1);
  expect(() => engine.call("js", { code: "repeat" }, scope())).toThrow(
    "cleanup is unconfirmed",
  );
  expect((await records()).filter((entry) => entry.name === "js")).toHaveLength(
    1,
  );
});

it("does not erase an observed result when its caller cancels during cleanup", async () => {
  const { engine, retired, records } = await open();
  const cancel = new AbortController();
  const result = engine.call(
    "js",
    { code: "cleanup-delay" },
    scope(cancel.signal),
  );
  await expect
    .poll(async () =>
      (await records()).some((entry) => entry.name === "turn_ended"),
    )
    .toBe(true);
  cancel.abort(new Error("user stopped"));
  expect(await result).toEqual({
    result: { content: [{ type: "text", text: "effect confirmed" }] },
    cleanup: "ended",
  });
  expect(retired).toHaveBeenCalledTimes(1);
});

it("forwards the exact elicitation and the real response without a permission cache", async () => {
  const received: unknown[] = [];
  const { engine } = await open({
    onElicitation: async (params) => {
      received.push(params);
      return {
        action: "accept",
        content: { reason: "user reviewed" },
        _meta: { persist: "session" },
      };
    },
  });
  const result = await engine.call("js", { code: "approval" }, scope());
  expect(received).toEqual([
    {
      mode: "form",
      message: "Allow the fixture to use its test window?",
      requestedSchema: {
        type: "object",
        properties: { reason: { type: "string" } },
        required: ["reason"],
      },
      _meta: { fixture: "original request", persist: ["session"] },
    },
  ]);
  const response = result.result.content[0];
  expect(response?.type).toBe("text");
  expect(response?.type === "text" && JSON.parse(response.text)).toEqual({
    action: "accept",
    content: { reason: "user reviewed" },
    _meta: { persist: "session" },
  });
});

it("cancels a pending approval, rejects late work and ends the original turn", async () => {
  const requested = Promise.withResolvers<AbortSignal>();
  const answer = Promise.withResolvers<ElicitResult>();
  const { engine, records } = await open({
    onElicitation: async (_params, { signal }) => {
      requested.resolve(signal);
      return await answer.promise;
    },
  });
  const cancelled = new AbortController();
  const call = engine.call("js", { code: "approval" }, scope(cancelled.signal));
  const rejected = expect(call).rejects.toThrow("outcome is unconfirmed");
  const requestSignal = await requested.promise;
  cancelled.abort(new Error("stop"));
  await rejected;
  expect(requestSignal.aborted).toBe(true);
  answer.resolve({ action: "accept", content: { reason: "too late" } });
  expect(
    (await records()).filter((entry) => entry.name === "turn_ended")[0]?.args
      ?.hook_event_name,
  ).toBe("Interrupt");
  expect(() => engine.call("js", { code: "later" }, scope())).toThrow();
});

it("pauses execution time while a person decides and resumes the same operation", async () => {
  const requested = Promise.withResolvers<void>();
  const answer = Promise.withResolvers<ElicitResult>();
  const { engine, retired } = await open({
    onElicitation: async () => {
      requested.resolve();
      return await answer.promise;
    },
  });
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "performance"] });
  const call = engine.call("js", { code: "approval" }, scope());
  await requested.promise;
  await vi.advanceTimersByTimeAsync(240_000);
  expect(retired).not.toHaveBeenCalled();
  answer.resolve({ action: "decline" });
  expect((await call).cleanup).toBe("ended");
  vi.useRealTimers();
});

it("refuses overlapping calls without disturbing the admitted operation", async () => {
  const requested = Promise.withResolvers<void>();
  const answer = Promise.withResolvers<ElicitResult>();
  const { engine, records } = await open({
    onElicitation: async () => {
      requested.resolve();
      return await answer.promise;
    },
  });
  const first = engine.call("js", { code: "approval" }, scope());
  await requested.promise;
  expect(() =>
    engine.call("js_reset", {}, scope(undefined, "foreign-turn")),
  ).toThrow("active control operation");
  answer.resolve({ action: "decline" });
  expect((await first).cleanup).toBe("ended");
  expect(
    (await records()).filter((entry) => entry.name === "js_reset"),
  ).toEqual([]);
});

it.each([
  "crash-after-effect",
  "invalid-result",
  "oversized-result",
  "large-valid-result",
  "drift",
])("retires after %s without replaying the action", async (code) => {
  const { engine, retired, records } = await open();
  await expect(engine.call("js", { code }, scope())).rejects.toThrow(
    /unconfirmed/,
  );
  expect(retired).toHaveBeenCalledTimes(1);
  expect(() => engine.call("js", { code }, scope())).toThrow();
  expect((await records()).filter((entry) => entry.name === "js")).toHaveLength(
    1,
  );
});

it("uses only the selected process environment without ambient desktop state or credentials", async () => {
  vi.stubEnv("DISPLAY", ":fixture");
  vi.stubEnv("COMPUTER_AMBIENT_SECRET", "private");
  const { engine } = await open({
    env: { DISPLAY: ":selected", COMPUTER_CONFIGURED: "explicit" },
  });
  expect(
    (await engine.call("js", { code: "environment" }, scope())).result.content,
  ).toEqual([
    {
      type: "text",
      text: JSON.stringify({ display: ":selected", configured: "explicit" }),
    },
  ]);
});

it("preserves the selected sandbox metadata through execution and cleanup", async () => {
  const sandbox = {
    permissionProfile: { type: "managed", network: { mode: "restricted" } },
    sandboxCwd: "file:///work",
  };
  const { engine, records } = await open({
    env: {
      NODE_REPL_REQUEST_META: JSON.stringify({
        "codex/sandbox-state-meta": sandbox,
        "x-codex-turn-metadata": {
          session_id: "connection",
          turn_id: "bootstrap",
        },
      }),
    },
  });
  await engine.call("js", { code: "hello" }, scope());
  const calls = (await records()).filter((entry) => entry.kind === "call");
  expect(calls).toHaveLength(2);
  for (const call of calls)
    expect(call.meta).toEqual({
      "codex/sandbox-state-meta": sandbox,
      "x-codex-turn-metadata": {
        session_id: "control-session-1",
        turn_id: "control-turn-1",
        call_id: "original-call-1",
      },
    });
});

it("rejects a shell command string and does not expose child stderr payloads", async () => {
  await expect(
    Engine.connect(
      {
        command: "node --eval bad",
        args: [],
        env: {},
        ownershipPath: "/unused",
      },
      {
        signal: new AbortController().signal,
        onElicitation: async () => ({ action: "cancel" }),
        onRetired() {},
      },
    ),
  ).rejects.toMatchObject({
    name: "EngineStartupFailure",
    cleanup: "closed",
    cause: expect.objectContaining({
      message: expect.stringContaining("absolute executable"),
    }),
  });
  await expect(open({ mode: "stderr-missing-instructions" })).rejects.toThrow(
    "engine connection could not be established",
  );
});

it("close cancels and joins pending work and repeated close shares settlement", async () => {
  const requested = Promise.withResolvers<void>();
  const { engine, retired } = await open({
    onElicitation: async (_params, { signal }) => {
      requested.resolve();
      return await new Promise<ElicitResult>((resolve) =>
        signal.addEventListener("abort", () => resolve({ action: "cancel" }), {
          once: true,
        }),
      );
    },
  });
  const result = engine
    .call("js", { code: "approval" }, scope())
    .catch((error: unknown) => error);
  await requested.promise;
  const first = engine.close();
  expect(engine.close()).toBe(first);
  await first;
  expect(await result).toBeInstanceOf(Error);
  expect(retired).toHaveBeenCalledTimes(1);
});

it("an expired control lease cancels its active operation", async () => {
  const requested = Promise.withResolvers<void>();
  const { engine, lease, retired } = await open({
    onElicitation: async () => {
      requested.resolve();
      return await new Promise(() => {});
    },
  });
  const result = engine
    .call("js", { code: "approval" }, scope())
    .catch((error: unknown) => error);
  await requested.promise;
  lease.abort(new Error("lease expired"));
  expect(await result).toBeInstanceOf(Error);
  await engine.close();
  expect(retired).toHaveBeenCalledTimes(1);
});

it("stops its owned POSIX worker group before releasing durable ownership", async () => {
  if (process.platform === "win32") return;
  const { engine } = await open();
  const result = await engine.call("js", { code: "worker" }, scope());
  const pid = Number((result.result.content[0] as { text: string }).text);
  expect(pid).toBeGreaterThan(0);
  await engine.close();
  await expect
    .poll(async () => {
      try {
        if (process.platform === "linux") {
          const state = await readFile(`/proc/${pid}/stat`, "utf8");
          return state.slice(state.lastIndexOf(")") + 2).startsWith("Z");
        }
        process.kill(pid, 0);
        return false;
      } catch (error) {
        return (
          error instanceof Error &&
          "code" in error &&
          ["ENOENT", "ESRCH"].includes(String(error.code))
        );
      }
    })
    .toBe(true);
}, 15_000);

it("refuses a second engine until its original process owner closes", async () => {
  const first = await open();
  await expect(
    open({ ownershipPath: first.ownershipPath }),
  ).rejects.toMatchObject({
    name: "EngineStartupFailure",
    cleanup: "unconfirmed",
  });
  expect(
    (await first.engine.call("js", { code: "still original" }, scope()))
      .cleanup,
  ).toBe("ended");
  await first.engine.close();
  const next = await open({ ownershipPath: first.ownershipPath });
  expect(
    (await next.engine.call("js", { code: "new lease" }, scope())).cleanup,
  ).toBe("ended");
});

it("waits for descendant group disappearance after the original child has exited", async () => {
  const { engine, ownershipPath } = await open();
  const { pid } = JSON.parse(await readFile(ownershipPath, "utf8")) as {
    pid: number;
  };
  const target = process.platform === "win32" ? pid : -pid;
  const kill = process.kill.bind(process);
  let pendingExitObservations = 3;
  const observed = vi
    .spyOn(process, "kill")
    .mockImplementation((id, signal) => {
      try {
        return kill(id, signal);
      } catch (cause) {
        if (
          id === target &&
          signal === 0 &&
          (cause as NodeJS.ErrnoException).code === "ESRCH" &&
          pendingExitObservations > 0
        ) {
          pendingExitObservations--;
          return true;
        }
        throw cause;
      }
    });
  try {
    await engine.close();
    await expect(readFile(ownershipPath)).rejects.toMatchObject({
      code: "ENOENT",
    });
    expect(pendingExitObservations).toBe(0);
  } finally {
    observed.mockRestore();
  }
});

it.skipIf(process.platform === "win32")(
  "joins an orphaned worker that finishes after its original parent exits",
  async () => {
    const { engine, records, ownershipPath } = await open();
    const result = await engine.call("js", { code: "orphan-worker" }, scope());
    const pid = Number(
      (result.result.content[0] as { type: "text"; text: string }).text,
    );
    cleanups.push(async () => {
      try {
        process.kill(pid, "SIGKILL");
      } catch (cause) {
        if ((cause as NodeJS.ErrnoException).code !== "ESRCH") throw cause;
      }
    });
    await engine.close();
    expect(
      (await records()).some(
        (entry) => entry.kind === "orphan-ended" && entry.pid === pid,
      ),
    ).toBe(true);
    expect(() => process.kill(pid, 0)).toThrow();
    await expect(readFile(ownershipPath)).rejects.toMatchObject({
      code: "ENOENT",
    });
  },
);

it.each(["uncommitted", "malformed", "oversized", "locked"])(
  "retains %s ownership for explicit reconciliation",
  async (state) => {
    const directory = await mkdtemp(
      join(tmpdir(), "flame-computer-ownership-"),
    );
    cleanups.push(() => rm(directory, { recursive: true, force: true }));
    const ownershipPath = join(directory, "engine-ownership.json");
    if (state === "locked")
      await writeFile(ownershipPath + ".lock", "unresolved admission", "utf8");
    else
      await writeFile(
        ownershipPath,
        state === "oversized"
          ? " ".repeat(4097)
          : state === "malformed"
            ? "{"
            : JSON.stringify({
                owner: randomUUID(),
                platform: process.platform,
                pid: null,
                nativeTurn: null,
              }),
        "utf8",
      );
    try {
      await open({ ownershipPath });
      throw new Error("unresolved ownership was accepted");
    } catch (error) {
      expect(error).toBeInstanceOf(EngineStartupFailure);
      expect(error).toMatchObject({ cleanup: "unconfirmed" });
    }
    expect(
      await readFile(
        state === "locked" ? ownershipPath + ".lock" : ownershipPath,
        "utf8",
      ),
    ).not.toBe("");
  },
);

it.skipIf(process.platform === "win32")(
  "refuses a nonregular ownership marker without blocking on its writer",
  async () => {
    const directory = await mkdtemp(
      join(tmpdir(), "flame-computer-ownership-pipe-"),
    );
    cleanups.push(() => rm(directory, { recursive: true, force: true }));
    const ownershipPath = join(directory, "engine-ownership.json");
    await promisify(execFile)("mkfifo", [ownershipPath]);
    await expect(open({ ownershipPath })).rejects.toMatchObject({
      cleanup: "unconfirmed",
    });
    expect((await lstat(ownershipPath)).isFIFO()).toBe(true);
  },
);

it("reclaims a committed marker only after its actual prior group no longer exists", async () => {
  const directory = await mkdtemp(
    join(tmpdir(), "flame-computer-ended-owner-"),
  );
  cleanups.push(() => rm(directory, { recursive: true, force: true }));
  const child = spawn(process.execPath, ["-e", ""], {
    detached: process.platform !== "win32",
    stdio: "ignore",
  });
  const pid = child.pid;
  await new Promise<void>((resolve, reject) => {
    child.once("close", () => resolve());
    child.once("error", reject);
  });
  expect(pid).toBeGreaterThan(0);
  const ownershipPath = join(directory, "engine-ownership.json");
  await writeFile(
    ownershipPath,
    JSON.stringify({
      owner: randomUUID(),
      platform: process.platform,
      pid,
      nativeTurn: null,
    }),
    "utf8",
  );
  const { engine } = await open({ ownershipPath });
  expect((await engine.call("js", { code: "fresh" }, scope())).cleanup).toBe(
    "ended",
  );
});

it("a crashed backend cannot launch beside its surviving engine", async () => {
  if (process.platform === "win32") return;
  const directory = await mkdtemp(
    join(tmpdir(), "flame-computer-crash-owner-"),
  );
  cleanups.push(() => rm(directory, { recursive: true, force: true }));
  const ownershipPath = join(directory, "engine-ownership.json");
  const log = join(directory, "engine.jsonl");
  const ready = join(directory, "ready");
  const bundle = await rolldown({
    input: fileURLToPath(new URL("./fixtures/owner.ts", import.meta.url)),
    platform: "node",
    resolve: { mainFields: ["module", "main"] },
  });
  const entry = join(directory, "owner.mjs");
  try {
    await bundle.write({ file: entry, format: "esm" });
  } finally {
    await bundle.close();
  }
  const owner = spawn(
    process.execPath,
    [entry, fixture, ownershipPath, log, ready],
    { stdio: ["ignore", "ignore", "pipe"] },
  );
  let diagnostics = "";
  owner.stderr.setEncoding("utf8");
  owner.stderr.on("data", (text: string) => {
    diagnostics = (diagnostics + text).slice(-8192);
  });
  const ownerClosed = new Promise<void>((resolve) =>
    owner.once("close", () => resolve()),
  );
  let enginePid: number | undefined;
  cleanups.push(async () => {
    if (owner.exitCode === null && owner.signalCode === null)
      owner.kill("SIGKILL");
    await ownerClosed;
    if (enginePid) {
      try {
        process.kill(-enginePid, "SIGKILL");
      } catch (error) {
        if (!(
          error instanceof Error &&
          "code" in error &&
          error.code === "ESRCH"
        ))
          throw error;
      }
    }
  });
  await expect
    .poll(async () => {
      if (owner.exitCode !== null) throw new Error(diagnostics);
      return await readFile(ready, "utf8");
    })
    .toBe("ready");
  await expect
    .poll(async () => (await readFile(log, "utf8")).includes('"name":"js"'))
    .toBe(true);
  const record = JSON.parse(await readFile(ownershipPath, "utf8")) as {
    pid: number;
  };
  enginePid = record.pid;
  owner.kill("SIGKILL");
  await ownerClosed;
  expect(() => process.kill(-record.pid, 0)).not.toThrow();
  await expect(open({ ownershipPath })).rejects.toMatchObject({
    name: "EngineStartupFailure",
    cleanup: "unconfirmed",
  });
  expect(
    (await readFile(log, "utf8"))
      .trim()
      .split("\n")
      .filter((line) => JSON.parse(line).kind === "start"),
  ).toHaveLength(1);
}, 30_000);
