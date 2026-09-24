import * as stylex from "@stylexjs/stylex";
import type { RefObject } from "react";
import { Icon, OptionRow, TextButton, vocab } from "@/ui";
import { face, type as typeStep } from "@/styles/tokens.stylex";
import { useT } from "@/lib/i18n";
import { type FileMentions, MENTION_FETCH_LIMIT } from "../application/fileMentions";
import { suggestionOptionId } from "../application/suggestions";
import { SuggestionPopup } from "./SuggestionPopup";
import { suggestionStyles } from "./suggestionStyles";

interface Props {
  mentions: FileMentions;
  anchor: RefObject<HTMLElement | null>;
}

export function FileMentionPopup({ mentions, anchor }: Props) {
  const t = useT();
  return (
    <SuggestionPopup
      open={mentions.open}
      heading={t("composer.mention.heading")}
      index={mentions.index}
      onDismiss={mentions.dismiss}
      anchor={anchor}
      footer={
        mentions.truncated && (
          <p {...stylex.props(suggestionStyles.note, typeStep.uiXs)}>
            {t("composer.mention.truncated", { count: MENTION_FETCH_LIMIT })}
          </p>
        )
      }
    >
      {mentions.status === "ready" ? (
        mentions.items.map((path, i) => {
          const slash = path.lastIndexOf("/");
          const dir = slash >= 0 ? path.slice(0, slash + 1) : "";
          const name = slash >= 0 ? path.slice(slash + 1) : path;
          return (
            <OptionRow
              key={path}
              layout="glyph"
              id={suggestionOptionId(i)}
              tabIndex={-1}
              selected={i === mentions.index}
              title={path}
              onMouseEnter={() => mentions.setIndex(i)}
              onMouseDown={(event) => {
                event.preventDefault();
                mentions.accept(path);
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
        })
      ) : (
        <div role="status" {...stylex.props(suggestionStyles.state, typeStep.uiSm)}>
          <span>{t(`composer.mention.status.${mentions.status}`)}</span>
          {mentions.status === "error" && (
            <TextButton
              tone="accent"
              onMouseDown={(event) => event.preventDefault()}
              onClick={mentions.retry}
            >
              {t("common.retry")}
            </TextButton>
          )}
        </div>
      )}
    </SuggestionPopup>
  );
}
