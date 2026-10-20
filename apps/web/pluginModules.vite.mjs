import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { HOST_MODULES } from "@flame/component-sdk/build";

const prefix = "virtual:flame-plugin-module:";
const require = createRequire(import.meta.url);
const reactModules = new Set([
  "react",
  "react-dom",
  "react-dom/client",
  "react/jsx-runtime",
  "react/jsx-dev-runtime",
]);
const defaultModules = new Set(["react", "react-dom", "react-dom/client"]);

export function pluginModules() {
  let modules;
  let config;
  const namedExports = new Map();
  return {
    name: "flame-public-plugin-modules",
    async configResolved(value) {
      config = value;
      const ui = JSON.parse(
        await readFile(new URL("../../packages/ui/package.json", import.meta.url), "utf8"),
      );
      modules = [
        ...new Set([
          ...HOST_MODULES,
          ...Object.keys(ui.exports)
            .filter((key) => key !== "." && !key.endsWith(".css"))
            .map((key) => `@flame/ui/${key.slice(2)}`),
        ]),
      ];
      for (const module of modules) {
        if (!reactModules.has(module)) continue;
        namedExports.set(
          module,
          Object.keys(require(module)).filter(
            (name) => name !== "default" && name !== "__esModule",
          ),
        );
      }
    },
    resolveId(id) {
      if (id.startsWith(prefix)) return `\0${id}`;
    },
    load(id) {
      if (!id.startsWith(`\0${prefix}`)) return;
      const dependency = id.slice(prefix.length + 1);
      const names = namedExports.get(dependency);
      // CommonJS has no statically enumerable star exports. Explicit names retain
      // the installed React contract and still bind the host's single instance.
      const exports = names ? `{${names.join(",")}}` : "*";
      return `export ${exports} from ${JSON.stringify(dependency)};${defaultModules.has(dependency) ? `export {default} from ${JSON.stringify(dependency)};` : ""}`;
    },
    buildStart() {
      if (config.command !== "build") return;
      modules.forEach((module, index) =>
        this.emitFile({
          type: "chunk",
          id: prefix + module,
          name: `plugin-module-${index}`,
          preserveSignature: "strict",
        }),
      );
    },
    transformIndexHtml: {
      order: "post",
      handler(_html, context) {
        const imports = Object.fromEntries(
          modules.map((module) => {
            if (context.bundle) {
              const chunk = Object.values(context.bundle).find(
                (value) =>
                  value.type === "chunk" && value.facadeModuleId === `\0${prefix}${module}`,
              );
              if (!chunk) throw new Error(`shared plugin module was not emitted: ${module}`);
              const missing = (namedExports.get(module) ?? []).filter(
                (name) => !chunk.exports.includes(name),
              );
              if (defaultModules.has(module) && !chunk.exports.includes("default"))
                missing.push("default");
              if (missing.length)
                throw new Error(
                  `shared plugin module ${module} omits exports: ${missing.join(", ")}`,
                );
              return [module, `${config.base}${chunk.fileName}`];
            }
            return [module, `/@id/__x00__${prefix}${module}`];
          }),
        );
        return [
          {
            tag: "script",
            attrs: { type: "importmap" },
            children: JSON.stringify({ imports }),
            injectTo: "head-prepend",
          },
        ];
      },
    },
  };
}
