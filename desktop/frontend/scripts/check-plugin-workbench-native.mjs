import assert from "node:assert/strict";
import { execFile, spawn } from "node:child_process";
import { promisify } from "node:util";
import {
  cp,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  realpath,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import { randomUUID } from "node:crypto";
import { homedir, tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

assert.equal(process.platform, "darwin");
assert.ok(process.argv[2], "run through the desktop task graph");
const execute = promisify(execFile);
const frontend = fileURLToPath(new URL("..", import.meta.url));
const desktop = dirname(frontend);
const repository = dirname(desktop);
const identifier = `com.tangerg.flame.native-gate.${randomUUID()}`;
const containerName = identifier.replaceAll(".", "-");
const sandbox = join(homedir(), "Library", "Containers", identifier);
const canary = join(homedir(), "Library", `${identifier}.canary`);
await assert.rejects(stat(sandbox), { code: "ENOENT" });
await assert.rejects(stat(canary), { code: "ENOENT" });
const directory = await realpath(await mkdtemp(join(tmpdir(), "flame-plugin-workbench-")));
const fixture = join(directory, "fixture");
const workspace = `/state/${randomUUID()}/workspace`;
await assert.rejects(stat(workspace), { code: "ENOENT" });
const model = "claude-native-fixture";
const title = "Native portable pages";
const memoryFact = "Native project memory fact";
const userFact = "Native user memory fact";
const scheduleTitle = "Native saved schedule";
let native;
let nativeOutput = "";
let nativeExit;
let endpoint;
let idempotencyNamespace;
let mcpEndpoint;
let runtimeModule;
let failure;
const installed = {};
let resultPackage;
let fileRendererPackage;
const mcpToken = randomUUID();
const manifest = JSON.parse(
  await readFile(join(repository, "runtime/contract/manifest.json"), "utf8"),
);

async function command(program, args, options = {}) {
  return execute(program, args, {
    cwd: desktop,
    timeout: 120000,
    maxBuffer: 4 * 1024 * 1024,
    ...options,
  });
}

async function waitFor(label, read, accept, timeout = 20000) {
  const deadline = Date.now() + timeout;
  let observed;
  do {
    if (nativeExit)
      throw new Error(`native process ended: ${JSON.stringify(nativeExit)}\n${nativeOutput}`);
    observed = await read();
    if (accept(observed)) return observed;
    await new Promise((resolve) => setTimeout(resolve, 100));
  } while (Date.now() < deadline);
  throw new Error(`${label} timed out: ${JSON.stringify(observed)}\n${nativeOutput}`);
}

async function rpc(method, params = {}) {
  const metadata = { protocolVersion: manifest.protocolVersion };
  const headers = { "Content-Type": "application/json" };
  if (manifest.methods.find((entry) => entry.name === method).idempotency !== "none") {
    headers["Idempotency-Key"] = randomUUID();
    headers["Idempotency-Namespace"] = idempotencyNamespace;
  }
  const response = await fetch(`${endpoint}/v2/rpc`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: randomUUID(),
      method,
      params: { ...params, _meta: metadata },
    }),
    signal: AbortSignal.timeout(20000),
  });
  assert.equal(response.status, 200, `${method}: ${await response.clone().text()}`);
  if (response.headers.get("Content-Type").startsWith("text/event-stream")) {
    const frames = (await response.text())
      .split("\n")
      .filter((line) => line.startsWith("data:"))
      .map((line) => JSON.parse(line.slice(5)));
    assert.ok(frames.length > 1, `${method} must produce recorded execution events`);
    assert.equal(frames[0].error, undefined, JSON.stringify(frames[0]));
    return frames[0].result;
  }
  const message = await response.json();
  assert.equal(message.error, undefined, `${method}: ${JSON.stringify(message.error)}`);
  return message.result;
}

async function tool(name, args = {}) {
  const response = await fetch(mcpEndpoint, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${mcpToken}` },
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: randomUUID(),
      method: "tools/call",
      params: { name, arguments: args },
    }),
    signal: AbortSignal.timeout(20000),
  });
  assert.equal(response.status, 200);
  const message = await response.json();
  assert.equal(message.error, undefined, JSON.stringify(message.error));
  assert.equal(message.result.isError, false, JSON.stringify(message.result));
  const value = message.result.content[0].text;
  return JSON.parse(value);
}

const evaluate = (js) => tool("js_eval", { js });
// Wails' bound-method MCP tool imports a second SDK and replaces the app's event dispatcher.
async function bound(name, args = []) {
  const outcome = await evaluate(`
  const module = await import(${JSON.stringify(runtimeModule)});
  const runtimes = Object.values(module).filter(value => value?.Call?.ByName && value?.Events?.On);
  if (runtimes.length !== 1) throw new Error('production Wails SDK namespace is unavailable');
  try {
    return {ok: true, value: await runtimes[0].Call.ByName(${JSON.stringify(name)}, ...${JSON.stringify(args)})};
  } catch (error) {
    return {ok: false, error: error.message};
  }
`);
  if (!outcome.ok) throw new Error(outcome.error);
  return typeof outcome.value === "string" ? JSON.parse(outcome.value) : outcome.value;
}
const pages = () => bound("main.NativeWorkbenchGate.Pages");
const readGuest = (id, js) => bound("main.NativeWorkbenchGate.EvaluatePage", [id, js]);
async function guest(id, js) {
  const evaluation = await readGuest(id, js);
  if (!evaluation.ready) throw new Error("native workbench: page is not ready");
  const result = JSON.parse(evaluation.value);
  assert.equal(result?.error, undefined, JSON.stringify(result));
  return result;
}

async function clickText(text, selector = 'button, [role="option"]') {
  await waitFor(
    `control ${text}`,
    () =>
      evaluate(
        `return [...document.querySelectorAll(${JSON.stringify(selector)})].some(e => e.textContent.trim() === ${JSON.stringify(text)} && e.getBoundingClientRect().width > 0)`,
      ),
    Boolean,
  );
  await evaluate(
    `const e = [...document.querySelectorAll(${JSON.stringify(selector)})].find(e => e.textContent.trim() === ${JSON.stringify(text)} && e.getBoundingClientRect().width > 0); e.click(); return true`,
  );
}

async function clickLabel(label) {
  const selector = `[aria-label=${JSON.stringify(label)}]`;
  await waitFor(
    `control ${label}`,
    () => evaluate(`return !!document.querySelector(${JSON.stringify(selector)})`),
    Boolean,
  );
  await evaluate(`document.querySelector(${JSON.stringify(selector)}).click(); return true`);
}

async function noPages() {
  await waitFor(
    "page retirement",
    pages,
    (value) => value.pages.length === 0 && value.physical === 0,
  );
}

async function pageWith(text) {
  return waitFor(
    `native page ${text}`,
    async () => {
      const state = await pages();
      assert.equal(state.physical, state.pages.length, "physical pages and their owner must agree");
      const visible = state.pages.filter((page) => page.visible && page.ready);
      if (state.pages.length !== 1 || visible.length !== 1) return { state };
      const page = visible[0];
      const evaluation = await readGuest(page.id, "document.body.innerText");
      if (!evaluation.ready) return { state, retired: page.id };
      const content = JSON.parse(evaluation.value);
      assert.equal(content?.error, undefined, JSON.stringify(content));
      return content.includes(text) ? { ...page, content } : { state, content };
    },
    (value) => Boolean(value.id),
  );
}

async function choosePanel(panel) {
  await tool("window_control", { action: "focus" });
  await waitFor("focused workbench", () => evaluate("return !document.hidden"), Boolean);
  await clickLabel("Browse panels");
  await clickText(panel, '[role="option"]');
  await waitFor(
    "catalog dismissal",
    () =>
      evaluate(
        "return {hidden:document.hidden, popups:[...document.querySelectorAll('[data-base-ui-portal] [data-open], [data-base-ui-portal] [data-ending-style]')].map(e => ({tag:e.tagName, role:e.getAttribute('role'), open:e.hasAttribute('data-open'), ending:e.hasAttribute('data-ending-style'), animation:getComputedStyle(e).animationName, transition:getComputedStyle(e).transitionProperty}))}",
      ),
    (value) => value.popups.length === 0,
  );
}

async function openPanel(panel, text) {
  await choosePanel(panel);
  const page = await pageWith(text);
  const isolation = await guest(
    page.id,
    "({ webRTC: typeof RTCPeerConnection, bindings: typeof _wails, origin: location.origin })",
  );
  assert.deepEqual(isolation, { webRTC: "undefined", bindings: "undefined", origin: "null" });
  return page;
}

async function closePanel(panel, id) {
  await clickLabel(`Close ${panel}`);
  await noPages();
  await assert.rejects(guest(id, "document.body.innerText"), /page is not ready/);
}

async function refreshPage(id, method, binding, query, text) {
  const { installationId, digest, viewId } = binding;
  const canonicalMethod = {
    "plugins.readTrajectory": "sessions.trajectory",
    "plugins.readMemory": "agentMemory.list",
    "plugins.readSchedules": "schedules.list",
    "plugins.readUsage": "usage.summary",
    "plugins.readDiff": "workspace.diff.get",
    "plugins.readSkills": "skills.discovered.list",
    "plugins.readSkill": "skills.discovered.get",
    "plugins.readSkillLibrary": "skills.library.list",
    "plugins.readSkillProposals": "skills.proposals.list",
  }[method];
  assert.deepEqual(
    await rpc(method, { installationId, digest, viewId, ...query }),
    await rpc(canonicalMethod, query),
  );
  await guest(id, "document.getElementById('refresh').click(); true");
  const refreshed = await pageWith(text);
  assert.equal(refreshed.id, id, "refresh must preserve the current native instance");
}

try {
  console.log("Building production workbench and isolated Linux fixtures");
  await mkdir(fixture);
  await writeFile(join(fixture, "allowed"), "owned fixture\n");
  await writeFile(canary, "outside fixture\n", { mode: 0o600 });
  const { stdout: architecture } = await command("docker", [
    "info",
    "--format",
    "{{.Architecture}}",
  ]);
  const goarch = { aarch64: "arm64", x86_64: "amd64" }[architecture.trim()];
  assert.ok(goarch, "Docker must run a supported Linux architecture");
  const buildEnvironment = { ...process.env, CGO_ENABLED: "0", GOOS: "linux", GOARCH: goarch };
  await command("go", ["build", "-o", join(fixture, "runtime"), "./cmd/flame"], {
    cwd: join(repository, "runtime"),
    env: buildEnvironment,
  });
  await command(
    "go",
    ["build", "-o", join(fixture, "provider"), "./testdata/plugin-workbench/provider.go"],
    { env: buildEnvironment },
  );
  await cp(join(repository, "plugins"), join(fixture, "plugins"), { recursive: true });
  await mkdir(join(fixture, "skills/native-user"), { recursive: true });
  await writeFile(
    join(fixture, "skills/native-user/SKILL.md"),
    "---\nname: native-user\ndescription: Native user workflow\n---\nRead canonical facts before changing behavior.\n",
  );
  await mkdir(join(fixture, "config"));
  await writeFile(
    join(fixture, "config/config.yaml"),
    `provider: anthropic\nmodel: ${model}\napiKey: offline-native-fixture\nbaseURL: http://127.0.0.1:18080\nsandbox:\n  shell: false\nserver:\n  listen: 0.0.0.0:17171\n  noLocalToken: true\n  corsOrigins: ["*"]\n`,
  );
  await command("docker", ["network", "create", containerName]);
  await command("docker", [
    "run",
    "--detach",
    "--pull=never",
    "--name",
    containerName,
    "--network",
    containerName,
    "--read-only",
    "--cap-drop=ALL",
    "--security-opt=no-new-privileges",
    "--tmpfs",
    "/state:rw,size=256m",
    "--tmpfs",
    "/root:rw,size=32m",
    "--tmpfs",
    "/tmp:rw,size=32m",
    "--publish",
    "127.0.0.1::17171",
    "--mount",
    `type=bind,src=${fixture},dst=/fixture,readonly`,
    "--workdir",
    "/fixture",
    "--env",
    "FLAME_HOME=/state/flame",
    "golang:1.27.0",
    "sh",
    "-c",
    `mkdir -p ${workspace} /state/flame/runtime; cp -R /fixture/skills /state/flame/runtime/skills; /fixture/provider & exec /fixture/runtime`,
  ]);
  await command("docker", ["exec", containerName, "git", "--version"]);
  const { stdout: address } = await command("docker", ["port", containerName, "17171/tcp"]);
  assert.match(address.trim(), /^127\.0\.0\.1:\d+$/);
  endpoint = `http://${address.trim()}`;
  await waitFor(
    "isolated Runtime readiness",
    async () => {
      try {
        const response = await fetch(`${endpoint}/v2/health/ready`, {
          signal: AbortSignal.timeout(1000),
        });
        return response.ok;
      } catch (error) {
        if (
          ["ECONNREFUSED", "UND_ERR_SOCKET"].includes(error.cause?.code) ||
          error.name === "TimeoutError"
        )
          return false;
        throw error;
      }
    },
    Boolean,
  );
  const discovery = await rpc("runtime.discover");
  idempotencyNamespace = discovery.capabilities.limits.idempotency.namespace;
  for (const name of [
    "trajectory",
    "memory",
    "schedules",
    "usage",
    "skills",
    "diff",
    "diagnostics",
    "icon-gallery",
  ]) {
    const installation = await rpc("plugins.install", { source: `/fixture/plugins/${name}` });
    assert.deepEqual(installation.selected.diagnostics, []);
    const view = installation.selected.views[0];
    const binding = {
      installationId: installation.id,
      digest: installation.selected.digest,
      viewId: view.id,
    };
    await rpc("plugins.approve", { installationId: installation.id, digest: binding.digest });
    const enabled = await rpc("plugins.setEnablement", {
      installationId: installation.id,
      enabled: true,
    });
    assert.equal(enabled.presentation, "admitted");
    const resource = await rpc("plugins.readView", binding);
    assert.equal(
      resource.html,
      await readFile(join(fixture, "plugins", name, "views", `${name}.html`), "utf8"),
    );
    installed[name] = { ...binding, title: `${installation.selected.name} · ${view.title}` };
  }
  resultPackage = await rpc("plugins.install", { source: "/fixture/plugins/tool-results" });
  assert.deepEqual(resultPackage.selected.diagnostics, []);
  await rpc("plugins.approve", {
    installationId: resultPackage.id,
    digest: resultPackage.selected.digest,
  });
  await rpc("plugins.setEnablement", { installationId: resultPackage.id, enabled: true });
  fileRendererPackage = await rpc("plugins.install", { source: "/fixture/plugins/file-renderers" });
  assert.deepEqual(fileRendererPackage.selected.diagnostics, []);
  await rpc("plugins.approve", {
    installationId: fileRendererPackage.id,
    digest: fileRendererPackage.selected.digest,
  });
  await rpc("plugins.setEnablement", { installationId: fileRendererPackage.id, enabled: true });
  await command("docker", [
    "exec",
    containerName,
    "git",
    "-C",
    workspace,
    "init",
    "--initial-branch=main",
  ]);
  await command("docker", [
    "exec",
    containerName,
    "sh",
    "-c",
    `printf 'baseline\n' > ${workspace}/native-diff.txt`,
  ]);
  await command("docker", [
    "exec",
    containerName,
    "sh",
    "-c",
    `printf '# Native Markdown fixture\n\nRecorded **file** content.\n' > ${workspace}/native-file.md`,
  ]);
  await command("docker", ["exec", containerName, "git", "-C", workspace, "add", "."]);
  await command("docker", [
    "exec",
    containerName,
    "git",
    "-C",
    workspace,
    "-c",
    "user.name=Native fixture",
    "-c",
    "user.email=native@example.invalid",
    "-c",
    "commit.gpgsign=false",
    "-c",
    "core.hooksPath=/dev/null",
    "commit",
    "-m",
    "baseline",
  ]);
  await command("docker", [
    "exec",
    containerName,
    "sh",
    "-c",
    `printf 'changed native diff\n' > ${workspace}/native-diff.txt; printf '\\000binary' > ${workspace}/binary.dat`,
  ]);
  const session = await rpc("sessions.create", { title, workspace: { path: workspace } });
  await rpc("agentMemory.add", {
    scope: "project",
    workspace: { path: workspace },
    content: memoryFact,
  });
  await rpc("agentMemory.add", { scope: "user", content: userFact });
  const schedule = await rpc("schedules.create", {
    title: scheduleTitle,
    instructions: "Observe the isolated workspace.",
    cron: "0 0 1 1 *",
    workspace: { path: workspace },
  });
  const disabledSchedule = await rpc("schedules.update", {
    id: schedule.id,
    expectedRevision: schedule.revision,
    enabled: false,
  });
  const run = await rpc("runs.start", {
    sessionId: session.id,
    input: [{ type: "text", text: "Record one observation." }],
  });
  const finished = await rpc("runs.get", { runId: run.runId });
  assert.equal(finished.status, "finished");
  assert.equal(finished.outcome.type, "completed");
  let usage = await rpc("usage.summary");
  assert.equal(usage.runs, 1);
  assert.ok(finished.metrics.usage, "a reported zero must retain its usage record");
  assert.equal(usage.total.inputTokens ?? 0, 0);
  assert.equal(usage.total.costUsd, undefined);

  await command("npm", ["run", "build"], { cwd: frontend });
  const runtimeChunks = (await readdir(join(frontend, "dist/assets"))).filter((name) =>
    /^wails-runtime-.*\.js$/.test(name),
  );
  assert.equal(runtimeChunks.length, 1, "the workbench must bundle one native SDK");
  runtimeModule = `/assets/${runtimeChunks[0]}`;
  const indexPath = join(frontend, "dist/index.html");
  const index = await readFile(indexPath, "utf8");
  const privateIndex = join(directory, "index.html");
  // These are existing user preferences; no test code replaces the production entrypoint.
  await writeFile(
    privateIndex,
    index.replace(
      "<head>",
      `<head><script>localStorage.setItem('flame.plugin.flame.builtin.runtime.endpoint', ${JSON.stringify(JSON.stringify(endpoint))}); localStorage.setItem('flame.locale','en');
      </script>`,
    ),
  );
  const mainPath = join(desktop, "main.go");
  const main = await readFile(mainPath, "utf8");
  const composition = "app := application.New(desktopApplicationOptions(host))";
  assert.equal(main.split(composition).length, 2);
  const privateMain = join(directory, "main.go");
  await writeFile(
    privateMain,
    main.replace(
      composition,
      "options := desktopApplicationOptions(host)\n\toptions.Logger = workbenchGateLogger()\n\toptions.Services = append(options.Services, application.NewService(&NativeWorkbenchGate{host: host}))\n\tapp := application.New(options)",
    ),
  );
  const carrierPath = join(desktop, "plugin_carrier_darwin.go");
  const carrier = await readFile(carrierPath, "utf8");
  const privateCarrier = join(directory, "carrier.go");
  const setup = "page := &nativePluginPage{pointer: pointer, handle: handle}";
  const teardown = "C.closePluginPage(p.pointer)";
  assert.equal(carrier.split(setup).length, 2);
  assert.equal(carrier.split(teardown).length, 2);
  await writeFile(
    privateCarrier,
    carrier
      .replace(
        "void closePluginPage(void *);",
        "void closePluginPage(void *);\nvoid observeWorkbenchPage(void *);\nvoid stopObservingWorkbenchPage(void *);",
      )
      .replace(setup, `C.observeWorkbenchPage(pointer)\n\t${setup}`)
      .replace(teardown, `C.stopObservingWorkbenchPage(p.pointer)\n\t${teardown}`),
  );
  const privateInspector = join(directory, "inspector.go");
  const inspector = await readFile(join(desktop, "testdata/plugin-workbench/inspect.m"), "utf8");
  await writeFile(privateInspector, `package main\n/*\n${inspector}\n*/\nimport "C"\n`);
  const overlay = join(directory, "overlay.json");
  await writeFile(
    overlay,
    JSON.stringify({
      Replace: {
        [mainPath]: privateMain,
        [indexPath]: privateIndex,
        [carrierPath]: privateCarrier,
        [join(desktop, "native_workbench_inspect.go")]: join(
          desktop,
          "testdata/plugin-workbench/inspect.go",
        ),
        [join(desktop, "native_workbench_inspector.go")]: privateInspector,
      },
    }),
  );
  const bundle = join(directory, "Flame native gate.app");
  const executable = join(bundle, "Contents/MacOS/flame");
  await mkdir(dirname(executable), { recursive: true });
  await command("go", [
    "build",
    "-overlay",
    overlay,
    "-tags",
    "production,mcp,nativeworkbenchprobe",
    "-ldflags",
    process.argv[2],
    "-o",
    executable,
    ".",
  ]);
  const floor = process.env.MACOSX_DEPLOYMENT_TARGET;
  assert.ok(floor, "the native gate must use the shipping deployment floor");
  await writeFile(
    join(bundle, "Contents/Info.plist"),
    `<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>${identifier}</string><key>CFBundleExecutable</key><string>flame</string><key>CFBundleName</key><string>Flame native gate</string><key>CFBundlePackageType</key><string>APPL</string><key>LSMinimumSystemVersion</key><string>${floor}</string></dict></plist>`,
  );
  const entitlements = join(directory, "entitlements.plist");
  await writeFile(
    entitlements,
    `<?xml version="1.0"?><plist version="1.0"><dict><key>com.apple.security.app-sandbox</key><true/><key>com.apple.security.network.client</key><true/><key>com.apple.security.network.server</key><true/><key>com.apple.security.temporary-exception.files.absolute-path.read-only</key><array><string>${fixture}/</string></array></dict></plist>`,
  );
  await command("codesign", ["--force", "--sign", "-", "--entitlements", entitlements, bundle]);
  await command("codesign", ["--verify", "--strict", bundle]);
  console.log("Running the signed production workbench");
  native = spawn(executable, [], {
    cwd: desktop,
    env: {
      ...process.env,
      FLAME_NATIVE_WORKBENCH_ID: identifier,
      FLAME_NATIVE_WORKBENCH_FIXTURE: join(fixture, "allowed"),
      FLAME_NATIVE_WORKBENCH_CANARY: canary,
      WAILS_MCP_HOST: "127.0.0.1",
      WAILS_MCP_PORT: "0",
      WAILS_MCP_TOKEN: mcpToken,
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  const exited = new Promise((resolve) =>
    native.once("exit", (code, signal) => {
      nativeExit = { code, signal };
      resolve(nativeExit);
    }),
  );
  native.stdout.on("data", (body) => {
    nativeOutput += body;
  });
  native.stderr.on("data", (body) => {
    nativeOutput += body;
  });
  mcpEndpoint = await waitFor(
    "native MCP startup",
    async () => nativeOutput.match(/http:\/\/127\.0\.0\.1:\d+\/mcp/)?.[0],
    Boolean,
  );
  await tool("window_control", { action: "focus" });
  await waitFor(
    "production sessions",
    () => evaluate(`return document.body.innerText.includes(${JSON.stringify(title)})`),
    Boolean,
  );
  await clickText(title);
  await clickLabel("Open right workspace");

  const trajectory = await openPanel(installed.trajectory.title, run.runId);
  await guest(
    trajectory.id,
    "document.querySelectorAll('details').forEach(e => e.open = true); true",
  );
  await pageWith("Native fixture observation");
  const refreshedRun = await rpc("runs.start", {
    sessionId: session.id,
    input: [{ type: "text", text: "Refresh trajectory observation." }],
  });
  assert.equal((await rpc("runs.get", { runId: refreshedRun.runId })).outcome.type, "completed");
  await refreshPage(
    trajectory.id,
    "plugins.readTrajectory",
    installed.trajectory,
    { sessionId: session.id },
    refreshedRun.runId,
  );
  await guest(
    trajectory.id,
    "document.querySelectorAll('details').forEach(e => e.open = true); true",
  );
  await pageWith("Refresh trajectory observation.");
  await closePanel(installed.trajectory.title, trajectory.id);
  console.log("Trajectory: recorded Run and model observation, refresh, retirement passed");

  const memory = await openPanel(installed.memory.title, memoryFact);
  const refreshedMemoryFact = "Refreshed native memory fact";
  await rpc("agentMemory.add", {
    scope: "project",
    workspace: { path: workspace },
    content: refreshedMemoryFact,
  });
  await refreshPage(
    memory.id,
    "plugins.readMemory",
    installed.memory,
    { scope: "project", workspace: { path: workspace } },
    refreshedMemoryFact,
  );
  await clickText("User");
  const user = await pageWith(userFact);
  assert.notEqual(user.id, memory.id, "scope replacement must retire the captured project query");
  await assert.rejects(guest(memory.id, "document.body.innerText"), /page is not ready/);
  await closePanel(installed.memory.title, user.id);
  console.log("Memory: remote project and user scope, refresh, retirement passed");

  const schedules = await openPanel(installed.schedules.title, scheduleTitle);
  const refreshedScheduleTitle = "Refreshed native saved schedule";
  await rpc("schedules.update", {
    id: schedule.id,
    expectedRevision: disabledSchedule.revision,
    title: refreshedScheduleTitle,
  });
  await refreshPage(
    schedules.id,
    "plugins.readSchedules",
    installed.schedules,
    {},
    refreshedScheduleTitle,
  );
  await closePanel(installed.schedules.title, schedules.id);
  console.log("Schedules: Runtime-owned saved schedule, refresh, retirement passed");

  const report = await openPanel(installed.usage.title, "Recorded usage loaded.");
  usage = await rpc("usage.summary");
  assert.equal(usage.runs, 2);
  assert.ok(report.content.includes("Not reported"));
  assert.equal(
    await guest(
      report.id,
      "[...document.querySelectorAll('#totals > div')].find(e => e.querySelector('dt').textContent === 'Finished Runs').querySelector('dd').textContent",
    ),
    "2",
  );
  await refreshPage(report.id, "plugins.readUsage", installed.usage, {}, "Recorded usage loaded.");
  await clickText("7d");
  const period = await pageWith("Recorded usage loaded.");
  assert.notEqual(period.id, report.id, "period replacement must retire the captured report");
  await assert.rejects(guest(report.id, "document.body.innerText"), /page is not ready/);
  await rpc("plugins.revoke", { installationId: installed.usage.installationId });
  await noPages();
  await assert.rejects(guest(period.id, "document.body.innerText"), /page is not ready/);
  await rpc("plugins.approve", {
    installationId: installed.usage.installationId,
    digest: installed.usage.digest,
  });
  await rpc("plugins.setEnablement", {
    installationId: installed.usage.installationId,
    enabled: true,
  });
  const successor = await openPanel(installed.usage.title, "Recorded usage loaded.");
  assert.notEqual(successor.id, period.id, "reapproval must create a successor page");
  await closePanel(installed.usage.title, successor.id);
  console.log(
    "Usage: recorded zero/unpriced usage, period replacement, withdrawal and reapproval passed",
  );

  const diffPage = await openPanel(installed.diff.title, "Workspace diff loaded.");
  assert.match(
    await guest(diffPage.id, "document.getElementById('files').textContent"),
    /changed native diff/,
  );
  assert.match(
    await guest(diffPage.id, "document.getElementById('files').textContent"),
    /Binary content is not displayed/,
  );
  await refreshPage(
    diffPage.id,
    "plugins.readDiff",
    installed.diff,
    { workspace: { path: workspace }, mode: "worktree", format: "rows" },
    "Workspace diff loaded.",
  );
  await clickText("Branch", "[role=tab]");
  await waitFor("captured diff retirement", pages, (value) =>
    value.pages.every((page) => page.id !== diffPage.id),
  );
  const branchDiff = await pageWith("Workspace diff loaded.");
  assert.notEqual(branchDiff.id, diffPage.id);
  await assert.rejects(guest(diffPage.id, "document.body.innerText"), /page is not ready/);
  await closePanel(installed.diff.title, branchDiff.id);
  console.log(
    "Diff: canonical remote workspace, binary evidence, refresh and trusted baseline replacement passed",
  );
  const skillPage = await openPanel(installed.skills.title, "native-user");
  await refreshPage(
    skillPage.id,
    "plugins.readSkills",
    installed.skills,
    { workspace: { path: workspace } },
    "native-user",
  );
  await clickLabel("Read skill");
  await clickText("native-user · user", '[role="menuitem"]');
  const documentPage = await pageWith("Skill document");
  assert.notEqual(
    documentPage.id,
    skillPage.id,
    "document selection must retire the captured catalog",
  );
  await refreshPage(
    documentPage.id,
    "plugins.readSkill",
    installed.skills,
    { workspace: { path: workspace }, name: "native-user" },
    "Skill document",
  );
  await assert.rejects(guest(skillPage.id, "document.body.innerText"), /page is not ready/);
  await clickText("Personal", '[role="tab"]');
  const libraryPage = await pageWith("native-user");
  await refreshPage(
    libraryPage.id,
    "plugins.readSkillLibrary",
    installed.skills,
    {},
    "native-user",
  );
  await clickLabel("Select a personal Skill");
  await clickText("native-user · Active", '[role="menuitem"]');
  await clickText("Archive");
  const archivedPage = await pageWith("archived");
  assert.notEqual(
    archivedPage.id,
    libraryPage.id,
    "a trusted receipt must create a fresh projection",
  );
  assert.equal(
    (await rpc("skills.library.list")).data.find((skill) => skill.name === "native-user").lifecycle,
    "archived",
  );
  await clickText("Restore");
  await pageWith("active");
  await rpc("approval.setMode", { mode: "yolo" });
  const proposalSession = await rpc("sessions.create", {
    title: "Native proposal authoring",
    workspace: { path: workspace },
  });
  const proposalRun = await rpc("runs.start", {
    sessionId: proposalSession.id,
    input: [{ type: "text", text: "Save a native reusable Skill." }],
  });
  const proposalResult = await rpc("runs.get", { runId: proposalRun.runId });
  assert.equal(proposalResult.outcome.type, "completed", JSON.stringify(proposalResult));
  const proposed = (await rpc("skills.proposals.list", { workspace: { path: workspace } })).data;
  assert.equal(proposed.length, 1);
  await clickText("Review", '[role="tab"]');
  const proposalPage = await pageWith("native-proposal");
  await refreshPage(
    proposalPage.id,
    "plugins.readSkillProposals",
    installed.skills,
    { workspace: { path: workspace } },
    "native-proposal",
  );
  await clickLabel("Select a proposal");
  await clickText(
    `native-proposal · Personal · ${proposed[0].revision.slice(0, 8)}`,
    '[role="menuitem"]',
  );
  await clickText("Approve");
  await waitFor(
    "trusted proposal review",
    () => evaluate("return {text: document.querySelector('[role=dialog]')?.textContent}"),
    (value) =>
      value.text?.includes(proposed[0].instructions) &&
      value.text?.includes(proposed[0].description),
  );
  await clickText("Approve", '[role="dialog"] button');
  await pageWith("No proposals pending review.");
  assert.deepEqual(
    (await rpc("skills.proposals.list", { workspace: { path: workspace } })).data,
    [],
  );
  const preservedSkills = await rpc("skills.library.list");
  assert.ok(
    preservedSkills.data.some(
      (skill) => skill.name === "native-proposal" && skill.lifecycle === "active",
    ),
  );
  const finalSkillPage = await pageWith("No proposals pending review.");
  await closePanel(installed.skills.title, finalSkillPage.id);
  usage = await rpc("usage.summary");
  const preservedDiff = await rpc("workspace.diff.get", {
    workspace: { path: workspace },
    format: "rows",
  });
  assert.equal(usage.runs, 3);
  console.log(
    "Skills: canonical catalogs/documents, trusted archive/restore, proposal publication and page retirement passed",
  );

  await clickText("Native proposal authoring");
  await evaluate(
    `for(const e of document.querySelectorAll('[data-slot="agent-activity-disclosure"]:not([data-tool]) > [data-slot="agent-activity-header"] button[aria-expanded="false"]'))e.click(); return true`,
  );
  await waitFor(
    "recorded proposal tool",
    () => evaluate('return !!document.querySelector("[data-tool=propose_skill]")'),
    Boolean,
  );
  await evaluate(
    'const root = document.querySelector("[data-tool=propose_skill]"); const e = root.matches("button") ? root : root.querySelector("[aria-expanded]"); root.scrollIntoView({block: "center"}); if(e.getAttribute("aria-expanded") !== "true")e.click(); return true',
  );
  const resultPage = await pageWith("propose_skill");
  const originalResult = await guest(resultPage.id, 'document.querySelector("#output").innerText');
  assert.match(originalResult, /native-proposal/);
  await guest(resultPage.id, 'document.querySelector("#refresh").click(); true');
  await waitFor(
    "same recorded result",
    () => readGuest(resultPage.id, 'document.querySelector("#output").innerText'),
    (value) => value.ready && JSON.parse(value.value) === originalResult,
  );
  const canonicalResult = await rpc("sessions.trajectory", { sessionId: proposalSession.id });
  await rpc("plugins.revoke", { installationId: resultPackage.id });
  await noPages();
  await assert.rejects(guest(resultPage.id, "document.body.innerText"), /page is not ready/);
  await waitFor(
    "generic recorded-result fallback",
    () =>
      evaluate(
        'return !!document.querySelector("[data-tool=propose_skill]") && document.body.innerText.includes("native-proposal")',
      ),
    Boolean,
  );
  assert.deepEqual(
    await rpc("sessions.trajectory", { sessionId: proposalSession.id }),
    canonicalResult,
  );
  await rpc("plugins.uninstall", { installationId: resultPackage.id });
  console.log(
    "Tool results: recorded Run output, renewed read admission, native retirement and generic fallback passed",
  );

  await choosePanel("Files");
  await clickText("native-file.md", '[role="treeitem"]');
  const filePage = await pageWith("Native Markdown fixture");
  const fileRequest = { workspace: { path: workspace }, path: "native-file.md" };
  const canonicalFile = await rpc("workspace.files.read", fileRequest);
  const originalFile = await guest(filePage.id, 'document.querySelector("#output").innerText');
  assert.match(originalFile, /Recorded file content/);
  await guest(filePage.id, 'document.querySelector("#refresh").click(); true');
  await waitFor(
    "same captured file",
    () => readGuest(filePage.id, 'document.querySelector("#output").innerText'),
    (value) => value.ready && JSON.parse(value.value) === originalFile,
  );
  await rpc("plugins.revoke", { installationId: fileRendererPackage.id });
  await noPages();
  await assert.rejects(guest(filePage.id, "document.body.innerText"), /page is not ready/);
  await waitFor(
    "generic file source",
    () => evaluate('return document.body.innerText.includes("Recorded **file** content.")'),
    Boolean,
  );
  assert.deepEqual(await rpc("workspace.files.read", fileRequest), canonicalFile);
  await rpc("plugins.uninstall", { installationId: fileRendererPackage.id });
  await closePanel("Files", filePage.id);
  console.log(
    "File renderers: captured remote file, renewed admission, native retirement and preserved source passed",
  );

  const diagnosticPage = await openPanel(installed.diagnostics.title, "Client diagnostics");
  const canonicalUsage = await rpc("usage.summary");
  await guest(diagnosticPage.id, 'document.querySelector("#refresh").click(); true');
  await waitFor(
    "local observation read",
    () => readGuest(diagnosticPage.id, 'document.querySelector("#refresh").disabled'),
    (value) => value.ready && JSON.parse(value.value) === false,
  );
  await clickText("Clear", "button");
  await waitFor(
    "cleared diagnostics page retirement",
    pages,
    (state) => !state.pages.some((page) => page.id === diagnosticPage.id),
  );
  const clearedPage = await pageWith("Client diagnostics");
  assert.notEqual(clearedPage.id, diagnosticPage.id);
  await assert.rejects(guest(diagnosticPage.id, "document.body.innerText"), /page is not ready/);
  assert.deepEqual(await rpc("usage.summary"), canonicalUsage);
  await rpc("plugins.revoke", { installationId: installed.diagnostics.installationId });
  await noPages();
  await closePanel(installed.diagnostics.title, clearedPage.id);
  console.log(
    "Client diagnostics: renewed read admission, trusted clear, retired page and unchanged Runtime usage passed",
  );

  const galleryPage = await openPanel(installed["icon-gallery"].title, "Flame icon gallery");
  await guest(
    galleryPage.id,
    'const search = document.querySelector("#filter"); search.value = "file-diff"; search.dispatchEvent(new Event("input")); true',
  );
  assert.match(
    await guest(galleryPage.id, 'document.querySelector("#icons").innerText'),
    /FileDiff/,
  );
  await rpc("plugins.revoke", { installationId: installed["icon-gallery"].installationId });
  await noPages();
  await assert.rejects(guest(galleryPage.id, "document.body.innerText"), /page is not ready/);
  await closePanel(installed["icon-gallery"].title, galleryPage.id);
  console.log("Icon gallery: static released SVGs, local search and native retirement passed");

  for (const name of Object.keys(installed))
    await rpc("plugins.uninstall", { installationId: installed[name].installationId });
  const preservedMemory = (
    await rpc("agentMemory.list", { scope: "project", workspace: { path: workspace } })
  ).data.map((item) => item.content);
  assert.deepEqual(new Set(preservedMemory), new Set([memoryFact, refreshedMemoryFact]));
  assert.equal((await rpc("schedules.list")).data[0].title, refreshedScheduleTitle);
  assert.deepEqual(await rpc("usage.summary"), usage);
  assert.deepEqual(
    await rpc("workspace.diff.get", { workspace: { path: workspace }, format: "rows" }),
    preservedDiff,
  );
  assert.deepEqual(await rpc("skills.library.list"), preservedSkills);
  assert.equal(
    (await rpc("sessions.trajectory", { sessionId: session.id })).data.some(
      (entry) => entry.run?.id === run.runId,
    ),
    true,
  );
  await noPages();
  await tool("window_control", { action: "close" });
  let shutdownDeadline;
  try {
    await Promise.race([
      exited,
      new Promise((_, reject) => {
        shutdownDeadline = setTimeout(() => reject(new Error("native shutdown timed out")), 10000);
      }),
    ]);
  } finally {
    clearTimeout(shutdownDeadline);
  }
  assert.deepEqual(nativeExit, { code: 0, signal: null });
} catch (error) {
  failure = error;
  if (mcpEndpoint && !nativeExit) {
    const state = await evaluate(
      "return {text: document.body.innerText, controls: [...document.querySelectorAll('button, [role=option]')].map(e => ({text:e.textContent.trim(), label:e.getAttribute('aria-label')}))}",
    ).catch((failure) => ({ error: failure.message }));
    console.error(JSON.stringify(state));
  }
  console.error(nativeOutput);
  const logs = await command("docker", ["logs", containerName]).catch(() => null);
  if (logs) console.error(logs.stdout, logs.stderr);
} finally {
  if (native && !nativeExit) {
    native.kill("SIGKILL");
    await new Promise((resolve) => native.once("exit", resolve));
  }
  const cleanup = [];
  try {
    await command("docker", ["rm", "--force", containerName]);
  } catch (error) {
    if (!error.stderr.includes("No such container")) cleanup.push(error);
  }
  try {
    await command("docker", ["network", "rm", containerName]);
  } catch (error) {
    if (!error.stderr.includes("not found")) cleanup.push(error);
  }
  const removed = await Promise.allSettled([
    rm(join(sandbox, "Data"), { recursive: true, force: true }),
    rm(canary, { force: true }),
    rm(directory, { recursive: true, force: true }),
  ]);
  for (const result of removed) if (result.status === "rejected") cleanup.push(result.reason);
  if (cleanup.length)
    failure = new AggregateError(
      failure ? [failure, ...cleanup] : cleanup,
      "native workbench gate cleanup failed",
    );
}
if (failure) throw failure;
console.log(
  JSON.stringify({
    result: "passed",
    views: Object.keys(installed).length,
    resultRendering: true,
    fileRendering: true,
    remoteWorkspace: true,
    preservedRuntimeFacts: true,
    nativePages: 0,
  }),
);
