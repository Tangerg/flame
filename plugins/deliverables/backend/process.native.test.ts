import { once } from "node:events";
import {
  mkdir,
  mkdtemp,
  readFile,
  realpath,
  rm,
  writeFile,
} from "node:fs/promises";
import { createConnection, createServer } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { expect, it } from "vitest";
import { LIMITS } from "../contracts/limits";
import { confinedPlan } from "./confinement";
import { runProcess, UnconfirmedProcessExit } from "./process";

const native = it.runIf(
  process.platform === "darwin" ||
    (process.env.FLAME_DELIVERABLES_NATIVE_TESTS === "1" &&
      process.platform === "linux" &&
      process.arch === "x64"),
);

const probe = `import { readFile, writeFile } from "node:fs/promises";
import { createConnection } from "node:net";
import { spawnSync } from "node:child_process";
const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);
const request = JSON.parse(Buffer.concat(chunks).toString("utf8"));
const attempt = async (effect) => {
  try { await effect(); return "allowed"; }
  catch (error) { return error.code ?? error.name; }
};
const result = {
  publicRead: await readFile(request.publicFile, "utf8"),
  privateRead: await attempt(() => readFile(request.privateFile)),
  write: await attempt(() => writeFile(request.target, "modified")),
};
result.network = await new Promise((resolve) => {
  const socket = createConnection({ host: "127.0.0.1", port: request.port });
  socket.setTimeout(2000);
  socket.once("connect", () => { socket.destroy(); resolve("allowed"); });
  socket.once("error", (error) => { socket.destroy(); resolve(error.code); });
  socket.once("timeout", () => { socket.destroy(); resolve("timeout"); });
});
if (request.nodePermissions || process.platform === "darwin") {
  result.child = await attempt(() => {
    const spawned = spawnSync(process.execPath, ["-e", "process.exit(0)"], { stdio: "ignore" });
    if (spawned.error) throw spawned.error;
    if (spawned.status !== 0) throw new Error("unexpected child failure");
  });
}
process.stdout.write(JSON.stringify(result));
`;

// The OS-only case prevents Node's filesystem checks from disguising a broken
// mount policy. Both cases still enter the production OS confinement owner.
native.each([false, true])(
  "enforces real OS confinement with Node permission checks %s",
  async (nodePermissions) => {
    const root = await mkdtemp(
      join(await realpath(tmpdir()), "flame-process-native-"),
    );
    const directory = {
      root: join(root, "invocation"),
      input: join(root, "invocation", "input"),
      output: join(root, "invocation", "output"),
      home: join(root, "invocation", "home"),
      temporary: join(root, "invocation", "tmp"),
    };
    const bundle = join(root, "bundle");
    const privateFile = join(root, "host-private.txt");
    const publicFile = join(bundle, "readable.txt");
    const script = join(bundle, "probe.mjs");
    const target = join(directory.output, "retained.txt");
    let connections = 0;
    let retain = false;
    const server = createServer((socket) => {
      connections += 1;
      socket.destroy();
    });
    try {
      await mkdir(directory.root, { mode: 0o700 });
      for (const path of [
        directory.input,
        directory.output,
        directory.home,
        directory.temporary,
        bundle,
      ])
        await mkdir(path, { mode: 0o700 });
      await writeFile(script, probe, { mode: 0o600 });
      await writeFile(publicFile, "readable", { mode: 0o600 });
      await writeFile(privateFile, "private fixture", { mode: 0o600 });
      await writeFile(target, "unchanged", { mode: 0o600 });
      const listening = once(server, "listening");
      server.listen(0, "127.0.0.1");
      await listening;
      const address = server.address();
      if (!address || typeof address === "string")
        throw new Error("native fixture requires an IP listener");
      const accepted = once(server, "connection");
      const control = createConnection({
        host: "127.0.0.1",
        port: address.port,
      });
      await Promise.all([once(control, "connect"), accepted]);
      control.destroy();
      expect(connections).toBe(1);
      expect(await readFile(privateFile, "utf8")).toBe("private fixture");
      const args = [
        `--max-old-space-size=${LIMITS.formatHeapMiB}`,
        "--disable-wasm-trap-handler",
        ...(nodePermissions
          ? ["--permission", `--allow-fs-read=${bundle}`]
          : []),
        script,
      ];
      const plan = confinedPlan(
        process.platform as "linux" | "darwin",
        directory,
        [bundle, dirname(process.execPath)],
        process.execPath,
        args,
        {
          memoryBytes: LIMITS.formatMemoryBytes,
          cpuSeconds: LIMITS.formatCpuSeconds,
          maximumFileBytes: 0,
          openFiles: LIMITS.formatOpenFiles,
          writable: false,
        },
      );
      const result = await runProcess(plan, AbortSignal.timeout(10_000), {
        input: Buffer.from(
          JSON.stringify({
            privateFile,
            publicFile,
            target,
            port: address.port,
            nodePermissions,
          }),
        ),
        inputBytes: 4096,
        stdoutBytes: 4096,
      });
      const observed = JSON.parse(result.stdout) as Record<string, unknown>;
      expect(observed.publicRead).toBe("readable");
      expect(observed.privateRead).toBe(
        nodePermissions
          ? "ERR_ACCESS_DENIED"
          : process.platform === "darwin"
            ? "EPERM"
            : "ENOENT",
      );
      expect(observed.write).toBe(
        nodePermissions
          ? "ERR_ACCESS_DENIED"
          : process.platform === "darwin"
            ? "EPERM"
            : "EROFS",
      );
      expect(
        process.platform === "darwin"
          ? ["EPERM", "EACCES"]
          : ["ECONNREFUSED", "ENETUNREACH", "EHOSTUNREACH"],
      ).toContain(observed.network);
      if (nodePermissions) expect(observed.child).toBe("ERR_ACCESS_DENIED");
      else if (process.platform === "darwin")
        expect(observed.child).toBe("EPERM");
      expect(connections).toBe(1);
      expect(await readFile(target, "utf8")).toBe("unchanged");
      expect(await readFile(privateFile, "utf8")).toBe("private fixture");
    } catch (cause) {
      retain = cause instanceof UnconfirmedProcessExit;
      throw cause;
    } finally {
      if (server.listening)
        await new Promise<void>((resolve, reject) => {
          server.close((error) => (error ? reject(error) : resolve()));
        });
      if (!retain) await rm(root, { recursive: true, force: true });
    }
  },
  20_000,
);

it.runIf(process.platform === "darwin")(
  "refuses an incompatible inherited profile without executing an unrestricted command",
  async () => {
    const root = await mkdtemp(
      join(await realpath(tmpdir()), "flame-process-inherited-"),
    );
    let retain = false;
    try {
      const directory = {
        root,
        input: join(root, "input"),
        output: join(root, "output"),
        home: join(root, "home"),
        temporary: join(root, "tmp"),
      };
      for (const path of [
        directory.input,
        directory.output,
        directory.home,
        directory.temporary,
      ])
        await mkdir(path, { mode: 0o700 });
      const executable = await realpath(process.execPath);
      const plan = confinedPlan(
        "darwin",
        directory,
        [dirname(executable)],
        executable,
        [
          "--disable-wasm-trap-handler",
          "-e",
          'process.stdout.write("unrestricted execution")',
        ],
        {
          memoryBytes: LIMITS.formatMemoryBytes,
          cpuSeconds: LIMITS.formatCpuSeconds,
          maximumFileBytes: 0,
          writable: false,
        },
      );
      const inherited = {
        ...plan,
        args: [
          "-p",
          "(version 1) (allow default) (deny network*)",
          plan.command,
          ...plan.args,
        ],
      };
      await expect(
        runProcess(inherited, AbortSignal.timeout(10_000)),
      ).rejects.toMatchObject({
        code: "process_failed",
      });
    } catch (cause) {
      retain = cause instanceof UnconfirmedProcessExit;
      throw cause;
    } finally {
      if (!retain) await rm(root, { recursive: true, force: true });
    }
  },
  20_000,
);

it.runIf(process.platform === "darwin")(
  "terminates and joins a real process exceeding its resident-memory threshold",
  async () => {
    const root = await mkdtemp(
      join(await realpath(tmpdir()), "flame-process-memory-"),
    );
    let retain = false;
    try {
      const directory = {
        root,
        input: join(root, "input"),
        output: join(root, "output"),
        home: join(root, "home"),
        temporary: join(root, "tmp"),
      };
      for (const path of [
        directory.input,
        directory.output,
        directory.home,
        directory.temporary,
      ])
        await mkdir(path, { mode: 0o700 });
      const executable = await realpath(process.execPath);
      const plan = confinedPlan(
        "darwin",
        directory,
        [dirname(executable)],
        executable,
        ["--disable-wasm-trap-handler", "-e", "setInterval(() => {}, 1000)"],
        {
          memoryBytes: 1,
          cpuSeconds: LIMITS.formatCpuSeconds,
          maximumFileBytes: 0,
          writable: false,
        },
      );
      await expect(
        runProcess(plan, AbortSignal.timeout(10_000)),
      ).rejects.toMatchObject({
        code: "process_memory_limit",
      });
    } catch (cause) {
      retain = cause instanceof UnconfirmedProcessExit;
      throw cause;
    } finally {
      if (!retain) await rm(root, { recursive: true, force: true });
    }
  },
  20_000,
);
