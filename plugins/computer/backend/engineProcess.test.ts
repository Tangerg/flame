import { mkdtemp, readFile, rm } from "node:fs/promises";
import { spawn } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, it, vi } from "vitest";
import { EngineProcess } from "./engineProcess";

vi.mock("node:child_process", async (original) => {
  const actual = await original<typeof import("node:child_process")>();
  return { ...actual, spawn: vi.fn(actual.spawn) };
});

it("stops delivering buffered frames when their consumer closes the connection", async () => {
  const directory = await mkdtemp(join(tmpdir(), "flame-process-retirement-"));
  const transport = new EngineProcess({
    command: process.execPath,
    args: [
      "-e",
      "process.stdin.resume(); process.stdin.on('end', () => process.exit(0));",
    ],
    ownershipPath: join(directory, "ownership.json"),
    env: {},
  });
  const spawned = vi.mocked(spawn);
  spawned.mockClear();
  const received = vi.fn();
  transport.onmessage = (message) => {
    received(message);
    if (received.mock.calls.length === 1) void transport.close();
  };
  try {
    await transport.start();
    const child = spawned.mock.results[0]?.value;
    if (!child?.stdout) throw new Error("fixture has no engine output");
    child.stdout.emit(
      "data",
      Buffer.from(
        '{"jsonrpc":"2.0","method":"first"}\n{"jsonrpc":"2.0","method":"retired"}\n',
      ),
    );
    expect(received).toHaveBeenCalledExactlyOnceWith({
      jsonrpc: "2.0",
      method: "first",
    });
  } finally {
    await transport.close();
    await rm(directory, { recursive: true });
  }
});

it.each(["transient denial", "persistent denial", "unexpected failure"])(
  "retains original process-group evidence through $0",
  async (mode) => {
    const directory = await mkdtemp(join(tmpdir(), "flame-process-evidence-"));
    const ownershipPath = join(directory, "ownership.json");
    const transport = new EngineProcess({
      command: process.execPath,
      args: [
        "-e",
        "process.stdin.resume(); process.stdin.on('end', () => process.exit(0));",
      ],
      ownershipPath,
      env: {},
    });
    await transport.start();
    const record = JSON.parse(await readFile(ownershipPath, "utf8")) as {
      pid: number;
    };
    const target = process.platform === "win32" ? record.pid : -record.pid;
    const original = process.kill.bind(process);
    const denial = Object.assign(
      new Error("injected liveness observation failure"),
      {
        code: mode === "unexpected failure" ? "EIO" : "EPERM",
      },
    );
    let probes = 0;
    const observation = vi
      .spyOn(process, "kill")
      .mockImplementation((pid, signal) => {
        if (pid === target && signal === 0) {
          probes++;
          if (mode !== "transient denial" || probes <= 3) throw denial;
        }
        return original(pid, signal);
      });
    try {
      const closing = transport.close();
      expect(transport.close()).toBe(closing);
      if (mode === "transient denial") {
        await closing;
        expect(probes).toBeGreaterThan(3);
        await expect(readFile(ownershipPath)).rejects.toMatchObject({
          code: "ENOENT",
        });
      } else {
        await expect(closing).rejects.toMatchObject({
          message: "computer: engine cleanup is unconfirmed",
          errors: [expect.objectContaining({ cause: denial })],
        });
        expect(JSON.parse(await readFile(ownershipPath, "utf8"))).toEqual(
          record,
        );
        if (mode === "persistent denial") expect(probes).toBeGreaterThan(1);
        else expect(probes).toBe(1);
      }
    } finally {
      observation.mockRestore();
      await expect
        .poll(() => {
          try {
            original(target, 0);
            return true;
          } catch (cause) {
            if ((cause as NodeJS.ErrnoException).code === "ESRCH") return false;
            throw cause;
          }
        })
        .toBe(false);
      await rm(directory, { recursive: true });
    }
  },
  15_000,
);
