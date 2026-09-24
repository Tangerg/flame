import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { discardOlderVersions, rehydrateOrDefault } from "@/lib/persistedStore";
import { WORKSPACE_DOCK_CATALOG } from "../application/navigation";
import type { WorkspaceViewMemory } from "../application/ports/navigationState";
import { DIFF_LAYOUTS, DIFF_MODES } from "../application/diffViewModel";

const CONTEXT_DOCK_STORAGE_KEY = "flame.context-dock";
const NON_NEGATIVE_DECIMAL = /^(0|[1-9]\d*)$/;

const persistedDockScopeSchema = z.object({
  dockViewIds: z.array(z.string().refine((id) => id !== WORKSPACE_DOCK_CATALOG)),
  lastViewId: z.string().nullable(),
  fileFocus: z.object({ path: z.string(), revision: z.string().regex(NON_NEGATIVE_DECIMAL) }),
  fileViewer: z.object({ path: z.string(), line: z.number().int().nonnegative() }).nullable(),
  memory: z
    .object({
      expandedDirs: z.array(z.string()),
      lastFilePath: z.string().nullable(),
      searchQuery: z.string(),
      searchPath: z.string(),
      diffMode: z.enum(DIFF_MODES),
      diffLayout: z.enum(DIFF_LAYOUTS),
      collapsedDiffFiles: z.array(z.string()),
    })
    .optional(),
});

const contextDockPersistSchema = z.object({
  sessionScopes: z.array(z.tuple([z.string(), persistedDockScopeSchema])),
});

type PersistedDockScope = z.infer<typeof persistedDockScopeSchema>;

interface WorkspaceFileViewer {
  path: string;
  line: number;
}

const EMPTY_MEMORY: WorkspaceViewMemory = {
  expandedDirs: [],
  lastFilePath: null,
  searchQuery: "",
  searchPath: "",
  diffMode: "worktree",
  diffLayout: "unified",
  collapsedDiffFiles: [],
};

function ancestorsOf(path: string): string[] {
  const parts = path.split("/").slice(0, -1);
  return parts.map((_, index) => parts.slice(0, index + 1).join("/"));
}

export class WorkspaceFileFocus {
  private constructor(
    readonly path: string,
    readonly revision: bigint,
  ) {}

  static empty(): WorkspaceFileFocus {
    return new WorkspaceFileFocus("", 0n);
  }

  static restore(path: string, revision: bigint): WorkspaceFileFocus {
    if (revision < 0n) throw new RangeError("Workspace file focus revision cannot be negative");
    return new WorkspaceFileFocus(path, revision);
  }

  moveTo(path: string): WorkspaceFileFocus {
    return new WorkspaceFileFocus(path, this.revision + 1n);
  }
}

interface ContextDockSessionScope {
  dockViewIds: string[];
  lastViewId: string | null;
  fileFocus: WorkspaceFileFocus;
  fileViewer: WorkspaceFileViewer | null;
  expandedToolIds: Set<string>;
  memory: WorkspaceViewMemory;
}

interface ContextDockState extends ContextDockSessionScope {
  activeSessionScopeId: string | null;
  sessionScopes: Map<string, ContextDockSessionScope>;
}

interface ContextDockActions {
  adoptDockLocation: (id: string) => void;
  closeDockTab: (id: string) => string | null;
  closeOtherDockTabs: (id: string) => void;
  closeAllDockTabs: () => void;
  reorderDockTab: (id: string, toIndex: number) => void;
  dockTabToShow: (defaultViewId: string) => string;
  focusFile: (path: string) => void;
  setFileViewer: (viewer: WorkspaceFileViewer | null) => void;
  remember: (change: Partial<WorkspaceViewMemory>) => void;
  revealTool: (id: string) => void;
  toggleExpandedTool: (id: string) => void;
  activateSessionScope: (sessionId: string) => string | null;
  forgetSessionScopes: (openSessionIds: string[]) => void;
}

function emptySessionScope(): ContextDockSessionScope {
  return {
    fileFocus: WorkspaceFileFocus.empty(),
    fileViewer: null,
    expandedToolIds: new Set<string>(),
    dockViewIds: [],
    lastViewId: null,
    memory: EMPTY_MEMORY,
  };
}

function cloneSessionScope(scope: ContextDockSessionScope): ContextDockSessionScope {
  return {
    fileFocus: scope.fileFocus,
    fileViewer: scope.fileViewer ? { ...scope.fileViewer } : null,
    expandedToolIds: new Set(scope.expandedToolIds),
    dockViewIds: [...scope.dockViewIds],
    lastViewId: scope.lastViewId,
    memory: scope.memory,
  };
}

function saveCurrentSessionScope(state: ContextDockState) {
  const scopes = new Map(state.sessionScopes);
  if (state.activeSessionScopeId) scopes.set(state.activeSessionScopeId, cloneSessionScope(state));
  return scopes;
}

function persistedSessionScopes(state: ContextDockState): [string, PersistedDockScope][] {
  return [...saveCurrentSessionScope(state)].map(([sessionId, scope]) => [
    sessionId,
    {
      dockViewIds: scope.dockViewIds,
      lastViewId: scope.lastViewId,
      fileFocus: { path: scope.fileFocus.path, revision: scope.fileFocus.revision.toString() },
      fileViewer: scope.fileViewer,
      memory: {
        ...scope.memory,
        expandedDirs: [...scope.memory.expandedDirs],
        collapsedDiffFiles: [...scope.memory.collapsedDiffFiles],
      },
    },
  ]);
}

function restorePersistedScope(scope: PersistedDockScope): ContextDockSessionScope {
  return {
    ...emptySessionScope(),
    dockViewIds: [...new Set(scope.dockViewIds)],
    lastViewId: scope.lastViewId,
    fileFocus: WorkspaceFileFocus.restore(scope.fileFocus.path, BigInt(scope.fileFocus.revision)),
    fileViewer: scope.fileViewer,
    memory: scope.memory ?? EMPTY_MEMORY,
  };
}

export const useContextDockStore = create<ContextDockState & ContextDockActions>()(
  persist(
    (set, get) => ({
      activeSessionScopeId: null,
      sessionScopes: new Map<string, ContextDockSessionScope>(),
      dockViewIds: [],
      lastViewId: null,
      fileFocus: WorkspaceFileFocus.empty(),
      fileViewer: null,
      expandedToolIds: new Set<string>(),
      memory: EMPTY_MEMORY,

      adoptDockLocation: (id) =>
        set((state) => ({
          dockViewIds:
            id === WORKSPACE_DOCK_CATALOG || state.dockViewIds.includes(id)
              ? state.dockViewIds
              : [...state.dockViewIds, id],
          lastViewId: id,
        })),
      closeDockTab: (id) => {
        const { dockViewIds, lastViewId } = get();
        const index = dockViewIds.indexOf(id);
        if (index < 0) return null;
        const remaining = dockViewIds.filter((viewId) => viewId !== id);
        const next = remaining[index] ?? remaining[index - 1] ?? null;
        set({ dockViewIds: remaining, lastViewId: lastViewId === id ? next : lastViewId });
        return next;
      },
      closeOtherDockTabs: (id) =>
        set((state) =>
          state.dockViewIds.includes(id) ? { dockViewIds: [id], lastViewId: id } : {},
        ),
      closeAllDockTabs: () => set({ dockViewIds: [], lastViewId: null }),
      reorderDockTab: (id, toIndex) =>
        set((state) => {
          const from = state.dockViewIds.indexOf(id);
          if (from < 0) return {};
          const to = Math.max(0, Math.min(state.dockViewIds.length - 1, toIndex));
          if (from === to) return {};
          const dockViewIds = [...state.dockViewIds];
          dockViewIds.splice(from, 1);
          dockViewIds.splice(to, 0, id);
          return { dockViewIds };
        }),
      dockTabToShow: (defaultViewId) => {
        const { dockViewIds, lastViewId } = get();
        if (dockViewIds.length === 0) return defaultViewId;
        return lastViewId !== null && dockViewIds.includes(lastViewId)
          ? lastViewId
          : (dockViewIds[0] ?? defaultViewId);
      },
      focusFile: (path) => set((state) => ({ fileFocus: state.fileFocus.moveTo(path) })),
      setFileViewer: (fileViewer) =>
        set((state) =>
          fileViewer
            ? {
                fileViewer,
                memory: {
                  ...state.memory,
                  lastFilePath: fileViewer.path,
                  expandedDirs: [
                    ...new Set([...state.memory.expandedDirs, ...ancestorsOf(fileViewer.path)]),
                  ],
                },
              }
            : { fileViewer },
        ),
      remember: (change) => set((state) => ({ memory: { ...state.memory, ...change } })),
      revealTool: (id) => {
        const expandedToolIds = new Set(get().expandedToolIds);
        expandedToolIds.add(id);
        set({ expandedToolIds });
      },
      toggleExpandedTool: (id) => {
        const next = new Set(get().expandedToolIds);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        set({ expandedToolIds: next });
      },
      activateSessionScope: (sessionId) => {
        const state = get();
        if (state.activeSessionScopeId === sessionId) return state.lastViewId;
        const sessionScopes = saveCurrentSessionScope(state);
        const nextScope = sessionId ? sessionScopes.get(sessionId) : undefined;
        const scope = nextScope ? cloneSessionScope(nextScope) : emptySessionScope();
        set({ activeSessionScopeId: sessionId, sessionScopes, ...scope });
        return scope.lastViewId;
      },
      forgetSessionScopes: (openSessionIds) =>
        set((state) => {
          const open = new Set(openSessionIds);
          const sessionScopes = new Map<string, ContextDockSessionScope>();
          for (const [sessionId, scope] of state.sessionScopes) {
            if (open.has(sessionId)) sessionScopes.set(sessionId, scope);
          }
          if (state.activeSessionScopeId && open.has(state.activeSessionScopeId)) {
            return { sessionScopes };
          }
          return {
            activeSessionScopeId: null,
            sessionScopes,
            ...emptySessionScope(),
          };
        }),
    }),
    {
      name: CONTEXT_DOCK_STORAGE_KEY,
      storage: createJSONStorage(() => localStorage),
      partialize: (state) => ({ sessionScopes: persistedSessionScopes(state) }),
      version: 2,
      migrate: discardOlderVersions,
      merge: rehydrateOrDefault(CONTEXT_DOCK_STORAGE_KEY, contextDockPersistSchema, (data) => ({
        sessionScopes: new Map<string, ContextDockSessionScope>(
          data.sessionScopes.map(([sessionId, scope]) => [sessionId, restorePersistedScope(scope)]),
        ),
      })),
    },
  ),
);
