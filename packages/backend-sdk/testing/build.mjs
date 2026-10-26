import { rolldown } from "rolldown";
import { fileURLToPath } from "node:url";
import { buildBackend } from "../build.mjs";

await buildBackend({
  input:
    process.argv[3] ?? fileURLToPath(new URL("./backend.ts", import.meta.url)),
  directory: process.argv[2],
  compiler: rolldown,
});
