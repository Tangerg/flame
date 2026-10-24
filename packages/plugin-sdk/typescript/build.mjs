import { isBuiltin } from "node:module";
import { dirname, isAbsolute, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { readFile } from "node:fs/promises";

const protocol = JSON.parse(
  await readFile(
    new URL("./src/protocol.generated.json", import.meta.url),
    "utf8",
  ),
);

export async function buildBackend({
  input,
  directory,
  compiler,
  entryFileName = "start.mjs",
}) {
  if (typeof compiler !== "function")
    throw new TypeError("backend compiler is required");
  if (!/^[a-zA-Z0-9._-]+\.mjs$/.test(entryFileName))
    throw new TypeError("backend entry file must be an mjs filename");
  const entry = resolve(input);
  const bootstrap = "\0flame:backend-bootstrap";
  const sdk = fileURLToPath(new URL("./src/node.ts", import.meta.url));
  const bundle = await compiler({
    input: bootstrap,
    platform: "node",
    resolve: { mainFields: ["module", "main"] },
    transform: { target: protocol.constants.nodeTarget },
    external: (id) => isBuiltin(id),
    plugins: [
      {
        name: "flame-backend-bootstrap",
        resolveId(id) {
          if (id.endsWith(".node"))
            throw new Error(
              "portable managed backends cannot contain native addons",
            );
          if (id === bootstrap) return id;
          if (id === "flame:bootstrap-sdk") return sdk;
          if (id === "flame:author-backend") return entry;
        },
        load(id) {
          if (id === bootstrap)
            return 'import { startBackend } from "flame:bootstrap-sdk";\nawait startBackend({ load: () => import("flame:author-backend") });\n';
        },
        moduleParsed(info) {
          if (info.code === null || info.id.endsWith(".json")) return;
          const ast = this.parse(info.code, {
            lang: /\.[cm]?tsx?$/.test(info.id) ? "ts" : "js",
          });
          const visit = (node) => {
            if (!node || typeof node !== "object") return;
            if (
              node.type === "ImportExpression" &&
              (node.source.type !== "Literal" ||
                typeof node.source.value !== "string")
            )
              throw new Error(
                "backend dynamic imports must name a build-time module",
              );
            if (
              node.type === "CallExpression" &&
              node.callee.type === "Identifier" &&
              node.callee.name === "require" &&
              (node.arguments.length !== 1 ||
                node.arguments[0].type !== "Literal" ||
                typeof node.arguments[0].value !== "string")
            )
              throw new Error("backend require must name a build-time module");
            for (const [key, child] of Object.entries(node)) {
              if (["loc", "start", "end"].includes(key)) continue;
              if (Array.isArray(child)) child.forEach(visit);
              else visit(child);
            }
          };
          visit(ast);
        },
      },
    ],
    onwarn(warning, report) {
      if (
        warning.code === "UNRESOLVED_IMPORT" ||
        warning.code === "UNRESOLVED_ENTRY"
      )
        throw new Error(warning.message);
      report(warning);
    },
  });
  try {
    const result = await bundle.write({
      dir: directory,
      format: "esm",
      entryFileNames: entryFileName,
      chunkFileNames: "[name]-[hash].mjs",
      assetFileNames: "[name]-[hash][extname]",
      sourcemap: true,
      sourcemapPathTransform(source) {
        if (source === bootstrap) return "flame-bootstrap.mjs";
        const path = (
          isAbsolute(source) ? source : resolve(directory, source)
        ).replaceAll("\\", "/");
        if (path.includes("/node_modules/"))
          return "dependencies/" + path.split("/node_modules/").at(-1);
        if (path.includes("/packages/"))
          return "packages/" + path.split("/packages/").at(-1);
        return (
          "author/" +
          relative(dirname(entry), path)
            .replaceAll("\\", "/")
            .replaceAll("../", "parent/")
        );
      },
    });
    const files = new Set(result.output.map((output) => output.fileName));
    for (const output of result.output) {
      if (
        output.type === "chunk" &&
        [...output.imports, ...output.dynamicImports].some(
          (id) => !files.has(id) && !isBuiltin(id),
        )
      )
        throw new Error(
          "backend output contains an unpackaged external module",
        );
    }
  } finally {
    await bundle.close();
  }
}
