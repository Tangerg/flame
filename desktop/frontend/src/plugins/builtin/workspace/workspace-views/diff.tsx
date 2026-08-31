import { useEffect, useId, useRef, useState } from "react";
import { DataView, DiffStat, FilePath, Icon, Pressable, ScrollArea, Segmented } from "@/ui";
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
    <section
      {...{ [FILE_ANCHOR]: file.path }}
      className="mb-2 overflow-hidden rounded-md border-[0.5px] border-field first:mt-2 last:mb-0"
    >
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
        <span className="flex min-w-0 flex-1 items-baseline gap-1.5">
          {header.previousPath && (
            <>
              <FilePath path={header.previousPath} className="shrink-[100] text-fg-faint" />
              <Icon name="arrow-right" size="xs" className="shrink-0 opacity-60" />
            </>
          )}
          <FilePath path={header.path} className="shrink" />
        </span>
        <DiffStat added={header.added ?? 0} removed={header.removed ?? 0} />
        <Icon
          name="chevron-down"
          size="sm"
          className={cn("shrink-0 opacity-50 transition-transform", collapsed && "-rotate-90")}
        />
      </Pressable>
      {!collapsed && (
        <div id={panelId}>
          {file.binary ? (
            <p className="m-0 px-3 py-2 font-mono text-ui-sm text-fg-faint">{t("diff.binary")}</p>
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
  const { fileFocus, files, gitEnabled, isError, isLoading, notARepo, view } =
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
      <span className="mx-2">·</span>
      <span>{t("diff.fileCount", { count: view.subtext.fileCount })}</span>
    </>
  ) : undefined;

  return (
    <AgentWorkspaceView>
      <ViewHeader
        icon="diff"
        title={mode === "base" ? "diff.branchCompare" : "diff.workingTree"}
        titleStrong
        sub={sub}
        actions={
          <div className="flex items-center gap-2">
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
        <ScrollArea ref={scrollRef} className="min-w-0 px-2 pb-2">
          <DataView
            items={gitEnabled ? files : []}
            isLoading={isLoading}
            isError={isError && !notARepo}
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
              icon: "diff",
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
                  <p className="m-0 px-3 py-2 font-mono text-ui-sm text-fg-faint">
                    {t("diff.truncated")}
                  </p>
                )}
              </>
            )}
          </DataView>
        </ScrollArea>
      </AgentViewSplit>
    </AgentWorkspaceView>
  );
}
