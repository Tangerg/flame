import * as stylex from "@stylexjs/stylex";
import { useEffect, useId, useRef, useState } from "react";
import {
  DataView,
  DiffStat,
  FilePath,
  Icon,
  Pressable,
  ScrollArea,
  Segmented,
  chevron,
  vocab,
} from "@/ui";
import { AgentViewNavigatorToggle, AgentViewSplit, AgentWorkspaceView } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import type { DiffLayout } from "./views/DiffView";
import { DiffView } from "./views/DiffView";
import { ReviewFileTree } from "./views/ReviewFileTree";
import { ViewHeader } from "./views/ViewHeader";
import { cn } from "@/lib/classNames";
import { gitOffEmpty, notARepoEmpty } from "./views/vcsGate";
import { focusWorkspaceFile } from "@/plugins/builtin/workspace/application/navigation";
import {
  type WorkspaceDiffMode,
  type WorkspaceFileDiff,
  workspaceDiffFileHeader,
  useWorkspaceDiffView,
} from "@/plugins/builtin/workspace/application/diffViewModel";
import { color, space, type as typeStep } from "@/styles/tokens.stylex";
import { codeStyles as cs } from "./views/viewStyles";
const df = stylex.create({
  pathLine: { display: "flex", minWidth: 0, flex: 1, alignItems: "baseline", gap: space.s1_5 },
  // The old path yields first and by a wide margin: what matters is where the file IS now.
  oldPath: { flexShrink: 100, color: color.fgFaint },
  glyphStep: { opacity: "var(--glyph-step)" },
  newPath: { flexShrink: 1 },
  glyph: { opacity: "var(--glyph-step)" },
  sep: { marginInline: space.s2 },
  scroller: { minWidth: 0, paddingInline: space.s2, paddingBottom: space.s2 },
});

const FILE_ANCHOR = "data-diff-file";

function FileCard({
  file,
  layout,
  collapsed,
  onToggle,
}: {
  file: WorkspaceFileDiff;
  layout: DiffLayout;
  collapsed: boolean;
  onToggle: () => void;
}) {
  const t = useT();
  const panelId = useId();
  const header = workspaceDiffFileHeader(file);
  return (
    <section {...{ [FILE_ANCHOR]: file.path }} {...stylex.props(cs.fileCard)}>
      <Pressable
        type="button"
        data-chrome-focus=""
        aria-expanded={!collapsed}
        aria-controls={panelId}
        onClick={onToggle}
        className={cn(
          "flex h-8 w-full min-w-0 items-center gap-2 border-0 bg-sunken px-3",
          "text-left font-mono text-ui-sm text-fg-muted transition-colors hover:text-fg",
        )}
      >
        <span {...stylex.props(df.pathLine)}>
          {header.previousPath && (
            <>
              <FilePath path={header.previousPath} className={stylex.props(df.oldPath).className} />
              <Icon
                name="arrow-right"
                size="xs"
                className={stylex.props(vocab.hold, df.glyph).className}
              />
            </>
          )}
          <FilePath path={header.path} className={stylex.props(df.newPath).className} />
        </span>
        <DiffStat added={header.added ?? 0} removed={header.removed ?? 0} />
        <Icon
          name="chevron-down"
          size="sm"
          className={stylex.props(chevron.base, df.glyphStep, collapsed && chevron.shut).className}
        />
      </Pressable>
      {!collapsed && (
        <div id={panelId}>
          {file.binary ? (
            <p {...stylex.props(cs.note, typeStep.uiSm)}>{t("diff.binary")}</p>
          ) : (
            <DiffView rows={file.rows} layout={layout} path={file.path} />
          )}
        </div>
      )}
    </section>
  );
}

export function DiffWorkspaceSurface() {
  const t = useT();
  const [mode, setMode] = useState<WorkspaceDiffMode>("worktree");
  const [layout, setLayout] = useState<DiffLayout>("unified");
  const [navigatorOpen, setNavigatorOpen] = useState(true);
  const [collapsedFiles, setCollapsedFiles] = useState<ReadonlySet<string>>(() => new Set());
  const { fileFocus, files, gitEnabled, isError, isLoading, notARepo, retry, view } =
    useWorkspaceDiffView(mode);
  const hasFiles = (files?.length ?? 0) > 0;

  const scrollRef = useRef<HTMLDivElement>(null);
  const scrollToFile = (path: string) => {
    const anchor = scrollRef.current?.querySelector(`[${FILE_ANCHOR}="${CSS.escape(path)}"]`);
    if (!anchor) return false;
    anchor.scrollIntoView({ block: "start" });
    return true;
  };
  const toggleFile = (path: string) => {
    setCollapsedFiles((previous) => {
      const next = new Set(previous);
      if (!next.delete(path)) next.add(path);
      return next;
    });
  };

  const consumedFocusRevision = useRef<bigint | null>(null);
  useEffect(() => {
    if (!files || consumedFocusRevision.current === fileFocus.revision) return;
    if (!fileFocus.path || scrollToFile(fileFocus.path)) {
      consumedFocusRevision.current = fileFocus.revision;
    }
  }, [fileFocus.path, fileFocus.revision, files]);

  const sub = view.subtext ? (
    <>
      <DiffStat added={view.subtext.added} removed={view.subtext.removed} />
      <span {...stylex.props(df.sep)}>·</span>
      <span>{t("diff.fileCount", { count: view.subtext.fileCount })}</span>
    </>
  ) : undefined;

  return (
    <AgentWorkspaceView>
      <ViewHeader
        icon="diff"
        title={mode === "base" ? "diff.branchCompare" : "diff.workingTree"}
        sub={sub}
        actions={
          <div {...stylex.props(vocab.line, vocab.min)}>
            <Segmented
              ariaLabel={t("diff.layoutAria")}
              value={layout}
              onChange={setLayout}
              options={[
                { value: "unified", label: t("diff.layout.unified") },
                { value: "split", label: t("diff.layout.split") },
              ]}
            />
            <Segmented
              ariaLabel={t("diff.baselineAria")}
              value={mode}
              onChange={setMode}
              options={[
                { value: "worktree", label: t("diff.mode.worktree") },
                { value: "base", label: t("diff.mode.branch") },
              ]}
            />
            {hasFiles && (
              <AgentViewNavigatorToggle
                open={navigatorOpen}
                onToggle={() => setNavigatorOpen((open) => !open)}
                showLabel={t("diff.files.show")}
                hideLabel={t("diff.files.hide")}
              />
            )}
          </div>
        }
      />
      <AgentViewSplit
        navigator={
          navigatorOpen && hasFiles ? (
            <ReviewFileTree
              files={files ?? []}
              selectedPath={fileFocus.path}
              onSelectFile={focusWorkspaceFile}
              onClose={() => setNavigatorOpen(false)}
            />
          ) : undefined
        }
      >
        <ScrollArea ref={scrollRef} className={stylex.props(df.scroller).className}>
          <DataView
            items={gitEnabled ? files : []}
            isLoading={isLoading}
            isError={isError && !notARepo}
            onRetry={retry}
            skeletonCount={10}
            empty={
              !gitEnabled
                ? gitOffEmpty("diff")
                : notARepo
                  ? notARepoEmpty("diff")
                  : {
                      icon: "diff" as const,
                      title: t("diff.empty.title"),
                      sub: t("diff.empty.sub"),
                    }
            }
            error={{
              title: mode === "base" ? t("diff.error.noBaseline") : t("diff.error.loadFailed"),
              sub: mode === "base" ? t("diff.error.noBaselineSub") : t("diff.error.loadFailedSub"),
            }}
          >
            {(fileDiffs) => (
              <>
                {fileDiffs.map((file) => (
                  <FileCard
                    key={file.path}
                    file={file}
                    layout={layout}
                    collapsed={collapsedFiles.has(file.path)}
                    onToggle={() => toggleFile(file.path)}
                  />
                ))}
                {view.truncated && (
                  <p {...stylex.props(cs.note, typeStep.uiSm)}>{t("diff.truncated")}</p>
                )}
              </>
            )}
          </DataView>
        </ScrollArea>
      </AgentViewSplit>
    </AgentWorkspaceView>
  );
}
