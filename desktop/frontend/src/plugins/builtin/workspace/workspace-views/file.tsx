import * as stylex from "@stylexjs/stylex";
import { Activity, useLayoutEffect, useRef, useState } from "react";
import { copyText } from "@/lib/clipboard";
import { isImeKey } from "@/lib/ime";
import { lookupExtensionByKey } from "@/plugins/sdk";
import { WORKSPACE_FILE_RENDERER } from "@/plugins/sdk/kernelPoints";
import {
  Button,
  DataView,
  EmptyState,
  FilePath,
  IconButton,
  Popover,
  Segmented,
  TextButton,
  TextField,
} from "@/ui";
import { space, type as typeStep } from "@/styles/tokens.stylex";
import { openWorkspacePath, revealWorkspacePath } from "../adapters/desktopReveal";
import { fileKind, isUnsupportedFileRead } from "../application/fileKind";
import { useT } from "@/lib/i18n";
import { FileView } from "./views/FileView";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
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
import type { WorkspaceFileViewer } from "../application/ports/navigationState";
import { FileTree } from "./views/FileTree";

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

function extensionOf(path: string): string {
  const dot = path.lastIndexOf(".");
  return dot > path.lastIndexOf("/") ? path.slice(dot + 1).toLowerCase() : "";
}

export function FileViewTab() {
  const viewer = useWorkspaceFileViewer();
  return (
    <>
      <Activity mode={viewer ? "hidden" : "visible"}>
        <FileBrowser />
      </Activity>
      {viewer && <FilePreview viewer={viewer} />}
    </>
  );
}

function FileBrowser() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const query = useWorkspaceListFiles(workspace.status === "ready" ? { cwd } : undefined);
  return (
    <WorkspaceViewLayout scrollInset="flush" icon="folder" title="workspace.view.title.file">
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

function FilePreview({ viewer }: { viewer: WorkspaceFileViewer }) {
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

  const renderer = lookupExtensionByKey(WORKSPACE_FILE_RENDERER, extensionOf(viewer.path));
  const wholeFile = data !== undefined && !moreBefore && !moreAfter;
  const [mode, setMode] = useState<"rendered" | "source">("rendered");
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

  const unsupported = isUnsupportedFileRead(error);
  const kind = fileKind(viewer.path);

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
      titleFace="mono"
      icon="filetext"
      title={viewer.path}
      dockIdentity={<FilePath path={viewer.path} />}
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
            icon="copy"
            size="sm"
            title={t("file.copyPath")}
            onClick={() => void copyText(viewer.path)}
          />
          {cwd && (
            <IconButton
              icon="folder-open"
              size="sm"
              title={t("file.reveal")}
              onClick={() => void revealWorkspacePath(cwd, viewer.path)}
            />
          )}
          <IconButton
            icon="arrow-left"
            size="sm"
            title={t("file.backToFiles")}
            onClick={closeWorkspaceFile}
          />
        </>
      }
      sub={sub}
    >
      {unsupported ? (
        <EmptyState
          icon={kind === "image" ? "image" : "file"}
          title={t(kind === "image" ? "file.unsupported.image" : "file.unsupported.binary")}
          sub={t("file.unsupported.sub")}
          action={
            cwd && (
              <Button size="sm" onClick={() => void openWorkspacePath(cwd, viewer.path)}>
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
                  const Renderer = renderer;
                  return <Renderer path={viewer.path} content={items[0]!.content} />;
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
        render={<IconButton icon="crosshair" size="sm" title={t("file.goToLine")} />}
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
