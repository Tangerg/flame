import { createRoot } from "react-dom/client";
import App from "./App";
import { disposeContainer, initializeDesktopHost } from "./main/container";
import { DesktopRenderer } from "./main/renderer";
import { applyWindowChrome, watchWindowChrome } from "./main/windowChrome";
import { disposeOnHmr } from "./lib/hmr";
import "./styles/markdown.css";
import "./styles/overlays.css";
// LAST of the three. `globals.css` ends with the touch-device reveal override, whose whole
// job is to beat a rest state any of the sheets above it declares — a markdown table's action
// strip is exactly that, and at equal specificity the later sheet wins. These two used to sit
// as `@import` rules at the bottom of `globals.css`, which CSS ignores; moving them to the
// entry is what made the order real, and the order the file already documented is this one.
import "./styles/globals.css";
import "./styles/stylex.css";

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
