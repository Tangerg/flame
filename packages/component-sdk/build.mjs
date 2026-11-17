import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import profile from "./runtime-profile.generated.json" with { type: "json" };
export const HOST_MODULES = Object.freeze(profile.modules);
export const HOST_MODULE_PROFILE = profile.profile;
export const BROWSER_TARGETS = Object.freeze([...profile.browserTargets]);

// Private dependencies are bundled; these modules are supplied by the actual
// workbench renderer. The format is identical for released and development code.
export async function buildComponent({ input, directory, compiler }) {
  if (typeof compiler !== "function")
    throw new TypeError("component compiler is required");
  const bundle = await compiler({
    input,
    platform: "browser",
    moduleTypes: { ".png": "dataurl" },
    external: (id) => HOST_MODULES.includes(id),
    transform: {
      target: BROWSER_TARGETS,
      jsx: { runtime: "automatic", development: false },
      define: { "process.env.NODE_ENV": JSON.stringify("production") },
    },
    plugins: [
      {
        name: "flame-component-text",
        async resolveId(source, importer) {
          if (!source.endsWith("?raw")) return;
          const resolved = await this.resolve(source.slice(0, -4), importer);
          if (!resolved || resolved.external)
            throw new Error(
              "component text imports must resolve to private files",
            );
          return `${resolved.id}?raw`;
        },
        async load(id) {
          if (!id.endsWith("?raw")) return;
          const source = new TextDecoder("utf-8", {
            fatal: true,
            ignoreBOM: true,
          }).decode(await readFile(id.slice(0, -4)));
          return `export default ${JSON.stringify(source)};`;
        },
      },
      {
        name: "flame-component-literal-whitespace",
        generateBundle(_options, outputs) {
          for (const output of Object.values(outputs)) {
            if (output.type !== "chunk") continue;
            const edits = [];
            const visit = (node, parent) => {
              if (!node || typeof node !== "object") return;
              if (
                node.type === "TemplateLiteral" &&
                node.expressions.length === 0 &&
                parent?.type !== "TaggedTemplateExpression" &&
                /[ \t]+\r?\n/.test(node.quasis[0].value.raw)
              ) {
                // Literal whitespace is data; escape it without trimming the captured value.
                edits.push({
                  start: node.start,
                  end: node.end,
                  text: JSON.stringify(node.quasis[0].value.cooked),
                });
                return;
              }
              for (const [key, child] of Object.entries(node)) {
                if (["loc", "start", "end"].includes(key)) continue;
                if (Array.isArray(child))
                  child.forEach((value) => visit(value, node));
                else visit(child, node);
              }
            };
            visit(this.parse(output.code));
            for (const edit of edits.toSorted(
              (left, right) => right.start - left.start,
            ))
              output.code =
                output.code.slice(0, edit.start) +
                edit.text +
                output.code.slice(edit.end);
          }
        },
      },
      {
        name: "flame-component-import-graph",
        moduleParsed(info) {
          if (
            info.code === null ||
            info.id.endsWith(".json") ||
            info.id.endsWith(".png")
          )
            return;
          const ast = this.parse(info.code, {
            lang: info.id.endsWith(".tsx")
              ? "tsx"
              : info.id.endsWith(".jsx")
                ? "jsx"
                : /\.[cm]?ts$/.test(info.id)
                  ? "ts"
                  : "js",
          });
          const visit = (node) => {
            if (!node || typeof node !== "object") return;
            if (
              node.type === "ImportExpression" &&
              (node.source.type !== "Literal" ||
                typeof node.source.value !== "string")
            )
              throw new Error(
                "component dynamic imports must name a build-time module",
              );
            if (
              node.type === "CallExpression" &&
              node.callee.type === "Identifier" &&
              node.callee.name === "require" &&
              (node.arguments.length !== 1 ||
                node.arguments[0].type !== "Literal" ||
                typeof node.arguments[0].value !== "string")
            )
              throw new Error(
                "component require must name a build-time module",
              );
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
      if (
        warning.code === "MODULE_LEVEL_DIRECTIVE" &&
        warning.message.includes("use client")
      )
        return;
      report(warning);
    },
  });
  try {
    const emitted = await bundle.write({
      dir: directory,
      format: "esm",
      codeSplitting: false,
      entryFileNames: "component.js",
      assetFileNames: "[name]-[hash][extname]",
      minify: true,
    });
    const entry = emitted.output.find(
      (output) => output.type === "chunk" && output.isEntry,
    );
    if (!entry) throw new Error("component build did not emit an entry module");
    const paths = new Set(emitted.output.map((output) => output.fileName));
    const sharedSpecifiers = new Set();
    const modules = emitted.output
      .filter((output) => output.type === "chunk")
      .map((output) => {
        for (const dependency of [
          ...output.imports,
          ...output.dynamicImports,
        ]) {
          if (paths.has(dependency)) continue;
          if (!HOST_MODULES.includes(dependency))
            throw new Error(
              `component output contains an unpackaged module: ${dependency}`,
            );
          sharedSpecifiers.add(dependency);
        }
        return {
          path: output.fileName,
          imports: output.imports.toSorted(),
          dynamicImports: output.dynamicImports.toSorted(),
          exports: output.exports.toSorted(),
        };
      })
      .sort((left, right) => left.path.localeCompare(right.path, "en"));
    const graph = {
      schemaVersion: profile.moduleGraph.schemaVersion,
      entry: entry.fileName,
      modules,
      sharedSpecifiers: [...sharedSpecifiers].sort(),
    };
    await writeFile(
      join(directory, profile.moduleGraph.filename),
      JSON.stringify(graph, null, 2) + "\n",
    );
    return { exports: entry.exports, graph };
  } finally {
    await bundle.close();
  }
}
