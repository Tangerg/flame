import { readFile } from "node:fs/promises";
import { defineConfig, type Plugin } from "vite";
import path from "node:path";
import visual from "./vite.visual.config.ts";
import { pluginModules } from "./pluginModules.vite.mjs";

const entry = "io.github.tangerg.flame/components/component.js";
const resource = `/v2/plugin-resources/baseline/${entry}`;

function providerResource(): Plugin {
  return {
    name: "composition-baseline-provider-resource",
    configurePreviewServer(server) {
      server.middlewares.use(async (request, response, next) => {
        if (request.url !== resource) return next();
        try {
          const bytes = await readFile(
            new URL(`../../examples/plugins/composition/provider/${entry}`, import.meta.url),
          );
          response.setHeader("Content-Type", "text/javascript");
          response.setHeader("Cache-Control", "no-store");
          response.end(bytes);
        } catch (error) {
          next(error);
        }
      });
    },
  };
}

export default defineConfig({
  ...visual,
  plugins: [...visual.plugins!, pluginModules(), providerResource()],
  build: {
    ...visual.build,
    outDir: "../.cache/composition-dist",
    rollupOptions: { input: path.resolve(import.meta.dirname, "visual/composition.html") },
  },
  preview: { host: "127.0.0.1", port: 4176, strictPort: true },
});
