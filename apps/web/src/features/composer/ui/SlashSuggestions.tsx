import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n/index";
import { OptionRow } from "@/ui/index";
import { vocab } from "@flame/ui";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { type useSlashSuggestions } from "@/features/composer/application/slashSuggestions";
import { type SuggestionNavigation } from "@/features/composer/ui/useSuggestionNavigation";
import { suggestionStyles } from "@/features/composer/ui/suggestionStyles";
interface Props {
  slash: ReturnType<typeof useSlashSuggestions>;
  navigation: SuggestionNavigation;
}

export function SlashSuggestions({ slash, navigation }: Props) {
  const t = useT();
  return (
    <>
      {slash.items.map((command, i) => (
        <OptionRow
          key={command.cmd}
          layout="glyph"
          {...navigation.getOptionProps(i)}
          onClick={() => slash.accept(command)}
        >
          <code {...stylex.props(suggestionStyles.command, typeStep.uiSm)}>{command.cmd}</code>
          <span {...stylex.props(vocab.truncate, vocab.muted)}>{t(command.spec.description)}</span>
        </OptionRow>
      ))}
    </>
  );
}
