import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { Icon, Pressable } from "@/ui";
import { cn } from "@/lib/classNames";
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
  // One indent step per level, drawn as an empty cell so the glyph column stays aligned.
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
        className={cn(
          "flex w-full items-center gap-1.5 rounded-md px-1.5 py-1 text-left text-ui-md text-fg transition-colors hover:bg-hover",
          selectedPath === entry.path && !isDir && "bg-selected",
        )}
        style={indent}
        onClick={() => (isDir ? setExpanded((v) => !v) : onSelectFile(entry.path))}
      >
        {isDir ? (
          <Icon
            name="chevron-down"
            size="xs"
            className={cn("shrink-0 transition-transform", !expanded && "-rotate-90")}
          />
        ) : (
          <span {...stylex.props(ft.indent)} />
        )}
        <Icon
          name={isDir ? "folder" : "file"}
          size="sm"
          className={stylex.props(vs.hold).className}
        />
        <span {...stylex.props(vs.truncate)}>{entry.name}</span>
      </Pressable>
      {isDir && expanded && (
        <div>
          {isLoading && (
            <div
              className={stylex.props(ft.note, vs.caption, typeStep.uiMd).className}
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
