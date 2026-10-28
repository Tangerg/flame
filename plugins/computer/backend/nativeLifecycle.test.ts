import { chmod, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { execFile } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { afterEach, expect, it } from "vitest";
import { Engine } from "./engine";

const cleanups: Array<() => Promise<unknown>> = [];
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup();
});

const fixture = fileURLToPath(
  new URL("./fixtures/engine.mjs", import.meta.url),
);
const wrapper = fileURLToPath(
  new URL(
    "../package/io.github.tangerg.flame/engine/native-sky.mjs",
    import.meta.url,
  ),
);
const turn = {
  sessionId: "owned-session",
  turnId: "owned-turn",
  callId: "owned-call",
  signal: new AbortController().signal,
};

it.each([
  ["import", "late native dispatch refused"],
  ["pending", "native dispatch joined before cleanup"],
])("enforces the native turn fence during %s", async (mode, expected) => {
  const { stdout } = await promisify(execFile)(
    process.execPath,
    [
      fileURLToPath(new URL("./fixtures/native-boundary.mjs", import.meta.url)),
      mode,
    ],
    { timeout: 5000 },
  );
  expect(stdout).toBe(expected + "\n");
});

async function openNative(mode = "success") {
  const directory = await mkdtemp(join(tmpdir(), "flame-computer-native-"));
  cleanups.push(() => rm(directory, { recursive: true, force: true }));
  const log = join(directory, "native.jsonl");
  const gate = join(directory, "continue");
  const service = join(directory, "service.mjs");
  const client = join(directory, "client.mjs");
  const helper = join(directory, "helper");
  const ownershipPath = join(directory, "ownership.json");
  await writeFile(log, "");
  await writeFile(
    service,
    `import {appendFileSync} from 'node:fs';
export async function handleRpc(request) {
  appendFileSync(process.env.NATIVE_TEST_LOG, JSON.stringify({kind:'effect',request})+'\\n');
  if (request.args[0] === 'native-crash') process.exit(23);
  return 'original native result';
}`,
  );
  await writeFile(
    client,
    `import {appendFileSync,existsSync} from 'node:fs';
export class MacComputerUseClient {
  async request(method, request, options) {
    appendFileSync(process.env.NATIVE_TEST_LOG,JSON.stringify({kind:'native-end',method,request,options})+'\\n');
    if (process.env.NATIVE_TEST_MODE === 'failed') throw new Error('original native refusal');
    if (process.env.NATIVE_TEST_MODE === 'delayed') while (!existsSync(process.env.NATIVE_TEST_GATE)) await new Promise(resolve=>setTimeout(resolve,5));
    appendFileSync(process.env.NATIVE_TEST_LOG,JSON.stringify({kind:'native-confirmed'})+'\\n');
    return {};
  }
}`,
  );
  await writeFile(
    helper,
    `#!${process.execPath}
const {appendFileSync} = require('node:fs');
appendFileSync(process.env.NATIVE_TEST_LOG,JSON.stringify({kind:'helper',args:process.argv.slice(2)})+'\\n');
process.exit(process.env.NATIVE_TEST_MODE === 'helper-failed' ? 2 : 0);
`,
  );
  await chmod(helper, 0o700);
  const config = {
    command: process.execPath,
    args: [fixture],
    ownershipPath,
    env: {
      COMPUTER_NATIVE_ENABLED: "1",
      COMPUTER_FIXTURE_LOG: log,
      NODE_REPL_TRUSTED_SERVICES: JSON.stringify({ sky: wrapper }),
      NATIVE_TEST_LOG: log,
      NATIVE_TEST_MODE: mode,
      NATIVE_TEST_GATE: gate,
    },
    native: { service, client, helper, wrapper },
  };
  const hooks = {
    signal: new AbortController().signal,
    onElicitation: async () => ({ action: "cancel" as const }),
    onRetired() {},
  };
  const engine = await Engine.connect(config, hooks);
  cleanups.push(() => engine.close().catch(() => {}));
  return {
    engine,
    config,
    hooks,
    gate,
    ownershipPath,
    async records() {
      const source = await readFile(log, "utf8");
      return source
        .split("\n")
        .filter(Boolean)
        .map((line) => JSON.parse(line) as Record<string, unknown>);
    },
  };
}

it("waits for the trusted worker's original native acknowledgement before completing the action", async () => {
  const { engine, records, gate, ownershipPath } = await openNative("delayed");
  let settled = false;
  const executing = engine.call("js", { code: "native" }, turn).finally(() => {
    settled = true;
  });
  await expect
    .poll(async () =>
      (await records()).some((entry) => entry.kind === "native-end"),
    )
    .toBe(true);
  expect(settled).toBe(false);
  expect(JSON.parse(await readFile(ownershipPath, "utf8")).nativeTurn).toEqual({
    sessionId: turn.sessionId,
    turnId: turn.turnId,
    callId: turn.callId,
  });
  await writeFile(gate, "continue");
  expect(await executing).toEqual({
    result: { content: [{ type: "text", text: "original native result" }] },
    cleanup: "ended",
  });
  const entries = await records();
  const nativeEntries = entries.filter(
    (entry) => !["call", "start"].includes(String(entry.kind)),
  );
  expect(nativeEntries.map((entry) => entry.kind)).toEqual([
    "effect",
    "native-end",
    "native-confirmed",
    "helper",
  ]);
  expect(nativeEntries[1]).toEqual({
    kind: "native-end",
    method: "ComputerUseIPCCodexTurnEndedRequest",
    request: { threadID: turn.sessionId, turnID: turn.turnId },
    options: {
      codexMetadata: {
        session_id: turn.sessionId,
        turn_id: turn.turnId,
        call_id: turn.callId,
      },
      timeoutSeconds: 15,
    },
  });
  expect(nativeEntries[3]?.args).toEqual([
    "turn-ended",
    JSON.stringify({
      type: "agent-turn-complete",
      "thread-id": turn.sessionId,
      "turn-id": turn.turnId,
    }),
  ]);
  expect(
    JSON.parse(await readFile(ownershipPath, "utf8")).nativeTurn,
  ).toBeNull();
  await engine.close();
  await expect(readFile(ownershipPath)).rejects.toMatchObject({
    code: "ENOENT",
  });
});

it("ends each native turn while retaining the original persistent worker", async () => {
  const { engine, records } = await openNative();
  expect((await engine.call("js", { code: "native" }, turn)).cleanup).toBe(
    "ended",
  );
  expect(
    (await engine.call("js_reset", {}, { ...turn, turnId: "reset-turn" }))
      .cleanup,
  ).toBe("ended");
  expect(
    (
      await engine.call(
        "js",
        { code: "native" },
        { ...turn, turnId: "next-turn" },
      )
    ).cleanup,
  ).toBe("ended");
  expect(
    (await records()).filter((entry) => entry.kind === "helper"),
  ).toHaveLength(2);
});

it("refuses native launch when the selected trusted service omits its lifecycle owner", async () => {
  const { engine, config, hooks, records } = await openNative();
  await engine.close();
  await expect(
    Engine.connect(
      {
        ...config,
        env: {
          ...config.env,
          NODE_REPL_TRUSTED_SERVICES: JSON.stringify({
            sky: "@oai/sky/service",
          }),
        },
      },
      hooks,
    ),
  ).rejects.toMatchObject({ cleanup: "closed" });
  expect(
    (await records()).filter((entry) => entry.kind === "effect"),
  ).toHaveLength(0);
});

it("keeps a known result but retains native ownership when only the helper reports success", async () => {
  const { engine, records, ownershipPath, config, hooks } =
    await openNative("failed");
  expect(await engine.call("js", { code: "native" }, turn)).toEqual({
    result: { content: [{ type: "text", text: "original native result" }] },
    cleanup: "unconfirmed",
  });
  await expect(engine.close()).rejects.toThrow("cleanup is unconfirmed");
  expect(
    (await records())
      .filter((entry) => !["call", "start"].includes(String(entry.kind)))
      .map((entry) => entry.kind),
  ).toEqual(["effect", "native-end", "helper"]);
  expect(
    JSON.parse(await readFile(ownershipPath, "utf8")).nativeTurn,
  ).not.toBeNull();
  await expect(Engine.connect(config, hooks)).rejects.toMatchObject({
    cleanup: "unconfirmed",
  });
});

it("refuses native work that outlives the original control turn", async () => {
  const { engine, records } = await openNative();
  expect((await engine.call("js", { code: "native-late" }, turn)).cleanup).toBe(
    "ended",
  );
  await expect
    .poll(async () =>
      (await records()).some((entry) => entry.kind === "late-native-refused"),
    )
    .toBe(true);
  expect(
    (await records()).filter((entry) => entry.kind === "effect"),
  ).toHaveLength(1);
});

it("retains an unknown native action after its worker dies without replaying it", async () => {
  const { engine, records, ownershipPath, config, hooks } = await openNative();
  await expect(
    engine.call("js", { code: "native-crash" }, turn),
  ).rejects.toThrow("unconfirmed");
  await expect(engine.close()).rejects.toThrow("cleanup is unconfirmed");
  expect(
    (await records()).filter((entry) => entry.kind === "effect"),
  ).toHaveLength(1);
  expect(
    JSON.parse(await readFile(ownershipPath, "utf8")).nativeTurn,
  ).not.toBeNull();
  await expect(Engine.connect(config, hooks)).rejects.toMatchObject({
    cleanup: "unconfirmed",
  });
});

it("does not convert failed signed-helper cleanup into a successful completed lease", async () => {
  const { engine, records, ownershipPath } = await openNative("helper-failed");
  expect((await engine.call("js", { code: "native" }, turn)).cleanup).toBe(
    "unconfirmed",
  );
  await expect(engine.close()).rejects.toThrow("cleanup is unconfirmed");
  expect(
    (await records()).filter((entry) => entry.kind === "helper"),
  ).toHaveLength(1);
  expect(
    JSON.parse(await readFile(ownershipPath, "utf8")).nativeTurn,
  ).toBeNull();
});
