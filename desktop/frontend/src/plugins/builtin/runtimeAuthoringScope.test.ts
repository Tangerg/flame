import { afterEach, describe, expect, it, vi } from "vitest";
import { configureNavigator, navigator, type Navigator } from "@/lib/navigation";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import * as imageInput from "@/plugins/builtin/chat/composer/public/input";
import { installComposerStatePorts } from "@/plugins/builtin/chat/composer/adapters/composerStatePorts";
import { useComposerStore } from "@/plugins/builtin/chat/composer/adapters/composerStore";
import { useContextDockStore } from "@/plugins/builtin/workspace/adapters/contextDockStore";
import { installWorkspaceNavigationPort } from "@/plugins/builtin/workspace/adapters/navigationStatePort";
import { bindWorkspaceSessionNavigation } from "@/plugins/builtin/workspace/application/sessionNavigationSync";
import {
  activateWorkspaceSessionScope,
  adoptWorkspaceSessionScope,
  forgetWorkspaceSessionScopes,
  openWorkspaceFile,
} from "@/plugins/builtin/workspace/public/navigation";
import type { RuntimeServerScope } from "@/plugins/builtin/runtime/public/services";
import {
  getActiveSessionId,
  getAgentSessionLifecycleSnapshot,
  selectAgentSession,
  subscribeActiveSessionId,
  subscribeAgentSessionLifecycle,
} from "@/plugins/builtin/agent/public/session";
import * as abandoned from "@/plugins/builtin/agent/application/session/discardAbandonedDraft";
import { installAgentSessionScope } from "@/plugins/builtin/agent/adapters/agentSessionScope";
import { useAgentSessionStore } from "@/plugins/builtin/agent/adapters/agentSessionStore";

const sessions = {
  getActiveSessionId,
  getLifecycleSnapshot: getAgentSessionLifecycleSnapshot,
  subscribeActiveSessionId,
  subscribeLifecycle: subscribeAgentSessionLifecycle,
};
const disposers: (() => void)[] = [];
const composer = () => useComposerStore.getState();
const refs = () => useAgentSessionStore.getState();
const dock = () => useContextDockStore.getState();

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
  const discard = vi.spyOn(abandoned, "discardAbandonedDraft").mockImplementation(() => undefined);
  disposers.push(configureNavigator(navigation));
  disposers.push(installAgentSessionScope(scope, () => endpoint));
  disposers.push(installComposerStatePorts(sessions, () => endpoint, scope));
  disposers.push(installWorkspaceNavigationPort(() => endpoint));
  disposers.push(
    bindWorkspaceSessionNavigation({
      ...sessions,
      adoptSessionScope: adoptWorkspaceSessionScope,
      activateSessionScope: activateWorkspaceSessionScope,
      forgetSessionScopes: forgetWorkspaceSessionScopes,
    }),
  );
  return {
    targetA,
    targetB,
    discard,
    replace(target: string) {
      endpoint = target;
      for (const callback of callbacks) callback();
    },
  };
}

describe("Runtime target authoring ownership", () => {
  it("preserves each target's drafts and file context when Session IDs coincide", () => {
    const targets = installTargets("roundtrip");
    selectAgentSession("shared");
    refs().markDraft("shared");
    composer().setValue("draft on A");
    composer().addImages([{ mime: "image/png", data: "image-on-A" }]);
    composer().addPaste("paste on A");
    composer().pushHistory("history on A");
    openWorkspaceFile("src/only-on-A.ts", 12);
    navigator().go({ view: "files", subagent: "child-on-A", settings: "connection" });
    const keyA = useComposerStore.persist.getOptions().name!;
    const storedA = localStorage.getItem(keyA);
    targets.discard.mockClear();

    targets.replace(targets.targetB);

    expect(navigator().get()).toEqual({
      session: "",
      view: null,
      dock: null,
      subagent: null,
      settings: "connection",
    });
    expect(refs().openSessionIds).toEqual([]);
    expect(refs().draftSessionIds).toEqual(new Set());
    expect(composer().composer.draft).toMatchObject({ value: "", images: [], pastes: [] });
    expect(dock().fileViewer).toBeNull();
    expect(targets.discard).not.toHaveBeenCalled();
    expect(localStorage.getItem(keyA)).toBe(storedA);

    selectAgentSession("shared");
    composer().setValue("draft on B");
    openWorkspaceFile("src/only-on-B.ts", 24);
    const keyB = useComposerStore.persist.getOptions().name!;
    expect(keyB).not.toBe(keyA);
    targets.discard.mockClear();

    targets.replace(targets.targetA);

    expect(targets.discard).not.toHaveBeenCalled();
    expect(refs().openSessionIds).toEqual(["shared"]);
    expect(refs().draftSessionIds).toEqual(new Set(["shared"]));
    expect(refs().freshDraftSessionIds).toEqual(new Set(["shared"]));
    selectAgentSession("shared");
    expect(composer().composer.draft).toMatchObject({
      value: "draft on A",
      images: [{ mime: "image/png", data: "image-on-A" }],
      pastes: [{ text: "paste on A" }],
    });
    expect(composer().historyPrev()).toBe(true);
    expect(composer().composer.draft.value).toBe("history on A");
    expect(dock().fileViewer).toEqual({ path: "src/only-on-A.ts", line: 12 });

    targets.replace(targets.targetB);
    selectAgentSession("shared");
    expect(composer().composer.draft.value).toBe("draft on B");
    expect(composer().composer.draft.images).toEqual([]);
    expect(dock().fileViewer).toEqual({ path: "src/only-on-B.ts", line: 24 });
  });

  it("keeps authoring and image work for replacement at the same endpoint", async () => {
    const targets = installTargets("same-endpoint");
    selectAgentSession("shared");
    refs().markDraft("shared");
    composer().setValue("retained during token refresh or process restart");
    const reading = deferred<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading.promise);
    composer().addImageFiles([new File([""], "pending.png")]);
    const key = useComposerStore.persist.getOptions().name!;

    targets.replace(targets.targetA);
    reading.resolve({ mime: "image/png", data: "accepted", name: "pending.png" });
    await vi.waitFor(() => expect(composer().composer.draft.images).toHaveLength(1));

    expect(getActiveSessionId()).toBe("shared");
    expect(refs().draftSessionIds).toEqual(new Set(["shared"]));
    expect(composer().composer.draft.value).toBe(
      "retained during token refresh or process restart",
    );
    expect(useComposerStore.persist.getOptions().name).toBe(key);
  });

  it("retires delayed image results even after switching away and back to the same Session", async () => {
    const targets = installTargets("delayed-image");
    selectAgentSession("shared");
    const reading = deferred<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading.promise);
    composer().addImageFiles([new File([""], "pending.png")]);

    targets.replace(targets.targetB);
    selectAgentSession("shared");
    targets.replace(targets.targetA);
    selectAgentSession("shared");
    reading.resolve({ mime: "image/png", data: "retired", name: "pending.png" });
    await reading.promise;
    await Promise.resolve();
    await Promise.resolve();

    expect(composer().composer.draft.images).toEqual([]);
  });

  it("does not delete a successor draft when the router delivers target cleanup later", () => {
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
    selectAgentSession("shared");
    targets.discard.mockClear();

    targets.replace(targets.targetB);
    refs().holdOpen("shared");
    refs().markDraft("shared");
    memory.go({ settings: "connection" });
    expect(getActiveSessionId()).toBe("shared");
    for (const navigate of pending.splice(0)) navigate();

    expect(getActiveSessionId()).toBe("");
    expect(targets.discard).not.toHaveBeenCalled();
    expect(refs().draftSessionIds.has("shared")).toBe(true);
    selectAgentSession("shared");
    targets.discard.mockClear();
    selectAgentSession("another");
    expect(targets.discard).toHaveBeenCalledWith("shared");
  });

  it("hydrates only the restored endpoint after renderer replacement and discards unscoped state", async () => {
    const targets = installTargets("cold-renderer");
    selectAgentSession("shared");
    composer().setValue("cold draft on A");
    openWorkspaceFile("src/A.ts", 1);
    const unscopedAgent = localStorage.getItem(useAgentSessionStore.persist.getOptions().name!)!;
    const unscopedComposer = localStorage.getItem(useComposerStore.persist.getOptions().name!)!;
    const unscopedDock = localStorage.getItem(useContextDockStore.persist.getOptions().name!)!;
    targets.replace(targets.targetB);
    selectAgentSession("shared");
    refs().markDraft("shared");
    composer().setValue("cold draft on B");
    openWorkspaceFile("src/B.ts", 2);
    localStorage.setItem("flame.agent-session", unscopedAgent);
    localStorage.setItem("flame.composer", unscopedComposer);
    localStorage.setItem("flame.context-dock", unscopedDock);

    vi.resetModules();
    const [agent, input, workspace] = await Promise.all([
      import("@/plugins/builtin/agent/adapters/agentSessionStore"),
      import("@/plugins/builtin/chat/composer/adapters/composerStore"),
      import("@/plugins/builtin/workspace/adapters/contextDockStore"),
    ]);
    expect(agent.useAgentSessionStore.persist.hasHydrated()).toBe(false);
    expect(input.useComposerStore.persist.hasHydrated()).toBe(false);
    expect(workspace.useContextDockStore.persist.hasHydrated()).toBe(false);
    expect(agent.useAgentSessionStore.getState().openSessionIds).toEqual([]);
    expect(input.useComposerStore.getState().composer.draft.value).toBe("");

    agent.activateAgentSessionStorage(targets.targetB);
    input.activateComposerStorage(targets.targetB);
    workspace.activateContextDockStorage(targets.targetB);
    input.useComposerStore.getState().loadSession("shared");
    workspace.useContextDockStore.getState().activateSessionScope("shared");

    expect(agent.useAgentSessionStore.getState().openSessionIds).toEqual(["shared"]);
    expect(agent.useAgentSessionStore.getState().draftSessionIds).toEqual(new Set(["shared"]));
    expect(agent.useAgentSessionStore.getState().freshDraftSessionIds).toEqual(new Set());
    expect(input.useComposerStore.getState().composer.draft.value).toBe("cold draft on B");
    expect(workspace.useContextDockStore.getState().fileViewer).toEqual({
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
