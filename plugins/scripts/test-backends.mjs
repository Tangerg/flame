import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { stageNode } from "../../tooling/node-companion.mjs";

const node = await stageNode({
  os: process.platform,
  arch: process.arch === "x64" ? "amd64" : process.arch,
  output: fileURLToPath(new URL("../../out/toolchains", import.meta.url)),
  offline: true,
});
execFileSync(
  node,
  [
    fileURLToPath(
      new URL("../../node_modules/vitest/vitest.mjs", import.meta.url),
    ),
    "run",
    "usagereports/backend",
  ],
  {
    cwd: fileURLToPath(new URL("../", import.meta.url)),
    stdio: "inherit",
    env: process.env,
  },
);
