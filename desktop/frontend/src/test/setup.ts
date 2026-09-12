import { afterEach, beforeEach } from "vitest";
import { MotionGlobalConfig } from "motion/react";
import { useConfigStore } from "@/plugins/sdk/config";
import { usePluginErrorStore } from "@/plugins/sdk/errors";
import { useNotificationStore } from "@/plugins/sdk/notifications";
import { resetKernelForTest } from "@/plugins/sdk/testKernel";
import {
  useContextDockStore,
  WorkspaceFileFocus,
} from "@/plugins/builtin/workspace/adapters/contextDockStore";
import { configureNavigator } from "@/lib/navigation";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import { installAgentDefaultSessionPort } from "@/plugins/builtin/agent/adapters/agentDefaultSessionPort";
import { installAgentRuntimeGateway } from "@/plugins/builtin/agent/adapters/agentRuntimeGateway";
import { installAgentStatePorts } from "@/plugins/builtin/agent/adapters/agentStatePorts";
import {
  getAgentSessionLifecycleSnapshot,
  getActiveSessionId,
  subscribeAgentSessionLifecycle,
  subscribeActiveSessionId,
} from "@/plugins/builtin/agent/public/session";
import type { AgentSessions } from "@/plugins/builtin/agent/public/services";
import { installComposerStatePorts } from "@/plugins/builtin/chat/composer/adapters/composerStatePorts";
import {
  installRuntimeCapabilityPort,
  resetRuntimeConnectionForTest,
} from "@/plugins/builtin/runtime/adapters/runtimeConnectionProjection";
import { installWorkspaceNavigationPort } from "@/plugins/builtin/workspace/adapters/navigationStatePort";

const testAgentSessions: AgentSessions = {
  getActiveSessionId,
  getLifecycleSnapshot: getAgentSessionLifecycleSnapshot,
  subscribeActiveSessionId,
  subscribeLifecycle: subscribeAgentSessionLifecycle,
};

configureNavigator(createMemoryNavigator());
installAgentStatePorts();
installAgentDefaultSessionPort();
installAgentRuntimeGateway();
installComposerStatePorts(testAgentSessions);
installWorkspaceNavigationPort();
installRuntimeCapabilityPort();

MotionGlobalConfig.skipAnimations = true;

beforeEach(async () => {
  await resetKernelForTest();
  configureNavigator(createMemoryNavigator());
  installAgentStatePorts();
  installAgentDefaultSessionPort();
  installAgentRuntimeGateway();
  installComposerStatePorts(testAgentSessions);
  installWorkspaceNavigationPort();
  resetRuntimeConnectionForTest();
  installRuntimeCapabilityPort();
  usePluginErrorStore.setState({ log: [] });
  useNotificationStore.setState({ log: [] });
  useConfigStore.setState({ values: new Map(), subscribers: new Map() });
  useContextDockStore.setState({
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
