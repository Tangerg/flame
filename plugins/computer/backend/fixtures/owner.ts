import { Engine } from "../engine";

const [fixture, ownershipPath, log] = process.argv.slice(2);
const notify = process.send?.bind(process);
if (!fixture || !ownershipPath || !log || !notify)
  throw new Error("fixture arguments and an IPC channel are required");
const engine = await Engine.connect(
  {
    command: process.execPath,
    args: [fixture],
    ownershipPath,
    env: { COMPUTER_FIXTURE_MODE: "ignore-eof", COMPUTER_FIXTURE_LOG: log },
  },
  {
    signal: new AbortController().signal,
    onElicitation: async (request) => {
      notify(request);
      return await new Promise<never>(() => {});
    },
    onRetired() {},
  },
);
await engine.call(
  "js",
  { code: "approval" },
  {
    sessionId: "crashing-control-session",
    turnId: "crashing-control-turn",
    callId: "crashing-call",
    signal: new AbortController().signal,
  },
);
