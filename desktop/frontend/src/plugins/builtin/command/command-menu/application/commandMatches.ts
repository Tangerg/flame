/** A command as the menu shows it: the label already resolved, because only the render knows
 *  the active locale, and the combo so a person can learn the key from the row they just ran. */
export interface CommandChoice {
  id: string;
  label: string;
  combo?: string;
}

/**
 * An empty query answers with everything, unlike the session finder: this surface exists to
 * show what the application can do, and a blank list would make it impossible to learn.
 */
export function matchCommands(commands: readonly CommandChoice[], query: string): CommandChoice[] {
  const needle = query.trim().toLowerCase();
  const matched =
    needle === ""
      ? [...commands]
      : commands.filter((command) => command.label.toLowerCase().includes(needle));
  return matched.sort((a, b) => a.label.localeCompare(b.label));
}
