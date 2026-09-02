import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

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

export default defineConfig({
  plugins: [react(), tailwindcss()],
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
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  build: {
    // Desktop app loads from disk, so total size matters far less than what is
    // on the STARTUP path. Splitting vendor deps also means Wails updates ship
    // only changed chunks. The startup budget and the assertion that the heavy
    // lazy features stay off that path both live in check-bundle-size.mjs.
    // Keep Vite's raw warning just above the known monoliths so it catches a
    // new unclassified mega-chunk without reporting the same reviewed features
    // on every clean build.
    chunkSizeWarningLimit: 1600,
    rollupOptions: {
      output: {
        // A chunk is as eager as its most eager member: if any module in it is
        // statically reachable from the entry, Vite modulepreloads the whole
        // chunk and a dynamic import of anything else inside it buys nothing.
        // So the grouping below separates deps by WHEN they are needed, not by
        // subject matter — and the substring tests are anchored (trailing `/`)
        // because `node_modules/react` also matches `react-markdown`, which is
        // how this function came to describe a grouping it wasn't producing.
        manualChunks(id: string) {
          // Vite's dynamic-import preload helper. Every module holding a
          // dynamic import imports it STATICALLY, so whichever chunk it lands
          // in becomes a static dependency of the entry — and left unassigned,
          // Rolldown folds a lone virtual module like this into a big
          // neighbour. That is how a monolith kept reaching the startup path:
          // first the 1.36 MB markdown chunk, then, once that was split, the
          // 755 KB shiki chunk. It gets its own chunk for the same reason
          // rolldown-runtime does — a shared runtime helper is nobody's
          // feature, and a name already used by another group gets merged back
          // into whatever chunk Rolldown picks.
          if (id.includes("vite/preload-helper")) return "preload-helper";
          // The Wails runtime, which has to stay OFF the startup path: importing it has
          // side effects — it installs listeners and starts talking to a host — and this
          // same bundle runs in a plain browser and in the visual fixtures, where there
          // is no host. `desktopHost.ts` imports it dynamically for exactly that reason,
          // and naming its chunk is what makes the split reliable instead of incidental:
          // unnamed, it is a lone module Rolldown may fold into a big neighbour, which is
          // how the entry acquired a monolith twice already.
          if (id.includes("node_modules/@wailsio/runtime")) return "wails-runtime";
          // Stable vendor deps
          if (
            id.includes("node_modules/react/") ||
            id.includes("node_modules/react-dom/") ||
            id.includes("node_modules/scheduler/")
          )
            return "vendor";
          if (id.includes("node_modules/motion")) return "vendor-motion";
          if (id.includes("node_modules/zustand")) return "vendor";
          // Headless interaction primitives
          if (id.includes("node_modules/@base-ui")) return "base-ui";
          // TanStack
          if (id.includes("node_modules/@tanstack")) return "tanstack";
          // Brand marks are deliberately NOT named here. The composer pulls in the ten it
          // draws with whatever imports them, and the icon gallery globs the whole catalogue
          // behind a lazy route; naming the package would put that glob on the startup path,
          // which is the one place its own laziness cannot reach. Lucide needs no rule at
          // all — `ui/icons` imports the ninety it draws by name and the rest tree-shakes.
          // Markdown pipeline — eager: every rendered message goes through it.
          if (
            id.includes("node_modules/react-markdown/") ||
            id.includes("node_modules/remark-") ||
            id.includes("node_modules/rehype-") ||
            id.includes("node_modules/unist-") ||
            id.includes("node_modules/mdast-")
          )
            return "markdown";
          // Syntax highlighting — lazy: dynamic-imported on the first code
          // block. Kept out of "markdown" above for the reason stated there;
          // sharing that chunk is how ~1.3 MB of grammars reached the startup
          // path while still looking like a lazy feature in every report.
          if (id.includes("node_modules/shiki")) return "shiki";
          // KaTeX JS is statically reachable through rehype-katex, while its
          // stylesheet is deliberately imported only after a message contains
          // math. Giving both the same manual chunk name makes Rolldown attach
          // the dynamic CSS to the eager JS group and emit it in index.html.
          // Keep the style on its loader's lazy edge with a distinct identity.
          if (id.includes("node_modules/katex/dist/katex.min.css")) return "katex-style";
          // Math rendering JS.
          if (id.includes("node_modules/katex/")) return "katex";
          // Mermaid
          if (id.includes("node_modules/beautiful-mermaid")) return "mermaid";
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
          {
            const otel = /node_modules\/@opentelemetry\/([^/]+)/.exec(id);
            if (otel) return `otel-${otel[1]}`;
          }
          // Leave unrelated dependencies to Rollup's graph-aware chunking. A
          // catch-all vendor bucket merged otherwise independent lazy features
          // into one 9MB raw chunk and defeated the explicit boundaries above.
          return undefined;
        },
      },
    },
    target: "chrome131",
  },
});
