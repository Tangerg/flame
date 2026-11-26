import { spawn } from "node:child_process";
import { mkdir, writeFile, copyFile } from "node:fs/promises";
import { createConnection } from "node:net";
import { join } from "node:path";
import { createInterface } from "node:readline";
import { afterAll, beforeAll, beforeEach, expect, inject } from "vitest";
import {
  createFlameClient,
  type FlameClient,
  createHttpTransport,
  JSONRPC_VERSION,
} from "@flame/runtime-client";
import type { WireMethodName } from "@flame/runtime-client/methods";
import {
  HTTP_ENDPOINTS,
  HTTP_HEADERS,
  PROTOCOL_VERSION,
  RPC_HTTP,
  type MCPTool,
  type PluginInstallation,
  type ServerCapabilities,
} from "@flame/runtime-client/wire";
import { fileURLToPath } from "node:url";
import { startFakeProvider, type FakeProvider, type ProviderGate } from "./fakeProvider";
import { processStartupMilliseconds, waitUntilReady, within } from "./observation";

const mcpFixtureSource = fileURLToPath(new URL("./mcp-ping.mjs", import.meta.url));

export const managedSkillName = "runtime-http-e2e";

const runtimeFixtureStartupMilliseconds = 60_000;

const runtimeListen = "127.0.0.1:0";
const listenerAnnouncement = "[flame] http listening on ";
const sandboxExecutable = "/usr/bin/sandbox-exec";
const unavailableIsolationProfile = `(version 1)(allow default)(deny file-read-metadata (literal "${sandboxExecutable}"))`;

// One fixture per suite file: a runner, a Runtime and a scripted provider that
// live for the whole file, with live accessors for state tests replace.
export function useRuntimeFixture(
  suite: string,
  { isolationUnavailable: defaultIsolationUnavailable = false } = {},
) {
  const environmentRoot = inject("runtimeHttpEnvironmentRoot");
  // Each suite owns a fresh Runtime, so its workspace and durable state start
  // empty; they stay out of the shared distribution beside the executables.
  const suiteRoot = join(environmentRoot, "suites", suite);
  const root = join(suiteRoot, "workspace");
  const runtimeHome = join(suiteRoot, "home");
  const runtimeData = join(suiteRoot, "runtime-data");
  const runtimeStore = join(runtimeData, "runtime");
  let baseUrl = "";
  let mcpFixturePath = "";
  let providerBaseUrl = "";
  let runtime: ReturnType<typeof spawn> | undefined;
  let processRunner:
    | {
        child: ReturnType<typeof spawn>;
        exited: Promise<{ code: number | null; signal: NodeJS.Signals | null }>;
      }
    | undefined;
  let runnerOutput = "";
  let provider: FakeProvider | undefined;
  let providerGate: ProviderGate | undefined;
  let client: FlameClient | undefined;
  let negotiated: ServerCapabilities | undefined;
  let processOutput = "";
  const runtimeExecutable = join(environmentRoot, "flame-e2e");
  const cliExecutable = join(environmentRoot, "flame-cli-e2e");
  const managedOwners = new Map<"memory" | "schedules", PluginInstallation>();

  const getInstallation = async (id: string) => {
    if (!client) throw new Error("runtime client was not initialized");
    const installation = (await client.plugins.list()).data.find((item) => item.id === id);
    if (!installation) throw new Error(`installation ${id} is unavailable`);
    return installation;
  };

  const activateManagedOwner = async (feature: "memory" | "schedules", readyBefore?: number) => {
    if (!client) throw new Error("runtime client was not initialized");
    let installation = (await client.plugins.list()).data.find(
      (item) => item.selected.name === `flame.${feature}`,
    );
    if (!installation) {
      const receipt = await client.plugins.install({
        source: join(environmentRoot, "plugins", feature),
      });
      installation = await getInstallation(receipt.installationId);
      expect(installation.selected.digest).toBe(receipt.digest);
    }
    expect(installation.selected.diagnostics).toEqual([]);
    if (!installation.enabled) {
      const id = installation.id;
      await client.plugins.setEnablement({ installationId: id, enabled: true });
      installation = await getInstallation(id);
      if (!installation.enabled) throw new Error("managed owner did not enable");
    }
    if (feature === "memory")
      await client.plugins.selectProvider({
        role: "memory",
        installationId: installation.id,
        contribution: "memory-provider",
      });
    managedOwners.set(feature, installation);
    const server = {
      origin: { type: "installation" as const, installationId: installation.id },
      name: feature,
    };
    await awaitMCPTools(
      server,
      (tools) =>
        tools.some(
          (tool) => tool.name === (feature === "memory" ? "search_memory" : "list_schedules"),
        ),
      readyBefore,
    );
    return installation;
  };

  const awaitMCPTools = async (
    server: Parameters<FlameClient["mcp"]["listTools"]>[0],
    ready: (tools: MCPTool[]) => boolean,
    readyBefore = Date.now() + processStartupMilliseconds,
  ) => {
    if (!client) throw new Error("runtime client was not initialized");
    while (!ready((await client.mcp.listTools(server)).data)) {
      if (Date.now() >= readyBefore) {
        const statuses = (await client.mcp.list()).data.map(({ id, status }) => ({ id, status }));
        throw new Error(
          `MCP source did not become ready: ${JSON.stringify(server)}; servers=${JSON.stringify(statuses)}; ${processOutput}`,
        );
      }
      await new Promise((resolve) => setTimeout(resolve, 25));
    }
  };

  const awaitManagedOwners = async () => {
    if (!client) throw new Error("runtime client was not initialized");
    for (const [feature, installation] of managedOwners) {
      await awaitMCPTools(
        { origin: { type: "installation", installationId: installation.id }, name: feature },
        (tools) =>
          tools.some(
            (tool) => tool.name === (feature === "memory" ? "search_memory" : "list_schedules"),
          ),
      );
    }
  };

  const runtimeEnvironment = () => ({
    ...process.env,
    HOME: runtimeHome,
    FLAME_HOME: runtimeData,
    FLAME_PROVIDER: "openai-compatible",
    FLAME_MODEL: "e2e-model",
    FLAME_APIKEY: "e2e-placeholder-key",
    FLAME_BASEURL: providerBaseUrl,
    OPENAI_COMPATIBLE_API_KEY: "e2e-placeholder-key",
    OPENAI_API_KEY: "",
    FLAME_SERVER_LISTEN: runtimeListen,
    FLAME_SERVER_NOLOCALTOKEN: "true",
    OTEL_SDK_DISABLED: "true",
  });

  const startRunnerProcess = async (readyBefore = Date.now() + processStartupMilliseconds) => {
    const directory = join(runtimeStore, "runner");
    const socketPath = join(directory, "control.sock");
    const child = spawn(
      join(environmentRoot, "frame-runner"),
      ["--directory", directory, "--socket", socketPath],
      { cwd: root, stdio: ["ignore", "pipe", "pipe"] },
    );
    child.stdout.on("data", (chunk: Buffer) => {
      runnerOutput += chunk.toString();
    });
    child.stderr.on("data", (chunk: Buffer) => {
      runnerOutput += chunk.toString();
    });
    const exited = new Promise<{ code: number | null; signal: NodeJS.Signals | null }>(
      (resolve, reject) => {
        child.once("error", reject);
        child.once("close", (code, signal) => resolve({ code, signal }));
      },
    );
    processRunner = { child, exited };
    const stopped = exited.then(({ code, signal }) => {
      throw new Error(
        `runner exited before fixture shutdown (${code}, ${signal}): ${runnerOutput}`,
      );
    });
    // The fixture owns this process across deliberate Host crashes. This probe
    // only orders startup; Runtime still performs the original identity handshake.
    while (Date.now() < readyBefore) {
      const socket = createConnection(socketPath);
      try {
        await within(
          Promise.race([
            new Promise<void>((resolve, reject) => {
              socket.once("connect", resolve);
              socket.once("error", reject);
            }),
            stopped,
          ]),
          "runner control listener",
          readyBefore - Date.now(),
        );
        return;
      } catch (error) {
        const code = (error as NodeJS.ErrnoException).code;
        if (code !== "ENOENT" && code !== "ECONNREFUSED") throw error;
      } finally {
        socket.destroy();
      }
      await new Promise((resolve) => setTimeout(resolve, 25));
    }
    throw new Error(`timed out waiting for runner control listener: ${runnerOutput}`);
  };

  const stopRunnerProcess = async () => {
    const owner = processRunner;
    if (!owner) return;
    if (owner.child.exitCode !== null || owner.child.signalCode !== null)
      throw new Error(`runner exited before fixture shutdown: ${runnerOutput}`);
    if (!owner.child.kill("SIGTERM")) throw new Error("failed to signal the original runner");
    const result = await within(owner.exited, "original runner shutdown");
    if (result.code !== 0 || result.signal !== null)
      throw new Error(
        `original runner shutdown failed (${result.code}, ${result.signal}): ${runnerOutput}`,
      );
    processRunner = undefined;
  };

  const startRuntimeProcess = async ({
    readyBefore = Date.now() + processStartupMilliseconds,
    isolationUnavailable = defaultIsolationUnavailable,
  } = {}) => {
    processOutput = "";
    baseUrl = "";
    const restricted = isolationUnavailable && process.platform === "darwin";
    const executable = restricted ? sandboxExecutable : runtimeExecutable;
    const args = restricted ? ["-p", unavailableIsolationProfile, runtimeExecutable] : [];
    const child = spawn(executable, args, {
      cwd: root,
      env: runtimeEnvironment(),
      stdio: ["ignore", "pipe", "pipe"],
    });
    runtime = child;
    const listener = Promise.withResolvers<string>();
    child.stdout.on("data", (chunk: Buffer) => {
      processOutput += chunk.toString();
    });
    createInterface({ input: child.stderr }).on("line", (line) => {
      processOutput += `${line}\n`;
      if (line.startsWith(listenerAnnouncement))
        listener.resolve(line.slice(listenerAnnouncement.length));
    });
    const stopped = new Promise<never>((_, reject) => {
      child.once("error", reject);
      child.once("close", (code, signal) => {
        reject(new Error(`Runtime exited before readiness (${code}, ${signal}): ${processOutput}`));
      });
    });
    const address = await within(
      Promise.race([listener.promise, stopped]),
      "Runtime's bound address",
      readyBefore - Date.now(),
    );
    const url = new URL(`http://${address}`);
    if (url.port === "0") throw new Error(`Runtime advertised the unbound listener ${address}`);
    baseUrl = url.origin;
    await Promise.race([waitUntilReady(baseUrl, () => processOutput, readyBefore), stopped]);
  };

  const stopRuntimeProcess = async () => {
    const child = runtime;
    if (!child) return;
    if (child.exitCode === null && child.signalCode === null) {
      const exited = new Promise<void>((resolve) => child.once("exit", () => resolve()));
      if (!child.kill("SIGTERM")) throw new Error("failed to signal Runtime shutdown");
      try {
        await within(exited, "Runtime graceful shutdown");
      } catch (error) {
        if (child.exitCode === null && child.signalCode === null) child.kill("SIGKILL");
        try {
          await within(exited, "Runtime forced shutdown");
        } catch (forcedError) {
          throw new AggregateError([error, forcedError], "Runtime shutdown failed");
        }
        throw error;
      }
    }
    if (child.exitCode !== 0 || child.signalCode !== null)
      throw new Error(
        `Runtime shutdown failed (${child.exitCode}, ${child.signalCode}): ${processOutput}`,
      );
  };

  const killRuntimeProcess = async () => {
    const child = runtime;
    if (!child || child.exitCode !== null) return;
    const exited = new Promise<void>((resolve) => child.once("exit", () => resolve()));
    if (!child.kill("SIGKILL")) throw new Error("failed to send SIGKILL to Runtime");
    await within(exited, "Runtime SIGKILL exit");
  };

  const createRuntimeClient = (fetch?: typeof globalThis.fetch) =>
    createFlameClient(createHttpTransport({ baseUrl, fetch }), {
      capabilities: () => negotiated,
      requestMeta: () => ({
        protocolVersion: PROTOCOL_VERSION,
        clientInfo: { name: "runtime-http-e2e", version: "1" },
        clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
      }),
    });

  const postRPCDocument = (
    body: string,
    headers: Record<string, string> = {},
  ): Promise<Response> => {
    const endpoint = HTTP_ENDPOINTS.rpc;
    return fetch(new URL(endpoint.path, baseUrl), {
      method: endpoint.method,
      headers: {
        Accept: RPC_HTTP.documentMediaType,
        [HTTP_HEADERS.contentType]: RPC_HTTP.documentMediaType,
        ...headers,
      },
      body,
    });
  };

  const postRPC = (method: WireMethodName, params: unknown, headers?: Record<string, string>) =>
    postRPCDocument(
      JSON.stringify({ jsonrpc: JSONRPC_VERSION, id: method, method, params }),
      headers,
    );

  const createSharedClient = () =>
    createFlameClient(createHttpTransport({ baseUrl }), {
      capabilities: () => negotiated,
      requestMeta: () => ({
        protocolVersion: PROTOCOL_VERSION,
        clientInfo: { name: "shared-client-e2e", version: "1" },
        clientCapabilities: {
          features: { childRuns: { enabled: true } },
          interruptTypes: ["approval", "question"],
        },
      }),
    });

  const startCLI = (args: string[]) => {
    const child = spawn(cliExecutable, ["--runtime-url", baseUrl, ...args], {
      cwd: root,
      env: {
        ...process.env,
        FLAME_HOME: join(environmentRoot, "cli-data"),
        XDG_CONFIG_HOME: join(environmentRoot, "cli-config"),
        FLAME_RUNTIME_TOKEN: "",
        FLAME_PROVIDER: "",
        FLAME_MODEL: "",
        FLAME_APIKEY: "",
        FLAME_BASEURL: "",
        OTEL_SDK_DISABLED: "true",
      },
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk: Buffer) => {
      stdout += chunk.toString();
    });
    child.stderr.on("data", (chunk: Buffer) => {
      stderr += chunk.toString();
    });
    const done = new Promise<{ code: number | null; stdout: string; stderr: string }>((resolve) => {
      child.once("error", (error) => resolve({ code: null, stdout, stderr: error.message }));
      child.once("close", (code) => resolve({ code, stdout, stderr }));
    });
    return { child, done };
  };

  beforeAll(async () => {
    const readyBefore = Date.now() + runtimeFixtureStartupMilliseconds;
    await Promise.all([mkdir(root, { recursive: true }), mkdir(runtimeHome, { recursive: true })]);
    mcpFixturePath = join(suiteRoot, "mcp-fixture.mjs");
    await copyFile(mcpFixtureSource, mcpFixturePath);
    const managedSkillDirectory = join(runtimeStore, "skills", managedSkillName);
    await mkdir(managedSkillDirectory, { recursive: true });
    await writeFile(
      join(managedSkillDirectory, "SKILL.md"),
      `---\nname: ${managedSkillName}\ndescription: Exercise the real Runtime skill lifecycle.\n---\n\nRun the Runtime HTTP E2E suite.\n`,
    );
    provider = await startFakeProvider(() => providerGate);
    providerBaseUrl = provider.baseUrl;

    await startRunnerProcess(readyBefore);
    await startRuntimeProcess({ readyBefore });
    client = createRuntimeClient();
    negotiated = (await client.runtime.discover()).capabilities;
    for (const feature of ["memory", "schedules"] as const)
      await activateManagedOwner(feature, readyBefore);
  }, runtimeFixtureStartupMilliseconds);

  beforeEach(async () => {
    if (client) negotiated = (await client.runtime.discover()).capabilities;
    for (const feature of ["memory", "schedules"] as const)
      if (!managedOwners.has(feature)) await activateManagedOwner(feature);
    await awaitManagedOwners();
  });

  afterAll(async () => {
    const failures: unknown[] = [];
    for (const close of [
      () => client?.close(),
      stopRuntimeProcess,
      stopRunnerProcess,
      () => provider?.close(),
    ]) {
      try {
        await close();
      } catch (failure) {
        failures.push(failure);
      }
    }
    if (failures.length === 1) throw failures[0];
    if (failures.length > 1)
      throw new AggregateError(failures, "runtime HTTP fixture shutdown failed");
  }, 10_000);

  return {
    environmentRoot,
    root,
    runtimeHome,
    runtimeStore,
    managedOwners,
    get baseUrl() {
      return baseUrl;
    },
    get mcpFixturePath() {
      return mcpFixturePath;
    },
    get providerBaseUrl() {
      return providerBaseUrl;
    },
    get processOutput() {
      return processOutput;
    },
    get client(): FlameClient {
      if (!client) throw new Error("runtime client was not initialized");
      return client;
    },
    set client(value: FlameClient | undefined) {
      client = value;
    },
    get negotiated() {
      return negotiated;
    },
    set negotiated(value: ServerCapabilities | undefined) {
      negotiated = value;
    },
    set providerGate(gate: ProviderGate | undefined) {
      providerGate = gate;
    },
    activateManagedOwner,
    getInstallation,
    awaitMCPTools,
    awaitManagedOwners,
    startRuntimeProcess,
    stopRuntimeProcess,
    killRuntimeProcess,
    createRuntimeClient,
    postRPC,
    postRPCDocument,
    createSharedClient,
    startCLI,
  };
}
