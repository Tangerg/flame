import * as stylex from "@stylexjs/stylex";
import type { ReactElement } from "react";
import { basename } from "@/lib/path";
import { ContextMenu } from "@/ui";
import { useT } from "@/lib/i18n";
import { writeToClipboard } from "@/lib/clipboard";
import { color, space, type as typeStep, weight } from "@/styles/tokens.stylex";

const si = stylex.create({
  bar: { display: "flex", minWidth: 0, flexShrink: 1, alignItems: "center", gap: space.s2 },
  // The working directory is context, so it is the first thing the bar gives up: hidden below
  // the wide breakpoint, and capped even above it.
  cwd: {
    display: { default: "none", "@media (min-width: 1024px)": "inline" },
    minWidth: 0,
    maxWidth: "160px",
    flexShrink: 1,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    fontFamily: "var(--font-mono)",
    color: color.fgFaint,
  },
  sep: {
    display: { default: "none", "@media (min-width: 1024px)": "inline" },
    flexShrink: 0,
    color: color.fgFaint,
  },
  title: {
    margin: 0,
    minWidth: 0,
    maxWidth: "420px",
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    fontWeight: weight.semibold,
    color: color.fg,
  },
});

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
          <div {...stylex.props(si.bar)}>
            {workspacePath && (
              <>
                <span
                  title={workspacePath}
                  className={stylex.props(si.cwd, typeStep.uiSm).className}
                >
                  {basename(workspacePath)}
                </span>
                <span aria-hidden {...stylex.props(si.sep, typeStep.uiSm)}>
                  /
                </span>
              </>
            )}
            {/* The name of what the reader is looking at, and the only heading above the
                turns — which are h2. It was a span, so a populated transcript published an
                outline that started at its second rung. */}
            <h1 title={title} className={stylex.props(si.title, typeStep.uiSm).className}>
              {title}
            </h1>
          </div>
        }
      />
      <ContextMenu.Content>
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
