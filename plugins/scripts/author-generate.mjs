import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { generateContracts } from "./contracts.mjs";

const project = fileURLToPath(new URL("./", import.meta.url));
await generateContracts(project, {
  command: join(
    project,
    "tools/qimu" + (process.platform === "win32" ? ".exe" : ""),
  ),
});
