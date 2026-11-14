import { sharedRuntimePackages } from "./shared-runtime.mjs";
import { defineConfig } from "vite";
import { pluginModules } from "./pluginModules.vite.mjs";
import { webArtifacts } from "./webArtifacts.vite.mjs";
import { BROWSER_TARGETS } from "@flame/component-sdk/build";
import react from "@vitejs/plugin-react";
import { stylexPlugin } from "./stylex.vite.mjs";
import path from "node:path";
import { LAB_DEVELOPMENT_SERVER, LAB_PREVIEW_SERVER } from "./labServers.ts";

export default defineConfig({
  plugins: [react(), stylexPlugin(), pluginModules(), webArtifacts()],
  resolve: {
    dedupe: sharedRuntimePackages,
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  optimizeDeps: {
    // The linked icon barrel is served as source, fanning out to hundreds of module requests per preview.
    // Worker-only imports must be discovered before the first highlight, which
    // otherwise replaces the editing document during dependency optimization.
    include: ["@flame/icons/react", "shiki/core", "shiki/engine/javascript"],
  },
  server: {
    host: LAB_DEVELOPMENT_SERVER.host,
    port: LAB_DEVELOPMENT_SERVER.port,
    strictPort: true,
  },
  build: {
    target: [...BROWSER_TARGETS],
    outDir: "../../out/web/lab",
    emptyOutDir: true,
    // Each preview document consumes the same complete stylesheet graph.
    cssCodeSplit: false,
    rolldownOptions: {
      input: ["lab/index.html", "lab/preview.html", "lab/fixture.html"].map((entry) =>
        path.resolve(import.meta.dirname, entry),
      ),
    },
  },
  preview: {
    host: LAB_PREVIEW_SERVER.host,
    port: LAB_PREVIEW_SERVER.port,
    strictPort: true,
  },
});
