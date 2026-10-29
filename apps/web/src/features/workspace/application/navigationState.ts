export interface WorkspaceFileViewer {
  path: string;
  line: number;
}

export interface WorkspaceInspection {
  readonly viewId: string;
  readonly origin: HTMLElement;
  readonly activeElement: Element | null;
  readonly returnViewId: string | null;
  readonly returnDockId: string | null;
  readonly returnSubagentRunId: string | null;
  readonly returnFileViewer: WorkspaceFileViewer | null;
  readonly previous: WorkspaceInspection | null;
}

export type WorkspaceViewPresentation = "dock" | "main";

export type WorkspaceChatPresentation = "preserve-dock" | "hide-dock";

export interface WorkspaceViewMemory {
  expandedDirs: readonly string[];
  lastFilePath: string | null;
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

export interface WorkspaceNavigationPort {
  useActiveViewId(): string | null;
  useDock(): WorkspaceDockSnapshot;
  useInspection(): WorkspaceInspection | null;
  returnFromInspection(
    inspection: WorkspaceInspection,
    presentation?: WorkspaceViewPresentation,
  ): boolean;
  useFileFocus(): WorkspaceFileFocusSnapshot;
  useSubagentRunId(): string | null;
  openSubagentRun(runId: string | null, origin?: HTMLElement): void;
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
  selectChat(presentation?: WorkspaceChatPresentation): void;
  openView(id: string, origin?: HTMLElement): void;
  openViewInDock(id: string, origin?: HTMLElement): void;
  selectDockView(id: string): void;
  closeDockView(id: string): void;
  closeOtherDockViews(id: string): void;
  closeAllDockViews(): void;
  reorderDockView(id: string, toIndex: number): void;
  collapseDock(): void;
  showDock(
    defaultViewId: string,
    presentation?: WorkspaceViewPresentation,
    origin?: HTMLElement,
  ): void;
  closeView(id: string): void;
  activeViewId(): string | null;
  dock(): WorkspaceDockSnapshot;
  setSettingsPane(pane: string): void;
  focusFile(path: string): void;
  openFile(path: string, line?: number, origin?: HTMLElement): void;
  closeFile(): void;
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
