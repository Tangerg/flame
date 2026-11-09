import { sharedRuntimePackages } from "./shared-runtime.mjs";
import { defineConfig, type Plugin } from "vite";
import { readFile } from "node:fs/promises";
import { pluginModules } from "./pluginModules.vite.mjs";
import react from "@vitejs/plugin-react";
import { stylexPlugin } from "./stylex.vite.mjs";
import path from "node:path";

const compositionEntry = "io.github.tangerg.flame/components/component.js";
const compositionResource = `v2/plugin-resources/baseline/${compositionEntry}`;
const compositionSource = new URL(
  `../../examples/plugins/composition/provider/package/${compositionEntry}`,
  import.meta.url,
);

function compositionResourcePlugin(): Plugin {
  return {
    name: "lab-composition-resource",
    configureServer(server) {
      server.middlewares.use(async (request, response, next) => {
        if (request.url !== `/${compositionResource}`) return next();
        try {
          response.setHeader("Content-Type", "text/javascript");
          response.setHeader("Cache-Control", "no-store");
          response.end(await readFile(compositionSource));
        } catch (error) {
          next(error);
        }
      });
    },
    async generateBundle() {
      this.emitFile({
        type: "asset",
        fileName: compositionResource,
        source: await readFile(compositionSource),
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), stylexPlugin(), pluginModules(), compositionResourcePlugin()],
  resolve: {
    dedupe: sharedRuntimePackages,
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  optimizeDeps: {
    // The linked icon barrel is served as source, fanning out to hundreds of module requests per preview.
    include: ["@flame/icons/react"],
  },
  server: {
    host: "127.0.0.1",
    port: 4174,
    strictPort: true,
  },
  build: {
    outDir: "../../out/web/lab",
    emptyOutDir: true,
    // StyleX appends extracted rules to one CSS asset; every preview document needs it.
    cssCodeSplit: false,
    rollupOptions: {
      input: ["lab/index.html", "lab/preview.html", "lab/fixture.html"].map((entry) =>
        path.resolve(import.meta.dirname, entry),
      ),
    },
  },
  preview: {
    host: "127.0.0.1",
    port: 4175,
    strictPort: true,
  },
});
