import { Fragment } from "react";
import { openWorkspaceFile } from "@/plugins/builtin/workspace/public/navigation";
import { Pressable } from "@/ui";

export function FileRefLink({
  path,
  line,
  column = 0,
}: {
  path: string;
  line: number;
  column?: number;
}) {
  const label = line > 0 ? `${path}:${line}${column > 0 ? `:${column}` : ""}` : path;
  return (
    <Pressable
      render={<span />}
      nativeButton={false}
      data-slot="file-reference"
      aria-label={label}
      onClick={(event) => {
        const selection = event.currentTarget.ownerDocument.getSelection();
        if (
          event.detail > 0 &&
          selection &&
          !selection.isCollapsed &&
          selection.containsNode(event.currentTarget, true)
        )
          return;
        openWorkspaceFile(path, line, event.currentTarget);
      }}
    >
      {label.split("/").map((segment, index) => (
        <Fragment key={index}>
          {index > 0 && "/"}
          {index > 0 && (index > 1 || !label.startsWith("/")) && <wbr />}
          {segment}
        </Fragment>
      ))}
    </Pressable>
  );
}
