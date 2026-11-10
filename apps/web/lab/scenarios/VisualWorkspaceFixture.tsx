import { useLayoutEffect, useRef, useState } from "react";
import { Checkbox } from "@flame/ui";
import { Slot } from "@/extensions/host/Slot";
import { AppToaster } from "@/features/notifications/toaster/index";
import { useActiveWorkspaceViewId, useWorkspaceDock } from "@/features/workspace/public/navigation";
import {
  useDockWidth,
  useSidebarDrawer,
  useSidebarWidth,
} from "@/features/workspace/public/sidebarDrawer";
import { AgentAppShell } from "@/ui/agent/index";
import { AgentRow } from "@/ui/agent/index";
import { AgentSurfaceHeader } from "@/ui/agent/index";
import { useLocale } from "@/lib/i18n";
import {
  VISUAL_WORKSPACE_STATE_TITLES,
  VISUAL_WORKSPACE_STATES,
  type VisualWorkspaceState,
} from "./workspaceFixtureStates";
import * as stylex from "@stylexjs/stylex";
import { fx } from "./fixtureStyles";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { observationTargetName } from "../observations";
import { localize } from "../story";

function WorkspaceStateSidebar({
  state,
  observeLayout,
  onObserveLayoutChange,
}: {
  state: VisualWorkspaceState;
  observeLayout: boolean;
  onObserveLayoutChange: (observe: boolean) => void;
}) {
  const locale = useLocale() === "zh" ? "zh-CN" : "en";
  const listRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const land = () => {
      const target =
        list.querySelector<HTMLElement>(":focus") ??
        list.querySelector<HTMLElement>("[data-active]");
      if (!target) return;
      const bounds = target.getBoundingClientRect();
      const frame = list.getBoundingClientRect();
      const top = frame.top + list.clientTop;
      const bottom = top + list.clientHeight;
      if (bounds.top < top) list.scrollTop += bounds.top - top;
      else if (bounds.bottom > bottom) {
        list.scrollTop += bounds.bottom - bottom;
      }
      list.scrollTop = Math.floor(list.scrollTop);
    };
    land();
    const observer = new ResizeObserver(land);
    observer.observe(list);
    return () => observer.disconnect();
  }, [state]);
  return (
    <div data-fixture-chrome="" {...stylex.props(fx.pane)}>
      <AgentSurfaceHeader corner="drawer" divider={false} />
      <div ref={listRef} {...stylex.props(fx.scroller)}>
        <span {...stylex.props(fx.listHead, typeStep.uiMd)}>Workspace states</span>
        {VISUAL_WORKSPACE_STATES.map((candidate) => (
          <AgentRow
            key={candidate}
            icon={
              candidate === "settings"
                ? "settings"
                : candidate.includes("error")
                  ? "alert"
                  : "panel-r"
            }
            active={candidate === state}
            revealOverflow
          >
            {localize(VISUAL_WORKSPACE_STATE_TITLES[candidate], locale)}
          </AgentRow>
        ))}
      </div>
      <div {...stylex.props(fx.footGap)} />
      <div {...stylex.props(fx.listFoot, fx.ink)}>
        <Checkbox
          checked={observeLayout}
          onCheckedChange={onObserveLayoutChange}
          label="Observe layout"
        />
      </div>
    </div>
  );
}

interface FocusDeparture {
  target: string | null;
  related: string | null;
  focused: boolean;
}

const layoutRect = (element: Element | null) => element?.getBoundingClientRect().toJSON() ?? null;

function workspaceReadingTarget(
  reader: HTMLElement | null,
  active: Element | null,
  previous: Element | null,
) {
  let target = previous;
  if (
    active &&
    reader?.contains(active) &&
    active.closest('[data-slot="panel-body"]') &&
    (active.matches('button, a[href], input, select, textarea, [role="button"]') ||
      active.hasAttribute("aria-label"))
  ) {
    target = active;
  }
  return target?.isConnected && reader?.contains(target) ? target : null;
}

function captureComposerLayout(view: Window, composer: HTMLElement | null) {
  if (!composer) return null;
  const scrollport = view.document.querySelector<HTMLElement>('[data-slot="composer-scrollport"]');
  const footer = composer.querySelector<HTMLElement>('[data-slot="composer-footer"]');
  return {
    rect: layoutRect(composer),
    width: view.getComputedStyle(composer).width,
    clientWidth: composer.clientWidth,
    total: composer.scrollWidth,
    scrollport: scrollport
      ? {
          top: scrollport.scrollTop,
          height: scrollport.clientHeight,
          total: scrollport.scrollHeight,
        }
      : null,
    footer: footer ? { rect: layoutRect(footer), total: footer.scrollWidth } : null,
    actions: Array.from(footer?.querySelectorAll("button") ?? []).map((action) => {
      const label = action.querySelector('[data-slot="composer-chip-label"]');
      return {
        rect: layoutRect(action),
        label: label ? view.getComputedStyle(label).display : null,
      };
    }),
  };
}

function captureWorkspaceLayout({
  view,
  scope,
  reader,
  reading,
  controls,
  composer,
  active,
  blur,
}: {
  view: Window;
  scope: string | null;
  reader: HTMLElement | null;
  reading: Element | null;
  controls: Element | null;
  composer: HTMLElement | null;
  active: Element | null;
  blur: FocusDeparture | null;
}) {
  const document = view.document;
  const dock = reader?.closest(".agent-context-dock") ?? null;
  const row = dock?.closest(".agent-dock-row") ?? null;
  return {
    view: scope,
    window: {
      width: view.innerWidth,
      height: view.innerHeight,
      documentWidth: document.documentElement.clientWidth,
      dpr: view.devicePixelRatio,
      focused: document.hasFocus(),
    },
    visualViewport: view.visualViewport
      ? {
          width: view.visualViewport.width,
          height: view.visualViewport.height,
          scale: view.visualViewport.scale,
        }
      : null,
    dock: {
      state: row?.getAttribute("data-dock"),
      presentation: row?.getAttribute("data-presentation"),
      rect: layoutRect(dock),
    },
    viewport: reader
      ? {
          rect: layoutRect(reader),
          width: reader.clientWidth,
          height: reader.clientHeight,
          top: reader.scrollTop,
          total: reader.scrollHeight,
          fontSize: view.getComputedStyle(reader).fontSize,
        }
      : null,
    controls: layoutRect(controls),
    focus: active ? { tag: active.localName, label: active.getAttribute("aria-label") } : null,
    blur,
    reading: reading ? { name: observationTargetName(reading), rect: layoutRect(reading) } : null,
    composer: captureComposerLayout(view, composer),
  };
}

function compactWorkspaceLayout(snapshot: ReturnType<typeof captureWorkspaceLayout>) {
  return {
    focused: snapshot.window.focused,
    focus: snapshot.focus,
    blur: snapshot.blur,
    window: [snapshot.window.width, snapshot.window.height],
    dock: snapshot.dock.state,
    viewport: snapshot.viewport
      ? [
          snapshot.viewport.width,
          snapshot.viewport.height,
          snapshot.viewport.top,
          snapshot.viewport.total,
        ]
      : null,
    reading: snapshot.reading
      ? {
          name: snapshot.reading.name,
          top: snapshot.reading.rect.top,
          bottom: snapshot.reading.rect.bottom,
        }
      : null,
    composer: snapshot.composer
      ? {
          width: snapshot.composer.rect?.width,
          css: snapshot.composer.width,
          total: snapshot.composer.total,
          scrollport: snapshot.composer.scrollport
            ? [
                snapshot.composer.scrollport.top,
                snapshot.composer.scrollport.height,
                snapshot.composer.scrollport.total,
              ]
            : null,
          footer: snapshot.composer.footer?.total,
          actions: snapshot.composer.actions.map((action) => [
            action.rect?.left,
            action.rect?.top,
            action.rect?.right,
            action.rect?.bottom,
            action.label,
          ]),
        }
      : null,
  };
}

function WorkspaceLayoutReadout({ enabled }: { enabled: boolean }) {
  const mainViewId = useActiveWorkspaceViewId();
  const dock = useWorkspaceDock();
  const scope = mainViewId ?? dock.activeViewId;
  const output = useRef<HTMLOutputElement>(null);
  const readingOutput = useRef<HTMLOutputElement>(null);
  useLayoutEffect(() => {
    const node = output.current;
    const readingNode = readingOutput.current;
    if (!node || !readingNode) return;
    node.textContent = "";
    readingNode.textContent = "";
    if (!enabled) return;
    const document = node.ownerDocument;
    const view = document.defaultView!;
    const state = {
      frame: 0,
      reader: null as HTMLElement | null,
      reading: null as Element | null,
      observedReader: null as Element | null,
      observedControls: null as Element | null,
      observedBody: null as Element | null,
      observedComposer: null as Element | null,
      blur: null as FocusDeparture | null,
    };
    const capture = () => {
      state.frame = 0;
      const reader =
        Array.from(
          document.querySelectorAll<HTMLElement>('[data-slot="workspace-view-body"]'),
        ).find(
          (candidate) =>
            !candidate.closest('[inert], [aria-hidden="true"]') &&
            view.getComputedStyle(candidate).visibility !== "hidden",
        ) ?? (state.reader?.isConnected ? state.reader : null);
      state.reader = reader;
      const active = document.activeElement;
      state.reading = workspaceReadingTarget(reader, active, state.reading);
      const reading = state.reading;
      const controls = reader?.querySelector('[data-slot="panel-controls"]') ?? null;
      // Content can reflow inside an unchanged viewport.
      const body = reader?.querySelector('[data-slot="panel-body"]') ?? null;
      const composer = document.querySelector<HTMLElement>('[data-slot="composer-root"]');
      for (const [before, next] of [
        [state.observedReader, reader],
        [state.observedControls, controls],
        [state.observedBody, body],
        [state.observedComposer, composer],
      ]) {
        if (before === next) continue;
        if (before) observer.unobserve(before);
        if (next) observer.observe(next);
      }
      state.observedReader = reader;
      state.observedControls = controls;
      state.observedBody = body;
      state.observedComposer = composer;
      const snapshot = captureWorkspaceLayout({
        view,
        scope,
        reader,
        reading,
        controls,
        composer,
        active,
        blur: state.blur,
      });
      node.textContent = JSON.stringify(snapshot);
      // Native accessibility snapshots bound each text node independently.
      readingNode.textContent = JSON.stringify(compactWorkspaceLayout(snapshot));
    };
    const schedule = () => {
      if (!state.frame) state.frame = view.requestAnimationFrame(capture);
    };
    const onBlur = (event: FocusEvent) => {
      state.blur = {
        target: event.target instanceof Element ? event.target.localName : null,
        related: event.relatedTarget instanceof Element ? event.relatedTarget.localName : null,
        focused: document.hasFocus(),
      };
      schedule();
    };
    const observer = new ResizeObserver(schedule);
    observer.observe(document.documentElement);
    document.addEventListener("focusin", schedule);
    document.addEventListener("focusout", onBlur);
    document.addEventListener("click", schedule, true);
    document.addEventListener("scroll", schedule, { capture: true, passive: true });
    view.addEventListener("resize", schedule);
    view.addEventListener("blur", schedule);
    view.addEventListener("focus", schedule);
    view.visualViewport?.addEventListener("resize", schedule);
    schedule();
    return () => {
      observer.disconnect();
      if (state.frame) view.cancelAnimationFrame(state.frame);
      document.removeEventListener("focusin", schedule);
      document.removeEventListener("focusout", onBlur);
      document.removeEventListener("click", schedule, true);
      document.removeEventListener("scroll", schedule, true);
      view.removeEventListener("resize", schedule);
      view.removeEventListener("blur", schedule);
      view.removeEventListener("focus", schedule);
      view.visualViewport?.removeEventListener("resize", schedule);
      node.textContent = "";
      readingNode.textContent = "";
    };
  }, [enabled, scope]);
  return (
    <>
      <output
        ref={output}
        className="sr-only"
        aria-label="Workspace layout observation"
        aria-live="off"
        data-testid="workspace-layout-observation"
      />
      <output
        ref={readingOutput}
        className="sr-only"
        aria-label="Workspace reading observation"
        aria-live="off"
        data-testid="workspace-reading-observation"
      />
    </>
  );
}

function WorkspaceFixtureReadout({ state }: { state: VisualWorkspaceState }) {
  const dock = useWorkspaceDock();
  const activeMainViewId = useActiveWorkspaceViewId();
  const dockWidthRatio = useDockWidth().width;

  return (
    <>
      <output className="sr-only" data-testid="requested-workspace-state">
        {state}
      </output>
      <output className="sr-only" data-testid="active-dock-view">
        {dock.activeViewId ?? ""}
      </output>
      <output className="sr-only" data-testid="dock-open">
        {String(dock.open)}
      </output>
      <output className="sr-only" data-testid="dock-view-ids">
        {dock.viewIds.join(",")}
      </output>
      <output className="sr-only" data-testid="active-main-view">
        {activeMainViewId ?? ""}
      </output>
      <output className="sr-only" data-testid="persisted-dock-ratio">
        {dockWidthRatio}
      </output>
    </>
  );
}

export function VisualWorkspaceFixture({ state }: { state: VisualWorkspaceState }) {
  const [observeLayout, setObserveLayout] = useState(false);
  const settingsOpen = useActiveWorkspaceViewId() === "settings";
  const sidebar = useSidebarDrawer();
  const sidebarWidth = useSidebarWidth();

  return (
    <AgentAppShell
      sidebarLabel="Workspace fixture states"
      sidebarResizeLabel="Resize the workspace fixture sidebar"
      sidebarOpen={!settingsOpen && !sidebar.collapsed}
      sidebarWidth={sidebarWidth.width}
      onResize={sidebarWidth.setWidth}
      onSidebarToggle={sidebar.toggle}
      sidebarExpandLabel="Expand the workspace fixture sidebar"
      sidebarCollapseLabel="Collapse the workspace fixture sidebar"
      sidebar={
        settingsOpen ? undefined : (
          <WorkspaceStateSidebar
            state={state}
            observeLayout={observeLayout}
            onObserveLayoutChange={setObserveLayout}
          />
        )
      }
      main={
        <div {...stylex.props(fx.contents)} data-testid="workspace-state" data-state={state}>
          <Slot name="app.main" />
        </div>
      }
      overlay={
        <>
          <Slot name="app.overlay" />
          <WorkspaceFixtureReadout state={state} />
          <WorkspaceLayoutReadout enabled={observeLayout} />
          <AppToaster />
        </>
      }
    />
  );
}
