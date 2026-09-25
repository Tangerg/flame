import type { DiffLayout, WorkspaceDiffMode } from "../diffVocabulary";
import { createSingletonPort } from "@/lib/ports/singletonPort";

export interface WorkspaceFileViewer {
  path: string;
  line: number;
}

export interface WorkspaceViewMemory {
  expandedDirs: readonly string[];
  lastFilePath: string | null;
  searchQuery: string;
  searchPath: string;
  diffMode: WorkspaceDiffMode;
  diffLayout: DiffLayout;
  collapsedDiffFiles: readonly string[];
}

export interface WorkspaceFileFocusSnapshot {
  readonly path: string;
  readonly revision: bigint;
}

export interface WorkspaceColumnWidth {
  width: number;
  setWidth: (width: number) => void;
}

export interface WorkspaceOptionalColumnWidth {
  width: number | null;
  setWidth: (width: number) => void;
}

export interface WorkspaceDrawer {
  collapsed: boolean;
  toggle: () => void;
}

export interface WorkspaceDockSnapshot {
  open: boolean;
  viewIds: string[];
  activeViewId: string | null;
}

interface WorkspaceNavigationPort {
  useActiveViewId(): string | null;
  useDock(): WorkspaceDockSnapshot;
  useFileFocus(): WorkspaceFileFocusSnapshot;
  useSubagentRunId(): string | null;
  openSubagentRun(runId: string | null): void;
  useFileViewer(): WorkspaceFileViewer | null;
  useViewMemory(): WorkspaceViewMemory;
  remember(change: Partial<WorkspaceViewMemory>): void;
  useSettingsPaneTarget(): string | null;
  useExpandedToolIds(): Set<string>;
  useToggleTool(): (id: string) => void;
  useSidebarDrawer(): WorkspaceDrawer;
  useSidebarWidth(): WorkspaceColumnWidth;
  useDockWidth(): WorkspaceOptionalColumnWidth;
  toggleSidebar(): void;
  selectChat(): void;
  openView(id: string): void;
  openViewInDock(id: string): void;
  selectDockView(id: string): void;
  closeDockView(id: string): void;
  closeOtherDockViews(id: string): void;
  closeAllDockViews(): void;
  reorderDockView(id: string, toIndex: number): void;
  collapseDock(): void;
  showDock(defaultViewId: string): void;
  closeView(id: string): void;
  activeViewId(): string | null;
  dock(): WorkspaceDockSnapshot;
  setSettingsPane(pane: string): void;
  focusFile(path: string): void;
  openFile(path: string, line?: number): void;
  closeFile(): void;
  locateTool(id: string): void;
  // Adoption and activation are different moves that only look alike. At start-up
  // the location is the authority — a deep link or a surviving renderer already
  // names a dock — so the session takes it. Every later switch reverses that: the
  // session it moves to is the authority, and the location follows. Inferring
  // which one applies from "no scope is active" conflates them, because a session
  // list reconciliation clears that mid-run and the next switch then adopts the
  // dock belonging to the session it just left.
  adoptSessionScope(sessionId: string): void;
  activateSessionScope(sessionId: string): void;
  forgetSessionScopes(openSessionIds: string[]): void;
}

const port = createSingletonPort<WorkspaceNavigationPort>(
  "Workspace navigation port is not configured",
);

export const configureWorkspaceNavigationPort = port.configure;
export const workspaceNavigation = port.get;
