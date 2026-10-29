import { execFile, spawn } from "node:child_process";
import { EventEmitter } from "node:events";
import { PassThrough } from "node:stream";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { confinedPlan, type ProcessPlan } from "./confinement";
import { runProcess } from "./process";

vi.mock("node:child_process", () => ({ spawn: vi.fn(), execFile: vi.fn() }));

const plan: ProcessPlan = {
  command: "/usr/bin/bwrap",
  args: [],
  cwd: "/tmp/confined-fixture",
  env: {},
  containment: { kind: "namespace" },
};
let child: EventEmitter & {
  pid: number;
  stdin: PassThrough;
  stdout: PassThrough;
  stderr: PassThrough;
  kill: ReturnType<typeof vi.fn>;
};
let alive: boolean;
let accepted: Buffer[];

function finish(code: number | null) {
  if (!alive) return;
  alive = false;
  child.emit("exit", code, code === null ? "SIGKILL" : null);
  child.stdin.destroy();
  child.stdout.end();
  child.stderr.end();
  child.emit("close", code, code === null ? "SIGKILL" : null);
}

beforeEach(() => {
  alive = true;
  accepted = [];
  child = Object.assign(new EventEmitter(), {
    pid: 999999,
    stdin: new PassThrough(),
    stdout: new PassThrough(),
    stderr: new PassThrough(),
    kill: vi.fn(() => {
      finish(null);
      return true;
    }),
  });
  child.stdin.on("data", (bytes: Buffer) => accepted.push(Buffer.from(bytes)));
  vi.mocked(spawn).mockClear();
  vi.mocked(execFile).mockReset();
  vi.mocked(spawn).mockReturnValue(
    child as unknown as ReturnType<typeof spawn>,
  );
  vi.spyOn(process, "kill").mockImplementation((pid, signal) => {
    expect(pid).toBe(-child.pid);
    if (!alive)
      throw Object.assign(new Error("process absent"), { code: "ESRCH" });
    if (signal !== 0) finish(null);
    return true;
  });
});

afterEach(() => {
  finish(null);
  vi.restoreAllMocks();
});

describe("confined process streams", () => {
  it("refuses oversized input before starting a child", async () => {
    await expect(
      runProcess(plan, new AbortController().signal, {
        input: Buffer.from("1234"),
        inputBytes: 3,
      }),
    ).rejects.toMatchObject({ code: "process_input_limit" });
    expect(spawn).not.toHaveBeenCalled();
  });

  it("captures original input and accepts independent output and diagnostic limits", async () => {
    const input = Buffer.from("original packet");
    const running = runProcess(plan, new AbortController().signal, {
      input,
      inputBytes: input.length,
      stdoutBytes: 1024,
      stderrBytes: 4,
    });
    input.fill(0);
    await vi.waitFor(() => expect(child.stdin.writableFinished).toBe(true));
    child.stdout.write("x".repeat(1024));
    child.stderr.write("note");
    finish(0);
    await expect(running).resolves.toEqual({
      stdout: "x".repeat(1024),
      stderr: "note",
    });
    expect(Buffer.concat(accepted).toString()).toBe("original packet");
    expect(vi.mocked(spawn).mock.calls[0]![2]).toMatchObject({
      stdio: ["pipe", "pipe", "pipe"],
    });
  });

  it.each(["stdout", "stderr"] as const)(
    "terminates and joins a child exceeding its own %s limit",
    async (stream) => {
      const running = runProcess(plan, new AbortController().signal, {
        stdoutBytes: 3,
        stderrBytes: 3,
      });
      child[stream].write("1234");
      await expect(running).rejects.toMatchObject({
        code: "process_output_limit",
      });
      expect(alive).toBe(false);
    },
  );

  it("refuses input transport failure only after the child is stopped", async () => {
    const running = runProcess(plan, new AbortController().signal, {
      input: Buffer.from("packet"),
    });
    child.stdin.emit("error", new Error("broken input"));
    await expect(running).rejects.toMatchObject({
      code: "process_input_failed",
    });
    expect(alive).toBe(false);
  });
});

describe("single-process resident memory", () => {
  const single: ProcessPlan = {
    ...plan,
    containment: { kind: "single", residentLimitBytes: 1024 },
  };

  it.each(["over-limit", "unavailable", "invalid"] as const)(
    "stops and joins the child when memory is %s",
    async (observation) => {
      vi.mocked(execFile).mockImplementation((...args: unknown[]) => {
        const callback = args.at(-1) as (
          error: Error | null,
          stdout: string,
        ) => void;
        queueMicrotask(() =>
          callback(
            observation === "unavailable" ? new Error("probe failed") : null,
            observation === "invalid" ? "" : "2\n",
          ),
        );
        return {} as ReturnType<typeof execFile>;
      });
      const running = runProcess(single, new AbortController().signal);
      child.emit("spawn");
      await expect(running).rejects.toMatchObject({
        code:
          observation === "over-limit"
            ? "process_memory_limit"
            : "process_resources_unavailable",
      });
      expect(alive).toBe(false);
      expect(child.kill).toHaveBeenCalledWith("SIGKILL");
      expect(process.kill).not.toHaveBeenCalled();
      expect(execFile).toHaveBeenCalledWith(
        "/bin/ps",
        ["-o", "rss=", "-p", String(child.pid)],
        expect.objectContaining({ timeout: 1000, killSignal: "SIGKILL" }),
        expect.any(Function),
      );
    },
  );

  it("joins an in-flight observation after the child exits", async () => {
    let observe!: (error: Error | null, stdout: string) => void;
    vi.mocked(execFile).mockImplementation((...args: unknown[]) => {
      observe = args.at(-1) as typeof observe;
      return {} as ReturnType<typeof execFile>;
    });
    const running = runProcess(single, new AbortController().signal);
    child.emit("spawn");
    let settled = false;
    void running.then(() => {
      settled = true;
    });
    finish(0);
    await new Promise<void>((resolve) => setImmediate(resolve));
    expect(settled).toBe(false);
    observe(new Error("process has exited"), "");
    await expect(running).resolves.toEqual({ stdout: "", stderr: "" });
    expect(child.kill).not.toHaveBeenCalled();
  });

  it.each([0, 1])(
    "awaits the original exit status %i after the kernel reports an absent process",
    async (status) => {
      vi.mocked(execFile).mockImplementation((...args: unknown[]) => {
        const callback = args.at(-1) as (
          error: Error | null,
          stdout: string,
          stderr: string,
        ) => void;
        queueMicrotask(() =>
          callback(
            Object.assign(new Error("process absent"), { code: 1 }),
            "",
            "",
          ),
        );
        return {} as ReturnType<typeof execFile>;
      });
      const running = runProcess(single, new AbortController().signal);
      child.emit("spawn");
      await new Promise<void>((resolve) => setImmediate(resolve));
      expect(child.kill).not.toHaveBeenCalled();
      finish(status);
      if (status === 0)
        await expect(running).resolves.toEqual({ stdout: "", stderr: "" });
      else
        await expect(running).rejects.toMatchObject({ code: "process_failed" });
    },
  );
});

it("confines a parser without write or network authority using its own resource limits", () => {
  const directory = {
    root: "/tmp/confined-fixture",
    input: "/tmp/confined-fixture/input",
    output: "/tmp/confined-fixture/output",
    home: "/tmp/confined-fixture/home",
    temporary: "/tmp/confined-fixture/tmp",
  };
  const bounds = {
    memoryBytes: 2 * 1024 * 1024 * 1024,
    cpuSeconds: 45,
    maximumFileBytes: 0,
    openFiles: 64,
    writable: false,
  };
  const linux = confinedPlan(
    "linux",
    directory,
    ["/opt/installed-parser"],
    "/opt/installed-parser/node",
    ["--permission", "main.mjs"],
    bounds,
  );
  expect(linux.args).toContain("--unshare-all");
  expect(linux.args).not.toContain("--share-net");
  expect(linux.args).not.toContain("--bind");
  expect(linux.args).toContain("--as=2147483648");
  expect(linux.args).toContain("--nofile=64");
  expect(linux.args).toContain("--fsize=0");
  expect(linux.args).toContain("--cpu=45");
  expect(linux.args.join(" ")).toContain(
    "--remount-ro / --remount-ro /tmp --remount-ro /dev",
  );
  expect(linux.env).not.toHaveProperty("SAL_USE_VCLPLUGIN");
  expect(linux.env).not.toHaveProperty("MAX_CONCURRENCY");
  const mac = confinedPlan(
    "darwin",
    directory,
    ["/opt/installed-parser"],
    "/opt/installed-parser/node",
    ["--permission", "main.mjs"],
    bounds,
  );
  expect(mac.args[1]).toContain("(deny network*)");
  expect(mac.args[1]).toContain("(deny process-fork)");
  expect(mac.args[1]).toContain('(allow file-write*  (literal "/dev/null"))');
  expect(mac.args[5]).toContain("ulimit -n 64;");
  expect(mac.args[5]).toContain("ulimit -f 0;");
  expect(mac.args[2]).toBe("/bin/bash");
  expect(mac.args[5]).not.toContain("ulimit -v");
  expect(mac.containment).toEqual({
    kind: "single",
    residentLimitBytes: bounds.memoryBytes,
  });
});
