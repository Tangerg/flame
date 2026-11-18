import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, expect, it, vi } from "vitest";
import { rolldown } from "rolldown";
import { Engine, type EngineCallScope, type EngineHooks } from "./engine";

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
  const cleanupRelease = join(directory, "release-cleanup");
  const releaseCleanup = () => writeFile(cleanupRelease, "");
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
        COMPUTER_FIXTURE_CLEANUP_RELEASE: cleanupRelease,
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
  cleanups.push(releaseCleanup);
  return {
    engine,
    lease,
    retired,
    ownershipPath,
    releaseCleanup,
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

it("a crashed backend cannot launch beside its surviving engine", async () => {
  if (process.platform === "win32") return;
  const directory = await mkdtemp(
    join(tmpdir(), "flame-computer-crash-owner-"),
  );
  cleanups.push(() => rm(directory, { recursive: true, force: true }));
  const ownershipPath = join(directory, "engine-ownership.json");
  const log = join(directory, "engine.jsonl");
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
  const owner = spawn(process.execPath, [entry, fixture, ownershipPath, log], {
    stdio: ["ignore", "ignore", "pipe", "ipc"],
  });
  let diagnostics = "";
  owner.stderr!.setEncoding("utf8");
  owner.stderr!.on("data", (text: string) => {
    diagnostics = (diagnostics + text).slice(-8192);
  });
  const ownerClosed = new Promise<void>((resolve) =>
    owner.once("close", () => resolve()),
  );
  cleanups.push(async () => {
    if (owner.exitCode === null && owner.signalCode === null)
      owner.kill("SIGKILL");
    await ownerClosed;
    let ownership: { pid: number | null };
    try {
      ownership = JSON.parse(await readFile(ownershipPath, "utf8"));
    } catch (error) {
      if (error instanceof Error && "code" in error && error.code === "ENOENT")
        return;
      throw error;
    }
    if (ownership.pid !== null) {
      try {
        process.kill(-ownership.pid, "SIGKILL");
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
  const [request] = await Promise.race([
    once(owner, "message"),
    ownerClosed.then(() => {
      throw new Error(
        "fixture owner exited before request admission: " + diagnostics,
      );
    }),
  ]);
  const record = JSON.parse(await readFile(ownershipPath, "utf8")) as {
    pid: number;
  };
  expect(request).toMatchObject({
    message: "Allow the fixture to use its test window?",
  });
  expect((await readFile(log, "utf8")).includes('"name":"js"')).toBe(true);
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
