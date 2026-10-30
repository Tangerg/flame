import { createGoals } from "@/features/goal/application/goals";
import { GoalProvider } from "@/features/goal/public/goals";
import { MCPServerMutationOwner } from "@/features/mcp/application/mcpServerMutationOwner";
import { MCPServersProvider } from "@/features/mcp/public/servers";
import { createProviders } from "@/features/providers/adapters/providers";
import { ProvidersProvider } from "@/features/providers/public/providers";
import { createTheme } from "@/features/theme/adapters/theme";
import { ThemeProvider } from "@/features/theme/public/theme";
import { createNotifications } from "@/features/notifications/adapters/notifications";
import { NotificationProvider } from "@/features/notifications/public/notifier";
import { createInstalledPlugins } from "@/features/plugins/adapters/installedPlugins";
import { InstalledPluginsProvider } from "@/features/plugins/context";
import { IMAGE_FILENAME_STEM, PRODUCT_NAME } from "@/app/product";
import { createAgent } from "@/features/agent/adapters/agent";
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
import { type ReactNode } from "react";
import { ApplicationSurface } from "@/app/ApplicationSurface";
import App from "@/App";
import { createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
import { createBuiltinPlugins } from "@/app/builtinPlugins";
import { createClientHost } from "@/platform/clientHost";
import { createImageExport } from "@/foundation/imageExport";
import { ClientRenderer } from "@/app/renderer";
import { FAILURE_SCOPE, RootBoundary, StartupFailure } from "@/app/StartupFailure";
import { applyWindowChrome, watchWindowChrome } from "@/app/windowChrome";
import { disposeOnHmr } from "@/lib/hmr";
import "@/styles/markdown.css";
import "@/styles/overlays.css";
import "@/styles/globals.css";
const host = createClientHost();
const saveImage = createImageExport(IMAGE_FILENAME_STEM, host.saveFile);
const theme = createTheme(document, window);
const providers = createProviders();
const mcpServers = new MCPServerMutationOwner(null);

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
const goals = createGoals((sessionId) =>
  agent.access.synchronizeMountedAgentSession(sessionId, "replace-live"),
);
const composer = createComposer(
  {
    getActiveSessionId: agent.access.getActiveSessionId,
    getLifecycleSnapshot: agent.access.getAgentSessionLifecycleSnapshot,
    subscribeActiveSessionId: agent.access.subscribeActiveSessionId,
    subscribeLifecycle: agent.access.subscribeAgentSessionLifecycle,
    subscribeDeleted: agent.access.subscribeDeletedAgentSession,
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
  (request) => host.saveFile(request),
  (children) => (
    <ApplicationContext>
      <ApplicationSurface>{children}</ApplicationSurface>
    </ApplicationContext>
  ),
);
const plugins = createBuiltinPlugins(
  connection,
  saveImage,
  navigation.navigator,
  agent,
  composer,
  workspace,
  directory,
  notifications,
  installedPlugins,
  theme,
  providers,
  mcpServers,
  goals,
);

function ApplicationContext({ children }: { children: ReactNode }) {
  return (
    <GoalProvider value={goals.access}>
      <MCPServersProvider value={mcpServers}>
        <ProvidersProvider value={providers.access}>
          <ThemeProvider value={theme.access}>
            <RuntimeProvider value={connection.access}>
              <agent.Provider>
                <ComposerProvider value={composer.access}>
                  <WorkspaceProvider value={workspace.access}>
                    <WorkingDirectoryProvider value={directory.access}>
                      <ImageSaveProvider saveImage={saveImage}>
                        <NotificationProvider value={notifications.access}>
                          <InstalledPluginsProvider value={installedPlugins.access}>
                            {children}
                          </InstalledPluginsProvider>
                        </NotificationProvider>
                      </ImageSaveProvider>
                    </WorkingDirectoryProvider>
                  </WorkspaceProvider>
                </ComposerProvider>
              </agent.Provider>
            </RuntimeProvider>
          </ThemeProvider>
        </ProvidersProvider>
      </MCPServersProvider>
    </GoalProvider>
  );
}

const renderer = new ClientRenderer({
  initializeClientHost: connection.initialize,
  prepareWindowChrome: () => applyWindowChrome(host),
  watchWindowChrome: () => watchWindowChrome(host),
  mount() {
    const container = document.getElementById("root");
    const root = createRoot(container!);
    root.render(
      <RootBoundary reload={installedPlugins.reload}>
        <ApplicationContext>
          <App plugins={plugins} navigation={navigation} />
        </ApplicationContext>
      </RootBoundary>,
    );
    return root;
  },
  mountStartupFailure(error) {
    const root = createRoot(document.getElementById("root")!);
    root.render(
      <StartupFailure
        scope={FAILURE_SCOPE.startup}
        error={error}
        reload={installedPlugins.reload}
      />,
    );
    return root;
  },
  closeConnection: async () => {
    const failures: unknown[] = [];
    for (const dispose of [
      goals.dispose,
      () => mcpServers.dispose(),
      providers.dispose,
      notifications.dispose,
      composer.dispose,
      directory.dispose,
      workspace.dispose,
      agent.dispose,
      navigation.dispose,
      installedPlugins.dispose,
      theme.dispose,
    ]) {
      try {
        await dispose();
      } catch (error) {
        failures.push(error);
      }
    }
    if (failures.length) throw new AggregateError(failures, "Client owner disposal failed");
    // The control channel must remain usable while any original plugin exit or
    // acknowledgement is unconfirmed. A later controlled reload retries it.
    await connection.dispose();
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
