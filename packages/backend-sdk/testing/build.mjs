import protocol from "@flame/backend-sdk/protocol" with { type: "json" };
import { rolldown } from "rolldown";
import { fileURLToPath } from "node:url";
import { buildBackend } from "@qimu/node/build";

await buildBackend({
  target: protocol.constants.nodeTarget,
  maximumFrameBytes: protocol.constants.maximumFrameBytes,
  input:
    process.argv[3] ?? fileURLToPath(new URL("./backend.ts", import.meta.url)),
  directory: process.argv[2],
  compiler: rolldown,
});
