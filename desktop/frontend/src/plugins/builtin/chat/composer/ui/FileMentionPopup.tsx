import * as stylex from "@stylexjs/stylex";
import type { RefObject } from "react";
import { Icon, OptionRow, Popover, SectionLabel, vocab } from "@/ui";
import {
  MENTION_LISTBOX_ID,
  mentionOptionId,
} from "@/plugins/builtin/chat/composer/application/fileMentions";
import { face } from "@/styles/tokens.stylex";
import { useT } from "@/lib/i18n";
import { suggestionStyles } from "./suggestionStyles";

interface Props {
  open: boolean;
  items: string[];
  index: number;
  onPick: (path: string) => void;
  onHover: (i: number) => void;
  onDismiss: () => void;
  anchor: RefObject<HTMLElement | null>;
}

export function FileMentionPopup({
  open,
  items,
  index,
  onPick,
  onHover,
  onDismiss,
  anchor,
}: Props) {
  const t = useT();
  return (
    <Popover.Anchored
      open={open}
      onOpenChange={(next) => {
        if (!next) onDismiss();
      }}
      anchor={anchor}
      id={MENTION_LISTBOX_ID}
      role="listbox"
      aria-label={t("composer.mention.heading")}
      {...stylex.props(suggestionStyles.panel)}
    >
      <SectionLabel {...stylex.props(suggestionStyles.heading)}>
        {t("composer.mention.heading")}
      </SectionLabel>
      {items.map((path, i) => {
        const slash = path.lastIndexOf("/");
        const dir = slash >= 0 ? path.slice(0, slash + 1) : "";
        const name = slash >= 0 ? path.slice(slash + 1) : path;
        return (
          <OptionRow
            key={path}
            layout="glyph"
            id={mentionOptionId(i)}
            tabIndex={-1}
            selected={i === index}
            onMouseEnter={() => onHover(i)}
            onMouseDown={(e) => {
              e.preventDefault();
              onPick(path);
            }}
          >
            <Icon
              name="filetext"
              size="sm"
              className={stylex.props(vocab.hold, vocab.muted).className}
            />
            <span {...stylex.props(vocab.truncate, face.mono)}>
              <span {...stylex.props(suggestionStyles.directory)}>{dir}</span>
              <span {...stylex.props(suggestionStyles.name)}>{name}</span>
            </span>
          </OptionRow>
        );
      })}
    </Popover.Anchored>
  );
}
