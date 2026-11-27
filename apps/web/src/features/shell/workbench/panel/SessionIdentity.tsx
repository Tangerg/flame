import { useNotifications } from "@/features/notifications/public/notifier";
import * as stylex from "@stylexjs/stylex";
import { type ReactElement, type ReactNode, useId, useRef } from "react";
import { basename } from "@/lib/path";
import { ContextMenu, DropdownMenu } from "@/ui/index";
import { iconGlyph } from "@/ui/icons/index";
import { useT } from "@/lib/i18n/index";
import { writeToClipboard } from "@/lib/clipboard";
import { IconButton, Tooltip, useClampedText, vocab } from "@flame/ui";
import { color, face, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
const si = stylex.create({
  bar: { display: "flex", minWidth: 0, flexShrink: 1, alignItems: "center", gap: space.s2 },
  fill: { flex: 1 },
  summary: {
    display: "flex",
    minWidth: 0,
    flexShrink: 1,
    flexDirection: { default: "row", "@container conversation (max-width: 640px)": "column" },
    alignItems: { default: "center", "@container conversation (max-width: 640px)": "stretch" },
    gap: { default: space.s2, "@container conversation (max-width: 640px)": 0 },
  },
  cwd: {
    display: { default: "none", "@media (min-width: 1024px)": "inline" },
    minWidth: 0,
    maxWidth: "160px",
    flexShrink: 1,
    color: color.fgFaint,
  },
  sep: {
    display: { default: "none", "@media (min-width: 1024px)": "inline" },
    flexShrink: 0,
    color: color.fgFaint,
  },
  copySummary: {
    display: "grid",
    minWidth: 0,
    gap: space.s0_5,
  },
  path: {
    maxWidth: "280px",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
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
  detail?: ReactNode;
}

interface CopyChoice {
  id: string;
  label: string;
  path?: string;
  copy: () => void;
}

export function SessionIdentity({ sessionId, title, workspacePath, detail }: Props): ReactElement {
  const { messages } = useNotifications();
  const t = useT();
  const workspaceName = workspacePath ? basename(workspacePath) : undefined;
  const copies: CopyChoice[] = [
    ...(workspacePath
      ? [
          {
            id: "cwd",
            label: t("session.identity.copyCwd"),
            path: workspacePath,
            copy: () =>
              void writeToClipboard(messages.show, workspacePath, {
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
              void writeToClipboard(messages.show, sessionId, {
                successLabel: t("session.identity.copiedId"),
              }),
          },
        ]
      : []),
  ];

  const identity = (
    <div {...stylex.props(si.summary)}>
      <div {...stylex.props(si.bar)}>
        {workspacePath && (
          <>
            <Tooltip
              label={
                workspacePath !== workspaceName ? (
                  <span dir="ltr" {...stylex.props(si.path)}>
                    {workspacePath}
                  </span>
                ) : undefined
              }
              side="bottom"
            >
              <span {...stylex.props(si.cwd, vocab.truncate, typeStep.uiSm)}>{workspaceName}</span>
            </Tooltip>
            <span aria-hidden {...stylex.props(si.sep, typeStep.uiSm)}>
              /
            </span>
          </>
        )}
        <SessionHeading title={title} />
      </div>
      {detail}
    </div>
  );

  if (copies.length === 0) return identity;
  // Session ID keys both menus so an open menu retires with its Session instead of
  // retargeting its copy actions to the next one.
  return (
    <div key={sessionId} {...stylex.props(si.bar, si.fill)}>
      <ContextMenu.Root>
        <ContextMenu.Trigger render={identity} />
        <ContextMenu.Content aria-label={title}>
          <CopyMenuItems Item={ContextMenu.Item} copies={copies} />
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
          {/* Every entry copies, as its label says; a shared copy glyph would add nothing. */}
          <CopyMenuItems Item={DropdownMenu.Item} copies={copies} />
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </div>
  );
}

function CopyMenuItems({
  Item,
  copies,
}: {
  Item: typeof ContextMenu.Item | typeof DropdownMenu.Item;
  copies: readonly CopyChoice[];
}) {
  const detailPrefix = useId();
  return copies.map(({ id, label, path, copy }) => {
    const labelNode = <span {...stylex.props(vocab.truncate)}>{label}</span>;
    const detailId = path ? `${detailPrefix}-${id}` : undefined;
    return (
      <Item
        key={id}
        layout="pickPlain"
        label={label}
        aria-label={label}
        aria-describedby={detailId}
        onClick={copy}
      >
        {path ? (
          <span {...stylex.props(si.copySummary)}>
            {labelNode}
            <span
              id={detailId}
              dir="ltr"
              {...stylex.props(si.path, face.mono, typeStep.uiXs, vocab.muted)}
            >
              {path}
            </span>
          </span>
        ) : (
          labelNode
        )}
      </Item>
    );
  });
}

function SessionHeading({ title }: { title: string }) {
  const heading = useRef<HTMLHeadingElement>(null);
  const clipped = useClampedText(heading, title);
  return (
    <Tooltip label={title} disabled={!clipped} side="bottom">
      <h1 ref={heading} className={stylex.props(si.title, vocab.truncate, typeStep.uiMd).className}>
        {title}
      </h1>
    </Tooltip>
  );
}
