import * as stylex from "@stylexjs/stylex";
import type {
  FileChangeRowViewModel,
  FileChangesViewModel,
} from "@/plugins/builtin/workspace/application/fileChangesViewModel";
import { memo } from "react";
import { DiffStat, SectionLabel, toneInk, vocab } from "@/ui";
import { AgentRow } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { splitFilePath } from "@/lib/path";
import { face, space, type as typeStep } from "@/styles/tokens.stylex";

const fc = stylex.create({
  pad: { paddingInline: space.s1_5 },
  label: { paddingInline: space.s2, paddingBlock: space.s2 },
});

interface Props {
  view: FileChangesViewModel;
  onSelect: (path: string) => void;
}

export const FilesChanged = memo(function FilesChanged({ view, onSelect }: Props) {
  const t = useT();

  return (
    <div {...stylex.props(fc.pad)}>
      <SectionLabel
        className={stylex.props(fc.label).className}
        trailing={<DiffStat added={view.totalAdded} removed={view.totalRemoved} />}
      >
        {t("files.changed", { count: view.fileCount })}
      </SectionLabel>
      {view.rows.map((row) => (
        <FileRow key={row.path} row={row} onSelect={onSelect} />
      ))}
    </div>
  );
});

const FileRow = memo(function FileRow({
  row,
  onSelect,
}: {
  row: FileChangeRowViewModel;
  onSelect: (p: string) => void;
}) {
  const t = useT();
  const { directory, name } = splitFilePath(row.path);
  return (
    <AgentRow
      icon="file"
      active={row.active}
      aria-pressed={row.active}
      title={row.path}
      onClick={() => onSelect(row.path)}
      detail={directory || undefined}
      trailing={
        <span {...stylex.props(vocab.line, vocab.min, typeStep.uiXs)}>
          <span {...stylex.props(vocab.strong, toneInk[row.tag.tone], typeStep.ui2xs)}>
            {row.tag.letter}
          </span>
          {row.lineStats.kind === "binary" ? (
            <DiffStat added={0} removed={0} binary={t("files.binary")} />
          ) : (
            <DiffStat added={row.lineStats.added} removed={row.lineStats.removed} />
          )}
        </span>
      }
      styles={[face.mono]}
    >
      {name}
    </AgentRow>
  );
});
