import { execFile, spawn, type SpawnOptions } from "node:child_process";
import { EventEmitter } from "node:events";
import { PassThrough } from "node:stream";
import {
  chmod,
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
import { setTimeout as nativeDelay } from "node:timers/promises";
import { basename, join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Engine } from "./engine";
import {
  confinedPlan,
  SYSTEM_FILES,
  SYSTEM_ROOTS,
  type PrivateDirectory,
  type ProcessPlan,
} from "./confinement";
import { runProcess, UnconfirmedProcessExit } from "./process";
import { LIMITS } from "../contracts/limits";

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

function simulate(
  command: string,
  args: readonly string[],
  options: SpawnOptions,
) {
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
      } else if (basename(executable) === "office") {
        const path = argv.at(-1)!;
        source = await readFile(path);
        profile = await readFile(
          join(
            String(options.cwd),
            "home",
            "libreoffice",
            "user",
            "registrymodifications.xcu",
          ),
          "utf8",
        );
        const format = argv[argv.indexOf("--convert-to") + 1]!;
        const output = join(
          argv[argv.indexOf("--outdir") + 1]!,
          format === "pdf" ? "source.pdf" : "source.xlsx",
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
    const path = join(tools, tool);
    await writeFile(path, "#!/bin/sh\nexit 0\n", { mode: 0o700 });
    environment[`FLAME_DELIVERABLES_${tool.toUpperCase()}`] = path;
  }
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
  it("renders the captured bytes into actual PDF/PNG assets with a private profile and honest evidence", async () => {
    const bytes = Buffer.from("PK original document");
    const accepted = Buffer.from(bytes);
    const rendering = engine.render(
      "docx",
      bytes,
      new AbortController().signal,
    );
    bytes.fill(0);
    const result = await rendering;
    expect(source).toEqual(accepted);
    expect(result.files.map((file) => [file.filename, file.mime])).toEqual([
      ["rendered.pdf", "application/pdf"],
      ["page-0001.png", "image/png"],
      ["page-0002.png", "image/png"],
    ]);
    expect(result.files[0]!.bytes).toEqual(PDF);
    expect(result.files[1]!.bytes).toEqual(PNG);
    expect(result.report.pages).toEqual([
      { number: 1, filename: "page-0001.png" },
      { number: 2, filename: "page-0002.png" },
    ]);
    expect(result.report.extractedText).toBe(text);
    expect(result.report.fontStatus).toBe("unverified");
    expect(result.report.checks).toContainEqual(
      expect.objectContaining({
        code: "visual_review_required",
        severity: "warning",
      }),
    );
    expect(profile).toContain('oor:name="DisableMacrosExecution"');
    expect(profile).toContain(
      'oor:name="OOXMLRecalcMode" oor:op="fuse"><value>0</value>',
    );
    expect(await readdir(join(data, "engine"))).toEqual([]);
    for (const call of vi.mocked(spawn).mock.calls) {
      expect(call[0]).toBe(confinementCommand);
      const options = call[2] as SpawnOptions;
      expect(options.shell).toBe(false);
      expect(options.env).not.toHaveProperty("PRIVATE_HOST_TOKEN");
      expect(options.env).not.toHaveProperty("HTTP_PROXY");
      expect(options.env?.HOME).toContain("invocation-");
      expect(options.env?.MAX_CONCURRENCY).toBe(
        String(LIMITS.processConcurrency),
      );
      if (process.platform === "darwin") {
        expect(call[1]?.[1]).toContain("(deny network*)");
        expect(call[1]?.[2]).toBe("/bin/bash");
        expect(call[1]?.[5]).toContain("ulimit -t 120;");
      } else {
        expect(call[1]).toContain("--unshare-all");
        expect(call[1]).toContain(`--as=${LIMITS.processMemoryBytes}`);
      }
    }
  });

  it("recalculates into a separate output with exact source/output hashes and preserves the original bytes", async () => {
    const original = Buffer.from("PK original workbook");
    const result = await engine.recalculate(
      original,
      new AbortController().signal,
    );
    expect(source).toEqual(original);
    expect(result.bytes).toEqual(changedOutput);
    expect(result.report.sourceSha256).toMatch(/^[a-f0-9]{64}$/u);
    expect(result.report.outputSha256).not.toBe(result.report.sourceSha256);
    expect(result.report.recalculation).toContain("OOXMLRecalcMode=0");
    const conversion = vi
      .mocked(spawn)
      .mock.calls.find((call) =>
        (call[1] as string[]).includes("--convert-to"),
      )!;
    const argv = conversion[1] as string[];
    expect(argv[argv.indexOf("--convert-to") + 1]).toBe(
      "xlsx:Calc MS Excel 2007 XML",
    );
    expect(argv.at(-1)).toContain("/input/source.xlsx");
    expect(argv[argv.indexOf("--outdir") + 1]).toContain("/output");
    expect(await readdir(join(data, "engine"))).toEqual([]);
  });

  it.each(["missing", "linked", "directory", "oversized"] as const)(
    "does not interpret a zero process exit with %s output as success",
    async (failure) => {
      mode = failure;
      await expect(
        engine.render(
          "docx",
          Buffer.from("PK source"),
          new AbortController().signal,
        ),
      ).rejects.toMatchObject({
        code: expect.stringMatching(/^engine_output_/u),
      });
      expect(await readdir(join(data, "engine"))).toEqual([]);
    },
  );

  it.each(["0", "101", "-1", "NaN", "9007199254740993"])(
    "refuses an unbounded or invalid reported page count %s before page rendering",
    async (pages) => {
      pageCount = pages;
      await expect(
        engine.render("pdf", PDF, new AbortController().signal),
      ).rejects.toMatchObject({ code: "engine_page_limit" });
      expect(
        vi
          .mocked(spawn)
          .mock.calls.some((call) =>
            (call[1] as string[]).includes("-singlefile"),
          ),
      ).toBe(false);
    },
  );

  it("keeps absent text extraction and truncated text distinct from complete evidence", async () => {
    const withoutText = new Engine(
      {
        ...environment,
        FLAME_DELIVERABLES_OFFICE: "",
        FLAME_DELIVERABLES_PDFTOTEXT: "",
      },
      data,
    );
    const report = (
      await withoutText.render("pdf", PDF, new AbortController().signal)
    ).report;
    expect(report.checks).toContainEqual(
      expect.objectContaining({
        code: "text_unavailable",
        severity: "warning",
      }),
    );
    await withoutText.close();
    text = "汉".repeat(LIMITS.renderTextBytes);
    const full = (await engine.render("pdf", PDF, new AbortController().signal))
      .report;
    expect(Buffer.byteLength(full.extractedText)).toBeLessThanOrEqual(
      LIMITS.renderTextBytes,
    );
    expect(full.extractedText).not.toContain("\ufffd");
    expect(full.checks).toContainEqual(
      expect.objectContaining({ code: "text_truncated" }),
    );
  });

  it("refuses concurrent work without queuing and joins cancellation before deleting scratch", async () => {
    mode = "hang";
    const rendering = engine.render("pdf", PDF, new AbortController().signal);
    await vi.waitFor(() => expect(groups.size).toBe(1));
    expect(() =>
      engine.render("pdf", PDF, new AbortController().signal),
    ).toThrow("already active");
    const rejected = expect(rendering).rejects.toMatchObject({
      name: "AbortError",
    });
    await engine.close();
    await rejected;
    expect(groups.size).toBe(0);
    expect(await readdir(join(data, "engine"))).toEqual([]);
    expect(() =>
      engine.render("pdf", PDF, new AbortController().signal),
    ).toThrow("closed");
  });

  it("leaves scratch owned and refuses reuse when a child exit cannot be confirmed", async () => {
    mode = "hang";
    const controller = new AbortController();
    const rendering = engine.render("pdf", PDF, controller.signal);
    await vi.waitFor(() => expect(groups.size).toBe(1));
    kills.mockImplementation(() => {
      throw Object.assign(new Error("cannot signal"), { code: "EPERM" });
    });
    const rejected = expect(rendering).rejects.toBeInstanceOf(
      UnconfirmedProcessExit,
    );
    controller.abort();
    await rejected;
    expect(await readdir(join(data, "engine"))).toHaveLength(1);
    await expect(engine.close()).rejects.toBeInstanceOf(UnconfirmedProcessExit);
    expect(() =>
      engine.render("pdf", PDF, new AbortController().signal),
    ).toThrow("closed");
  });

  it("observes shutdown during successful cleanup before returning processed bytes", async () => {
    const actual =
      await vi.importActual<typeof import("node:fs/promises")>(
        "node:fs/promises",
      );
    const cleaning = Promise.withResolvers<void>();
    const release = Promise.withResolvers<void>();
    vi.mocked(rm).mockImplementationOnce(async (path, options) => {
      cleaning.resolve();
      await release.promise;
      await actual.rm(path, options);
    });
    const operation = engine.recalculate(
      Buffer.from("PK original workbook"),
      new AbortController().signal,
    );
    const failed = expect(operation).rejects.toMatchObject({
      name: "AbortError",
    });
    try {
      await cleaning.promise;
      expect(groups.size).toBe(0);
      const closing = engine.close();
      release.resolve();
      await failed;
      await closing;
      expect(await readdir(join(data, "engine"))).toEqual([]);
    } finally {
      release.resolve();
      vi.mocked(rm).mockImplementation(actual.rm);
    }
  });

  it("applies one invocation deadline and confirms cancellation before completing", async () => {
    mode = "hang";
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const rendering = engine.render("pdf", PDF, new AbortController().signal);
    const settled = rendering.then(
      (value) => ({ value }),
      (error: unknown) => ({ error }),
    );
    for (let attempt = 0; attempt < 200 && groups.size === 0; attempt += 1)
      await nativeDelay(10);
    expect(groups.size).toBe(1);
    await vi.advanceTimersByTimeAsync(LIMITS.processMs);
    expect(await settled).toMatchObject({ error: { name: "TimeoutError" } });
    expect(groups.size).toBe(0);
    expect(await readdir(join(data, "engine"))).toEqual([]);
  });

  it("fails closed when the installed confinement process rejects execution", async () => {
    mode = "process-failure";
    await expect(
      engine.render("pdf", PDF, new AbortController().signal),
    ).rejects.toMatchObject({ code: "process_failed" });
    expect(vi.mocked(spawn)).toHaveBeenCalledOnce();
    expect(vi.mocked(spawn).mock.calls[0]![0]).toBe(confinementCommand);
    expect(await readdir(join(data, "engine"))).toEqual([]);
  });

  it("rejects broad read roots, private-data overlap, and symlinked scratch before spawning", async () => {
    for (const root of ["/", "/home", "/workspace", "/tmp", "/opt"]) {
      const invalid = new Engine(
        {
          ...environment,
          FLAME_DELIVERABLES_TOOL_ROOTS: JSON.stringify([root]),
        },
        data,
      );
      expect(invalid.catalog().configured).toBe(false);
      expect(() =>
        invalid.render("pdf", PDF, new AbortController().signal),
      ).toThrow();
      await invalid.close();
    }
    const overlap = new Engine(
      {
        ...environment,
        FLAME_DELIVERABLES_TOOL_ROOTS: JSON.stringify([temporary]),
      },
      data,
    );
    await expect(
      overlap.render("pdf", PDF, new AbortController().signal),
    ).rejects.toMatchObject({ code: "engine_configuration" });
    await overlap.close();
    const outside = join(temporary, "outside");
    await mkdir(outside, { mode: 0o700 });
    await symlink(outside, join(data, "engine"));
    await expect(
      engine.render("pdf", PDF, new AbortController().signal),
    ).rejects.toMatchObject({ code: "engine_private_directory" });
    expect(spawn).not.toHaveBeenCalled();
    expect(await readdir(outside)).toEqual([]);
  });

  it.each([SYSTEM_ROOTS.linux[0], SYSTEM_FILES[0]])(
    "refuses private data nested within the fixed read root %s before spawning",
    async (root) => {
      const actual =
        await vi.importActual<typeof import("node:fs/promises")>(
          "node:fs/promises",
        );
      const nested = join(root, "private-fixture");
      vi.mocked(realpath).mockImplementation(async (path) =>
        path === nested || path === root ? path : actual.realpath(path),
      );
      vi.mocked(lstat).mockImplementation(async (path) =>
        actual.lstat(path === nested ? data : path),
      );
      const invalid = new Engine(environment, nested);
      try {
        await expect(
          invalid.render("pdf", PDF, new AbortController().signal),
        ).rejects.toMatchObject({ code: "engine_configuration" });
        expect(spawn).not.toHaveBeenCalled();
        expect(await readdir(data)).toEqual([]);
      } finally {
        vi.mocked(realpath).mockImplementation(actual.realpath);
        vi.mocked(lstat).mockImplementation(actual.lstat);
        await invalid.close();
      }
    },
  );

  it("refuses an existing shared scratch directory instead of changing its owner or permissions", async () => {
    await mkdir(join(data, "engine"), { mode: 0o700 });
    await chmod(join(data, "engine"), 0o755);
    await expect(
      engine.render("pdf", PDF, new AbortController().signal),
    ).rejects.toMatchObject({ code: "engine_private_directory" });
    expect(spawn).not.toHaveBeenCalled();
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

  it("admits only specific read roots and private writes and passes hostile-looking paths as argv", () => {
    const command = join(tools, 'space $() "quoted"');
    const args = ['--value="literal"', "$(do-not-run)"];
    const plan = confinedPlan("linux", directory(), [tools], command, args, {
      maximumFileBytes: 1234,
      memoryBytes: LIMITS.processMemoryBytes,
      cpuSeconds: Math.ceil(LIMITS.processMs / 1000),
      writable: true,
    });
    expect(plan.command).toBe("/usr/bin/bwrap");
    expect(plan.args.slice(-3)).toEqual([command, ...args]);
    expect(plan.args).toContain("--fsize=1234");
    expect(plan.args).toContain("--unshare-all");
    expect(plan.args).not.toContain("--share-net");
    expect(plan.args).not.toContain(process.env.HOME);
    const mac = confinedPlan("darwin", directory(), [tools], command, args, {
      maximumFileBytes: 1234,
      memoryBytes: LIMITS.processMemoryBytes,
      cpuSeconds: Math.ceil(LIMITS.processMs / 1000),
      writable: true,
    });
    expect(mac.command).toBe("/usr/bin/sandbox-exec");
    expect(mac.args[1]).toContain("(deny process-fork)");
    expect(mac.args[1]).toContain("(deny network*)");
    expect(mac.args[1]).not.toContain("(allow default)");
    expect(mac.args.slice(-3)).toEqual([command, ...args]);
    expect(mac.args[5]).not.toContain(command);
    expect(mac.args[5]).toContain("ulimit -f 1;");
  });

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

  it("kills an overproducing process instead of retaining unbounded diagnostics", async () => {
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
        child.emit("exit", null, "SIGKILL");
        child.emit("close", null, "SIGKILL");
      });
      return child as unknown as ReturnType<typeof spawn>;
    });
    const running = runProcess(
      {
        command: "/usr/bin/bwrap",
        args: [],
        cwd: data,
        env: {},
        containment: { kind: "namespace" },
      },
      new AbortController().signal,
    );
    child.stderr.write(Buffer.alloc(LIMITS.outputBytes + 1, 65));
    await expect(running).rejects.toMatchObject({
      code: "process_output_limit",
    });
    expect(groups.size).toBe(0);
  });
});
