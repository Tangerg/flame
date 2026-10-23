import * as stylex from "@stylexjs/stylex";
import { type RefObject } from "react";
import { basename } from "@/lib/path";
import { Icon } from "@/ui/index";
import { OptionRow } from "@/ui/index";
import { TextButton } from "@/ui/index";
import { fileIconName } from "@/ui/index";
import { vocab } from "@flame/ui";
import { face, type as typeStep } from "@flame/ui/tokens.stylex";
import { useT } from "@/lib/i18n/index";
import { type FileMentions } from "@/features/composer/application/fileMentions";
import { suggestionOptionId } from "@/features/composer/application/suggestions";
import { SuggestionPopup } from "@/features/composer/ui/SuggestionPopup";
import { suggestionStyles } from "@/features/composer/ui/suggestionStyles";
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
      status={
        mentions.status === "ready" ? undefined : (
          <>
            <span {...stylex.props(suggestionStyles.stateText)}>
              <span>{t(`composer.mention.status.${mentions.status}`)}</span>
              {mentions.failure && (
                <span {...stylex.props(vocab.muted, typeStep.uiXs)}>{mentions.failure}</span>
              )}
            </span>
            {mentions.status === "error" && (
              <TextButton
                tone="accent"
                onMouseDown={(event) => event.preventDefault()}
                onClick={mentions.retry}
              >
                {t("common.retry")}
              </TextButton>
            )}
          </>
        )
      }
    >
      {mentions.items.map((path, i) => {
        const name = basename(path);
        const dir = path.slice(0, path.length - name.length);
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
              name={fileIconName(path)}
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
    </SuggestionPopup>
  );
}
