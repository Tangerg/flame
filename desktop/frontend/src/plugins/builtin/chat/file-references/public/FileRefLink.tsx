// ONE component behind both the tool-output linkifier and the markdown-prose one: divergent
// styling would read as two different kinds of link.

import { openWorkspaceFile } from "@/plugins/builtin/workspace/public/navigation";
import { TextButton } from "@/ui";

export function FileRefLink({ path, line }: { path: string; line: number }) {
  return (
    <TextButton
      type="button"
      onClick={() => openWorkspaceFile(path, line)}
      className="border-0 bg-transparent p-0 font-mono text-accent underline decoration-transparent transition-colors hover:decoration-current"
    >
      {line > 0 ? `${path}:${line}` : path}
    </TextButton>
  );
}
