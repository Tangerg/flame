import { sharedRuntimePackages } from "./shared-runtime.mjs";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";
import { STYLEX_CONDITIONS, stylexPlugin } from "./stylex.vite.mjs";
import { pluginModules } from "./pluginModules.vite.mjs";
import { webArtifacts } from "./webArtifacts.vite.mjs";
import { BROWSER_TARGETS } from "@flame/component-sdk/build";
import { artifactGraph } from "./artifactGraph.vite.mjs";

// The dev-server port is declared once, as VITE_PORT in the Taskfile, and handed to both
// halves of the dev loop: to Vite as `--port`, to the webview as `wails3 dev -port`. It
// reaches this file as an environment variable because ONE thing here cannot be set from
// the command line — see `hmr` below.
//
// Absent means Vite is running on its own (a browser, the visual fixtures), and then none
// of this applies: there is no webview whose origin differs from Vite's, so Vite's own
// defaults are right and pinning a port would only make two dev servers collide.
const webviewPort = process.env.WAILS_VITE_PORT;
const host = "127.0.0.1";

// Named chunks; the first rule with a matching path wins.
const CHUNK_RULES: readonly (readonly [chunk: string, paths: readonly string[]])[] = [
  // The Wails runtime, which has to stay OFF the startup path: importing it has
  // side effects — it installs listeners and starts talking to a host — and this
  // same bundle runs in a plain browser and in the visual fixtures, where there
  // is no host. `platform/clientHost.ts` imports it dynamically for exactly that reason,
  // and naming its chunk is what makes the split reliable instead of incidental:
  // unnamed, it is a lone module Rolldown may fold into a big neighbour, which is
  // how the entry acquired a monolith twice already.
  ["wails-runtime", ["node_modules/@wailsio/runtime"]],
  ["vendor", ["node_modules/react/", "node_modules/react-dom/", "node_modules/scheduler/"]],
  ["vendor-motion", ["node_modules/motion"]],
  ["vendor", ["node_modules/zustand"]],
  ["base-ui", ["node_modules/@base-ui"]],
  ["tanstack", ["node_modules/@tanstack"]],
  // Brand marks are deliberately NOT named here. The composer pulls in the ten it
  // draws with whatever imports them, and the icon gallery globs the whole catalogue
  // behind a lazy route; naming the package would put that glob on the startup path,
  // which is the one place its own laziness cannot reach.
  // Markdown pipeline — eager: every rendered message goes through it.
  [
    "markdown",
    [
      "node_modules/react-markdown/",
      "node_modules/remark-",
      "node_modules/rehype-",
      "node_modules/unist-",
      "node_modules/mdast-",
    ],
  ],
  // Syntax highlighting — lazy: dynamic-imported on the first code
  // block. Kept out of "markdown" above for the reason stated there;
  // sharing that chunk is how ~1.3 MB of grammars reached the startup
  // path while still looking like a lazy feature in every report.
  [
    "shiki",
    [
      "node_modules/@shikijs/core/",
      "node_modules/@shikijs/engine-javascript/",
      "node_modules/@shikijs/vscode-textmate/",
    ],
  ],
  // KaTeX JS is statically reachable through rehype-katex, while its
  // stylesheet is a dependency of the lazy Markdown renderer. Giving
  // both the same manual chunk name makes Rolldown attach the lazy CSS
  // to the eager JS group and emit it in index.html. Keep the style on
  // the renderer's lazy edge with a distinct identity.
  ["katex-style", ["node_modules/katex/dist/katex.min.css"]],
  ["katex", ["node_modules/katex/"]],
  ["mermaid", ["node_modules/pele/dist/esm/lazy.js"]],
];

export default defineConfig({
  plugins: [react(), stylexPlugin(), pluginModules(), artifactGraph(), webArtifacts()],
  // Wails serves the webview through its OWN dev server on a different port, so the page
  // origin is NOT Vite's. Without `hmr.clientPort` the Vite HMR client in the WebView
  // opens its WebSocket against the page origin instead of Vite — the handshake fails
  // silently, and updates compile but never reach the window. Pinning the port (strict,
  // no fallback drift) plus clientPort makes the HMR socket deterministic.
  //
  // `127.0.0.1` and not `localhost`: the two disagreed here once, and `localhost`
  // resolves to ::1 on some machines, which leaves the WebView pointed at a port nothing
  // is listening on.
  server: webviewPort
    ? {
        host,
        port: Number(webviewPort),
        strictPort: true,
        hmr: { protocol: "ws", host, clientPort: Number(webviewPort) },
      }
    : undefined,
  resolve: {
    conditions: STYLEX_CONDITIONS,
    dedupe: sharedRuntimePackages,
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  build: {
    // Browser and native hosts share this distribution. Keep native-only code
    // behind its platform boundary and retain independent lazy feature entries.
    // Keep Vite's raw warning just above the known monoliths so it catches a
    // new unclassified mega-chunk without reporting the same reviewed features
    // on every clean build.
    chunkSizeWarningLimit: 1600,
    rolldownOptions: {
      output: {
        // A chunk is as eager as its most eager member: if any module in it is
        // statically reachable from the entry, Vite modulepreloads the whole
        // chunk and a dynamic import of anything else inside it buys nothing.
        // So the grouping below separates deps by WHEN they are needed, not by
        // subject matter — and the substring tests are anchored (trailing `/`)
        // because `node_modules/react` also matches `react-markdown`, which is
        // how this function came to describe a grouping it wasn't producing.
        codeSplitting: {
          groups: [
            {
              // Claim the shared helper before feature groups recursively capture
              // dependencies; otherwise its eager callers also load the lazy engine.
              name: "preload-helper",
              test: /vite\/preload-helper/,
              priority: 1,
            },
            {
              name(id: string) {
                for (const [chunk, paths] of CHUNK_RULES)
                  if (paths.some((path) => id.includes(path))) return chunk;
                // OpenTelemetry, ONE CHUNK PER PACKAGE, which is not the tidy
                // grouping it looks like it should be.
                //
                // Only `api` and `api-logs` are eager: app code on ordinary paths
                // opens spans and emits metrics against them, and `lib/metrics` is
                // imported by the reducer itself. The SDKs and exporters are reachable
                // only through the dynamic `setupObservability`. But every hand-drawn
                // two-group split measured here put the whole family back on the
                // startup path — the API and the SDK share modules, and once they are
                // named into groups Rolldown resolves that overlap by folding the
                // shared modules in with the SDK, so the eager API edge drags 110 KB
                // of exporters along behind it.
                //
                // Per package, each one's eagerness is decided by its own edges
                // instead, and the SDKs fall off the entry: 122.8 KB to 12.2 KB.
                // The extra chunk count costs nothing — these are read from a local
                // disk, and only the startup path is a budget.
                const otel = /node_modules\/@opentelemetry\/([^/]+)/.exec(id);
                if (otel) return `otel-${otel[1]}`;
                // Leave unrelated dependencies to Rollup's graph-aware chunking. A
                // catch-all vendor bucket merged otherwise independent lazy features
                // into one 9MB raw chunk and defeated the explicit boundaries above.
                return null;
              },
            },
          ],
        },
      },
    },
    target: [...BROWSER_TARGETS],
  },
});
