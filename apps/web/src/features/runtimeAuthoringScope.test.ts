import { afterEach, describe, expect, it, vi } from "vitest";
import { stubInstance } from "@/test/stubInstance";
import { type Navigator } from "@/lib/navigation";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import { createRuntimeEndpoint } from "@/features/connection/application/runtimeEndpoint";
import { createRuntimeConnectionProjection } from "@/features/connection/adapters/runtimeConnectionProjection";
import { type RuntimeServerScope } from "@/features/connection/public/services";
import { createAgent, type AgentOwner } from "@/features/agent/adapters/agent";
import {
  createComposer,
  type ComposerOwner,
} from "@/features/composer/adapters/composerStatePorts";
import { createWorkspace, type WorkspaceOwner } from "@/features/workspace/adapters/workspace";
import { createBrowserHost } from "@/platform/browserHost";
import { createAgentSessionStore } from "@/features/agent/adapters/agentSessionStore";
import { createComposerStore } from "@/features/composer/adapters/composerStore";
import { createContextDockStore } from "@/features/workspace/adapters/contextDockStore";
import * as imageInput from "@/features/composer/public/input";
import { bindWorkspaceSessionNavigation } from "@/features/workspace/application/sessionNavigationSync";
import {
  activateWorkspaceSessionScope,
  adoptWorkspaceSessionScope,
  forgetWorkspaceSessionScopes,
  openWorkspaceFile,
} from "@/features/workspace/public/navigation";
import {
  closeActiveAgentSession,
  getActiveSessionId,
  getAgentSessionLifecycleSnapshot,
  selectAgentSession,
  subscribeActiveSessionId,
  subscribeAgentSessionLifecycle,
  subscribeDeletedAgentSession,
} from "@/features/agent/public/session";
import { type AgentRuntimeGateway } from "@/features/agent/application/runtimeGateway";

let testClient: {
  navigation: Navigator;
  agent: AgentOwner;
  composer: ComposerOwner;
  workspace: WorkspaceOwner;
};

const disposers: (() => void)[] = [];

const composer = () => testClient.composer.store.getState();

const refs = () => testClient.agent.sessions.store.getState();

const dock = () => testClient.workspace.dock.store.getState();

afterEach(() => {
  for (const dispose of disposers.splice(0).reverse()) dispose();
  vi.restoreAllMocks();
});

function installTargets(name: string, navigation: Navigator = createMemoryNavigator()) {
  const targetA = `https://${name}-a.test`;
  const targetB = `https://${name}-b.test`;
  let endpoint = targetA;
  const callbacks = new Set<() => void>();
  const scope: RuntimeServerScope = {
    subscribeReplacement(callback) {
      callbacks.add(callback);
      return () => callbacks.delete(callback);
    },
  };
  const projection = createRuntimeConnectionProjection();
  const runtime = {
    capabilities: projection.capabilities,
    status: projection.status,
    endpoint: createRuntimeEndpoint({
      read: () => ({ endpoint }),
      defaultTarget: () => ({ endpoint: targetA }),
      replace(target) {
        endpoint = target.endpoint;
        for (const callback of callbacks) callback();
      },
    }),
  };
  const client = () => {
    throw new Error("Authoring test does not perform remote operations");
  };
  const stream = {
    connectionGeneration: () => projection.store.getState().connectionGeneration,
    subscribeConnection: (listener: () => void) => projection.store.subscribe(listener),
    reportConnectionLoss: () => undefined,
  };
  const agent = createAgent(client, navigation, runtime, stream, scope);
  const sessions = {
    getActiveSessionId: () => getActiveSessionId(agent.access),
    getLifecycleSnapshot: () => getAgentSessionLifecycleSnapshot(agent.access),
    subscribeActiveSessionId: (listener: (id: string) => void) =>
      subscribeActiveSessionId(agent.access, listener),
    subscribeLifecycle: (listener: Parameters<typeof subscribeAgentSessionLifecycle>[1]) =>
      subscribeAgentSessionLifecycle(agent.access, listener),
    subscribeDeleted: (listener: (id: string) => void) =>
      subscribeDeletedAgentSession(agent.access, listener),
  };
  const composer = createComposer(sessions, () => endpoint, scope);
  const workspace = createWorkspace(
    client,
    navigation,
    createBrowserHost(),
    () => false,
    agent.access,
    runtime,
  );
  testClient = { navigation, agent, composer, workspace };
  disposers.push(
    () => projection.dispose(),
    () => agent.dispose(),
    () => composer.dispose(),
    () => workspace.dispose(),
  );
  const deleteSession = vi.fn().mockResolvedValue(undefined);
  disposers.push(
    stubInstance(agent.access.runtime, { deleteSession } as unknown as AgentRuntimeGateway),
  );
  agent.start();
  composer.start();
  disposers.push(
    bindWorkspaceSessionNavigation({
      ...sessions,
      adoptSessionScope: (id) => adoptWorkspaceSessionScope(workspace.access, id),
      activateSessionScope: (id) => activateWorkspaceSessionScope(workspace.access, id),
      forgetSessionScopes: (ids) => forgetWorkspaceSessionScopes(workspace.access, ids),
    }),
  );
  return {
    targetA,
    targetB,
    deleteSession,
    replace(target: string) {
      endpoint = target;
      for (const callback of callbacks) callback();
    },
  };
}

describe("Runtime target authoring ownership", () => {
  it("preserves each target's drafts and file context when Session IDs coincide", () => {
    const targets = installTargets("roundtrip");
    selectAgentSession(testClient.agent.access, "shared");
    composer().setValue("history on A");
    composer().captureSubmission().consume("history on A");
    composer().setValue("draft on A");
    composer().addImages([{ mime: "image/png", data: "image-on-A" }]);
    composer().addPaste("paste on A");
    openWorkspaceFile(testClient.workspace.access, "src/only-on-A.ts", 12);
    testClient.navigation.go({ view: "files", subagent: "child-on-A", settings: "connection" });
    const keyA = testClient.composer.store.persist.getOptions().name!;
    const storedA = localStorage.getItem(keyA);
    targets.deleteSession.mockClear();

    targets.replace(targets.targetB);

    expect(testClient.navigation.get()).toEqual({
      session: "",
      view: null,
      dock: null,
      subagent: null,
      settings: "connection",
    });
    expect(refs().openSessionIds).toEqual([]);
    expect(composer().composer.draft).toMatchObject({ value: "", images: [], pastes: [] });
    expect(dock().fileViewer).toBeNull();
    expect(targets.deleteSession).not.toHaveBeenCalled();
    expect(localStorage.getItem(keyA)).toBe(storedA);

    selectAgentSession(testClient.agent.access, "shared");
    composer().setValue("draft on B");
    openWorkspaceFile(testClient.workspace.access, "src/only-on-B.ts", 24);
    const keyB = testClient.composer.store.persist.getOptions().name!;
    expect(keyB).not.toBe(keyA);
    targets.deleteSession.mockClear();

    targets.replace(targets.targetA);

    expect(targets.deleteSession).not.toHaveBeenCalled();
    expect(refs().openSessionIds).toEqual(["shared"]);
    selectAgentSession(testClient.agent.access, "shared");
    expect(composer().composer.draft).toMatchObject({
      value: "draft on A",
      images: [{ mime: "image/png", data: "image-on-A" }],
      pastes: [{ text: "paste on A" }],
    });
    expect(composer().historyPrev()).toBe(true);
    expect(composer().composer.draft.value).toBe("history on A");
    expect(dock().fileViewer).toEqual({ path: "src/only-on-A.ts", line: 12 });

    targets.replace(targets.targetB);
    selectAgentSession(testClient.agent.access, "shared");
    expect(composer().composer.draft.value).toBe("draft on B");
    expect(composer().composer.draft.images).toEqual([]);
    expect(dock().fileViewer).toEqual({ path: "src/only-on-B.ts", line: 24 });
  });

  it("keeps authoring and image work for replacement at the same endpoint", async () => {
    const targets = installTargets("same-endpoint");
    selectAgentSession(testClient.agent.access, "shared");
    composer().setValue("retained during token refresh or process restart");
    const reading = deferred<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading.promise);
    composer().addImageFiles([new File([""], "pending.png")]);
    const key = testClient.composer.store.persist.getOptions().name!;

    targets.replace(targets.targetA);
    reading.resolve({ mime: "image/png", data: "accepted", name: "pending.png" });
    await vi.waitFor(() => expect(composer().composer.draft.images).toHaveLength(1));

    expect(getActiveSessionId(testClient.agent.access)).toBe("shared");
    expect(composer().composer.draft.value).toBe(
      "retained during token refresh or process restart",
    );
    expect(testClient.composer.store.persist.getOptions().name).toBe(key);
  });

  it("retires delayed image results even after switching away and back to the same Session", async () => {
    const targets = installTargets("delayed-image");
    selectAgentSession(testClient.agent.access, "shared");
    const reading = deferred<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading.promise);
    composer().addImageFiles([new File([""], "pending.png")]);

    targets.replace(targets.targetB);
    selectAgentSession(testClient.agent.access, "shared");
    targets.replace(targets.targetA);
    selectAgentSession(testClient.agent.access, "shared");
    reading.resolve({ mime: "image/png", data: "retired", name: "pending.png" });
    await reading.promise;
    await Promise.resolve();
    await Promise.resolve();

    expect(composer().composer.draft.images).toEqual([]);
  });

  it("does not delete a successor Session's authoring when the router delivers target cleanup later", () => {
    const memory = createMemoryNavigator();
    const pending: (() => void)[] = [];
    const delayed: Navigator = {
      ...memory,
      go(patch, options) {
        if (patch.session === "" && options?.replace) {
          pending.push(() => memory.go(patch, options));
        } else memory.go(patch, options);
      },
    };
    const targets = installTargets("delayed-route", delayed);
    selectAgentSession(testClient.agent.access, "shared");
    targets.deleteSession.mockClear();

    targets.replace(targets.targetB);
    refs().holdOpen("shared");
    memory.go({ settings: "connection" });
    expect(getActiveSessionId(testClient.agent.access)).toBe("shared");
    for (const navigate of pending.splice(0)) navigate();

    expect(getActiveSessionId(testClient.agent.access)).toBe("");
    expect(targets.deleteSession).not.toHaveBeenCalled();
    selectAgentSession(testClient.agent.access, "shared");
    composer().setValue("important unsent input");
    composer().addPaste("unsent source text");
    composer().addImages([{ mime: "image/png", data: "unsent-image" }]);
    const draft = composer().composer.draft;
    targets.deleteSession.mockClear();
    selectAgentSession(testClient.agent.access, "another");
    expect(targets.deleteSession).not.toHaveBeenCalled();
    expect(refs().openSessionIds).toContain("shared");
    selectAgentSession(testClient.agent.access, "shared");
    expect(composer().composer.draft).toEqual(draft);
    expect(closeActiveAgentSession(testClient.agent.access)).toBe(true);
    expect(targets.deleteSession).not.toHaveBeenCalled();
    selectAgentSession(testClient.agent.access, "shared");
    expect(composer().composer.draft).toEqual(draft);
    testClient.agent.access.state.deleteSession("shared");
    selectAgentSession(testClient.agent.access, "shared");
    expect(composer().composer.draft).toMatchObject({ value: "", images: [], pastes: [] });
    expect(composer().composer.durableDraftTexts().has("shared")).toBe(false);
  });

  it("hydrates only the restored endpoint after renderer replacement and discards unscoped state", async () => {
    const targets = installTargets("cold-renderer");
    selectAgentSession(testClient.agent.access, "shared");
    composer().setValue("cold draft on A");
    openWorkspaceFile(testClient.workspace.access, "src/A.ts", 1);
    const unscopedAgent = localStorage.getItem(
      testClient.agent.sessions.store.persist.getOptions().name!,
    )!;
    const unscopedComposer = localStorage.getItem(
      testClient.composer.store.persist.getOptions().name!,
    )!;
    const unscopedDock = localStorage.getItem(
      testClient.workspace.dock.store.persist.getOptions().name!,
    )!;
    targets.replace(targets.targetB);
    selectAgentSession(testClient.agent.access, "shared");
    composer().setValue("cold draft on B");
    openWorkspaceFile(testClient.workspace.access, "src/B.ts", 2);
    localStorage.setItem("flame.agent-session", unscopedAgent);
    localStorage.setItem("flame.composer", unscopedComposer);
    localStorage.setItem("flame.context-dock", unscopedDock);

    const agent = createAgentSessionStore();
    const input = createComposerStore();
    const workspace = createContextDockStore();
    disposers.push(() => input.dispose());
    expect(agent.store.persist.hasHydrated()).toBe(false);
    expect(input.store.persist.hasHydrated()).toBe(false);
    expect(workspace.store.persist.hasHydrated()).toBe(false);
    expect(agent.store.getState().openSessionIds).toEqual([]);
    expect(input.store.getState().composer.draft.value).toBe("");

    agent.activate(targets.targetB);
    input.activate(targets.targetB);
    workspace.activate(targets.targetB);
    input.store.getState().loadSession("shared");
    workspace.store.getState().activateSessionScope("shared");

    expect(agent.store.getState().openSessionIds).toEqual(["shared"]);
    expect(input.store.getState().composer.draft.value).toBe("cold draft on B");
    expect(workspace.store.getState().fileViewer).toEqual({
      path: "src/B.ts",
      line: 2,
    });
    expect(localStorage.getItem("flame.agent-session")).toBeNull();
    expect(localStorage.getItem("flame.composer")).toBeNull();
    expect(localStorage.getItem("flame.context-dock")).toBeNull();
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}
