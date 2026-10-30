import { appendFileSync, existsSync } from "node:fs";
import { spawn } from "node:child_process";
import { connect } from "node:net";
import * as nativeAdapter from "../../package/io.github.tangerg.flame/engine/native-sky.mjs";
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ElicitResultSchema,
  ListToolsRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";

const mode = process.env.COMPUTER_FIXTURE_MODE;
const nativeHooks = [];
let native;
if (process.env.COMPUTER_NATIVE_ENABLED === "1") {
  globalThis.nodeRepl = {
    env: process.env,
    requestMeta: {},
    nativePipe: {
      createConnection(path) {
        return new Promise((resolve, reject) => {
          const socket = connect(path, () => resolve(socket));
          socket.once("error", reject);
        });
      },
    },
    addTurnEndedHandler(handler) {
      nativeHooks.push(handler);
    },
  };
  native = nativeAdapter;
}
const log = (value) => {
  if (process.env.COMPUTER_FIXTURE_LOG)
    appendFileSync(
      process.env.COMPUTER_FIXTURE_LOG,
      JSON.stringify(value) + "\n",
    );
};
const server = new Server(
  { name: "computer-engine-fixture", version: "1" },
  {
    instructions: mode?.includes("missing-instructions")
      ? undefined
      : "Use the original fixture instructions. Preserve all media.",
    capabilities: { tools: { listChanged: true } },
  },
);
const tools = [
  {
    name: "js",
    description: "Original JavaScript operation description.",
    inputSchema: {
      type: "object",
      properties: { code: { type: "string" }, timeout_ms: { type: "integer" } },
      required: ["code"],
      additionalProperties: false,
    },
    ...(mode === "output-schema"
      ? {
          outputSchema: {
            type: "object",
            properties: { value: { type: "integer" } },
            required: ["value"],
            additionalProperties: false,
          },
        }
      : {}),
    ...(mode === "required-task"
      ? { execution: { taskSupport: "required" } }
      : {}),
  },
  {
    name: "js_reset",
    description: "Original reset description.",
    inputSchema: { type: "object", additionalProperties: false },
  },
  {
    name: "turn_ended",
    description: "Host-only turn cleanup.",
    inputSchema: { type: "object", additionalProperties: true },
  },
  {
    name: "js_add_node_module_dir",
    inputSchema: { type: "object", additionalProperties: true },
  },
];
const workers = [];
let value = 0;
let rejectCleanup = false;
let hangCleanup = false;
let delayedCleanup = false;
server.setRequestHandler(ListToolsRequestSchema, () => ({
  tools:
    mode === "missing-reset"
      ? tools.filter((tool) => tool.name !== "js_reset")
      : tools,
}));
server.setRequestHandler(CallToolRequestSchema, async (request, extra) => {
  if (native) globalThis.nodeRepl.requestMeta = request.params._meta;
  log({
    kind: "call",
    name: request.params.name,
    args: request.params.arguments,
    meta: request.params._meta,
  });
  if (request.params.name === "turn_ended") {
    for (const hook of nativeHooks) await hook.run(request.params.arguments);
    if (hangCleanup) return await new Promise(() => {});
    if (delayedCleanup) {
      log({ kind: "cleanup-entered" });
      while (!existsSync(process.env.COMPUTER_FIXTURE_CLEANUP_RELEASE))
        await new Promise((resolve) => setTimeout(resolve, 10));
      log({ kind: "cleanup-released" });
    }
    return { content: [], ...(rejectCleanup ? { isError: true } : {}) };
  }
  if (request.params.name === "js_reset") {
    value = 0;
    return { content: [{ type: "text", text: "reset" }] };
  }
  const code = request.params.arguments.code;
  if (native && code.startsWith("native")) {
    const result = await native.handleRpc({
      type: "execute",
      method: "fixture",
      args: [code],
    });
    if (code === "native-late")
      setTimeout(() => {
        void native
          .handleRpc({ type: "execute", method: "fixture", args: ["late"] })
          .then(
            () => log({ kind: "late-native-accepted" }),
            () => log({ kind: "late-native-refused" }),
          );
      }, 100);
    return { content: [{ type: "text", text: result }] };
  }
  if (code === "increment")
    return { content: [{ type: "text", text: String(++value) }] };
  if (code === "exact-result") {
    process.stdout.write(
      `{"jsonrpc":"2.0","id":${JSON.stringify(extra.requestId)},"result":{"content":[{"type":"text","text":"exact","_meta":{"wide":9007199254740993},"extension":{"wide":9007199254740993}}],"structuredContent":{"wide":9007199254740993,"decimal":1.0000000000000001,"exponent":1e1000},"_meta":{"wide":9007199254740993,"negativeZero":-0}}}\n`,
    );
    return await new Promise(() => {});
  }
  if (code === "structured-output")
    return { content: [], structuredContent: { value: 12 } };
  if (code === "invalid-structured-output")
    return { content: [], structuredContent: { value: "invalid" } };
  if (code === "media")
    return {
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
    };
  if (code === "known-error")
    return {
      content: [{ type: "text", text: "known original refusal" }],
      isError: true,
    };
  if (code === "cleanup-fails") {
    rejectCleanup = true;
    return { content: [{ type: "text", text: "effect confirmed" }] };
  }
  if (code === "cleanup-hangs") {
    hangCleanup = true;
    return { content: [{ type: "text", text: "effect confirmed" }] };
  }
  if (code === "cleanup-delay") {
    delayedCleanup = true;
    return { content: [{ type: "text", text: "effect confirmed" }] };
  }
  if (code === "approval") {
    const response = await server.elicitInput(
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
      { signal: extra.signal, timeout: 2 ** 31 - 1 },
    );
    return { content: [{ type: "text", text: JSON.stringify(response) }] };
  }
  if (code === "restricted-approval") {
    const response = await server.request(
      {
        method: "elicitation/create",
        params: {
          message:
            "This original request includes a constraint outside the supported form vocabulary.",
          requestedSchema: {
            type: "object",
            properties: { reason: { type: "string", pattern: "^required$" } },
            required: ["reason"],
          },
          _meta: { fixture: "original restricted request" },
        },
      },
      ElicitResultSchema,
      { signal: extra.signal },
    );
    return { content: [{ type: "text", text: JSON.stringify(response) }] };
  }
  if (code === "hold") {
    await new Promise((resolve) => {
      if (extra.signal.aborted) resolve();
      else extra.signal.addEventListener("abort", resolve, { once: true });
    });
    log({ kind: "cancelled" });
    return { content: [] };
  }
  if (code === "crash-after-effect") {
    log({ kind: "effect" });
    process.exit(23);
  }
  if (code === "invalid-result")
    return { content: [{ type: "image", data: "missing mime type" }] };
  if (code === "oversized-result") {
    process.stdout.write("x".repeat(16 * 1024 * 1024 + 1));
    return await new Promise(() => {});
  }
  if (code === "large-valid-result")
    return {
      content: [{ type: "text", text: "x".repeat(16 * 1024 * 1024 - 512) }],
    };
  if (code === "drift") {
    await server.notification({ method: "notifications/tools/list_changed" });
    return await new Promise(() => {});
  }
  if (code === "environment")
    return {
      content: [
        {
          type: "text",
          text: JSON.stringify({
            display: process.env.DISPLAY,
            configured: process.env.COMPUTER_CONFIGURED,
            ambient: process.env.COMPUTER_AMBIENT_SECRET,
          }),
        },
      ],
    };
  if (code === "orphan-worker") {
    const worker = spawn(
      process.execPath,
      [
        "-e",
        `const {appendFileSync} = require('node:fs');
process.on('SIGTERM', () => setTimeout(() => {
  appendFileSync(process.env.COMPUTER_FIXTURE_LOG, JSON.stringify({kind:'orphan-ended',pid:process.pid})+'\\n');
  process.exit(0);
}, 50));
process.stdout.write('ready\\n');
setInterval(() => {}, 1000);`,
      ],
      {
        stdio: ["ignore", "pipe", "ignore"],
        env: { COMPUTER_FIXTURE_LOG: process.env.COMPUTER_FIXTURE_LOG },
      },
    );
    await new Promise((resolve, reject) => {
      worker.stdout.once("data", resolve);
      worker.once("error", reject);
    });
    worker.stdout.destroy();
    log({ kind: "orphan-worker", pid: worker.pid });
    return { content: [{ type: "text", text: String(worker.pid) }] };
  }
  if (code === "worker") {
    const worker = spawn(
      process.execPath,
      ["-e", "process.on('SIGTERM', () => {}); setInterval(() => {}, 1000)"],
      { stdio: "inherit" },
    );
    workers.push(worker);
    log({ kind: "worker", pid: worker.pid });
    return { content: [{ type: "text", text: String(worker.pid) }] };
  }
  return { content: [{ type: "text", text: code }] };
});

if (mode?.includes("stderr"))
  process.stderr.write(
    "permission denied: private-fixture-secret\n" + "x".repeat(8192),
  );
log({ kind: "start", pid: process.pid });
const transport = new StdioServerTransport();
async function finish() {
  for (const worker of workers) {
    const exited = new Promise((resolve) => worker.once("exit", resolve));
    worker.kill("SIGKILL");
    await exited;
  }
  process.exit(0);
}
if (mode === "ignore-eof") {
  // The stdio transport does not close the server on EOF; keep the orphan fixture alive.
  setInterval(() => {}, 1_000);
} else {
  server.onclose = () => {
    void finish();
  };
  process.on("SIGTERM", () => {
    void finish();
  });
}
await server.connect(transport);
