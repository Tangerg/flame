import * as stylex from "@stylexjs/stylex";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import {
  DataView,
  DiffStat,
  FilePath,
  Icon,
  IconButton,
  OptionRow,
  Popover,
  Pressable,
  ScrollArea,
  SearchField,
  Segmented,
  Tag,
  chevron,
  vocab,
} from "@/ui";
import { AgentWorkspaceView } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import type {
  DiffLayout,
  WorkspaceDiffMode,
} from "@/plugins/builtin/workspace/application/diffVocabulary";
import { DiffView } from "./views/DiffView";
import { ViewHeader } from "./views/ViewHeader";
import { gitOffEmpty, notARepoEmpty } from "./views/vcsGate";
import {
  type WorkspaceFileDiff,
  workspaceDiffFileHeader,
  useWorkspaceDiffView,
} from "@/plugins/builtin/workspace/application/diffViewModel";
import { color, face, space, surface, type as typeStep } from "@/styles/tokens.stylex";
import {
  rememberWorkspaceView,
  useWorkspaceViewMemory,
} from "@/plugins/builtin/workspace/application/navigation";
import { codeStyles as cs, viewStyles as vs } from "./views/viewStyles";

const fade = stylex.keyframes({
  from: { outlineColor: surface.fieldFocus },
  to: { outlineColor: "transparent" },
});

const df = stylex.create({
  pathLine: { display: "flex", minWidth: 0, flex: 1, alignItems: "baseline", gap: space.s1_5 },
  oldPath: { flexShrink: 100, color: color.fgFaint },
  glyphStep: { opacity: "var(--glyph-step)" },
  newPath: { flexShrink: 1 },
  glyph: { opacity: "var(--glyph-step)" },
  sep: { marginInline: space.s2 },
  scroller: { minWidth: 0, paddingInline: space.s2, paddingBottom: space.s2 },
  baseline: { overflowWrap: "anywhere" },
  sticky: { position: "sticky", top: 0, zIndex: 1 },
  card: { position: "relative" },
  offscreen: { contentVisibility: "auto" },
  flash: {
    position: "absolute",
    inset: 0,
    zIndex: 2,
    pointerEvents: "none",
    borderRadius: "inherit",
    outlineWidth: "1.5px",
    outlineStyle: "solid",
    outlineColor: "transparent",
    outlineOffset: "-1.5px",
    animationName: fade,
    animationDuration: "900ms",
    animationTimingFunction: "ease-in",
  },
  picker: {
    display: "flex",
    width: "320px",
    maxHeight: "360px",
    flexDirection: "column",
    gap: space.s1,
    padding: space.s1,
  },
  pickerList: { minHeight: 0, overflowY: "auto" },
  pickerEmpty: { paddingInline: space.s2, paddingBlock: space.s3, color: color.fgFaint },
});

const FILE_ANCHOR = "data-diff-file";

function FileCard({
  file,
  layout,
  collapsed,
  flashToken,
  onToggle,
}: {
  file: WorkspaceFileDiff;
  layout: DiffLayout;
  collapsed: boolean;
  flashToken: number | undefined;
  onToggle: () => void;
}) {
  const t = useT();
  const panelId = useId();
  const header = workspaceDiffFileHeader(file);
  return (
    <section {...{ [FILE_ANCHOR]: file.path }} {...stylex.props(cs.fileCard, df.card)}>
      {flashToken !== undefined && (
        <span key={flashToken} aria-hidden {...stylex.props(df.flash)} />
      )}
      <Pressable
        type="button"
        data-chrome-focus=""
        aria-expanded={!collapsed}
        aria-controls={panelId}
        onClick={onToggle}
        {...stylex.props(vs.diffFileHeader, df.sticky, typeStep.uiSm, face.mono)}
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
        <Tag>{t(`diff.status.${header.status}`)}</Tag>
        <DiffStat added={header.added ?? 0} removed={header.removed ?? 0} />
        <Icon
          name="chevron-down"
          size="sm"
          className={stylex.props(chevron.base, df.glyphStep, collapsed && chevron.shut).className}
        />
      </Pressable>
      {!collapsed && (
        <div
          id={panelId}
          {...stylex.props(df.offscreen)}
          style={{ containIntrinsicBlockSize: `auto calc(${file.rows.length} * 1lh)` }}
        >
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
  const memory = useWorkspaceViewMemory();
  const mode: WorkspaceDiffMode = memory.diffMode;
  const layout: DiffLayout = memory.diffLayout;
  const setMode = (diffMode: WorkspaceDiffMode) => rememberWorkspaceView({ diffMode });
  const setLayout = (diffLayout: DiffLayout) => rememberWorkspaceView({ diffLayout });
  const collapsedFiles = useMemo(
    () => new Set(memory.collapsedDiffFiles),
    [memory.collapsedDiffFiles],
  );
  const setCollapsedFiles = (next: (previous: ReadonlySet<string>) => ReadonlySet<string>) =>
    rememberWorkspaceView({ collapsedDiffFiles: [...next(collapsedFiles)] });
  const { fileFocus, files, gitEnabled, error, isLoading, notARepo, retry, view } =
    useWorkspaceDiffView(mode);

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

  const [revealed, setRevealed] = useState<{ path: string; token: number } | null>(null);
  const target = (path: string) => (previous: typeof revealed) => ({
    path,
    token: (previous?.token ?? 0) + 1,
  });
  const reveal = (path: string) => {
    setCollapsedFiles((previous) => {
      if (!previous.has(path)) return previous;
      const next = new Set(previous);
      next.delete(path);
      return next;
    });
    setRevealed(target(path));
  };

  const [consumedFocus, setConsumedFocus] = useState<bigint | null>(null);
  if (files && consumedFocus !== fileFocus.revision) {
    const focused = fileFocus.path;
    const present = focused !== "" && files.some((file) => file.path === focused);
    if (focused === "" || present) setConsumedFocus(fileFocus.revision);
    if (present) setRevealed(target(focused));
  }

  const scrolledToken = useRef(0);
  useEffect(() => {
    if (!revealed || scrolledToken.current === revealed.token) return;
    if (scrollToFile(revealed.path)) scrolledToken.current = revealed.token;
  }, [revealed, collapsedFiles]);

  const step = (direction: 1 | -1) => {
    const port = scrollRef.current;
    if (!port || !files || files.length === 0) return;
    const anchors = [...port.querySelectorAll<HTMLElement>(`[${FILE_ANCHOR}]`)];
    const top = port.getBoundingClientRect().top;
    const current = anchors.findIndex((anchor) => anchor.getBoundingClientRect().bottom > top + 1);
    const index = Math.min(
      anchors.length - 1,
      Math.max(0, (current === -1 ? 0 : current) + direction),
    );
    const path = anchors[index]?.getAttribute(FILE_ANCHOR);
    if (path) reveal(path);
  };
  const allCollapsed =
    files !== undefined && files.length > 0 && collapsedFiles.size >= files.length;

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
            {files && files.length > 1 && (
              <>
                <DiffFilePicker files={files} onPick={reveal} />
                <IconButton
                  icon="chevron-up"
                  size="sm"
                  title={t("diff.nav.previous")}
                  onClick={() => step(-1)}
                />
                <IconButton
                  icon="chevron-down"
                  size="sm"
                  title={t("diff.nav.next")}
                  onClick={() => step(1)}
                />
                <IconButton
                  icon={allCollapsed ? "unfold-horizontal" : "fold"}
                  size="sm"
                  title={allCollapsed ? t("diff.nav.expandAll") : t("diff.nav.collapseAll")}
                  onClick={() =>
                    setCollapsedFiles(() =>
                      allCollapsed ? new Set() : new Set(files.map((file) => file.path)),
                    )
                  }
                />
              </>
            )}
            <IconButton
              icon="columns"
              size="sm"
              active={layout === "split"}
              aria-pressed={layout === "split"}
              title={t("diff.layout.split")}
              onClick={() => setLayout(layout === "split" ? "unified" : "split")}
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
          </div>
        }
      />
      {view.baseline && (
        <p
          {...stylex.props(vs.gutter, typeStep.uiSm, vocab.muted, vocab.pretty, df.baseline)}
          title={view.baseline.type === "emptyTree" ? undefined : view.baseline.commit}
        >
          {t(`diff.baseline.${view.baseline.type}`, {
            commit: view.baseline.type === "emptyTree" ? "" : view.baseline.commit.slice(0, 12),
          })}
        </p>
      )}
      <ScrollArea ref={scrollRef} className={stylex.props(df.scroller).className}>
        <DataView
          items={gitEnabled ? files : []}
          isLoading={isLoading}
          failure={notARepo ? undefined : error}
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
                  flashToken={revealed?.path === file.path ? revealed.token : undefined}
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
    </AgentWorkspaceView>
  );
}

function DiffFilePicker({
  files,
  onPick,
}: {
  files: readonly WorkspaceFileDiff[];
  onPick: (path: string) => void;
}) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const needle = query.trim().toLocaleLowerCase();
  const shown = needle
    ? files.filter((file) => file.path.toLocaleLowerCase().includes(needle))
    : files;
  const pick = (path: string) => {
    onPick(path);
    setOpen(false);
    setQuery("");
  };
  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger render={<IconButton icon="list" size="sm" title={t("diff.nav.picker")} />} />
      <Popover.Content align="end" sideOffset={6}>
        <div {...stylex.props(df.picker)}>
          <SearchField
            size="sm"
            value={query}
            onValueChange={setQuery}
            placeholder={t("diff.nav.filter")}
            aria-label={t("diff.nav.filter")}
            onKeyDown={(event) => {
              if (event.key === "Enter" && shown[0]) pick(shown[0].path);
            }}
          />
          <div {...stylex.props(df.pickerList)}>
            {shown.length === 0 ? (
              <p {...stylex.props(df.pickerEmpty, typeStep.uiSm)}>{t("diff.nav.none")}</p>
            ) : (
              shown.map((file) => (
                <OptionRow key={file.path} layout="glyph" onClick={() => pick(file.path)}>
                  <Tag>{t(`diff.status.${file.status}`)}</Tag>
                  <FilePath path={file.path} />
                </OptionRow>
              ))
            )}
          </div>
        </div>
      </Popover.Content>
    </Popover.Root>
  );
}
