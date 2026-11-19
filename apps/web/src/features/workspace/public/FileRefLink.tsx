import { ButtonPrimitive } from "@flame/ui";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { createContext, Fragment, use, type RefObject } from "react";
import { openWorkspaceFile } from "@/features/workspace/public/navigation";

export interface FileReferencePresentation {
  origin: RefObject<HTMLElement | null>;
  onOpen: () => void;
}

export const FileReferencePresentationContext = createContext<FileReferencePresentation | null>(
  null,
);

export function FileRefLink({ path, line, label }: { path: string; line: number; label: string }) {
  const workspaceAccess = useWorkspace();

  const presentation = use(FileReferencePresentationContext);
  return (
    <ButtonPrimitive
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
        openWorkspaceFile(
          workspaceAccess,
          path,
          line,
          presentation?.origin.current ?? event.currentTarget,
        );
        presentation?.onOpen();
      }}
    >
      {label.split("/").map((segment, index) => (
        <Fragment key={index}>
          {index > 0 && "/"}
          {index > 0 && (index > 1 || !label.startsWith("/")) && <wbr />}
          {segment}
        </Fragment>
      ))}
    </ButtonPrimitive>
  );
}
