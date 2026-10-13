import { readFile } from "node:fs/promises";
import { HOST_MODULES } from "@flame/plugin-sdk/build";

const prefix = "virtual:flame-plugin-module:";

export function pluginModules() {
  let modules;
  let config;
  return {
    name: "flame-public-plugin-modules",
    async configResolved(value) {
      config = value;
      const ui = JSON.parse(
        await readFile(new URL("../../ui/package.json", import.meta.url), "utf8"),
      );
      modules = [
        ...new Set([
          ...HOST_MODULES,
          ...Object.keys(ui.exports)
            .filter((key) => key !== "." && !key.endsWith(".css"))
            .map((key) => `@flame/ui/${key.slice(2)}`),
        ]),
      ];
    },
    resolveId(id) {
      if (id.startsWith(prefix)) return `\0${id}`;
    },
    load(id) {
      if (!id.startsWith(`\0${prefix}`)) return;
      const dependency = id.slice(prefix.length + 1);
      return `export * from ${JSON.stringify(dependency)};${["react", "react-dom", "react-dom/client"].includes(dependency) ? `export {default} from ${JSON.stringify(dependency)};` : ""}`;
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
