import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { useT } from "@/lib/i18n";
import { DataView, Icon, Pressable, chevron, vocab } from "@/ui";
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
  onSelectFile: (path: string) => void;
}

const ft = stylex.create({
  indent: { width: space.s3, flexShrink: 0 },
  pad: { paddingInline: space.s2, paddingBlock: space.s1_5 },
});

function TreeNode({ entry, cwd, depth, onSelectFile }: NodeProps) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);
  const isDir = entry.type === "dir";
  const {
    data: children,
    isLoading,
    isError,
    refetch,
  } = useWorkspaceListFiles(isDir && expanded ? { cwd, path: entry.path } : undefined);
  const indent = { paddingLeft: `${depth * 12 + 6}px` };

  return (
    <div>
      <Pressable
        type="button"
        aria-expanded={isDir ? expanded : undefined}
        className={stylex.props(vs.treeRow, vs.treeRowInset, typeStep.uiMd).className}
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
          <DataView
            items={children}
            isLoading={isLoading}
            isError={isError}
            onRetry={refetch}
            skeletonCount={1}
            error={{ title: t("dataView.error.title"), size: "compact" }}
            empty={{ icon: "folder", title: t("file.empty.title"), size: "compact" }}
          >
            {(rows) =>
              rows.map((child) => (
                <TreeNode
                  key={child.path}
                  entry={child}
                  cwd={cwd}
                  depth={depth + 1}
                  onSelectFile={onSelectFile}
                />
              ))
            }
          </DataView>
        </div>
      )}
    </div>
  );
}

export function FileTree({
  entries,
  cwd,
  onSelectFile,
}: {
  entries: WorkspaceFileEntry[];
  cwd?: string;
  onSelectFile: (path: string) => void;
}) {
  return (
    <div {...stylex.props(ft.pad)}>
      {entries.map((e) => (
        <TreeNode key={e.path} entry={e} cwd={cwd} depth={0} onSelectFile={onSelectFile} />
      ))}
    </div>
  );
}
