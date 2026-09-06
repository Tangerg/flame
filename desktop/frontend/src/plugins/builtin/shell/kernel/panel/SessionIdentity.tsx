import type { ReactElement } from "react";
import { basename } from "@/lib/path";
import { ContextMenu } from "@/ui";
import { useT } from "@/lib/i18n";
import { writeToClipboard } from "@/lib/clipboard";

interface Props {
  sessionId: string;
  title: string;
  workspacePath?: string;
}

/**
 * The header's answer to "which session am I in, and where is it running". Both facts are
 * shown lossily — the path as its basename, and only above `lg`; the title truncated — so
 * both carry a `title` and both can be copied whole, which is where the reference puts them
 * too.
 */
export function SessionIdentity({ sessionId, title, workspacePath }: Props): ReactElement {
  const t = useT();

  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger
        render={
          <div className="flex min-w-0 shrink items-center gap-2">
            {workspacePath && (
              <>
                <span
                  title={workspacePath}
                  className="hidden min-w-0 max-w-[160px] shrink truncate font-mono text-ui-sm text-fg-faint lg:inline"
                >
                  {basename(workspacePath)}
                </span>
                <span aria-hidden className="hidden shrink-0 text-ui-sm text-fg-faint lg:inline">
                  /
                </span>
              </>
            )}
            {/* The name of what the reader is looking at, and the only heading above the
                turns — which are h2. It was a span, so a populated transcript published an
                outline that started at its second rung. */}
            <h1
              title={title}
              className="m-0 min-w-0 max-w-[420px] truncate text-ui-sm font-semibold text-fg"
            >
              {title}
            </h1>
          </div>
        }
      />
      <ContextMenu.Content className="min-w-[var(--menu-min-width)]">
        {workspacePath && (
          <ContextMenu.IconItem
            icon="copy"
            onSelect={() =>
              void writeToClipboard(workspacePath, {
                successLabel: t("session.identity.copiedCwd"),
              })
            }
          >
            {t("session.identity.copyCwd")}
          </ContextMenu.IconItem>
        )}
        {sessionId !== "" && (
          <ContextMenu.IconItem
            icon="copy"
            onSelect={() =>
              void writeToClipboard(sessionId, { successLabel: t("session.identity.copiedId") })
            }
          >
            {t("session.identity.copyId")}
          </ContextMenu.IconItem>
        )}
      </ContextMenu.Content>
    </ContextMenu.Root>
  );
}
