import { afterEach, beforeEach } from "vitest";
import { MotionGlobalConfig } from "motion/react";
import { useConfigStore } from "@/extensions/config";
import { usePluginErrorStore } from "@/extensions/errors";
import { useNotificationStore } from "@/features/notifications/public/notifications";
import { resetKernelForTest } from "@/test/kernel";
import {
  useContextDockStore,
  WorkspaceFileFocus,
} from "@/features/workspace/adapters/contextDockStore";
import { configureNavigator } from "@/lib/navigation";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import { installAgentDefaultSessionPort } from "@/features/agent/adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "@/features/agent/adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "@/features/agent/adapters/agentStatePorts";
import { activateAgentSessionStorage } from "@/features/agent/adapters/agentSessionStore";
import {
  getAgentSessionLifecycleSnapshot,
  getActiveSessionId,
  subscribeAgentSessionLifecycle,
  subscribeDeletedAgentSession,
  subscribeActiveSessionId,
} from "@/features/agent/public/session";
import { type AgentSessions } from "@/features/agent/public/services";
import { installComposerStatePorts } from "@/features/composer/adapters/composerStatePorts";
import {
  installRuntimeCapabilityPort,
  resetRuntimeConnectionForTest,
} from "@/features/connection/adapters/runtimeConnectionProjection";
import { installWorkspaceNavigationPort } from "@/features/workspace/adapters/navigationStatePort";
const testAgentSessions: AgentSessions = {
  getActiveSessionId,
  getLifecycleSnapshot: getAgentSessionLifecycleSnapshot,
  subscribeActiveSessionId,
  subscribeLifecycle: subscribeAgentSessionLifecycle,
  subscribeDeleted: subscribeDeletedAgentSession,
};

const unavailableRuntime = (): never => {
  throw new Error("Runtime test client is not configured");
};

const testEndpoint = () => "https://runtime.test";

const testScope = { subscribeReplacement: () => () => undefined };

configureNavigator(createMemoryNavigator());

activateAgentSessionStorage(testEndpoint());

installAgentStatePorts();

installAgentDefaultSessionPort(unavailableRuntime, () => undefined);

installAgentRuntimeGateway(unavailableRuntime);

installComposerStatePorts(testAgentSessions, testEndpoint, testScope);

installWorkspaceNavigationPort(testEndpoint);

installRuntimeCapabilityPort();

MotionGlobalConfig.skipAnimations = true;

beforeEach(async () => {
  await resetKernelForTest();
  configureNavigator(createMemoryNavigator());
  activateAgentSessionStorage(testEndpoint());
  installAgentStatePorts();
  installAgentDefaultSessionPort(unavailableRuntime, () => undefined);
  installAgentRuntimeGateway(unavailableRuntime);
  installComposerStatePorts(testAgentSessions, testEndpoint, testScope);
  installWorkspaceNavigationPort(testEndpoint);
  resetRuntimeConnectionForTest();
  installRuntimeCapabilityPort();
  usePluginErrorStore.setState({ log: [] });
  useNotificationStore.setState({ log: [] });
  useConfigStore.setState({ values: new Map(), subscribers: new Map() });
  useContextDockStore.setState({
    inspection: null,
    activeSessionScopeId: null,
    sessionScopes: new Map(),
    dockViewIds: [],
    lastViewId: null,
    fileFocus: WorkspaceFileFocus.empty(),
    fileViewer: null,
    expandedToolIds: new Set<string>(),
  });
});

afterEach(() => {
  try {
    localStorage.clear();
  } catch {}
});
