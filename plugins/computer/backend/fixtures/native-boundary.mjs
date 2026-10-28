import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import * as adapter from "../../package/io.github.tangerg.flame/engine/native-sky.mjs";
import "./native-client.mjs";

const directory = await mkdtemp(join(tmpdir(), "flame-native-import-"));
const pending = process.argv[2] === "pending";
try {
  const service = join(directory, "service.mjs");
  const client = fileURLToPath(new URL("./native-client.mjs", import.meta.url));
  await writeFile(
    service,
    pending
      ? `export async function handleRpc() {
  globalThis.nativeFixture.events.push('dispatch');
  globalThis.nativeFixture.loading.resolve();
  await globalThis.nativeFixture.loaded.promise;
  globalThis.nativeFixture.events.push('result');
  return 'original result';
}
`
      : `globalThis.nativeFixture.loading.resolve();
await globalThis.nativeFixture.loaded.promise;
export function handleRpc() { globalThis.nativeFixture.effects++; }
`,
  );
  const fixture = {
    loading: Promise.withResolvers(),
    loaded: Promise.withResolvers(),
    cleaned: Promise.withResolvers(),
    effects: 0,
    events: [],
  };
  globalThis.nativeFixture = fixture;
  let ended;
  globalThis.nodeRepl = {
    env: {
      FLAME_COMPUTER_NATIVE_HOST: JSON.stringify({
        service,
        client,
        socket: join(directory, "unused.sock"),
        token: "a".repeat(64),
      }),
    },
    requestMeta: {
      "x-codex-turn-metadata": {
        session_id: "original-session",
        turn_id: "original-turn",
        call_id: "original-call",
      },
    },
    addTurnEndedHandler(handler) {
      ended = handler;
    },
    nativePipe: {
      createConnection() {
        const socket = new EventEmitter();
        socket.write = (message) => {
          const { method } = JSON.parse(message);
          queueMicrotask(() => {
            socket.emit("data", Buffer.from('{"accepted":true}\n'));
            if (method === "ended") fixture.cleaned.resolve();
          });
        };
        socket.end = () => {};
        return socket;
      },
    },
  };
  const operation = adapter.handleRpc({ type: "execute" });
  void operation.catch(() => {});
  await fixture.loading.promise;
  await ended.run({
    session_id: "original-session",
    turn_id: "original-turn",
  });
  if (pending) {
    await new Promise(setImmediate);
    assert.deepEqual(fixture.events, ["dispatch"]);
    fixture.loaded.resolve();
    assert.equal(await operation, "original result");
    await fixture.cleaned.promise;
    assert.deepEqual(fixture.events, ["dispatch", "result", "native-end"]);
    process.stdout.write("native dispatch joined before cleanup\n");
  } else {
    await fixture.cleaned.promise;
    fixture.loaded.resolve();
    await assert.rejects(operation, /native turn ended before admission/);
    assert.equal(fixture.effects, 0);
    process.stdout.write("late native dispatch refused\n");
  }
} finally {
  await rm(directory, { recursive: true, force: true });
}
