import { Buffer } from "node:buffer";
import { isAbsolute } from "node:path";
import { pathToFileURL } from "node:url";

let configured;
let original;
let client;
const turns = new Map();

function configuration() {
  if (configured) return configured;
  const runtime = globalThis.nodeRepl;
  if (
    typeof runtime?.nativePipe?.createConnection !== "function" ||
    typeof runtime?.addTurnEndedHandler !== "function"
  )
    throw new Error(
      "computer: native lifecycle requires the trusted CUA worker",
    );
  const value = JSON.parse(runtime.env.FLAME_COMPUTER_NATIVE_HOST);
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    Object.keys(value).sort().join(",") !== "client,service,socket,token" ||
    [value.client, value.service, value.socket].some(
      (path) =>
        typeof path !== "string" || !isAbsolute(path) || path.includes("\0"),
    ) ||
    !/^[a-f0-9]{64}$/.test(value.token)
  )
    throw new Error("computer: native lifecycle configuration is invalid");
  configured = value;
  runtime.addTurnEndedHandler({
    timeoutMs: 4000,
    run(event) {
      const key = JSON.stringify([event?.session_id, event?.turn_id]);
      const turn = turns.get(key);
      if (!turn) return;
      turn.ending = true;
      // The MCP hook acknowledges scheduling; the Flame host separately awaits native IPC proof.
      turn.cleanup ??= endNative(turn).finally(() => turns.delete(key));
      void turn.cleanup.catch(() => {});
    },
  });
  return configured;
}

function metadata() {
  let value = globalThis.nodeRepl.requestMeta?.["x-codex-turn-metadata"];
  if (ArrayBuffer.isView(value))
    value = JSON.parse(Buffer.from(value).toString("utf8"));
  else if (typeof value === "string") value = JSON.parse(value);
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    [value.session_id, value.turn_id, value.call_id].some(
      (entry) => typeof entry !== "string" || !entry || entry.length > 256,
    )
  )
    throw new Error(
      "computer: native operation requires captured control identities",
    );
  return JSON.parse(JSON.stringify(value));
}

function host(method, turn) {
  const selected = configuration();
  return new Promise((resolve, reject) => {
    let socket;
    let done = false;
    let bytes = Buffer.alloc(0);
    const timer = setTimeout(
      () => finish(new Error("computer: native lifecycle host timed out")),
      15_000,
    );
    function finish(error) {
      if (done) return;
      done = true;
      clearTimeout(timer);
      socket?.end();
      if (error) reject(error);
      else resolve();
    }
    Promise.resolve()
      .then(() =>
        globalThis.nodeRepl.nativePipe.createConnection(selected.socket),
      )
      .then((connection) => {
        if (done) {
          connection.end();
          return;
        }
        socket = connection;
        socket.on("error", (cause) =>
          finish(
            new Error("computer: native lifecycle connection failed", {
              cause,
            }),
          ),
        );
        socket.on("close", () =>
          finish(new Error("computer: native lifecycle connection ended")),
        );
        socket.on("data", (chunk) => {
          if (bytes.length + chunk.length > 1024)
            return finish(
              new Error("computer: native lifecycle response exceeds capacity"),
            );
          bytes = Buffer.concat([bytes, Buffer.from(chunk)]);
          const newline = bytes.indexOf(10);
          if (newline < 0) return;
          try {
            const result = JSON.parse(
              new TextDecoder("utf-8", { fatal: true }).decode(
                bytes.subarray(0, newline),
              ),
            );
            if (
              !result ||
              result.accepted !== true ||
              Object.keys(result).length !== 1
            )
              throw new Error("computer: native lifecycle request was refused");
            finish();
          } catch (cause) {
            finish(cause);
          }
        });
        socket.write(
          Buffer.from(
            JSON.stringify({
              token: selected.token,
              method,
              sessionId: turn.metadata.session_id,
              turnId: turn.metadata.turn_id,
              callId: turn.metadata.call_id,
            }) + "\n",
          ),
        );
      })
      .catch((cause) => finish(cause));
  });
}

async function endNative(turn) {
  try {
    await Promise.allSettled(turn.pending);
    const selected = configuration();
    if (!client) {
      const module = await import(pathToFileURL(selected.client).href);
      client = new module.MacComputerUseClient();
    }
    await client.request(
      "ComputerUseIPCCodexTurnEndedRequest",
      {
        threadID: turn.metadata.session_id,
        turnID: turn.metadata.turn_id,
      },
      { codexMetadata: turn.metadata, timeoutSeconds: 15 },
    );
    await host("ended", turn);
  } catch (cause) {
    await host("failed", turn).catch(() => {});
    throw cause;
  }
}

export async function handleRpc(request) {
  const selected = configuration();
  original ??= import(pathToFileURL(selected.service).href);
  let turn;
  if (request?.type !== "setup") {
    const current = metadata();
    const key = JSON.stringify([current.session_id, current.turn_id]);
    turn = turns.get(key);
    if (!turn) {
      turn = { metadata: current, ending: false, pending: new Set() };
      turns.set(key, turn);
    }
    if (turn.ending || turn.metadata.call_id !== current.call_id)
      throw new Error(
        "computer: native operation belongs to an ended control turn",
      );
  }
  const service = await original;
  if (turn) {
    if (turn.ending)
      throw new Error("computer: native turn ended before admission");
    await host("admit", turn);
    if (turn.ending)
      throw new Error("computer: native turn ended before dispatch");
  }
  const result = service.handleRpc(request);
  if (!turn) return result;
  const pending = Promise.resolve(result);
  turn.pending.add(pending);
  try {
    return await pending;
  } finally {
    turn.pending.delete(pending);
  }
}
