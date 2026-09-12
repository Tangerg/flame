import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { Icon, Pressable, chevron, vocab } from "@/ui";
import {
  type WorkspaceFileEntry,
  useWorkspaceListFiles,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import { space, type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./viewStyles";

interface NodeProps {
  entry: WorkspaceFileEntry;
  cwd?: string;
  depth: number;
  selectedPath?: string;
  onSelectFile: (path: string) => void;
}

const ft = stylex.create({
  indent: { width: space.s3, flexShrink: 0 },
  note: { paddingBlock: space.s1 },
  pad: { paddingInline: space.s2, paddingBlock: space.s1_5 },
});

function TreeNode({ entry, cwd, depth, selectedPath, onSelectFile }: NodeProps) {
  const [expanded, setExpanded] = useState(false);
  const isDir = entry.type === "dir";
  const { data: children, isLoading } = useWorkspaceListFiles(
    isDir && expanded ? { cwd, path: entry.path } : undefined,
  );
  const indent = { paddingLeft: `${depth * 12 + 6}px` };

  return (
    <div>
      <Pressable
        type="button"
        className={
          stylex.props(
            vs.treeRow,
            vs.treeRowInset,
            typeStep.uiMd,
            selectedPath === entry.path && !isDir && vs.treeRowSelected,
          ).className
        }
        style={indent}
        onClick={() => (isDir ? setExpanded((v) => !v) : onSelectFile(entry.path))}
      >
        {isDir ? (
          <Icon
            name="chevron-down"
            size="xs"
            className={stylex.props(chevron.base, !expanded && chevron.shut).className}
          />
        ) : (
          <span {...stylex.props(ft.indent)} />
        )}
        <Icon
          name={isDir ? "folder" : "file"}
          size="sm"
          className={stylex.props(vocab.hold).className}
        />
        <span {...stylex.props(vocab.truncate)}>{entry.name}</span>
      </Pressable>
      {isDir && expanded && (
        <div>
          {isLoading && (
            <div
              className={stylex.props(ft.note, vocab.faint, typeStep.uiMd).className}
              style={{ paddingLeft: `${(depth + 1) * 12 + 6}px` }}
            >
              …
            </div>
          )}
          {(children ?? []).map((c) => (
            <TreeNode
              key={c.path}
              entry={c}
              cwd={cwd}
              depth={depth + 1}
              selectedPath={selectedPath}
              onSelectFile={onSelectFile}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export function FileTree({
  entries,
  cwd,
  selectedPath,
  onSelectFile,
}: {
  entries: WorkspaceFileEntry[];
  cwd?: string;
  selectedPath?: string;
  onSelectFile: (path: string) => void;
}) {
  return (
    <div {...stylex.props(ft.pad)}>
      {entries.map((e) => (
        <TreeNode
          key={e.path}
          entry={e}
          cwd={cwd}
          depth={0}
          selectedPath={selectedPath}
          onSelectFile={onSelectFile}
        />
      ))}
    </div>
  );
}
