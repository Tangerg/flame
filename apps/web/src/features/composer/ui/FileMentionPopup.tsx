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
  retryRef: RefObject<HTMLButtonElement | null>;
  onReturnFocus: () => void;
}

export function FileMentionPopup({ mentions, anchor, retryRef, onReturnFocus }: Props) {
  const t = useT();
  const retryAction = mentions.retry;
  // Base UI recovers removed focus targets; hand focus over before an intent removes Retry.
  function dismiss() {
    const retry = retryRef.current;
    if (retry && retry.ownerDocument.activeElement === retry) onReturnFocus();
    mentions.dismiss();
  }
  return (
    <SuggestionPopup
      open={mentions.open}
      heading={t("composer.mention.heading")}
      index={mentions.index}
      onDismiss={dismiss}
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
            {retryAction && (
              <TextButton
                ref={retryRef}
                tone="accent"
                pending={retryAction.status === "pending"}
                className={stylex.props(vocab.hold).className}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => {
                  onReturnFocus();
                  retryAction.run();
                }}
                onKeyDown={(event) => {
                  if (event.key !== "Tab" || event.altKey || event.ctrlKey || event.metaKey) return;
                  if (event.shiftKey) {
                    event.preventDefault();
                    onReturnFocus();
                  } else {
                    dismiss();
                  }
                }}
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
            aria-label={path}
            title={path}
            onMouseMove={() => mentions.setIndex(i)}
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
            <span {...stylex.props(vocab.min, face.mono, suggestionStyles.path)}>
              <span {...stylex.props(vocab.min, suggestionStyles.directory)}>{dir}</span>
              <span {...stylex.props(vocab.truncate, vocab.hold, suggestionStyles.name)}>
                {name}
              </span>
            </span>
          </OptionRow>
        );
      })}
    </SuggestionPopup>
  );
}
