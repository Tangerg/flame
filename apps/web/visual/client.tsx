import { createProviders } from "@/features/providers/adapters/providers";
import { ProvidersProvider } from "@/features/providers/public/providers";
import { IMAGE_FILENAME_STEM } from "@/app/product";
import { ApplicationCalloutLabels } from "@/app/calloutLabels";
import { createImageExport } from "@/foundation/imageExport";
import { type ReactNode } from "react";
import { type FlameClient } from "@flame/runtime-client";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import { createBrowserHost } from "@/platform/browserHost";
import { createRuntimeConnectionProjection } from "@/features/connection/adapters/runtimeConnectionProjection";
import {
  createRuntimeEndpoint,
  type RuntimeEndpointConfiguration,
} from "@/features/connection/application/runtimeEndpoint";
import { RuntimeProvider, type RuntimeAccess } from "@/features/connection/public/runtime";
import { createAgentFixture } from "@/test/agentFixture";
import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { submitAgentInput } from "@/features/agent/application/input/submitAgentInput";
import { type AgentInput } from "@/features/agent/public/input";
import { useStopCurrentRootRun } from "@/features/agent/public/run";
import { createComposer } from "@/features/composer/adapters/composerStatePorts";
import { ComposerProvider } from "@/features/composer/public/composer";
import { createWorkspace } from "@/features/workspace/adapters/workspace";
import { WorkspaceProvider } from "@/features/workspace/public/workspace";
import { createWorkingDirectoryPicker } from "@/features/navigation/adapters/workingDirectorySelection";
import { WorkingDirectoryProvider } from "@/features/navigation/public/workingDirectory";
import { ImageSaveProvider } from "@/features/chat/public/imageSave";
import { createNotifications } from "@/features/notifications/adapters/notifications";
import { NotificationProvider } from "@/features/notifications/public/notifier";
import { createInstalledPlugins } from "@/features/plugins/adapters/installedPlugins";
import { InstalledPluginsProvider } from "@/features/plugins/context";
import { createTheme } from "@/features/theme/adapters/theme";
import { ThemeProvider } from "@/features/theme/public/theme";
import { VISUAL_RUNTIME_GENERATION } from "./agentFixtureFacts";

function useRecordedChatSession() {
  const agent = useAgentDependencies();
  const stop = useStopCurrentRootRun();
  return {
    send: (input: AgentInput) => submitAgentInput(agent, agent.state.getActiveSessionId(), input),
    stop: () => stop?.(),
  };
}

/** One browser fixture owns these feature instances; production has no default instance. */
export function createVisualClient(runtimeClient: () => FlameClient) {
  const host = createBrowserHost();
  const saveImage = createImageExport(IMAGE_FILENAME_STEM, host.saveFile);
  const theme = createTheme(document, window);
  const providers = createProviders();
  const navigation = createMemoryNavigator();
  const projection = createRuntimeConnectionProjection();
  projection.store.setState({ connectionGeneration: VISUAL_RUNTIME_GENERATION });
  const replacements = new Set<() => void>();
  const scope = {
    subscribeReplacement(listener: () => void) {
      replacements.add(listener);
      return () => {
        replacements.delete(listener);
      };
    },
  };
  let configuration: RuntimeEndpointConfiguration = {
    read: () => ({ endpoint: "https://visual.flame.test" }),
    defaultTarget: () => ({ endpoint: "https://visual.flame.test" }),
    replace() {
      throw new Error("Visual Runtime endpoint is not bound");
    },
    reset() {
      throw new Error("Visual Runtime endpoint is not bound");
    },
  };
  const runtime: RuntimeAccess = {
    capabilities: projection.capabilities,
    status: projection.status,
    endpoint: createRuntimeEndpoint({
      read: () => configuration.read(),
      defaultTarget: () => configuration.defaultTarget(),
      replace: (next) => {
        if (!configuration.replace(next)) return false;
        for (const listener of replacements) listener();
        return true;
      },
      reset: () => {
        if (!configuration.reset()) return false;
        for (const listener of replacements) listener();
        return true;
      },
    }),
  };
  const stream = {
    connectionGeneration: () => projection.store.getState().connectionGeneration,
    subscribeConnection: (listener: () => void) => projection.store.subscribe(listener),
    reportConnectionLoss: () => undefined,
  };
  const agent = createAgentFixture(runtimeClient, navigation, runtime, stream, scope);
  agent.dependencies = {
    ...agent.dependencies,
    defaultSession: { useDefaultChatSession: useRecordedChatSession },
  };
  agent.start();
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
  const workspace = createWorkspace(
    runtimeClient,
    navigation,
    host,
    () => false,
    agent.access,
    runtime,
  );
  workspace.dock.activate(runtime.endpoint.currentRuntimeEndpoint());
  const directory = createWorkingDirectoryPicker(
    async () => "/Users/visual/selected-project",
    () => true,
    runtime.endpoint.currentRuntimeEndpoint,
  );
  const notifications = createNotifications(host, agent.access, workspace.access, "Flame");
  const installedPlugins = createInstalledPlugins(
    runtimeClient,
    runtime.endpoint.currentRuntimeEndpoint,
    theme.access,
    (request) => host.saveFile(request),
    (children) => <Wrapper>{children}</Wrapper>,
  );
  function Wrapper({ children }: { children: ReactNode }) {
    return (
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
    );
  }
  return {
    host,
    theme,
    providers,
    navigation,
    projection,
    runtime,
    stream,
    scope,
    agent,
    composer,
    workspace,
    directory,
    notifications,
    installedPlugins,
    Wrapper,
    bindEndpoint(next: RuntimeEndpointConfiguration) {
      configuration = next;
    },
    async dispose() {
      providers.dispose();
      notifications.dispose();
      composer.dispose();
      directory.dispose();
      workspace.dispose();
      agent.dispose();
      projection.dispose();
      theme.dispose();
      replacements.clear();
      await installedPlugins.dispose();
    },
  };
}
export type VisualClient = ReturnType<typeof createVisualClient>;
