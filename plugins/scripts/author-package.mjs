import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";
import { buildPlugin } from "./build.mjs";

const { values } = parseArgs({
  options: { output: { type: "string" }, archives: { type: "string" } },
});
await buildPlugin({
  source: fileURLToPath(new URL("./", import.meta.url)),
  output: values.output,
  archives: values.archives,
  archiveTool: fileURLToPath(
    new URL(
      "./tools/archive" + (process.platform === "win32" ? ".exe" : ""),
      import.meta.url,
    ),
  ),
});
