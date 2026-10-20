import { runtimeRequestMeta } from "@/app/runtimeProtocol";
import "@flame/ui/styles.css";
import { createRoot } from "react-dom/client";
import App from "@/App";
import { createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
import { createBuiltinPlugins } from "@/app/builtinPlugins";
import { createClientHost } from "@/platform/clientHost";
import { ClientRenderer } from "@/app/renderer";
import { FAILURE_SCOPE, RootBoundary, StartupFailure } from "@/app/StartupFailure";
import { applyWindowChrome, watchWindowChrome } from "@/app/windowChrome";
import { disposeOnHmr } from "@/lib/hmr";
import "@/styles/markdown.css";
import "@/styles/overlays.css";
import "@/styles/globals.css";
const host = createClientHost();

const connection = createRuntimeConnection(host, runtimeRequestMeta);

const plugins = createBuiltinPlugins(connection, host);

const renderer = new ClientRenderer({
  initializeClientHost: connection.initialize,
  prepareWindowChrome: () => applyWindowChrome(host),
  watchWindowChrome: () => watchWindowChrome(host),
  mount() {
    const container = document.getElementById("root");
    const root = createRoot(container!);
    root.render(
      <RootBoundary>
        <App plugins={plugins} />
      </RootBoundary>,
    );
    return root;
  },
  closeConnection: connection.dispose,
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

disposeOnHmr(() => {
  window.removeEventListener("beforeunload", teardown);
  teardown();
});

void renderer.start().catch((error: unknown) => {
  console.error("[desktop] startup failed:", error);
  createRoot(document.getElementById("root")!).render(
    <StartupFailure scope={FAILURE_SCOPE.startup} error={error} />,
  );
});
