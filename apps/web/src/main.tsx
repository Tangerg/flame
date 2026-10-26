import { createTheme } from "@/features/theme/adapters/theme";
import { ThemeProvider } from "@/features/theme/public/theme";
import { createNotifications } from "@/features/notifications/adapters/notifications";
import { NotificationProvider } from "@/features/notifications/public/notifier";
import { createInstalledPlugins } from "@/features/plugins/adapters/installedPlugins";
import { InstalledPluginsProvider } from "@/features/plugins/context";
import { PRODUCT_NAME } from "@/app/product";
import { createAgent } from "@/features/agent/adapters/agent";
import { AgentProvider } from "@/features/agent/public/agent";
import { createComposer } from "@/features/composer/adapters/composerStatePorts";
import { ComposerProvider } from "@/features/composer/public/composer";
import { createWorkspace } from "@/features/workspace/adapters/workspace";
import { WorkspaceProvider } from "@/features/workspace/public/workspace";
import { createWorkingDirectoryPicker } from "@/features/navigation/adapters/workingDirectorySelection";
import { WorkingDirectoryProvider } from "@/features/navigation/public/workingDirectory";
import { ImageSaveProvider } from "@/features/chat/public/imageSave";
import { createAppNavigation } from "@/router";
import { createStorage } from "@/extensions/storage";
import { createRuntimeMutationJournalStorage } from "@/features/connection/adapters/runtimeMutationJournalStorage";
import { RuntimeProvider } from "@/features/connection/public/runtime";
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
const theme = createTheme(document, window);

const connection = createRuntimeConnection(
  host,
  runtimeRequestMeta,
  createRuntimeMutationJournalStorage(createStorage("flame.builtin.runtime")),
);

const navigation = createAppNavigation();

const agent = createAgent(
  connection.client,
  navigation.navigator,
  connection.access,
  connection.stream,
  connection.scope,
);
const composer = createComposer(
  {
    getActiveSessionId: agent.access.state.getActiveSessionId,
    getLifecycleSnapshot: agent.access.state.getLifecycleSnapshot,
    subscribeActiveSessionId: agent.access.state.subscribeActiveSessionId,
    subscribeLifecycle: agent.access.state.subscribeLifecycle,
    subscribeDeleted: agent.access.state.subscribeDeletedSession,
  },
  connection.access.endpoint.currentRuntimeEndpoint,
  connection.scope,
);
const workspace = createWorkspace(
  connection.client,
  navigation.navigator,
  host,
  connection.localWorkspaceAvailable,
  agent.access,
  connection.access,
);
const directory = createWorkingDirectoryPicker(
  () => host.chooseWorkingDirectory(),
  connection.localWorkspaceAvailable,
  connection.access.endpoint.currentRuntimeEndpoint,
);
const notifications = createNotifications(host, agent.access, workspace.access, PRODUCT_NAME);
const installedPlugins = createInstalledPlugins(
  connection.client,
  connection.endpoint,
  theme.access,
);
const plugins = createBuiltinPlugins(
  connection,
  host,
  navigation.navigator,
  agent,
  composer,
  workspace,
  directory,
  notifications,
  installedPlugins,
  theme,
);

const renderer = new ClientRenderer({
  initializeClientHost: connection.initialize,
  prepareWindowChrome: () => applyWindowChrome(host),
  watchWindowChrome: () => watchWindowChrome(host),
  mount() {
    const container = document.getElementById("root");
    const root = createRoot(container!);
    root.render(
      <RootBoundary>
        <ThemeProvider value={theme.access}>
          <RuntimeProvider value={connection.access}>
            <AgentProvider value={agent.access}>
              <ComposerProvider value={composer.access}>
                <WorkspaceProvider value={workspace.access}>
                  <WorkingDirectoryProvider value={directory.access}>
                    <ImageSaveProvider saveImage={(source) => host.saveImage(source)}>
                      <NotificationProvider value={notifications.access}>
                        <InstalledPluginsProvider value={installedPlugins.access}>
                          <App plugins={plugins} navigation={navigation} />
                        </InstalledPluginsProvider>
                      </NotificationProvider>
                    </ImageSaveProvider>
                  </WorkingDirectoryProvider>
                </WorkspaceProvider>
              </ComposerProvider>
            </AgentProvider>
          </RuntimeProvider>
        </ThemeProvider>
      </RootBoundary>,
    );
    return root;
  },
  mountStartupFailure(error) {
    const root = createRoot(document.getElementById("root")!);
    root.render(<StartupFailure scope={FAILURE_SCOPE.startup} error={error} />);
    return root;
  },
  closeConnection: async () => {
    const failures: unknown[] = [];
    for (const dispose of [
      notifications.dispose,
      composer.dispose,
      directory.dispose,
      workspace.dispose,
      agent.dispose,
      navigation.dispose,
      installedPlugins.dispose,
      theme.dispose,
      connection.dispose,
    ]) {
      try {
        await dispose();
      } catch (error) {
        failures.push(error);
      }
    }
    if (failures.length) throw new AggregateError(failures, "Client owner disposal failed");
  },
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
});
