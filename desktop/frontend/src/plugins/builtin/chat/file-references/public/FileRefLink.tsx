import { Fragment } from "react";
import { openWorkspaceFile } from "@/plugins/builtin/workspace/public/navigation";
import { TextButton } from "@/ui";

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
    <TextButton
      render={<span />}
      nativeButton={false}
      shape="link"
      tone="accent"
      aria-label={label}
      onClick={() => openWorkspaceFile(path, line)}
    >
      {label.split("/").map((segment, index) => (
        <Fragment key={index}>
          {index > 0 && "/"}
          {index > 0 && (index > 1 || !label.startsWith("/")) && <wbr />}
          {segment}
        </Fragment>
      ))}
    </TextButton>
  );
}
