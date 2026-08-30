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
      onClick={() => openWorkspaceFile(path, line)}
      className="border-0 bg-transparent p-0 font-mono text-accent underline decoration-transparent transition-colors hover:decoration-current"
    >
      {line > 0 ? `${path}:${line}${column > 0 ? `:${column}` : ""}` : path}
    </TextButton>
  );
}
