import * as stylex from "@stylexjs/stylex";
import type { ReactElement } from "react";
import { basename } from "@/lib/path";
import { ContextMenu } from "@/ui";
import { useT } from "@/lib/i18n";
import { writeToClipboard } from "@/lib/clipboard";
import { color, space, type as typeStep, weight } from "@/styles/tokens.stylex";

const si = stylex.create({
  bar: { display: "flex", minWidth: 0, flexShrink: 1, alignItems: "center", gap: space.s2 },
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
    fontWeight: weight.medium,
    color: color.fg,
  },
});

interface Props {
  sessionId: string;
  title: string;
  workspacePath?: string;
}

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
            <h1 title={title} className={stylex.props(si.title, typeStep.uiMd).className}>
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
