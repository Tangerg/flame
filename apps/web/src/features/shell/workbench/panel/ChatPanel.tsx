import { Group, Panel } from "react-resizable-panels";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { useComposer } from "@/features/composer/public/composer";
import * as stylex from "@stylexjs/stylex";
import {
  Activity,
  Fragment,
  useCallback,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
  type ComponentProps,
  type FocusEvent,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
  type RefObject,
} from "react";
import { flushSync } from "react-dom";
import { PanelHost } from "@flame/ui/panel";
import { IconButton } from "@flame/ui";
import { iconGlyph } from "@/ui/icons/index";
import { ReturnButton } from "@/features/shell/workbench/panel/ReturnButton";
import { type AgentInput } from "@/features/agent/public/input";
import { CatalogPicker, type CatalogPickerGroup } from "@/ui/index";
import { ContentVisibility } from "@/ui/index";
import { AgentContentCard } from "@/ui/agent/index";
import {
  AgentContextDock,
  AgentDockRow,
  AgentDockSeparator,
  type AgentDockTab,
  AgentDockTabs,
} from "@/ui/agent/index";
import { AgentDockCatalog } from "@/ui/agent/index";
import { AgentDockToggle, AgentSurfaceHeader } from "@/ui/agent/index";
import {
  sessionDisplayTitle,
  useActiveSession,
  useActiveSessionId,
  useAgentSessions,
} from "@/features/agent/public/session";
import { registerComposerFocusPresentation } from "@/features/composer/public/focus";
import {
  closeWorkspaceView,
  collapseWorkspaceDock,
  openWorkspaceView,
  openWorkspaceViewInDock,
  showWorkspaceDock,
  useActiveWorkspaceViewId,
  useWorkspaceDock,
  useWorkspaceInspection,
} from "@/features/workspace/public/navigation";
import { useContextDockCatalog } from "@/features/workspace/public/contextDockCatalog";
import { type ContextDockDestinationGroup } from "@/features/workspace/public/contextDockCatalog";
import { useWorkspaceViews } from "@/features/workspace/public/views";
import { useDockLayout } from "@/features/workspace/public/sidebarDrawer";
import { Slot } from "@/extensions/host/Slot";
import { WORKSPACE_DOCK_CATALOG } from "@/features/workspace/public/navigation";
import { ChatStream } from "@/features/shell/workbench/panel/ChatStream";
import { TranscriptReaderProvider } from "@/ui/agent/transcript-reader";
import { RuntimeConnectionNotice } from "@/features/shell/workbench/panel/RuntimeConnectionNotice";
import { readingColumn as rc } from "@/features/chat/public/message/rendering";
import { space } from "@flame/ui/tokens.stylex";
import { RunAnnouncer } from "@/features/shell/workbench/panel/RunAnnouncer";
import { RunStatusPill } from "@/features/shell/workbench/panel/RunStatusPill";
import { SessionIdentity } from "@/features/shell/workbench/panel/SessionIdentity";
import { WorkspaceViewBody } from "@/features/shell/workbench/panel/WorkspaceViewBody";
import { WorkspaceWindowHeader } from "@/features/shell/workbench/panel/WorkspaceWindowHeader";
import { useT, type Translate } from "@/lib/i18n/index";
import { canPresentDock } from "@/lib/shellGeometry";
import { shellStyles as sh } from "@/features/shell/workbench/shellStyles";
import { useInspectionFocus } from "@/features/shell/workbench/panel/useInspectionFocus";
export interface ChatPanelHandle {
  focusComposer(): void;
}

interface Props {
  onSend: (input: AgentInput) => boolean;
  ref?: Ref<ChatPanelHandle>;
}

function SessionOwnedWorkspaceState({
  sessionId,
  children,
}: {
  sessionId: string;
  children: ReactNode;
}) {
  return <Fragment key={sessionId}>{children}</Fragment>;
}

function useDockCatalogGroups(
  groups: ContextDockDestinationGroup[],
  openViewIds: ReadonlySet<string>,
): CatalogPickerGroup[] {
  const t = useT();
  return groups.map((group) => ({
    id: group.id,
    label: t(group.title),
    items: group.destinations.map((destination) => ({
      id: destination.viewId,
      label: t(destination.title),
      keywords: [destination.viewId, group.id, t(group.title)],
      active: openViewIds.has(destination.viewId),
    })),
  }));
}

function WorkspaceViewPicker({
  triggerRef,
  groups,
  openViewIds,
  activeViewId,
  onSelect,
}: {
  triggerRef: RefObject<HTMLButtonElement | null>;
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
  activeViewId: string;
  onSelect: (viewId: string, origin?: HTMLElement) => void;
}) {
  const t = useT();
  const pickerGroups = useDockCatalogGroups(groups, openViewIds);

  return (
    <CatalogPicker
      groups={pickerGroups}
      label={t("dock.action.browse")}
      placeholder={t("dock.picker.placeholder")}
      emptyLabel={t("dock.picker.empty")}
      clearLabel={t("common.clearSearch")}
      trigger={
        <IconButton
          ref={triggerRef}
          icon={iconGlyph("plus")}
          size="sm"
          quiet
          aria-label={t("dock.action.browse")}
          data-dock-view-id={activeViewId}
          className={stylex.props(sh.toEnd).className}
        />
      }
      onSelect={(item) => {
        if (item.id === activeViewId) {
          const trigger = triggerRef.current;
          const active = trigger?.ownerDocument.activeElement;
          if (
            trigger?.isConnected &&
            (!active ||
              active === trigger.ownerDocument.body ||
              active === trigger.ownerDocument.documentElement)
          ) {
            trigger.focus({ preventScroll: true });
          }
          return;
        }
        onSelect(item.id, triggerRef.current ?? undefined);
      }}
    />
  );
}

function DockCatalogPage({
  groups,
  openViewIds,
  titleHidden,
}: {
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
  titleHidden: boolean;
}) {
  const workspaceAccess = useWorkspace();

  const t = useT();
  return (
    <AgentDockCatalog
      groups={useDockCatalogGroups(groups, openViewIds)}
      title={t("dock.catalog.title")}
      titleHidden={titleHidden}
      placeholder={t("dock.picker.placeholder")}
      emptyLabel={t("dock.picker.empty")}
      clearLabel={t("common.clearSearch")}
      onSelect={openWorkspaceViewInDock.bind(null, workspaceAccess)}
    />
  );
}

function DockHeader({
  tabs,
  groups,
  openViewIds,
  returnAction,
}: {
  tabs: AgentDockTab[];
  groups: ContextDockDestinationGroup[];
  openViewIds: ReadonlySet<string>;
  returnAction?: { label: string; onReturn: () => void };
}) {
  const workspaceAccess = useWorkspace();

  const t = useT();
  const pickerRef = useRef<HTMLButtonElement>(null);
  return (
    <AgentSurfaceHeader divider={false}>
      {returnAction && <ReturnButton label={returnAction.label} onReturn={returnAction.onReturn} />}
      <AgentDockTabs
        tabs={tabs}
        ariaLabel={t("dock.tabs.label")}
        fallbackFocus={pickerRef}
        reorder={{
          onMove: (id, toIndex) => {
            const result = workspaceAccess.navigation.reorderDockView(id, toIndex);
            const tab = tabs.find((tab) => tab.id === id);
            return result && tab ? t("dock.tabs.moved", { title: tab.title, ...result }) : null;
          },
          handleLabel: (title) => t("dock.tabs.reorder", { title }),
          moveLeftLabel: t("dock.tabs.moveLeft"),
          moveRightLabel: t("dock.tabs.moveRight"),
        }}
      />
      <WorkspaceViewPicker
        triggerRef={pickerRef}
        groups={groups}
        openViewIds={openViewIds}
        activeViewId={tabs.find((tab) => tab.active)?.id ?? WORKSPACE_DOCK_CATALOG}
        onSelect={openWorkspaceViewInDock.bind(null, workspaceAccess)}
      />
      <IconButton
        icon={iconGlyph("maximize")}
        size="sm"
        aria-label={t("dock.action.expand")}
        onClick={() => showWorkspaceDock(workspaceAccess, "main")}
      />
    </AgentSurfaceHeader>
  );
}

function ConversationContent({
  ready,
  sessionId,
  onSend,
}: {
  ready: boolean;
  sessionId: string;
  onSend: Props["onSend"];
}) {
  const t = useT();
  const session = useActiveSession();
  if (!ready)
    return (
      <div {...stylex.props(rc.box, rc.gutter, paneStyles.pendingConnection)}>
        <RuntimeConnectionNotice />
      </div>
    );
  return (
    <>
      <AgentSurfaceHeader corner="window">
        <SessionIdentity
          sessionId={sessionId}
          title={session ? sessionDisplayTitle(t, session.title) : t("session.draft")}
          workspacePath={session?.workspace.path}
          detail={<RunStatusPill />}
        />
        <Slot name="chat.header.meta" />
      </AgentSurfaceHeader>
      <TranscriptReaderProvider sessionId={sessionId}>
        <Slot name="chat.search" />
        <ChatStream onSend={onSend} />
      </TranscriptReaderProvider>
      <RunAnnouncer />
    </>
  );
}

const PANEL_SIZES = {
  main: { conversation: { minSize: 0, maxSize: 0 }, dock: { minSize: "100%" } },
  dock: { conversation: { minSize: 440 }, dock: { minSize: 320 } },
  closed: { conversation: { minSize: 440 }, dock: { minSize: 0, maxSize: 0 } },
} as const;

// Capacity commits can resize body portals and start ancestor observers, so the row
// publishes its width from animation frames before ResizeObserver delivery.
function useDockRowWidth() {
  const [rowWidth, setRowWidth] = useState<number | null>(null);
  const dockRowRef = useCallback((row: HTMLDivElement | null) => {
    if (!row) return;
    const view = row.ownerDocument.defaultView!;
    let width = row.clientWidth;
    let capacity = canPresentDock(width);
    let frame = 0;
    let retired = false;
    setRowWidth(width);
    const sample = () => {
      if (retired) return;
      frame = 0;
      const next = row.clientWidth;
      if (next !== width) {
        width = next;
        const nextCapacity = canPresentDock(width);
        if (nextCapacity !== capacity) {
          capacity = nextCapacity;
          flushSync(() => setRowWidth(width));
        } else {
          setRowWidth(width);
        }
      }
      if (!retired) frame = view.requestAnimationFrame(sample);
    };
    frame = view.requestAnimationFrame(sample);
    return () => {
      retired = true;
      view.cancelAnimationFrame(frame);
    };
  }, []);
  return [rowWidth, dockRowRef] as const;
}

function returnActionLabel(
  t: Translate,
  inspection: NonNullable<ReturnType<typeof useWorkspaceInspection>>,
  viewsById: ReadonlyMap<string, { title: string }>,
): string {
  const returnViewId = inspection.returnViewId ?? inspection.returnDockId;
  if (returnViewId === "subagents" && inspection.returnSubagentRunId)
    return t("dock.action.returnToTask");
  if (returnViewId === WORKSPACE_DOCK_CATALOG) return t("dock.action.returnToCatalog");
  const returnTitle = returnViewId ? viewsById.get(returnViewId)?.title : undefined;
  return returnTitle
    ? t("dock.action.returnToView", { view: t(returnTitle) })
    : t("dock.action.returnToConversation");
}

function applyComposerReadingLimit({ currentTarget, target }: FocusEvent<HTMLElement>) {
  // Portaled menus keep their opener's reading context through native focus handoffs.
  if (!currentTarget.contains(target)) return;
  const editing = target.closest('[data-slot="composer-scrollport"]');
  // Jump navigates the current frame without changing its editing or reading budget.
  if (!editing && target.closest('[data-slot="composer-overlay"]')) return;
  currentTarget.style.setProperty("--composer-reading-limit", editing ? "100%" : "calc(100% / 3)");
}

const CATALOG_VIEW = { title: "dock.catalog.title", dock: "workspace" } as const;

// Without room for the dock the toggle opens the view full-page instead.
function toggleChatDock(
  workspaceAccess: ReturnType<typeof useWorkspace>,
  dockAvailable: boolean,
  dockOpen: boolean,
) {
  if (!dockAvailable) showWorkspaceDock(workspaceAccess, "main");
  else if (dockOpen) collapseWorkspaceDock(workspaceAccess);
  else showWorkspaceDock(workspaceAccess);
}

type InspectionFocus = ReturnType<typeof useInspectionFocus>;

function WorkspaceViewStack({
  presentation,
  shown,
  presentedViewId,
  fullPaneViewId,
  addressable,
  mountedViewIds,
  mainViewRef,
  presentationRef,
  catalog,
  openViewIds,
}: {
  presentation: "main" | "dock";
  shown: boolean;
  presentedViewId: string | null;
  fullPaneViewId: string | null;
  addressable: boolean;
  mountedViewIds: ReadonlySet<string>;
  mainViewRef: InspectionFocus["mainViewRef"];
  presentationRef: InspectionFocus["presentationRef"];
  catalog: ReturnType<typeof useContextDockCatalog>;
  openViewIds: ReadonlySet<string>;
}) {
  const t = useT();
  const main = presentation === "main";
  return (
    <div {...stylex.props(sh.anchor)}>
      <Activity mode={presentedViewId === WORKSPACE_DOCK_CATALOG ? "visible" : "hidden"}>
        <div
          ref={fullPaneViewId === WORKSPACE_DOCK_CATALOG ? mainViewRef : undefined}
          role={main ? "region" : undefined}
          aria-label={main ? t(CATALOG_VIEW.title) : undefined}
          tabIndex={main ? -1 : undefined}
          data-focus-inset=""
          data-dock-view-id={WORKSPACE_DOCK_CATALOG}
          {...stylex.props(sh.overlay)}
        >
          <DockCatalogPage groups={catalog} openViewIds={openViewIds} titleHidden={main} />
        </div>
      </Activity>
      {/* Moving retained readers with the tab strip can reset their WebKit scroll positions. */}
      {[...mountedViewIds].sort().map((viewId) => {
        const visible = viewId === presentedViewId && shown;
        return (
          <div
            key={viewId}
            inert={!visible}
            aria-hidden={!visible || undefined}
            data-dock-view-id={addressable ? viewId : undefined}
            {...stylex.props(sh.overlay, !visible && paneStyles.inactiveView)}
          >
            <ContentVisibility visible={visible} fallbackFocus={presentationRef}>
              <PanelHost density={main ? "page" : "compact"}>
                <WorkspaceViewBody
                  ref={viewId === fullPaneViewId ? mainViewRef : undefined}
                  viewId={viewId}
                  presentation={presentation}
                />
              </PanelHost>
            </ContentVisibility>
          </div>
        );
      })}
    </div>
  );
}

type WorkspaceDock = ReturnType<typeof useWorkspaceDock>;
type WorkspaceInspection = ReturnType<typeof useWorkspaceInspection>;

// Which view the workspace presents, and where: a main view wins; otherwise a
// dock too narrow to sit beside the conversation shows an inspected view full
// pane instead.
function workspacePresentation({
  activeSessionId,
  dock,
  activeMainView,
  inspection,
  rowWidth,
}: {
  activeSessionId: string;
  dock: WorkspaceDock;
  activeMainView: string | null;
  inspection: WorkspaceInspection;
  rowWidth: number | null;
}) {
  const dockAvailable = canPresentDock(rowWidth ?? 0);
  const hasDockOwner = activeSessionId !== "";
  const showingCatalog = dock.activeViewId === WORKSPACE_DOCK_CATALOG;
  const dockOpen = hasDockOwner && dock.open && (showingCatalog || dock.viewIds.length > 0);
  const fullPaneViewId =
    activeMainView ??
    (dockOpen && rowWidth !== null && !dockAvailable && inspection?.viewId === dock.activeViewId
      ? dock.activeViewId
      : null);
  return {
    hasDockOwner,
    dockOpen,
    dockVisible: dockOpen && dockAvailable,
    ownedDockViewIds: hasDockOwner ? dock.viewIds : [],
    fullPaneViewId,
    inspectedFullPane: fullPaneViewId !== null && inspection?.viewId === fullPaneViewId,
  };
}

// Views stay mounted while open or while an inspection can return to them, so
// a return finds the reader where it was left.
function mountedWorkspaceViews(
  openViewIds: ReadonlySet<string>,
  inspection: WorkspaceInspection,
): Set<string> {
  const mounted = new Set(openViewIds);
  for (let source = inspection; source; source = source.previous) {
    if (source.returnViewId && source.returnViewId !== source.viewId)
      mounted.add(source.returnViewId);
  }
  mounted.delete(WORKSPACE_DOCK_CATALOG);
  return mounted;
}

function useDockTabs(
  viewIds: readonly string[],
  viewsById: ReadonlyMap<string, ReturnType<typeof useWorkspaceViews>[number]>,
  activeViewId: string | null,
) {
  const t = useT();
  const workspaceAccess = useWorkspace();
  const navigation = workspaceAccess.navigation;
  return viewIds.map((id) => {
    const view = viewsById.get(id);
    const title = view ? t(view.title) : id;
    const Badge = view?.badge;
    return {
      id,
      title,
      badge: Badge ? <Badge /> : undefined,
      active: id === activeViewId,
      onClose: () => navigation.closeDockView(id),
      closeLabel: `${t("common.close")} ${title}`,
      onCloseOthers: () => navigation.closeOtherDockViews(id),
      closeOthersLabel: t("dock.tabs.closeOthers"),
      onCloseAll: navigation.closeAllDockViews.bind(navigation),
      closeAllLabel: t("dock.tabs.closeAll"),
    };
  });
}

function ConversationPane({
  ref,
  hidden,
  fallbackFocus,
  ready,
  sessionId,
  onSend,
}: {
  ref: Ref<HTMLDivElement>;
  hidden: boolean;
  fallbackFocus: RefObject<HTMLElement | null>;
  ready: boolean;
  sessionId: string;
  onSend: Props["onSend"];
}) {
  const t = useT();
  return (
    <div
      ref={ref}
      inert={hidden}
      aria-hidden={hidden || undefined}
      tabIndex={-1}
      role="region"
      aria-label={t("shell.region.conversation")}
      data-focus-inset=""
      {...stylex.props(sh.paneNarrow, paneStyles.conversation, hidden && paneStyles.inactiveView)}
    >
      <ContentVisibility visible={!hidden} fallbackFocus={fallbackFocus}>
        <ConversationContent ready={ready} sessionId={sessionId} onSend={onSend} />
      </ContentVisibility>
    </div>
  );
}

// An inspected full-pane view returns to its source; any other closes.
function FullPaneHeader({
  viewId,
  view,
  returnAction,
  dockAvailable,
  onKeyDown,
  picker,
}: {
  viewId: string;
  view: ComponentProps<typeof WorkspaceWindowHeader>["view"];
  returnAction: { label: string; onReturn: () => void } | undefined;
  dockAvailable: boolean;
  onKeyDown: ((event: KeyboardEvent<HTMLElement>) => void) | undefined;
  picker: ReactNode;
}) {
  const workspaceAccess = useWorkspace();
  return (
    <WorkspaceWindowHeader
      view={view}
      exit={
        returnAction
          ? { kind: "return", ...returnAction }
          : { kind: "close", onClose: () => closeWorkspaceView(workspaceAccess, viewId) }
      }
      onOpenInDock={
        dockAvailable ? () => openWorkspaceViewInDock(workspaceAccess, viewId) : undefined
      }
      onKeyDown={onKeyDown}
    >
      {picker}
    </WorkspaceWindowHeader>
  );
}

export function ChatPanel({ onSend, ref }: Props) {
  const pickerRef = useRef<HTMLButtonElement>(null);
  const composerAccess = useComposer();
  const workspaceAccess = useWorkspace();

  const activeMainView = useActiveWorkspaceViewId();
  const dock = useWorkspaceDock();
  const catalog = useContextDockCatalog();
  const views = useWorkspaceViews();
  const { layout: dockLayout, save: saveDockLayout } = useDockLayout();
  const [initialDockLayout] = useState(() => dockLayout ?? undefined);
  const { data: sessions } = useAgentSessions();
  const activeSessionId = useActiveSessionId();
  const t = useT();
  const inspection = useWorkspaceInspection();
  const [rowWidth, dockRowRef] = useDockRowWidth();
  const dockAvailable = canPresentDock(rowWidth ?? 0);

  const {
    hasDockOwner,
    dockOpen,
    dockVisible,
    ownedDockViewIds,
    fullPaneViewId,
    inspectedFullPane,
  } = workspacePresentation({ activeSessionId, dock, activeMainView, inspection, rowWidth });
  const conversationReady = sessions !== undefined;
  const viewsReady = conversationReady || activeMainView !== null || dock.open;
  const {
    dockRef,
    conversationRef,
    mainViewRef,
    presentationRef,
    returnToSource,
    onMaterialKeyDown,
    requestComposerFocus,
  } = useInspectionFocus({
    sessionId: activeSessionId,
    inspection,
    mainViewId: activeMainView,
    fullPaneViewId,
    dockViewId: dock.activeViewId,
    dockViewIds: ownedDockViewIds,
    dockAvailable,
    dockVisible,
  });
  useLayoutEffect(
    () => registerComposerFocusPresentation(composerAccess, requestComposerFocus),
    [requestComposerFocus, composerAccess],
  );
  useImperativeHandle(ref, () => ({ focusComposer: requestComposerFocus }));

  const viewsById = new Map(views.map((view) => [view.id, view]));
  const returnAction = inspection
    ? { label: returnActionLabel(t, inspection, viewsById), onReturn: returnToSource }
    : undefined;

  const dockTabs = useDockTabs(ownedDockViewIds, viewsById, dock.activeViewId);
  const openViewIds = new Set(ownedDockViewIds);
  if (activeMainView !== null) openViewIds.add(activeMainView);
  const presentedMain = fullPaneViewId !== null;
  const panelSizes = PANEL_SIZES[presentedMain ? "main" : dockVisible ? "dock" : "closed"];
  const mountedViewIds = mountedWorkspaceViews(openViewIds, inspection);
  const presentedViewId = presentedMain ? fullPaneViewId : dock.activeViewId;

  return (
    <AgentContentCard
      label={t("shell.region.workspace")}
      onFocusCapture={applyComposerReadingLimit}
    >
      {presentedMain && (
        <FullPaneHeader
          viewId={fullPaneViewId}
          view={
            fullPaneViewId === WORKSPACE_DOCK_CATALOG ? CATALOG_VIEW : viewsById.get(fullPaneViewId)
          }
          returnAction={inspectedFullPane ? returnAction : undefined}
          dockAvailable={dockAvailable}
          onKeyDown={inspectedFullPane ? onMaterialKeyDown : undefined}
          picker={
            hasDockOwner &&
            fullPaneViewId !== WORKSPACE_DOCK_CATALOG &&
            viewsById.get(fullPaneViewId)?.dock !== undefined && (
              <WorkspaceViewPicker
                triggerRef={pickerRef}
                key={activeSessionId}
                groups={catalog}
                openViewIds={openViewIds}
                activeViewId={fullPaneViewId}
                onSelect={openWorkspaceView.bind(null, workspaceAccess)}
              />
            )
          }
        />
      )}
      <AgentDockRow
        ref={dockRowRef}
        open={dockVisible}
        presentation={presentedMain ? "main" : "dock"}
      >
        <Group
          orientation="horizontal"
          defaultLayout={initialDockLayout}
          onLayoutChanged={(layout, meta) => {
            if (meta.isUserInteraction) saveDockLayout(meta.requestedLayout ?? layout);
          }}
          style={{ flex: 1, width: "100%", minHeight: 0 }}
        >
          <Panel id="conversation" {...panelSizes.conversation}>
            <ConversationPane
              ref={conversationRef}
              hidden={presentedMain}
              fallbackFocus={presentationRef}
              ready={conversationReady}
              sessionId={activeSessionId}
              onSend={onSend}
            />
          </Panel>
          {!presentedMain && dockVisible && <AgentDockSeparator label={t("dock.action.resize")} />}
          <Panel id="dock" defaultSize={480} {...panelSizes.dock}>
            {viewsReady && (
              <SessionOwnedWorkspaceState sessionId={activeSessionId}>
                <AgentContextDock
                  value={presentedViewId === WORKSPACE_DOCK_CATALOG ? null : presentedViewId}
                  onValueChange={(id) => {
                    if (id !== null) workspaceAccess.navigation.selectDockView(id);
                  }}
                  ref={dockRef}
                  role={presentedMain ? "presentation" : undefined}
                  tabIndex={presentedMain ? undefined : -1}
                  aria-label={
                    presentedMain
                      ? undefined
                      : t(viewsById.get(dock.activeViewId ?? "")?.title ?? "dock.tabs.label")
                  }
                  data-focus-inset=""
                  onKeyDown={
                    activeMainView === null || inspectedFullPane ? onMaterialKeyDown : undefined
                  }
                >
                  {!presentedMain && dockVisible && (
                    <DockHeader
                      key={activeSessionId}
                      tabs={dockTabs}
                      groups={catalog}
                      openViewIds={openViewIds}
                      returnAction={returnAction}
                    />
                  )}
                  <WorkspaceViewStack
                    presentation={presentedMain ? "main" : "dock"}
                    shown={presentedMain || dockVisible}
                    presentedViewId={presentedViewId}
                    fullPaneViewId={fullPaneViewId}
                    addressable={activeMainView === null}
                    mountedViewIds={mountedViewIds}
                    mainViewRef={mainViewRef}
                    presentationRef={presentationRef}
                    catalog={catalog}
                    openViewIds={openViewIds}
                  />
                </AgentContextDock>
              </SessionOwnedWorkspaceState>
            )}
          </Panel>
        </Group>
        {viewsReady && !presentedMain && hasDockOwner && (
          <AgentDockToggle
            open={dockVisible}
            onToggle={() => toggleChatDock(workspaceAccess, dockAvailable, dockOpen)}
            showLabel={dockAvailable ? t("dock.action.show") : t("dock.action.openFull")}
            hideLabel={t("dock.action.hide")}
          />
        )}
      </AgentDockRow>
    </AgentContentCard>
  );
}

const paneStyles = stylex.create({
  conversation: {
    height: "100%",
    containerType: "inline-size",
    containerName: "conversation",
  },
  pendingConnection: { paddingBlock: space.s4 },
  // Removing layout makes WebKit reset the retained scrollport's reading position.
  inactiveView: { visibility: "hidden" },
});
