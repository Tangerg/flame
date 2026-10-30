import { HookTrustMutationOwner } from "@/features/hooks/application/hookTrust";
import { HookTrustProvider } from "@/features/hooks/public/trust";
import { createGoals } from "@/features/goal/application/goals";
import { GoalProvider } from "@/features/goal/public/goals";
import { MCPServerMutationOwner } from "@/features/mcp/application/mcpServerMutationOwner";
import { MCPServersProvider } from "@/features/mcp/public/servers";
import { createProviders } from "@/features/providers/adapters/providers";
import { ProvidersProvider } from "@/features/providers/public/providers";
import { IMAGE_FILENAME_STEM } from "@/app/product";
import { ApplicationCalloutLabels } from "@/app/calloutLabels";
import { type createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
import { type ClientHost } from "@/platform/host";
import { createTheme } from "@/features/theme/adapters/theme";
import { ThemeProvider } from "@/features/theme/public/theme";
import { createNotifications } from "@/features/notifications/adapters/notifications";
import { NotificationProvider } from "@/features/notifications/public/notifier";
import { createInstalledPlugins } from "@/features/plugins/adapters/installedPlugins";
import { InstalledPluginsProvider } from "@/features/plugins/context";
import { type ComponentType, type ReactNode } from "react";
import {
  render as renderReact,
  renderHook as renderReactHook,
  type RenderHookOptions,
  type RenderOptions,
} from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { type FlameClient } from "@flame/runtime-client";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import { queryClient } from "@/lib/queryClient";
import { createRuntimeConnectionProjection } from "@/features/connection/adapters/runtimeConnectionProjection";
import { createRuntimeEndpoint } from "@/features/connection/application/runtimeEndpoint";
import { RuntimeProvider, type RuntimeAccess } from "@/features/connection/public/runtime";
import { createAgentFixture } from "@/test/agentFixture";
import { createAgentViewSelectors } from "@/features/agent/adapters/agentViewSelectors";
import { createComposer } from "@/features/composer/adapters/composerStatePorts";
import { ComposerProvider } from "@/features/composer/public/composer";
import { createWorkspace } from "@/features/workspace/adapters/workspace";
import { WorkspaceProvider } from "@/features/workspace/public/workspace";
import { createWorkingDirectoryPicker } from "@/features/navigation/adapters/workingDirectorySelection";
import { WorkingDirectoryProvider } from "@/features/navigation/public/workingDirectory";
import { ImageSaveProvider } from "@/features/chat/public/imageSave";
import { createImageExport, type ImageExport } from "@/foundation/imageExport";
import { createBrowserHost } from "@/platform/browserHost";

export function createTestClient(
  options: {
    runtimeClient?: () => FlameClient;
    connection?: ReturnType<typeof createRuntimeConnection>;
    host?: ClientHost;
    start?: boolean;
    endpoint?: string;
    saveImage?: ImageExport;
  } = {},
) {
  const theme = createTheme(document, window);
  const providers = createProviders();
  const mcpServers = new MCPServerMutationOwner(null);
  const hookTrust = new HookTrustMutationOwner(null);
  const navigation = createMemoryNavigator();
  const projection = createRuntimeConnectionProjection();
  const replacements = new Set<() => void>();
  const scope = options.connection?.scope ?? {
    subscribeReplacement(listener: () => void) {
      replacements.add(listener);
      return () => {
        replacements.delete(listener);
      };
    },
  };
  let target = {
    endpoint: options.endpoint ?? "https://runtime.test",
    localToken: undefined as string | undefined,
  };
  const original = target;
  const runtime: RuntimeAccess = options.connection?.access ?? {
    capabilities: projection.capabilities,
    status: projection.status,
    endpoint: createRuntimeEndpoint({
      read: () => target,
      defaultTarget: () => original,
      replace(next) {
        if (target.endpoint === next.endpoint && target.localToken === next.localToken) {
          return false;
        }
        target = { ...next, localToken: next.localToken };
        for (const listener of replacements) listener();
        return true;
      },
      reset() {
        return this.replace(original);
      },
    }),
  };
  const stream = options.connection?.stream ?? {
    connectionGeneration: () => projection.store.getState().connectionGeneration,
    subscribeConnection: (listener: () => void) => projection.store.subscribe(listener),
    reportConnectionLoss: () => undefined,
  };
  const client =
    options.connection?.client ??
    options.runtimeClient ??
    (() => {
      throw new Error("Runtime test client is not configured");
    });
  const agent = createAgentFixture(client, navigation, runtime, stream, scope);
  if (options.start !== false) agent.start();
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
    runtime.endpoint.currentRuntimeEndpoint,
    scope,
  );
  if (options.start !== false) composer.start();
  const host = options.host ?? createBrowserHost();
  const saveImage = options.saveImage ?? createImageExport(IMAGE_FILENAME_STEM, host.saveFile);
  const workspace = createWorkspace(client, navigation, host, () => false, agent.access, runtime);
  workspace.dock.activate(runtime.endpoint.currentRuntimeEndpoint());
  const directory = createWorkingDirectoryPicker(
    () => Promise.resolve(null),
    () => false,
    runtime.endpoint.currentRuntimeEndpoint,
  );
  const notifications = createNotifications(host, agent.access, workspace.access, "Flame");
  const installedPlugins = createInstalledPlugins(
    client,
    runtime.endpoint.currentRuntimeEndpoint,
    theme.access,
    (request) => host.saveFile(request),
    (children) => <Wrapper>{children}</Wrapper>,
  );
  const selectors = createAgentViewSelectors(agent.projection.store, navigation);
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <HookTrustProvider value={hookTrust}>
          <GoalProvider value={goals.access}>
            <MCPServersProvider value={mcpServers}>
              <ProvidersProvider value={providers.access}>
                <ThemeProvider value={theme.access}>
                  <RuntimeProvider value={runtime}>
                    <agent.Provider>
                      <ComposerProvider value={composer.access}>
                        <WorkspaceProvider value={workspace.access}>
                          <WorkingDirectoryProvider value={directory.access}>
                            <ImageSaveProvider saveImage={saveImage}>
                              <NotificationProvider value={notifications.access}>
                                <InstalledPluginsProvider value={installedPlugins.access}>
                                  <ApplicationCalloutLabels>{children}</ApplicationCalloutLabels>
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
        </HookTrustProvider>
      </QueryClientProvider>
    );
  }
  function withWrapper(Additional?: ComponentType<{ children: ReactNode }>) {
    return Additional
      ? ({ children }: { children: ReactNode }) => (
          <Wrapper>
            <Additional>{children}</Additional>
          </Wrapper>
        )
      : Wrapper;
  }
  return {
    theme,
    providers,
    mcpServers,
    hookTrust,
    goals,
    navigation,
    runtime,
    projection,
    stream,
    scope,
    agent,
    composer,
    workspace,
    directory,
    selectors,
    notifications,
    installedPlugins,
    Wrapper,
    get sessionDependencies() {
      return {
        agentAccess: agent.dependencies,
        projection: agent.projection,
        sessions: agent.sessions,
        retries: agent.retries,
      };
    },
    render(ui: ReactNode, renderOptions?: RenderOptions) {
      return renderReact(ui, { ...renderOptions, wrapper: withWrapper(renderOptions?.wrapper) });
    },
    renderHook<Result, Props>(
      callback: (props: Props) => Result,
      renderOptions?: RenderHookOptions<Props>,
    ) {
      return renderReactHook(callback, {
        ...renderOptions,
        wrapper: withWrapper(renderOptions?.wrapper),
      });
    },
    async dispose() {
      hookTrust.dispose();
      goals.dispose();
      mcpServers.dispose();
      providers.dispose();
      notifications.dispose();
      theme.dispose();
      composer.dispose();
      directory.dispose();
      workspace.dispose();
      agent.dispose();
      projection.dispose();
      replacements.clear();
      queryClient.clear();
      await installedPlugins.dispose();
    },
  };
}
export type TestClient = ReturnType<typeof createTestClient>;
