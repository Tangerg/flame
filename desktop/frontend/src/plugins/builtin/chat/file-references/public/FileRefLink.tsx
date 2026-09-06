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
  return (
    <TextButton
      type="button"
      shape="link"
      tone="accent"
      onClick={() => openWorkspaceFile(path, line)}
    >
      {line > 0 ? `${path}:${line}${column > 0 ? `:${column}` : ""}` : path}
    </TextButton>
  );
}
