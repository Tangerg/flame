export function matchCommands<Row extends { label: string }>(
  commands: readonly Row[],
  query: string,
): Row[] {
  const needle = query.trim().toLowerCase();
  const matched =
    needle === ""
      ? [...commands]
      : commands.filter((command) => command.label.toLowerCase().includes(needle));
  return matched.sort((a, b) => a.label.localeCompare(b.label));
}
