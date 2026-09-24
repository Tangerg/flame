import { createRoot } from "react-dom/client";
import App from "./App";
import { disposeContainer, initializeDesktopHost } from "./main/container";
import { DesktopRenderer } from "./main/renderer";
import { FAILURE_SCOPE, RootBoundary, StartupFailure } from "./main/StartupFailure";
import { applyWindowChrome, watchWindowChrome } from "./main/windowChrome";
import { disposeOnHmr } from "./lib/hmr";
import "./styles/markdown.css";
import "./styles/overlays.css";
import "./styles/globals.css";
import "./styles/stylex.css";

const renderer = new DesktopRenderer({
  initializeDesktopHost,
  prepareWindowChrome: applyWindowChrome,
  watchWindowChrome,
  mount() {
    const container = document.getElementById("root");
    const root = createRoot(container!);
    root.render(
      <RootBoundary>
        <App />
      </RootBoundary>,
    );
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
  createRoot(document.getElementById("root")!).render(
    <StartupFailure scope={FAILURE_SCOPE.startup} error={error} />,
  );
});
