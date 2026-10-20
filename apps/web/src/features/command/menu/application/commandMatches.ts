export interface CommandChoice {
  key: string;
  label: string;
  icon?: string;
  combo?: string;
  run: () => void;
}

export function matchCommands(commands: readonly CommandChoice[], query: string): CommandChoice[] {
  const needle = query.trim().toLowerCase();
  const matched =
    needle === ""
      ? [...commands]
      : commands.filter((command) => command.label.toLowerCase().includes(needle));
  return matched.sort((a, b) => a.label.localeCompare(b.label));
}
