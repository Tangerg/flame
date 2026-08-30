import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { discardOlderVersions } from "@/lib/persistedStore";

const CONTEXT_DOCK_STORAGE_KEY = "flame.context-dock";
const NON_NEGATIVE_DECIMAL = /^(0|[1-9]\d*)$/;

const persistedDockScopeSchema = z.object({
  dockViewIds: z.array(z.string()),
  lastViewId: z.string().nullable(),
  fileFocus: z.object({ path: z.string(), revision: z.string().regex(NON_NEGATIVE_DECIMAL) }),
  fileViewer: z.object({ path: z.string(), line: z.number().int().nonnegative() }).nullable(),
});

const contextDockPersistSchema = z.object({
  sessionScopes: z.array(z.tuple([z.string(), persistedDockScopeSchema])),
});

type PersistedDockScope = z.infer<typeof persistedDockScopeSchema>;

// What the dock has OPEN per session — never which destination is showing, which belongs
// to the location so no flag here can disagree with the view on screen. `lastViewId` is the
// memory a re-open reads, written FROM the location and never back into it.

export interface WorkspaceFileViewer {
  path: string;
  line: number;
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
  /** The open tab set. Collapsing the dock is lossless: this survives it. */
  dockViewIds: string[];
  lastViewId: string | null;
  fileFocus: WorkspaceFileFocus;
  fileViewer: WorkspaceFileViewer | null;
  selectedToolId: string;
  expandedToolIds: Set<string>;
}

interface ContextDockState extends ContextDockSessionScope {
  /** null until the current renderer has adopted its URL-backed location. */
  activeSessionScopeId: string | null;
  sessionScopes: Map<string, ContextDockSessionScope>;
}

interface ContextDockActions {
  /** Hold `id` open. Which destination shows is the caller's navigation. */
  openDockTab: (id: string) => void;
  /** Adopt an already-authoritative location as open and last-shown in one write. */
  adoptDockLocation: (id: string) => void;
  /** Drop `id`; answers which tab should take its place, or null for none. */
  closeDockTab: (id: string) => string | null;
  /** Keep `id` and drop every sibling. */
  closeOtherDockTabs: (id: string) => void;
  closeAllDockTabs: () => void;
  /** Move `id` to `toIndex`, clamped into the open set. */
  reorderDockTab: (id: string, toIndex: number) => void;
  /** The destination a re-open should return to, given a fallback. */
  dockTabToShow: (defaultViewId: string) => string;
  rememberDockView: (id: string) => void;
  focusFile: (path: string) => void;
  setFileViewer: (path: string, line?: number) => void;
  setSelectedToolId: (id: string) => void;
  revealTool: (id: string) => void;
  toggleExpandedTool: (id: string) => void;
  /** Swap to `sessionId`'s scope; answers the destination it remembers. */
  activateSessionScope: (sessionId: string) => string | null;
  forgetSessionScopes: (openSessionIds: string[]) => void;
}

function emptySessionScope(): ContextDockSessionScope {
  return {
    fileFocus: WorkspaceFileFocus.empty(),
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
    dockViewIds: [],
    lastViewId: null,
  };
}

function cloneSessionScope(scope: ContextDockSessionScope): ContextDockSessionScope {
  return {
    fileFocus: scope.fileFocus,
    fileViewer: scope.fileViewer ? { ...scope.fileViewer } : null,
    selectedToolId: scope.selectedToolId,
    expandedToolIds: new Set(scope.expandedToolIds),
    dockViewIds: [...scope.dockViewIds],
    lastViewId: scope.lastViewId,
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
      selectedToolId: "",
      expandedToolIds: new Set<string>(),

      openDockTab: (id) =>
        set((state) => ({
          dockViewIds: state.dockViewIds.includes(id)
            ? state.dockViewIds
            : [...state.dockViewIds, id],
        })),
      adoptDockLocation: (id) =>
        set((state) => ({
          dockViewIds: state.dockViewIds.includes(id)
            ? state.dockViewIds
            : [...state.dockViewIds, id],
          lastViewId: id,
        })),
      closeDockTab: (id) => {
        const { dockViewIds } = get();
        const index = dockViewIds.indexOf(id);
        if (index < 0) return null;
        const remaining = dockViewIds.filter((viewId) => viewId !== id);
        set({ dockViewIds: remaining });
        // The tab that slid into its place, else the one before it.
        return remaining[index] ?? remaining[index - 1] ?? null;
      },
      closeOtherDockTabs: (id) =>
        set((state) => ({
          dockViewIds: state.dockViewIds.includes(id) ? [id] : state.dockViewIds,
        })),
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
      rememberDockView: (id) => set({ lastViewId: id }),
      focusFile: (path) => set((state) => ({ fileFocus: state.fileFocus.moveTo(path) })),
      setFileViewer: (path, line) => set({ fileViewer: { path, line: line ?? 0 } }),
      setSelectedToolId: (id) => set({ selectedToolId: id }),
      revealTool: (id) => {
        const expandedToolIds = new Set(get().expandedToolIds);
        expandedToolIds.add(id);
        set({ selectedToolId: id, expandedToolIds });
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
      merge: (persisted, current) => {
        if (persisted === undefined) return current;
        const parsed = contextDockPersistSchema.safeParse(persisted);
        if (!parsed.success) {
          console.warn(
            "[contextDockStore] discarding corrupted flame.context-dock:",
            parsed.error.issues,
          );
          return current;
        }
        const sessionScopes = new Map<string, ContextDockSessionScope>();
        for (const [sessionId, scope] of parsed.data.sessionScopes) {
          sessionScopes.set(sessionId, restorePersistedScope(scope));
        }
        return { ...current, sessionScopes };
      },
    },
  ),
);
