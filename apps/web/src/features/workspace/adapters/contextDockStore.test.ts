import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { WorkspaceFileFocus } from "@/features/workspace/adapters/contextDockStore";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});

const EMPTY = {
  activeSessionScopeId: null,
  sessionScopes: new Map(),
  dockViewIds: [],
  lastViewId: null,
  fileFocus: WorkspaceFileFocus.empty(),
  fileViewer: null,
  expandedToolIds: new Set<string>(),
};

const dock = () => testClient.workspace.dock.store.getState();

beforeEach(() => {
  localStorage.removeItem(testClient.workspace.dock.store.persist.getOptions().name!);
  testClient.workspace.dock.store.setState({ ...EMPTY, sessionScopes: new Map() });
});

describe("the round trip", () => {
  it("gets back everything it chose to persist", async () => {
    dock().activateSessionScope("s1");
    dock().adoptDockLocation("skills");
    dock().adoptDockLocation("diff");
    dock().setFileViewer({ path: "a/b.ts", line: 12 });
    dock().setToolExpanded("tool_1", true);

    const key = testClient.workspace.dock.store.persist.getOptions().name!;
    const payload = localStorage.getItem(key) ?? "null";
    const written = (JSON.parse(payload) as { state: unknown }).state;
    expect(written).toBeTruthy();

    testClient.workspace.dock.store.setState({ ...EMPTY, sessionScopes: new Map() });
    localStorage.setItem(key, payload);
    await testClient.workspace.dock.store.persist.rehydrate();

    const partialize = testClient.workspace.dock.store.persist.getOptions().partialize!;
    expect(JSON.parse(JSON.stringify(partialize(dock() as never)))).toEqual(written);
  });
});

describe("per-session scopes", () => {
  it("keeps each session's tabs and returns the destination it remembers", () => {
    dock().activateSessionScope("s1");
    dock().adoptDockLocation("diff");
    dock().focusFile("a.ts");

    expect(dock().activateSessionScope("s2")).toBeNull();
    expect(dock().dockViewIds).toEqual([]);
    expect(dock().fileFocus).toMatchObject({ path: "", revision: 0n });

    expect(dock().activateSessionScope("s1")).toBe("diff");
    expect(dock().dockViewIds).toEqual(["diff"]);
    expect(dock().fileFocus).toMatchObject({ path: "a.ts", revision: 1n });
  });

  it("forgets scopes for sessions no longer held open", () => {
    dock().activateSessionScope("s1");
    dock().adoptDockLocation("diff");
    dock().activateSessionScope("s2");

    dock().forgetSessionScopes(["s2"]);

    expect([...dock().sessionScopes.keys()]).toEqual([]);
  });

  it("retires the active scope when the Session owner has already released it", async () => {
    dock().activateSessionScope("s1");
    dock().adoptDockLocation("diff");
    dock().activateSessionScope("s2");
    dock().adoptDockLocation("skills");

    dock().forgetSessionScopes(["s1"]);

    expect(dock()).toMatchObject({
      activeSessionScopeId: null,
      dockViewIds: [],
      lastViewId: null,
    });
    await vi.waitFor(() => {
      const persisted = JSON.parse(
        localStorage.getItem(testClient.workspace.dock.store.persist.getOptions().name!) ?? "null",
      ) as {
        state: { sessionScopes: [string, unknown][] };
      };
      expect(persisted.state.sessionScopes.map(([sessionId]) => sessionId)).toEqual(["s1"]);
    });
  });

  it("restores inactive tabs and file targets after renderer replacement", async () => {
    dock().activateSessionScope("s1");
    dock().adoptDockLocation("skills");
    dock().adoptDockLocation("diff");
    dock().adoptDockLocation("file");
    dock().adoptDockLocation("diff");
    dock().focusFile("src/runtime.ts");
    dock().setFileViewer({ path: "src/runtime.ts", line: 42 });
    dock().setToolExpanded("call-from-retired-renderer", true);

    await vi.waitFor(() =>
      expect(
        localStorage.getItem(testClient.workspace.dock.store.persist.getOptions().name!),
      ).not.toBeNull(),
    );
    const persisted = JSON.parse(
      localStorage.getItem(testClient.workspace.dock.store.persist.getOptions().name!) ?? "null",
    ) as {
      state: { sessionScopes: [string, { fileFocus: { revision: string } }][] };
    };
    expect(persisted.state.sessionScopes[0]?.[1].fileFocus.revision).toBe("1");

    const replacementModule = await import("@/features/workspace/adapters/contextDockStore");
    const replacementOwner = replacementModule.createContextDockStore();
    replacementOwner.activate("https://runtime.test");
    const replacement = replacementOwner.store;
    const persistedReplacement = replacement as typeof replacement & {
      persist: { rehydrate: () => Promise<void> | void };
    };
    await persistedReplacement.persist.rehydrate();
    replacement.getState().activateSessionScope("s1");

    expect(replacement.getState()).toMatchObject({
      dockViewIds: ["skills", "diff", "file"],
      lastViewId: "diff",
      fileFocus: { path: "src/runtime.ts", revision: 1n },
      fileViewer: { path: "src/runtime.ts", line: 42 },
      expandedToolIds: new Set(),
    });
  });
});

describe("renderer storage validation", () => {
  it("rejects persisted catalog entries masquerading as tabs", async () => {
    dock().activateSessionScope("s1");
    dock().adoptDockLocation("diff");
    const key = testClient.workspace.dock.store.persist.getOptions().name!;
    const payload = JSON.parse(localStorage.getItem(key)!) as {
      state: { sessionScopes: [string, { dockViewIds: string[] }][] };
    };
    payload.state.sessionScopes[0]![1].dockViewIds = ["catalog"];
    testClient.workspace.dock.store.setState({ ...EMPTY, sessionScopes: new Map() });
    localStorage.setItem(key, JSON.stringify(payload));

    await testClient.workspace.dock.store.persist.rehydrate();
    dock().activateSessionScope("s1");

    expect(dock().dockViewIds).toEqual([]);
  });

  it("falls back to empty memory when the current payload is malformed", async () => {
    localStorage.setItem(
      testClient.workspace.dock.store.persist.getOptions().name!,
      JSON.stringify({
        state: { sessionScopes: [["s1", { dockViewIds: "not-an-array" }]] },
      }),
    );

    await testClient.workspace.dock.store.persist.rehydrate();

    expect(dock().sessionScopes.size).toBe(0);
    expect(dock().dockViewIds).toEqual([]);
  });
});
