import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { Activity, useCallback, useLayoutEffect, useRef, useState, type Ref } from "react";
import { fileExtension, splitFilePath } from "@/lib/path";
import { fileKind } from "@/lib/fileKind";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { PluginBoundary } from "@/extensions/host/PluginBoundary";
import { WORKSPACE_FILE_RENDERER } from "@/features/workspace/public/views";
import { DataView } from "@/ui/index";
import { Popover } from "@/ui/index";
import { Segmented } from "@flame/ui";
import { TextButton } from "@/ui/index";
import { Icon, fileIconName } from "@/ui/index";
import { Button, EmptyState, IconButton, isImeKey, TextField, useCopyFeedback } from "@flame/ui";
import { color, face, space, type as typeStep } from "@flame/ui/tokens.stylex";
import { localWorkspace } from "@/features/workspace/application/ports/localWorkspace";
import { workspaceErrorClassifier } from "@/features/workspace/application/ports/workspaceErrorClassifier";
import { useT } from "@/lib/i18n/index";
import { FileView } from "@/features/workspace/ui/files/FileView";
import { WorkspaceViewLayout } from "@/features/workspace/ui/WorkspaceViewLayout";
import { useActiveSessionWorkspace } from "@/features/agent/public/session";
import {
  useWorkspaceListFiles,
  useWorkspaceReadFile,
} from "@/features/workspace/application/workspaceQueries";
import {
  closeWorkspaceFile,
  openWorkspaceFile,
  useWorkspaceFileViewer,
} from "@/features/workspace/public/navigation";
import { type WorkspaceFileViewer } from "@/features/workspace/application/ports/navigationState";
import { FileTree } from "@/features/workspace/ui/files/FileTree";
import { selectFileRenderer } from "@/features/workspace/application/fileRenderer";
const WINDOW_RADIUS = 200;

const fp = stylex.create({
  workspace: { display: "flex", flex: 1, flexDirection: "column", minWidth: 0, minHeight: 0 },
  context: { display: "flex", flexDirection: "column", gap: space.s0_5, minWidth: 0 },
  identity: { display: "flex", alignItems: "center", gap: space.s1_5, minWidth: 0 },
  identityIcon: { flexShrink: 0, color: color.fgFaint },
  identityName: { minWidth: 0, overflowWrap: "anywhere" },
  edge: { display: "flex", justifyContent: "center", paddingBlock: space.s1_5 },
  goto: {
    display: "flex",
    width: "220px",
    flexDirection: "column",
    gap: space.s2,
    padding: space.s2,
  },
});

interface LineWindow {
  start?: number;
  end?: number;
}

function initialWindow(line: number): LineWindow {
  return line > 0 ? { start: Math.max(1, line - WINDOW_RADIUS), end: line + WINDOW_RADIUS } : {};
}

export function FileWorkspace() {
  const viewer = useWorkspaceFileViewer();
  const panelRef = useRef<HTMLDivElement>(null);
  const backRef = useRef<HTMLButtonElement>(null);
  const previousViewer = useRef(viewer);
  const returnFocus = useRef(false);
  const restoreFocus = useCallback((item: HTMLElement) => {
    if (!returnFocus.current) return;
    returnFocus.current = false;
    if (item.ownerDocument.activeElement === item.ownerDocument.body) item.focus();
  }, []);
  useLayoutEffect(() => {
    const previous = previousViewer.current;
    previousViewer.current = viewer;
    const panel = panelRef.current;
    if (!panel) return;
    returnFocus.current = false;
    const active = panel.ownerDocument.activeElement;
    if (active !== panel.ownerDocument.body && !panel.contains(active)) return;
    if (viewer) {
      backRef.current?.focus({ preventScroll: true });
    } else if (previous) {
      returnFocus.current = true;
      const item = panel.querySelector<HTMLElement>('[role="treeitem"][tabindex="0"]');
      if (item) restoreFocus(item);
    }
  }, [viewer, restoreFocus]);
  return (
    <div ref={panelRef} {...stylex.props(fp.workspace)}>
      <Activity mode={viewer ? "hidden" : "visible"}>
        <FileBrowser restoreFocus={restoreFocus} />
      </Activity>
      {viewer && <FilePreview viewer={viewer} backRef={backRef} />}
    </div>
  );
}

function FileBrowser({ restoreFocus }: { restoreFocus: (item: HTMLElement) => void }) {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const query = useWorkspaceListFiles(workspace.status === "ready" ? { cwd } : undefined);
  const scrollPort = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const item = scrollPort.current?.querySelector<HTMLElement>('[role="treeitem"][tabindex="0"]');
    if (item) restoreFocus(item);
  }, [query.data, restoreFocus]);
  return (
    <WorkspaceViewLayout scrollInset="flush" scrollRef={scrollPort}>
      <DataView
        items={query.data}
        isLoading={query.isLoading || workspace.status === "resolving"}
        failure={query.error}
        onRetry={query.refetch}
        unsupported={{ icon: "folder" }}
        skeletonCount={8}
        empty={{ icon: "folder", title: t("file.empty.title"), sub: t("file.empty.sub") }}
      >
        {(entries) => <FileTree entries={entries} cwd={cwd} onSelectFile={openWorkspaceFile} />}
      </DataView>
    </WorkspaceViewLayout>
  );
}

function FilePreview({
  viewer,
  backRef,
}: {
  viewer: WorkspaceFileViewer;
  backRef: Ref<HTMLButtonElement>;
}) {
  const t = useT();
  const { copied, copy } = useCopyFeedback(viewer.path);
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const targetLine = viewer.line;
  const [windowFor, setWindowFor] = useState<{ intent: object; window: LineWindow }>(() => ({
    intent: viewer,
    window: initialWindow(targetLine),
  }));
  const lineWindow = windowFor.intent === viewer ? windowFor.window : initialWindow(targetLine);
  const setLineWindow = (next: LineWindow) => setWindowFor({ intent: viewer, window: next });

  const { data, isPlaceholderData, isLoading, error, refetch } = useWorkspaceReadFile(
    workspace.status === "ready"
      ? {
          cwd,
          path: viewer.path,
          ...(lineWindow.start !== undefined ? { startLine: lineWindow.start } : {}),
          ...(lineWindow.end !== undefined ? { endLine: lineWindow.end } : {}),
        }
      : undefined,
  );

  const shownLines = data ? data.content.split("\n").length : 0;
  const firstShown = data?.startLine ?? 1;
  const lastShown = firstShown + shownLines - 1;
  const moreBefore = firstShown > 1;
  const moreAfter = data !== undefined && lastShown < data.totalLines;

  const renderers = useExtensionPoint(WORKSPACE_FILE_RENDERER);
  const renderer = selectFileRenderer(renderers, fileExtension(viewer.path));
  const wholeFile = data !== undefined && !data.truncated && !moreBefore && !moreAfter;
  const [modeFor, setModeFor] = useState<{ intent: object; mode: "rendered" | "source" }>(() => ({
    intent: viewer,
    mode: targetLine > 0 ? "source" : "rendered",
  }));
  const mode = modeFor.intent === viewer ? modeFor.mode : targetLine > 0 ? "source" : "rendered";
  const setMode = (mode: "rendered" | "source") => setModeFor({ intent: viewer, mode });
  const showRendered = renderer !== undefined && wholeFile && mode === "rendered";

  const scrollPort = useRef<HTMLDivElement>(null);
  // Bound to the window it requested; any later window or navigation retires it.
  const anchor = useRef<{ height: number; window: LineWindow } | null>(null);
  useLayoutEffect(() => {
    const host = scrollPort.current;
    if (!anchor.current || !host) return;
    if (anchor.current.window !== lineWindow) {
      anchor.current = null;
      return;
    }
    if (isPlaceholderData) return;
    host.scrollTop += host.scrollHeight - anchor.current.height;
    anchor.current = null;
  }, [data, isPlaceholderData, lineWindow]);

  const loadEarlier = () => {
    const host = scrollPort.current;
    const next = { start: Math.max(1, firstShown - WINDOW_RADIUS), end: lastShown };
    anchor.current = host ? { height: host.scrollHeight, window: next } : null;
    setLineWindow(next);
  };
  const loadLater = () => setLineWindow({ start: firstShown, end: lastShown + WINDOW_RADIUS });

  const unsupported = workspaceErrorClassifier().isUnsupportedFile(error);
  const kind = fileKind(viewer.path);
  const icon = fileIconName(viewer.path);
  const { directory, name } = splitFilePath(viewer.path);

  const sub = data ? (
    <span>
      {moreBefore || moreAfter
        ? t("file.range", { start: firstShown, end: lastShown, total: data.totalLines })
        : t("file.lines", { count: data.totalLines })}
      {data.truncated && ` · ${t("file.truncated")}`}
    </span>
  ) : undefined;

  return (
    <WorkspaceViewLayout
      scrollInset="flush"
      scrollRef={scrollPort}
      scrollLabel={viewer.path}
      navigation={
        <IconButton
          ref={backRef}
          icon={iconGlyph("chevron-left")}
          size="sm"
          aria-label={t("file.backToFiles")}
          data-chrome-focus=""
          onClick={closeWorkspaceFile}
        />
      }
      identity={
        <span title={viewer.path} {...stylex.props(fp.identity)}>
          <Icon name={icon} size="sm" className={stylex.props(fp.identityIcon).className} />
          <span {...stylex.props(fp.identityName)}>{name}</span>
        </span>
      }
      sub={
        <span {...stylex.props(fp.context)}>
          {directory && <span {...stylex.props(face.mono)}>{directory}/</span>}
          {sub}
        </span>
      }
      controls={
        renderer && wholeFile ? (
          <Segmented
            value={mode}
            onChange={setMode}
            ariaLabel={t("file.mode")}
            options={[
              { value: "rendered", label: t("file.mode.rendered") },
              { value: "source", label: t("file.mode.source") },
            ]}
          />
        ) : undefined
      }
      actions={
        <>
          {!unsupported && <GoToLine onGo={(line) => openWorkspaceFile(viewer.path, line)} />}
          <IconButton
            icon={iconGlyph(copied ? "check" : "copy")}
            size="sm"
            aria-label={t(copied ? "file.pathCopied" : "file.copyPath")}
            onClick={() => void copy()}
          />
          {cwd && localWorkspace().available() && (
            <IconButton
              icon={iconGlyph("folder-open")}
              size="sm"
              aria-label={t("file.reveal")}
              onClick={() => void localWorkspace().reveal(cwd, viewer.path)}
            />
          )}
        </>
      }
    >
      {unsupported ? (
        <EmptyState
          icon={iconGlyph(icon)}
          title={t(kind === "image" ? "file.unsupported.image" : "file.unsupported.binary")}
          sub={t("file.unsupported.sub")}
          action={
            cwd &&
            localWorkspace().available() && (
              <Button size="sm" onClick={() => void localWorkspace().open(cwd, viewer.path)}>
                {t("file.open")}
              </Button>
            )
          }
        />
      ) : (
        <DataView
          items={data ? [data] : []}
          isLoading={isLoading || workspace.status === "resolving"}
          failure={error}
          onRetry={refetch}
          skeletonCount={12}
          error={{ title: t("file.error.title"), sub: t("file.error.sub") }}
        >
          {(items) => (
            <div>
              {moreBefore && !showRendered && (
                <div {...stylex.props(fp.edge)}>
                  <TextButton tone="accent" size="sm" onClick={loadEarlier}>
                    {t("file.loadEarlier", { count: Math.min(WINDOW_RADIUS, firstShown - 1) })}
                  </TextButton>
                </div>
              )}
              {showRendered && renderer ? (
                (() => {
                  const Renderer = renderer.component;
                  const recorded = (
                    <FileView
                      path={viewer.path}
                      content={items[0]!.content}
                      startLine={items[0]!.startLine}
                      targetLine={targetLine}
                      intent={isPlaceholderData ? null : viewer}
                    />
                  );
                  return (
                    <PluginBoundary
                      key={renderer.id}
                      plugin={renderer.id}
                      label={renderer.title}
                      fallback={recorded}
                    >
                      <Renderer cwd={cwd} path={viewer.path} {...items[0]!} fallback={recorded} />
                    </PluginBoundary>
                  );
                })()
              ) : (
                <FileView
                  path={viewer.path}
                  content={items[0]!.content}
                  startLine={items[0]!.startLine}
                  targetLine={targetLine}
                  intent={isPlaceholderData ? null : viewer}
                />
              )}
              {moreAfter && !showRendered && (
                <div {...stylex.props(fp.edge)}>
                  <TextButton tone="accent" size="sm" onClick={loadLater}>
                    {t("file.loadLater", { count: WINDOW_RADIUS })}
                  </TextButton>
                </div>
              )}
            </div>
          )}
        </DataView>
      )}
    </WorkspaceViewLayout>
  );
}

function GoToLine({ onGo }: { onGo: (line: number) => void }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const line = Number(value);
  const valid = Number.isInteger(line) && line > 0;
  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger
        render={
          <IconButton icon={iconGlyph("crosshair")} size="sm" aria-label={t("file.goToLine")} />
        }
      />
      <Popover.Content align="end" sideOffset={6}>
        <div {...stylex.props(fp.goto)}>
          <TextField
            inputMode="numeric"
            aria-label={t("file.goToLine")}
            placeholder={t("file.goToLine.placeholder")}
            value={value}
            onChange={(event) => setValue(event.target.value.replace(/[^0-9]/g, ""))}
            onKeyDown={(event) => {
              if (event.key !== "Enter" || isImeKey(event.nativeEvent) || !valid) return;
              event.preventDefault();
              onGo(line);
              setOpen(false);
              setValue("");
            }}
            {...stylex.props(typeStep.uiMd)}
          />
        </div>
      </Popover.Content>
    </Popover.Root>
  );
}
