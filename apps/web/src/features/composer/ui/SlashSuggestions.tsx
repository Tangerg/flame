import * as stylex from "@stylexjs/stylex";
import { type RefObject } from "react";
import { useT } from "@/lib/i18n/index";
import { OptionRow } from "@/ui/index";
import { vocab } from "@flame/ui";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { type useSlashSuggestions } from "@/features/composer/application/slashSuggestions";
import { suggestionOptionId } from "@/features/composer/application/suggestions";
import { SuggestionPopup } from "@/features/composer/ui/SuggestionPopup";
import { suggestionStyles } from "@/features/composer/ui/suggestionStyles";
interface Props {
  slash: ReturnType<typeof useSlashSuggestions>;
  anchor: RefObject<HTMLElement | null>;
}

export function SlashSuggestions({ slash, anchor }: Props) {
  const t = useT();
  return (
    <SuggestionPopup
      open={slash.open}
      heading={t("composer.slash.heading")}
      index={slash.index}
      onDismiss={slash.dismiss}
      anchor={anchor}
    >
      {slash.items.map((command, i) => (
        <OptionRow
          key={command.cmd}
          layout="glyph"
          id={suggestionOptionId(i)}
          tabIndex={-1}
          selected={i === slash.index}
          onMouseEnter={() => slash.setIndex(i)}
          onMouseDown={(event) => {
            event.preventDefault();
            slash.accept(command);
          }}
        >
          <code {...stylex.props(suggestionStyles.command, typeStep.uiSm)}>{command.cmd}</code>
          <span {...stylex.props(vocab.truncate, vocab.muted)}>{t(command.spec.description)}</span>
        </OptionRow>
      ))}
    </SuggestionPopup>
  );
}
