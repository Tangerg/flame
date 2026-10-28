import { writeFile } from "node:fs/promises";
import { Engine } from "../engine";

const [fixture, ownershipPath, log, ready] = process.argv.slice(2);
if (!fixture || !ownershipPath || !log || !ready)
  throw new Error("fixture arguments are required");
const engine = await Engine.connect(
  {
    command: process.execPath,
    args: [fixture],
    ownershipPath,
    env: { COMPUTER_FIXTURE_MODE: "ignore-eof", COMPUTER_FIXTURE_LOG: log },
  },
  {
    signal: new AbortController().signal,
    onElicitation: async () => ({ action: "cancel" }),
    onRetired() {},
  },
);
await writeFile(ready, "ready", "utf8");
await engine.call(
  "js",
  { code: "hold" },
  {
    sessionId: "crashing-control-session",
    turnId: "crashing-control-turn",
    callId: "crashing-call",
    signal: new AbortController().signal,
  },
);
