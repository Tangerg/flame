import { sharedRuntimePackages } from "./shared-runtime.mjs";
import { defineConfig } from "vite";
import { pluginModules } from "./pluginModules.vite.mjs";
import { webArtifacts } from "./webArtifacts.vite.mjs";
import { BROWSER_TARGETS } from "@flame/component-sdk/build";
import react from "@vitejs/plugin-react";
import { STYLEX_CONDITIONS, stylexPlugin } from "./stylex.vite.mjs";
import path from "node:path";

export default defineConfig({
  plugins: [react(), stylexPlugin(), pluginModules(), webArtifacts()],
  resolve: {
    conditions: STYLEX_CONDITIONS,
    dedupe: sharedRuntimePackages,
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  optimizeDeps: {
    // The linked icon barrel is served as source, fanning out to hundreds of module requests per preview.
    // Worker-only imports must be discovered before the first highlight, which
    // otherwise replaces the editing document during dependency optimization.
    include: ["@flame/icons/react"],
  },
  server: {
    host: "127.0.0.1",
    port: 4174,
    strictPort: true,
  },
  build: {
    target: [...BROWSER_TARGETS],
    outDir: "../../out/web/lab",
    emptyOutDir: true,
    // Each preview document consumes the same complete stylesheet graph.
    cssCodeSplit: false,
    rolldownOptions: {
      input: [
        "lab/index.html",
        "lab/preview.html",
        "lab/fixture.html",
        "lab/a2ui.html",
        "lab/native-reading.html",
      ].map((entry) => path.resolve(import.meta.dirname, entry)),
    },
  },
  preview: {
    host: "127.0.0.1",
    port: 4175,
    strictPort: true,
  },
});
