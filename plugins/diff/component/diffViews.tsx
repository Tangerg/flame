import { type ReactNode, type RefObject } from "react";
import { failureText, type Translate } from "@flame/component-sdk";
import {
  ArrowLeft,
  Copy,
  ChevronLeft,
  ChevronRight,
  FileDiff as FileDiffIcon,
  List,
  Refresh,
} from "@flame/icons/react";
import type { DiffMode, WorkspaceFileChange } from "@flame/runtime-client/wire";
import {
  Button,
  CodeWrapButton,
  DiffBody,
  DiffFileHeader,
  DiffFileList,
  DiffFilePath,
  DiffStat,
  Disclosure,
  EmptyState,
  Icon,
  IconButton,
  Metadata,
  PanelSection,
  Hang,
  Select,
  Toolbar,
  type DiffFileSummary,
} from "@flame/ui";
import { type DiffFile, type Query, type Read } from "./diffRead";

const COMPARISONS: DiffMode[] = ["worktree", "base"];

const comparisonName = (t: Translate, mode: DiffMode) =>
  t(`comparison.${mode}`);

const fileSummary = (
  t: Translate,
  file: WorkspaceFileChange,
): DiffFileSummary => ({
  path: file.path,
  status:
    file.status === "modified" &&
    !file.binary &&
    file.added !== undefined &&
    file.removed !== undefined
      ? undefined
      : t(`file.status.${file.status}`),
  added: file.added,
  removed: file.removed,
  binary: file.binary,
  previous: file.previousPath
    ? { path: file.previousPath, label: t("file.renamed") }
    : undefined,
});

export function DiffReaderNavigation({
  t,
  files,
  index,
  filtered,
  changes,
  onOpen,
}: {
  t: Translate;
  files: readonly DiffFile[];
  index: number;
  filtered: boolean;
  changes: string | undefined;
  onOpen: (path: string, navigation: HTMLButtonElement) => void;
}) {
  return (
    <>
      <Metadata role="status">
        <span aria-hidden="true">
          {index + 1} / {files.length}
        </span>
        <span className="sr-only">
          {changes}
          {" · "}
          {t(filtered ? "navigation.positionMatching" : "navigation.position", {
            position: index + 1,
            total: files.length,
          })}
        </span>
      </Metadata>
      <IconButton
        icon={ChevronLeft}
        aria-label={t("nav.previous")}
        size="sm"
        disabled={index <= 0}
        onClick={(event) => onOpen(files[index - 1]!.path, event.currentTarget)}
      />
      <IconButton
        icon={ChevronRight}
        aria-label={t("nav.next")}
        size="sm"
        disabled={index < 0 || index >= files.length - 1}
        onClick={(event) => onOpen(files[index + 1]!.path, event.currentTarget)}
      />
    </>
  );
}

export function DiffOverview({
  t,
  files,
  matching,
  filtered,
  truncated,
  currentPath,
  listRef,
  onSelect,
}: {
  t: Translate;
  files: readonly DiffFile[];
  matching: readonly DiffFile[];
  filtered: boolean;
  truncated: boolean;
  currentPath: string | null;
  listRef: RefObject<HTMLElement | null>;
  onSelect: (path: string) => void;
}) {
  const textFiles = files.filter((item) => !item.binary);
  // A total is stated only when every text file reports that count.
  const total = (key: "added" | "removed") =>
    textFiles.every((item) => item[key] !== undefined)
      ? textFiles.reduce((sum, item) => sum + item[key]!, 0)
      : undefined;
  return (
    <PanelSection
      title={t(truncated ? "files.included" : "files.changed", {
        count: files.length,
      })}
      description={
        filtered
          ? t("files.match", { count: matching.length, total: files.length })
          : undefined
      }
      actions={
        textFiles.length > 0 ? (
          <DiffStat
            title={t("stat.title")}
            added={total("added")}
            removed={total("removed")}
          />
        ) : undefined
      }
    >
      {!matching.length && (
        <EmptyState size="compact" title={t("nomatch.title")} />
      )}
      <DiffFileList
        ref={listRef}
        files={matching.map((item) => fileSummary(t, item))}
        label={t("files.region")}
        currentPath={currentPath}
        onSelect={onSelect}
      />
    </PanelSection>
  );
}

export function DiffReadStatus({
  t,
  read,
  query,
}: {
  t: Translate;
  read: Read;
  query: Query;
}) {
  const accepted = read.accepted;
  const settled = !read.loading && !read.error;
  return (
    <>
      {read.loading && (
        <Metadata role="status">
          {t("status.reading", {
            name: comparisonName(t, query.mode).toLocaleLowerCase(),
          })}
        </Metadata>
      )}
      {read.error && (
        <Metadata role="alert">
          {t("status.error", {
            name: comparisonName(t, query.mode).toLocaleLowerCase(),
            error: failureText(read.error.cause, t("read.failed")),
          })}
        </Metadata>
      )}
      {accepted && (!settled || accepted.query !== query) && (
        <Metadata
          role="status"
          className={accepted.query.mode !== query.mode ? undefined : "sr-only"}
        >
          {t("status.stale", {
            name: comparisonName(t, accepted.query.mode).toLocaleLowerCase(),
          })}
        </Metadata>
      )}
      {accepted && settled && accepted.query === query && (
        <Metadata role="status" className="sr-only">
          {t("status.loaded", {
            name: comparisonName(t, accepted.query.mode).toLocaleLowerCase(),
          })}
        </Metadata>
      )}
    </>
  );
}

export function DiffScopeToolbar({
  t,
  scoped,
  mode,
  readingOverview,
  loading,
  failed,
  refreshRef,
  onShowFiles,
  onMode,
  onRefresh,
}: {
  t: Translate;
  scoped: boolean;
  mode: DiffMode;
  readingOverview: boolean;
  loading: boolean;
  failed: boolean;
  refreshRef: RefObject<HTMLButtonElement | null>;
  onShowFiles(): void;
  onMode(mode: DiffMode): void;
  onRefresh(): void;
}) {
  return (
    <Toolbar
      label={t("scope.label")}
      actions={
        <>
          {scoped && (
            <Button
              variant="ghost"
              size="sm"
              onClick={onShowFiles}
              aria-label={t("files.show")}
              pending={readingOverview}
            >
              <Icon glyph={List} size="sm" />
              {t("files.button")}
            </Button>
          )}
          {failed && <Metadata>{t("read.failedShort")}</Metadata>}
          <IconButton
            ref={refreshRef}
            icon={Refresh}
            aria-label={t("refresh")}
            size="md"
            pending={loading}
            onClick={onRefresh}
          />
        </>
      }
    >
      <Hang>
        <Select
          label={t("comparison.label")}
          value={mode}
          options={COMPARISONS.map((value) => ({
            value,
            label: t(`comparison.${value}`),
          }))}
          onValueChange={(value) => onMode(value as DiffMode)}
        />
      </Hang>
    </Toolbar>
  );
}

export function DiffReaderToolbar({
  t,
  navigation,
  failed,
  loading,
  returnsToFiles,
  onShowFiles,
  onRefresh,
}: {
  t: Translate;
  navigation: ReactNode;
  failed: boolean;
  loading: boolean;
  returnsToFiles: boolean;
  onShowFiles(): void;
  onRefresh(): void;
}) {
  return (
    <Toolbar
      label={t("navigation.label")}
      actions={
        <>
          {navigation}
          {failed && <Metadata>{t("read.failedShort")}</Metadata>}
          <IconButton
            icon={Refresh}
            aria-label={t("refresh")}
            size="sm"
            pending={loading}
            onClick={onRefresh}
          />
        </>
      }
    >
      <Button
        variant="ghost"
        size="sm"
        aria-label={returnsToFiles ? t("files.back") : t("files.show")}
        onClick={onShowFiles}
      >
        <Icon glyph={returnsToFiles ? ArrowLeft : List} size="sm" />
        {t("files.button")}
      </Button>
    </Toolbar>
  );
}

export type DiffLayout = "unified" | "split";

// Layout, wrapping and copy apply only to a readable text patch.
export function DiffReaderHeading({
  t,
  file,
  heading,
  readable,
  layout,
  onLayout,
  patchActions,
  onCopy,
}: {
  t: Translate;
  file: DiffFile;
  heading: RefObject<HTMLHeadingElement | null>;
  readable: boolean;
  layout: DiffLayout;
  onLayout(layout: DiffLayout): void;
  patchActions: { wrap: string; unwrap: string };
  onCopy(): void;
}) {
  return (
    <DiffFileHeader ref={heading} file={fileSummary(t, file)}>
      {!file.binary && readable && (
        <>
          <Select
            size="sm"
            label={t("layout.label")}
            value={layout}
            onValueChange={(value) => onLayout(value as DiffLayout)}
            options={[
              { value: "unified", label: t("layout.unified") },
              { value: "split", label: t("layout.split") },
            ]}
          />
          <CodeWrapButton labels={patchActions} size="sm" />
          <IconButton
            icon={Copy}
            size="sm"
            aria-label={t("patch.copy")}
            onClick={onCopy}
          />
        </>
      )}
    </DiffFileHeader>
  );
}

export function DiffManifestNotices({
  t,
  truncated,
  hasFiles,
  missingPath,
}: {
  t: Translate;
  truncated: boolean;
  hasFiles: boolean;
  missingPath: string | undefined;
}) {
  return (
    <>
      {truncated && hasFiles && <Metadata role="note">{t("partial")}</Metadata>}
      {!hasFiles && (
        <EmptyState
          icon={FileDiffIcon}
          size="compact"
          align="start"
          title={t(truncated ? "budget.empty" : "empty.title")}
        />
      )}
      {missingPath && (
        <Metadata>
          {t("missing")} <DiffFilePath path={missingPath} />
        </Metadata>
      )}
    </>
  );
}

export function DiffFileReader({
  t,
  file,
  rendered,
  layout,
  label,
}: {
  t: Translate;
  file: DiffFile;
  rendered: { source: string } | undefined;
  layout: "unified" | "split";
  label: string | undefined;
}) {
  return (
    <section aria-label={label}>
      {file.binary ? (
        <Metadata>{t("file.binary")}</Metadata>
      ) : rendered ? (
        <DiffBody
          key={file.path}
          patch={rendered.source}
          layout={layout}
          label={t("reader.region", { path: file.path })}
        />
      ) : (
        <Metadata>{t("file.noLineChanges")}</Metadata>
      )}
    </section>
  );
}

export function DiffDetails({
  t,
  accepted,
  root,
  timeFormat,
}: {
  t: Translate;
  accepted: NonNullable<Read["accepted"]>;
  root: string;
  timeFormat: Intl.DateTimeFormat;
}) {
  const baseline = accepted.diff.baseline;
  return (
    <Disclosure title={t("details.title")}>
      <Metadata>
        {baseline.type === "emptyTree"
          ? t("details.emptyTree")
          : accepted.query.mode === "worktree"
            ? t("details.worktree")
            : t("details.base")}
      </Metadata>
      <Metadata>
        {baseline.type === "emptyTree"
          ? t("baseline.emptyTree")
          : t("baseline.label", {
              ref:
                baseline.type === "head"
                  ? t("baseline.head")
                  : t("baseline.mergeBase"),
              commit: baseline.commit,
            })}
      </Metadata>
      <Metadata>
        <DiffFilePath path={root} />
      </Metadata>
      <Metadata>
        <time dateTime={accepted.at}>
          {t("read.at", { time: timeFormat.format(new Date(accepted.at)) })}
        </time>
      </Metadata>
    </Disclosure>
  );
}
