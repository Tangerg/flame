import * as stylex from "@stylexjs/stylex";
import { useMemo } from "react";
import type { RefObject } from "react";
import { useT } from "@/lib/i18n";
import { useSlashCommands } from "@/plugins/sdk";
import { OptionRow, Popover, SectionLabel, vocab } from "@/ui";
import { type as typeStep } from "@/styles/tokens.stylex";
import { suggestionStyles } from "./suggestionStyles";

interface Props {
  value: string;
  onPick: (cmd: string) => void;
  anchor: RefObject<HTMLElement | null>;
}

export function SlashSuggestions({ value, onPick, anchor }: Props) {
  const t = useT();
  const commands = useSlashCommands();

  const filtered = useMemo(() => {
    if (!value || !value.startsWith("/")) return [];
    const q = value.slice(1).toLowerCase();
    return commands
      .filter(({ cmd }) => cmd.slice(1).toLowerCase().startsWith(q))
      .sort((a, b) => a.cmd.localeCompare(b.cmd))
      .slice(0, 5);
  }, [value, commands]);

  return (
    <Popover.Anchored
      open={filtered.length > 0}
      anchor={anchor}
      aria-label={t("composer.slash.heading")}
      {...stylex.props(suggestionStyles.panel)}
    >
      <SectionLabel {...stylex.props(suggestionStyles.heading)}>
        {t("composer.slash.heading")}
      </SectionLabel>
      {filtered.map(({ cmd, spec }) => (
        <OptionRow key={cmd} layout="glyph" onClick={() => onPick(`${cmd} `)}>
          <code {...stylex.props(suggestionStyles.command, typeStep.uiSm)}>{cmd}</code>
          <span {...stylex.props(vocab.truncate, vocab.muted)}>{t(spec.description)}</span>
        </OptionRow>
      ))}
    </Popover.Anchored>
  );
}
