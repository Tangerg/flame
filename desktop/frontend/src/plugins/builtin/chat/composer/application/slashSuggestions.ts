import { useCallback, useMemo, useState } from "react";
import { useSlashCommands } from "@/plugins/sdk";
import { type SuggestionList, useSuggestionIndex } from "./suggestions";

type SlashCommand = ReturnType<typeof useSlashCommands>[number];

function slashToken(value: string, caret: number): string | null {
  if (!value.startsWith("/")) return null;
  const end = value.search(/\s/);
  const tokenEnd = end === -1 ? value.length : end;
  if (caret > tokenEnd) return null;
  return value.slice(0, tokenEnd);
}

function matchSlashCommands(
  commands: readonly SlashCommand[],
  token: string,
): SlashCommand[] {
  const query = token.slice(1).toLowerCase();
  return commands
    .filter(({ cmd }) => cmd.slice(1).toLowerCase().startsWith(query))
    .sort((a, b) => a.cmd.localeCompare(b.cmd));
}

interface Args {
  value: string;
  caret: number;
  apply: (text: string, caret: number) => void;
}

export function useSlashSuggestions({ value, caret, apply }: Args): SuggestionList<SlashCommand> {
  const commands = useSlashCommands();
  const [dismissedToken, setDismissedToken] = useState<string | null>(null);
  const token = slashToken(value, caret);
  const items = useMemo(
    () => (token === null ? [] : matchSlashCommands(commands, token)),
    [commands, token],
  );
  const complete = items.length === 1 && items[0]!.cmd === token;
  const open = token !== null && token !== dismissedToken && items.length > 0 && !complete;
  const { index, setIndex } = useSuggestionIndex(`${token}\0${items.length}`, items.length);

  const accept = useCallback(
    (command: SlashCommand) => {
      if (token === null) return;
      const rest = value.slice(token.length).replace(/^\s+/, "");
      const insert = `${command.cmd} `;
      apply(insert + rest, insert.length);
      setDismissedToken(null);
    },
    [token, value, apply],
  );

  const dismiss = useCallback(() => setDismissedToken(token), [token]);

  return { open, items, index, setIndex, accept, dismiss };
}
