import { createRoot } from "react-dom/client";
import App from "./App";
import { disposeContainer, initializeDesktopHost } from "./main/container";
import { DesktopRenderer } from "./main/renderer";
import { applyWindowChrome, watchWindowChrome } from "./main/windowChrome";
import { disposeOnHmr } from "./lib/hmr";
// Fonts: the native OS stack (SF Pro / PingFang on macOS) — see globals.css
// --font-sans. No bundled webfont; the system face is the premium, native
// default, loads instantly, and renders mixed CJK best.
import "./styles/globals.css";

// Deliberately NOT wrapped in StrictMode: its dev double-invoke surfaces benign "Maximum
// update depth" warnings from the persist-rehydrate and plugin-loader sequencing. The
// bundle ships without StrictMode anyway, so this matches what users see.

const renderer = new DesktopRenderer({
  initializeDesktopHost,
  prepareWindowChrome: applyWindowChrome,
  watchWindowChrome,
  mount() {
    const container = document.getElementById("root");
    const root = createRoot(container!);
    root.render(<App />);
    return root;
  },
  closeRuntime: disposeContainer,
  reportFailure(scope, error) {
    console.error(`[desktop] ${scope} failed:`, error);
  },
});

const teardown = () => {
  void renderer.dispose().catch((error: unknown) => {
    console.error("[desktop] teardown failed:", error);
  });
};
window.addEventListener("beforeunload", teardown);
disposeOnHmr(() => window.removeEventListener("beforeunload", teardown));

void renderer.start().catch((error: unknown) => {
  console.error("[desktop] startup failed:", error);
});
