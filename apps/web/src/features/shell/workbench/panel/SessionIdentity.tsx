import * as stylex from "@stylexjs/stylex";
import { type ReactElement } from "react";
import { basename } from "@/lib/path";
import { ContextMenu, DropdownMenu, Icon } from "@/ui/index";
import { iconGlyph } from "@/ui/icons/index";
import { useT } from "@/lib/i18n/index";
import { writeToClipboard } from "@/lib/clipboard";
import { IconButton, vocab } from "@flame/ui";
import { color, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
const si = stylex.create({
  bar: { display: "flex", minWidth: 0, flexShrink: 1, alignItems: "center", gap: space.s2 },
  cwd: {
    display: { default: "none", "@media (min-width: 1024px)": "inline" },
    minWidth: 0,
    maxWidth: "160px",
    flexShrink: 1,
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
  const copies: { id: string; label: string; copy: () => void }[] = [
    ...(workspacePath
      ? [
          {
            id: "cwd",
            label: t("session.identity.copyCwd"),
            copy: () =>
              void writeToClipboard(workspacePath, {
                successLabel: t("session.identity.copiedCwd"),
              }),
          },
        ]
      : []),
    ...(sessionId !== ""
      ? [
          {
            id: "id",
            label: t("session.identity.copyId"),
            copy: () =>
              void writeToClipboard(sessionId, { successLabel: t("session.identity.copiedId") }),
          },
        ]
      : []),
  ];

  const identity = (
    <div {...stylex.props(si.bar)}>
      {workspacePath && (
        <>
          <span
            title={workspacePath}
            className={stylex.props(si.cwd, vocab.truncate, typeStep.uiSm).className}
          >
            {basename(workspacePath)}
          </span>
          <span aria-hidden {...stylex.props(si.sep, typeStep.uiSm)}>
            /
          </span>
        </>
      )}
      <h1 title={title} className={stylex.props(si.title, vocab.truncate, typeStep.uiMd).className}>
        {title}
      </h1>
    </div>
  );

  if (copies.length === 0) return identity;
  // Session ID keys both menus so an open menu retires with its Session instead of
  // retargeting its copy actions to the next one.
  return (
    <div key={sessionId} {...stylex.props(si.bar)}>
      <ContextMenu.Root>
        <ContextMenu.Trigger render={identity} />
        <ContextMenu.Content aria-label={title}>
          {copies.map((item) => (
            <ContextMenu.IconItem key={item.id} icon="copy" onSelect={item.copy}>
              {item.label}
            </ContextMenu.IconItem>
          ))}
        </ContextMenu.Content>
      </ContextMenu.Root>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <IconButton
              icon={iconGlyph("more")}
              size="sm"
              aria-label={t("session.row.more", { title })}
            />
          }
        />
        <DropdownMenu.Content align="start" sideOffset={4} aria-label={title}>
          {copies.map((item) => (
            <DropdownMenu.Item key={item.id} layout="glyph" onClick={item.copy}>
              <Icon name="copy" size="md" />
              <span {...stylex.props(vocab.truncate)}>{item.label}</span>
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </div>
  );
}
