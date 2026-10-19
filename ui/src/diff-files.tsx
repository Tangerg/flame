import * as stylex from "@stylexjs/stylex";
import { Fragment, type ReactNode, type Ref } from "react";
import { FileDiff } from "@flame/icons/react";
import { DiffStat } from "./diff-stat";
import { Icon } from "./font-scale";
import { RecordButton, RecordGroup } from "./records";
import { color, layout, space, type, weight } from "./tokens.stylex";

export interface DiffFileSummary {
  path: string;
  status: string;
  added?: number;
  removed?: number;
  binary?: boolean;
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
  directory: {
    color: color.fgMuted,
    fontFamily: "var(--font-mono)",
    fontWeight: weight.regular,
    overflowWrap: "anywhere",
    minWidth: 0,
  },
  state: { color: color.fgMuted, fontWeight: weight.regular },
  glyph: { color: color.fgMuted, marginBlockStart: space.s0_5 },
  headingLine: {
    display: "flex",
    alignItems: "start",
    flexWrap: "wrap",
    gap: layout.controlGap,
    minWidth: 0,
  },
  headingCopy: {
    display: "grid",
    flex: "1 1 12rem",
    gap: space.s1,
    minWidth: 0,
  },
});

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
  return (
    <span {...stylex.props(styles.identity, styles.state, type.uiSm)}>
      <span>{file.status}</span>
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
    <RecordGroup ref={ref} variant="list" aria-label={label}>
      {files.map((file) => {
        const cut = file.path.lastIndexOf("/") + 1;
        return (
          <RecordButton
            key={file.path}
            title={file.path}
            data-diff-file={file.path}
            aria-current={file.path === currentPath ? "true" : undefined}
            onClick={(event) => onSelect(file.path, event.currentTarget)}
          >
            <Icon glyph={FileDiff} size="sm" {...stylex.props(styles.glyph)} />
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
  return (
    <div {...stylex.props(styles.headingLine)}>
      <div {...stylex.props(styles.headingCopy)}>
        <h2 ref={ref} tabIndex={-1} {...stylex.props(styles.name, type.uiMd)}>
          {file.path.slice(file.path.lastIndexOf("/") + 1)}
        </h2>
        <FileState file={file} />
      </div>
      {children}
    </div>
  );
}
