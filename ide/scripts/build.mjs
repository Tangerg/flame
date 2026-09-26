import { build } from "rolldown";
await build({
  input: "src/extension.ts",
  platform: "node",
  external: ["vscode"],
  output: { file: "dist/extension.cjs", format: "cjs", sourcemap: true },
});
