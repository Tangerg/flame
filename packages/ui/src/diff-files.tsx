import * as stylex from "@stylexjs/stylex";
import { Fragment, type ReactNode, type Ref } from "react";
import { DiffStat } from "./diff-stat";
import { useHeadingTag } from "./heading-level";
import { RecordButton, RecordGroup } from "./records";
import { color, layout, space, type, weight } from "./tokens.stylex";

export interface DiffFileSummary {
  path: string;
  /** Omitted when the line counts already say what happened to the file. */
  status?: string;
  added?: number;
  removed?: number;
  binary?: boolean;
  previous?: { path: string; label: string };
}

const styles = stylex.create({
  copy: { display: "grid", gap: space.s1, flex: 1, minWidth: 0 },
  identity: {
    display: "flex",
    alignItems: "baseline",
    flexWrap: "wrap",
    columnGap: space.s3,
    rowGap: space.s1,
    minWidth: 0,
  },
  name: {
    margin: 0,
    fontWeight: weight.medium,
    overflowWrap: "anywhere",
    minWidth: 0,
    color: color.fg,
  },
  clamp: {
    display: "-webkit-box",
    WebkitBoxOrient: "vertical",
    WebkitLineClamp: 2,
    overflow: "hidden",
  },
  directory: {
    color: color.fgMuted,
    fontFamily: "var(--font-mono)",
    fontWeight: weight.regular,
    overflowWrap: "anywhere",
    minWidth: 0,
  },
  state: { color: color.fgMuted, fontWeight: weight.regular },
  heading: { display: "grid", gap: space.s1, minWidth: 0 },
  headingLine: {
    display: "flex",
    alignItems: "center",
    flexWrap: "wrap",
    gap: layout.controlGap,
    minWidth: 0,
  },
  headingName: { flex: "1 1 8rem" },
  headingActions: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    maxWidth: "100%",
  },
});

const directoryEnd = (path: string) => path.lastIndexOf("/") + 1;

export function DiffFilePath({ path }: { path: string }) {
  return (
    <span dir="ltr" {...stylex.props(styles.directory, type.uiSm)}>
      {path.split("/").map((segment, index, segments) => (
        <Fragment key={index}>
          {segment}
          {index < segments.length - 1 && (
            <>
              {"/"}
              <wbr />
            </>
          )}
        </Fragment>
      ))}
    </span>
  );
}

function FileState({ file }: { file: DiffFileSummary }) {
  const { previous } = file;
  if (!previous && !file.status && file.binary) return null;
  return (
    <span {...stylex.props(styles.identity, styles.state, type.uiSm)}>
      {previous ? (
        <span {...stylex.props(styles.identity)}>
          <span>{previous.label}</span> <DiffFilePath path={previous.path} />
        </span>
      ) : (
        file.status && <span>{file.status}</span>
      )}
      {!file.binary && <DiffStat added={file.added} removed={file.removed} />}
    </span>
  );
}

export function DiffFileList({
  files,
  label,
  currentPath,
  onSelect,
  ref,
}: {
  files: readonly DiffFileSummary[];
  label: string;
  currentPath?: string | null;
  onSelect: (path: string, origin: HTMLButtonElement) => void;
  ref?: Ref<HTMLElement>;
}) {
  return (
    <RecordGroup
      ref={ref}
      variant="list"
      aria-label={label}
      hidden={!files.length}
    >
      {files.map((file) => {
        const cut = directoryEnd(file.path);
        return (
          <RecordButton
            key={file.path}
            title={file.path}
            data-diff-file={file.path}
            aria-current={file.path === currentPath ? "true" : undefined}
            onClick={(event) => onSelect(file.path, event.currentTarget)}
          >
            <span {...stylex.props(styles.copy)}>
              <span {...stylex.props(styles.name, type.uiMd)}>
                {file.path.slice(cut)}
              </span>
              {cut > 0 && <DiffFilePath path={file.path.slice(0, cut - 1)} />}
              <FileState file={file} />
            </span>
          </RecordButton>
        );
      })}
    </RecordGroup>
  );
}

export function DiffFileHeader({
  file,
  children,
  ref,
}: {
  file: DiffFileSummary;
  children?: ReactNode;
  ref?: Ref<HTMLHeadingElement>;
}) {
  const Tag = useHeadingTag();
  const cut = directoryEnd(file.path);
  return (
    <div {...stylex.props(styles.heading)}>
      <div {...stylex.props(styles.headingLine)}>
        <Tag
          ref={ref}
          tabIndex={-1}
          {...stylex.props(
            styles.name,
            styles.clamp,
            styles.headingName,
            type.uiMd,
          )}
        >
          {file.path.slice(cut)}
        </Tag>
        {children && (
          <div {...stylex.props(styles.headingActions)}>{children}</div>
        )}
      </div>
      {cut > 0 && <DiffFilePath path={file.path.slice(0, cut - 1)} />}
      <FileState file={file} />
    </div>
  );
}
