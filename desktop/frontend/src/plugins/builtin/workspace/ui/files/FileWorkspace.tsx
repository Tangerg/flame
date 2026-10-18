import { iconGlyph } from "@/ui/icons";
import * as stylex from "@stylexjs/stylex";
import { Activity, useCallback, useLayoutEffect, useRef, useState } from "react";
import { copyText } from "@flame/ui";
import { fileExtension } from "@/lib/path";
import { fileKind } from "@/lib/fileKind";
import { isImeKey } from "@/lib/ime";
import { useExtensionPoint } from "@/plugins/sdk";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { WORKSPACE_FILE_RENDERER } from "@/plugins/sdk/kernelPoints";
import { DataView, FilePath, Popover, Segmented, TextButton, fileIconName } from "@/ui";
import { Button, EmptyState, IconButton, TextField } from "@flame/ui";
import { space, type as typeStep } from "@flame/ui/tokens.stylex";
import { localWorkspace } from "../../application/ports/localWorkspace";
import { workspaceErrorClassifier } from "../../application/ports/workspaceErrorClassifier";
import { useT } from "@/lib/i18n";
import { FileView } from "./FileView";
import { WorkspaceViewLayout } from "../WorkspaceViewLayout";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import {
  useWorkspaceListFiles,
  useWorkspaceReadFile,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import {
  closeWorkspaceFile,
  openWorkspaceFile,
  useWorkspaceFileViewer,
} from "@/plugins/builtin/workspace/public/navigation";
import type { WorkspaceFileViewer } from "../../application/ports/navigationState";
import { FileTree } from "./FileTree";
import { selectFileRenderer } from "../../application/fileRenderer";

const WINDOW_RADIUS = 200;

const fp = stylex.create({
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
  const returnFocus = useRef(false);
  const restoreFocus = useCallback((item: HTMLElement) => {
    if (!returnFocus.current) return;
    returnFocus.current = false;
    if (item.ownerDocument.activeElement === item.ownerDocument.body) item.focus();
  }, []);
  return (
    <>
      <Activity mode={viewer ? "hidden" : "visible"}>
        <FileBrowser restoreFocus={restoreFocus} />
      </Activity>
      {viewer && (
        <FilePreview
          viewer={viewer}
          onBack={() => {
            returnFocus.current = true;
            closeWorkspaceFile();
          }}
        />
      )}
    </>
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

function FilePreview({ viewer, onBack }: { viewer: WorkspaceFileViewer; onBack: () => void }) {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const targetLine = viewer.line;
  const [windowFor, setWindowFor] = useState<{ intent: object; window: LineWindow }>(() => ({
    intent: viewer,
    window: initialWindow(targetLine),
  }));
  const lineWindow = windowFor.intent === viewer ? windowFor.window : initialWindow(targetLine);
  const setLineWindow = (next: LineWindow) => setWindowFor({ intent: viewer, window: next });

  const { data, isLoading, error, refetch } = useWorkspaceReadFile(
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
  const anchor = useRef<{ height: number } | null>(null);
  useLayoutEffect(() => {
    const host = scrollPort.current;
    if (!anchor.current || !host) return;
    host.scrollTop += host.scrollHeight - anchor.current.height;
    anchor.current = null;
  }, [data]);

  const loadEarlier = () => {
    const host = scrollPort.current;
    anchor.current = host ? { height: host.scrollHeight } : null;
    setLineWindow({ start: Math.max(1, firstShown - WINDOW_RADIUS), end: lastShown });
  };
  const loadLater = () => setLineWindow({ start: firstShown, end: lastShown + WINDOW_RADIUS });

  const unsupported = workspaceErrorClassifier().isUnsupportedFile(error);
  const kind = fileKind(viewer.path);
  const icon = fileIconName(viewer.path);

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
      identity={<FilePath path={viewer.path} />}
      actions={
        <>
          {renderer && wholeFile && (
            <Segmented
              value={mode}
              onChange={setMode}
              ariaLabel={t("file.mode")}
              options={[
                { value: "rendered", label: t("file.mode.rendered") },
                { value: "source", label: t("file.mode.source") },
              ]}
            />
          )}
          {!unsupported && <GoToLine onGo={(line) => openWorkspaceFile(viewer.path, line)} />}
          <IconButton
            icon={iconGlyph("copy")}
            size="sm"
            aria-label={t("file.copyPath")}
            onClick={() => void copyText(viewer.path)}
          />
          {cwd && localWorkspace().available() && (
            <IconButton
              icon={iconGlyph("folder-open")}
              size="sm"
              aria-label={t("file.reveal")}
              onClick={() => void localWorkspace().reveal(cwd, viewer.path)}
            />
          )}
          <IconButton
            icon={iconGlyph("arrow-left")}
            size="sm"
            aria-label={t("file.backToFiles")}
            onClick={onBack}
          />
        </>
      }
      sub={sub}
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
                      intent={viewer}
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
                  intent={viewer}
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
