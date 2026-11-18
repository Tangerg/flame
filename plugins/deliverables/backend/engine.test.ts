import { execFile, spawn } from "node:child_process";
import { EventEmitter } from "node:events";
import { PassThrough } from "node:stream";
import {
  lstat,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  realpath,
  rm,
  symlink,
  truncate,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Engine } from "./engine";
import { type PrivateDirectory, type ProcessPlan } from "./confinement";
import { runProcess } from "./process";
import { LIMITS } from "../contracts/limits";
import release from "./office-release.json";

const officeFixture = vi.hoisted(() => ({
  executable: "#!/bin/sh\nexit 0\n",
  manifest: "controlled native Office fixture\n",
}));

vi.mock("./office-release.json", async (original) => {
  const actual = await original<{ default: typeof release }>();
  const { createHash } = await import("node:crypto");
  const hash = (value: string) =>
    createHash("sha256").update(value).digest("hex");
  return {
    default: {
      ...actual.default,
      targets: Object.fromEntries(
        Object.entries(actual.default.targets).map(([arch, target]) => [
          arch,
          {
            ...target,
            manifestSHA256: hash(officeFixture.manifest),
            executableSHA256: hash(officeFixture.executable),
          },
        ]),
      ),
    },
  };
});

vi.mock("node:child_process", async (original) => {
  const actual = await original<typeof import("node:child_process")>();
  return {
    ...actual,
    spawn: vi.fn(actual.spawn),
    execFile: vi.fn(actual.execFile),
  };
});

vi.mock("node:fs/promises", async (original) => {
  const actual = await original<typeof import("node:fs/promises")>();
  return {
    ...actual,
    rm: vi.fn(actual.rm),
    lstat: vi.fn(actual.lstat),
    realpath: vi.fn(actual.realpath),
  };
});

const PDF = Buffer.from("%PDF-1.7\ncontrolled process fixture\n%%EOF");
const PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aXioAAAAASUVORK5CYII=",
  "base64",
);
let temporary: string;
let data: string;
let tools: string;
let environment: NodeJS.ProcessEnv;
let engine: Engine;
let pageCount: string;
let text: string;
let mode:
  | "normal"
  | "missing"
  | "linked"
  | "directory"
  | "oversized"
  | "invalid-response"
  | "hang"
  | "process-failure";
let nextPID: number;
let kills: ReturnType<typeof vi.spyOn>;
let source: Uint8Array | undefined;
let profile: string | undefined;
let changedOutput: Uint8Array;
const groups = new Map<number, () => void>();
const confinementCommand =
  process.platform === "darwin" ? "/usr/bin/sandbox-exec" : "/usr/bin/bwrap";

function absent(): never {
  throw Object.assign(new Error("process absent"), { code: "ESRCH" });
}

function simulate(command: string, args: readonly string[]) {
  const child = new EventEmitter() as EventEmitter & {
    pid: number;
    stdout: PassThrough;
    stderr: PassThrough;
    kill: (signal: NodeJS.Signals) => boolean;
  };
  child.pid = nextPID++;
  child.kill = (signal) => process.kill(-child.pid, signal);
  child.stdout = new PassThrough();
  child.stderr = new PassThrough();
  let ended = false;
  const finish = (code: number | null, signal: string | null = null) => {
    if (ended) return;
    ended = true;
    groups.delete(child.pid);
    child.emit("exit", code, signal);
    child.stdout.end();
    child.stderr.end();
    child.emit("close", code, signal);
  };
  groups.set(child.pid, () => finish(null, "SIGKILL"));
  void Promise.resolve()
    .then(async () => {
      child.emit("spawn");
      if (ended || mode === "hang") return;
      expect(command).toBe(confinementCommand);
      const split =
        command === "/usr/bin/sandbox-exec"
          ? args.indexOf("flame-deliverables")
          : args.lastIndexOf("--");
      const executable = args[split + 1]!;
      const argv = args.slice(split + 2);
      if (mode === "process-failure") {
        finish(1);
        return;
      }
      if (argv.includes("--version") || argv.includes("-v")) {
        child.stdout.write(`${basename(executable)} 1.0\n`);
      } else if (["office", "dsoffice"].includes(basename(executable))) {
        const native = basename(executable) === "dsoffice";
        const path = native
          ? argv[argv.indexOf("--input-path") + 1]!
          : argv.at(-1)!;
        source = await readFile(path);
        const profilePath = native
          ? argv[argv.indexOf("--profile-directory") + 1]!
          : fileURLToPath(
              argv
                .find((arg) => arg.startsWith("-env:UserInstallation="))!
                .slice("-env:UserInstallation=".length),
            );
        const profileFile = join(
          profilePath,
          "user",
          "registrymodifications.xcu",
        );
        expect((await lstat(profilePath)).mode & 0o777).toBe(0o700);
        expect((await lstat(profileFile)).mode & 0o777).toBe(0o600);
        profile = await readFile(profileFile, "utf8");
        const format =
          argv[argv.indexOf(native ? "--format" : "--convert-to") + 1]!;
        const output = native
          ? argv[argv.indexOf("--output-path") + 1]!
          : join(
              argv[argv.indexOf("--outdir") + 1]!,
              format === "pdf" ? "source.pdf" : "source.xlsx",
            );
        if (native)
          child.stdout.write(
            mode === "invalid-response"
              ? '{"ok":false}'
              : '{"ok":true,"missingFonts":[]}',
          );
        if (mode === "linked") await symlink(path, output);
        else if (mode === "directory") await mkdir(output);
        else if (mode !== "missing") {
          await writeFile(output, format === "pdf" ? PDF : changedOutput);
          if (mode === "oversized")
            await truncate(output, LIMITS.fileBytes + 1);
        }
      } else if (basename(executable) === "pdfinfo") {
        child.stdout.write(`Pages: ${pageCount}\nPage size: 612 x 792 pts\n`);
      } else if (basename(executable) === "pdftoppm") {
        await writeFile(`${argv.at(-1)}.png`, PNG);
      } else if (basename(executable) === "pdftotext") {
        await writeFile(argv.at(-1)!, text);
      } else throw new Error("unexpected fixture executable");
      finish(0);
    })
    .catch((error: unknown) => {
      child.emit("error", error);
      finish(1);
    });
  return child as unknown as ReturnType<typeof spawn>;
}

beforeEach(async () => {
  temporary = await mkdtemp(
    join(await realpath(tmpdir()), "flame-deliverables-engine-"),
  );
  data = join(temporary, "data");
  tools = join(temporary, "tools");
  await mkdir(data, { mode: 0o700 });
  await mkdir(tools, { mode: 0o700 });
  environment = {
    FLAME_DELIVERABLES_TOOL_ROOTS: JSON.stringify([tools]),
    PRIVATE_HOST_TOKEN: "never-forward",
  };
  for (const tool of ["office", "pdfinfo", "pdftoppm", "pdftotext"]) {
    const path = join(
      tools,
      tool === "office" && process.platform === "darwin"
        ? release.executable
        : tool,
    );
    await mkdir(join(path, ".."), { recursive: true });
    await writeFile(path, officeFixture.executable, { mode: 0o700 });
    environment[`FLAME_DELIVERABLES_${tool.toUpperCase()}`] = path;
  }
  await mkdir(join(tools, release.programDirectory), { recursive: true });
  await writeFile(join(tools, "prebuilds.json"), officeFixture.manifest);
  engine = new Engine(environment, data);
  pageCount = "2";
  text = "A readable report\n";
  mode = "normal";
  nextPID = 900000;
  source = undefined;
  profile = undefined;
  changedOutput = Buffer.from("PK recalculated fixture");
  groups.clear();
  vi.mocked(spawn).mockClear();
  vi.mocked(spawn).mockImplementation(simulate as typeof spawn);
  vi.mocked(execFile).mockImplementation((...args: unknown[]) => {
    const callback = args.at(-1) as (
      error: Error | null,
      stdout: string,
    ) => void;
    queueMicrotask(() => callback(null, "0\n"));
    return {} as ReturnType<typeof execFile>;
  });
  kills = vi.spyOn(process, "kill").mockImplementation((pid, signal) => {
    const stop = groups.get(-pid);
    if (!stop) return absent();
    if (signal !== 0) stop();
    return true;
  });
});

afterEach(async () => {
  for (const stop of groups.values()) stop();
  await engine.close().catch(() => {});
  await rm(temporary, { recursive: true, force: true });
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("professional rendering owner", () => {
  it("fails closed when the installed confinement process rejects execution", async () => {
    mode = "process-failure";
    await expect(
      engine.render("pdf", PDF, new AbortController().signal),
    ).rejects.toMatchObject({ code: "process_failed" });
    expect(vi.mocked(spawn)).toHaveBeenCalledOnce();
    expect(vi.mocked(spawn).mock.calls[0]![0]).toBe(confinementCommand);
    expect(await readdir(join(data, "engine"))).toEqual([]);
  });
});

describe("confined process boundary", () => {
  function directory(): PrivateDirectory {
    return {
      root: data,
      input: join(data, "input"),
      output: join(data, "output"),
      home: join(data, "home"),
      temporary: join(data, "tmp"),
    };
  }

  it("terminates descendants even after the parent returns zero and waits for close", async () => {
    let child!: EventEmitter & {
      pid: number;
      stdout: PassThrough;
      stderr: PassThrough;
    };
    vi.mocked(spawn).mockImplementation(() => {
      child = Object.assign(new EventEmitter(), {
        pid: 999999,
        stdout: new PassThrough(),
        stderr: new PassThrough(),
      });
      groups.set(child.pid, () => {
        groups.delete(child.pid);
        child.emit("close", 0, null);
      });
      return child as unknown as ReturnType<typeof spawn>;
    });
    const plan: ProcessPlan = {
      command: "/usr/bin/bwrap",
      args: [],
      cwd: data,
      env: {},
      containment: { kind: "namespace" },
    };
    const running = runProcess(plan, new AbortController().signal);
    child.emit("exit", 0, null);
    await expect(running).resolves.toEqual({ stdout: "", stderr: "" });
    expect(kills).toHaveBeenCalledWith(-child.pid, "SIGKILL");
    expect(groups.size).toBe(0);
  });
});
